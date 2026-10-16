#!/usr/bin/env python3
"""掃描 raw bytes 中自洽的 Pascal-like directory table 候選，不宣稱 filesystem。"""

from __future__ import annotations

import argparse
import hashlib
import json
import struct
import zipfile
from pathlib import Path


ENTRY_SIZE = 26
VALID_TYPES = set(range(0, 9))


def u16(data: bytes, offset: int) -> int:
    return struct.unpack_from("<H", data, offset)[0]


def name_at(data: bytes, offset: int, field_size: int) -> str | None:
    length = data[offset]
    if length == 0 or length > field_size - 1:
        return None
    raw = data[offset + 1 : offset + 1 + length]
    if not all(32 <= byte < 127 for byte in raw):
        return None
    return raw.decode("ascii")


def candidate(data: bytes, offset: int, min_files: int) -> dict[str, object] | None:
    if offset + ENTRY_SIZE * (min_files + 1) > len(data):
        return None
    if u16(data, offset + 4) != 0:
        return None
    volume_name = name_at(data, offset + 6, 8)
    if volume_name is None:
        return None
    blocks = u16(data, offset + 0x0E)
    file_count = u16(data, offset + 0x10)
    next_block = u16(data, offset + 2)
    if not 1 <= blocks <= 280 or not 1 <= file_count <= 77 or not 1 <= next_block <= blocks:
        return None
    if offset + ENTRY_SIZE * (file_count + 1) > len(data):
        return None

    entries: list[dict[str, object]] = []
    previous_next = None
    for index in range(file_count):
        entry_offset = offset + ENTRY_SIZE * (index + 1)
        start = u16(data, entry_offset)
        end = u16(data, entry_offset + 2)
        file_type = u16(data, entry_offset + 4)
        name = name_at(data, entry_offset + 6, 16)
        bytes_used = u16(data, entry_offset + 0x16)
        if name is None or file_type not in VALID_TYPES or not (0 <= start < end <= blocks):
            return None
        if bytes_used <= 0 or bytes_used > 512:
            return None
        if previous_next is not None and start != previous_next:
            return None
        previous_next = end
        entries.append({"name": name, "startBlock": start, "nextBlock": end, "fileType": file_type})
    if previous_next != blocks:
        return None
    if len({entry["name"] for entry in entries}) != len(entries):
        return None
    return {
        "rawOffset": offset,
        "volumeName": volume_name,
        "nextBlock": next_block,
        "blocksInVolume": blocks,
        "filesInDirectory": file_count,
        "entries": entries,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--disk", default="SUNDOG1.DSK")
    parser.add_argument("--min-files", type=int, default=2)
    args = parser.parse_args()
    with zipfile.ZipFile(args.archive) as archive:
        data = archive.read(args.disk)
    candidates = []
    for offset in range(0, len(data) - ENTRY_SIZE * (args.min_files + 1)):
        result = candidate(data, offset, args.min_files)
        if result is not None:
            candidates.append(result)
    print(
        json.dumps(
            {
                "disk": args.disk,
                "diskSha256": hashlib.sha256(data).hexdigest(),
                "candidateCount": len(candidates),
                "candidates": candidates,
                "evidence": {
                    "proven": "候選 raw bytes 能以 26-byte entry 與連續 block span 自洽解析",
                    "strongInference": "候選可能是 Pascal-like compact／embedded file table",
                    "unknown": "它是否為 filesystem directory，以及 boot／RWTS 是否消費它",
                },
            },
            ensure_ascii=False,
            indent=2,
            sort_keys=True,
        )
    )


if __name__ == "__main__":
    main()
