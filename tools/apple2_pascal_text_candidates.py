#!/usr/bin/env python3
"""Extract length-prefixed ASCII records from an Apple II disk image.

This is an evidence tool, not a filesystem decoder.  It only reports bytes in a
caller-supplied region and keeps the disk offset and raw record so a later
consumer analysis can confirm or reject the record layout.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path


def parse_int(value: str) -> int:
    return int(value, 0)


def is_ascii(value: bytes) -> bool:
    return all(0x20 <= byte <= 0x7E for byte in value)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("disk", type=Path)
    parser.add_argument("--start", type=parse_int, required=True)
    parser.add_argument("--end", type=parse_int, required=True)
    parser.add_argument("--stride", type=parse_int, default=16)
    parser.add_argument("--record-size", type=parse_int, default=16)
    parser.add_argument("--max-length", type=parse_int, default=31)
    args = parser.parse_args()

    raw = args.disk.read_bytes()
    if not (0 <= args.start < args.end <= len(raw)):
        parser.error("region is outside the disk image")
    if args.stride <= 0 or args.record_size <= 1:
        parser.error("stride and record-size must be positive")

    entries = []
    for offset in range(args.start, args.end, args.stride):
        record = raw[offset : offset + args.record_size]
        if len(record) < args.record_size:
            break
        length = record[0]
        value = record[1 : 1 + length]
        entry = {
            "offset": f"0x{offset:05x}",
            "prefix": length,
            "recordSize": len(record),
            "recordHex": record.hex(),
        }
        if 1 <= length <= args.max_length and len(value) == length and is_ascii(value):
            entry.update(
                {
                    "text": value.decode("ascii"),
                    "sourceStatus": "candidate",
                }
            )
        else:
            entry.update(
                {
                    "text": None,
                    "sourceStatus": "unknown",
                    "reason": "prefix is zero, too long, or not printable ASCII",
                }
            )
        entries.append(entry)

    result = {
        "schema": 1,
        "tool": "apple2_pascal_text_candidates",
        "disk": args.disk.name,
        "diskSize": len(raw),
        "diskSha256": hashlib.sha256(raw).hexdigest(),
        "region": {
            "start": f"0x{args.start:05x}",
            "end": f"0x{args.end:05x}",
            "stride": args.stride,
            "recordSize": args.record_size,
        },
        "entries": entries,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
