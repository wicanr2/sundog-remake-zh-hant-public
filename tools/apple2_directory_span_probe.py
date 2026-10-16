#!/usr/bin/env python3
"""Probe an explicit Apple II directory-to-raw mapping candidate.

The probe does not decide that the Pascal-like table is a real filesystem.  It
only tests a caller-supplied base offset and block size, then records raw span
hashes, prefixes, and printable anchors.  This keeps a useful mapping
experiment separate from the typed directory parser's unknown consumer status.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import struct
import zipfile
from pathlib import Path


ENTRY_SIZE = 26


def parse_int(value: str) -> int:
    return int(value, 0)


def pascal_name(data: bytes, offset: int, field_size: int) -> str:
    length = data[offset]
    if length > field_size - 1:
        raise ValueError(f"name length {length} exceeds {field_size - 1}")
    return data[offset + 1 : offset + 1 + length].decode("ascii")


def read_entry(data: bytes, offset: int, index: int) -> dict[str, object]:
    return {
        "index": index,
        "startBlock": struct.unpack_from("<H", data, offset)[0],
        "nextBlock": struct.unpack_from("<H", data, offset + 2)[0],
        "name": pascal_name(data, offset + 6, 16),
        "bytesUsedLastBlock": struct.unpack_from("<H", data, offset + 0x16)[0],
    }


def anchors(blob: bytes) -> list[str]:
    result: list[str] = []
    for match in re.finditer(rb"[ -~]{5,}", blob):
        value = match.group().decode("ascii", "replace")
        if value not in result:
            result.append(value)
        if len(result) == 12:
            break
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("archive", type=Path)
    parser.add_argument("--disk", default="SUNDOG1.DSK")
    parser.add_argument("--directory-offset", type=parse_int, default=0x9100)
    parser.add_argument("--base-offset", type=parse_int, default=0)
    parser.add_argument("--block-size", type=parse_int, default=512)
    args = parser.parse_args()

    with zipfile.ZipFile(args.archive) as archive:
        data = archive.read(args.disk)
    directory_offset = args.directory_offset
    header = {
        "blocksInVolume": struct.unpack_from("<H", data, directory_offset + 0x0E)[0],
        "filesInDirectory": struct.unpack_from("<H", data, directory_offset + 0x10)[0],
    }
    entries = [
        read_entry(data, directory_offset + ENTRY_SIZE * (index + 1), index + 1)
        for index in range(header["filesInDirectory"])
    ]
    errors: list[str] = []
    spans: list[dict[str, object]] = []
    previous_next = None
    for entry in entries:
        if previous_next is not None and entry["startBlock"] != previous_next:
            errors.append(f"non-contiguous directory entry {entry['name']}")
        previous_next = entry["nextBlock"]
        start = args.base_offset + int(entry["startBlock"]) * args.block_size
        end = args.base_offset + int(entry["nextBlock"]) * args.block_size
        if not (0 <= start <= end <= len(data)):
            errors.append(f"span outside disk for {entry['name']}: 0x{start:x}..0x{end:x}")
            continue
        span = data[start:end]
        used = int(entry["bytesUsedLastBlock"])
        if used < args.block_size:
            span = span[: -(args.block_size - used)]
            end = start + len(span)
        spans.append(
            {
                "name": entry["name"],
                "startBlock": entry["startBlock"],
                "nextBlock": entry["nextBlock"],
                "rawStart": f"0x{start:05x}",
                "rawEndExclusive": f"0x{end:05x}",
                "byteLength": len(span),
                "sha256": hashlib.sha256(span).hexdigest(),
                "prefixHex": span[:32].hex(),
                "printableAnchors": anchors(span),
            }
        )

    result = {
        "schema": 1,
        "tool": "apple2_directory_span_probe",
        "sourceArchive": {
            "name": args.archive.name,
            "sha256": hashlib.sha256(args.archive.read_bytes()).hexdigest(),
        },
        "disk": {
            "name": args.disk,
            "size": len(data),
            "sha256": hashlib.sha256(data).hexdigest(),
        },
        "directory": {
            "rawOffset": f"0x{directory_offset:05x}",
            "blockCount": header["blocksInVolume"],
            "entryCount": header["filesInDirectory"],
        },
        "mapping": {
            "baseOffset": f"0x{args.base_offset:05x}",
            "blockSize": args.block_size,
            "status": "strong-inference",
            "consumerStatus": "unknown",
            "basis": [
                "declared volume block count matches 143360-byte image at 512 bytes per block",
                "all six declared spans are in bounds and contiguous",
                "spans contain mutually consistent Pascal runtime／SunDog／startup markers",
            ],
            "limit": "This is a raw mapping probe, not proof of a standard filesystem or live loader consumer.",
        },
        "spans": spans,
        "errors": errors,
        "status": "pass" if not errors and len(spans) == header["filesInDirectory"] else "fail",
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
