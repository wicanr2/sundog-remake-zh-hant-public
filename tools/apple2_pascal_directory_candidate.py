#!/usr/bin/env python3
"""解析 SunDog Apple II 來源中的 Pascal-like 26-byte directory candidate。

這不是 Apple Pascal filesystem decoder：呼叫者必須明確提供 raw offset，工具只會解析
候選 bytes、保留原始定位，並輸出欄位是否自洽。"""

from __future__ import annotations

import argparse
import hashlib
import json
import struct
import zipfile
from pathlib import Path


ENTRY_SIZE = 26


def u16(data: bytes, offset: int) -> int:
    return struct.unpack_from("<H", data, offset)[0]


def pascal_name(data: bytes, offset: int, field_size: int) -> str:
    length = data[offset]
    if length > field_size - 1:
        raise ValueError(f"name length {length} exceeds field size {field_size - 1}")
    raw = data[offset + 1 : offset + 1 + length]
    return raw.decode("ascii")


def parse_header(data: bytes, offset: int) -> dict[str, object]:
    return {
        "rawOffset": offset,
        "systemAreaStartBlock": u16(data, offset),
        "nextBlock": u16(data, offset + 2),
        "fileType": u16(data, offset + 4),
        "volumeName": pascal_name(data, offset + 6, 8),
        "blocksInVolume": u16(data, offset + 0x0E),
        "filesInDirectory": u16(data, offset + 0x10),
        "lastAccess": u16(data, offset + 0x12),
        "date": u16(data, offset + 0x14),
        "reservedHex": data[offset + 0x16 : offset + ENTRY_SIZE].hex(),
    }


def parse_entry(data: bytes, offset: int, index: int) -> dict[str, object]:
    return {
        "index": index,
        "rawOffset": offset,
        "startBlock": u16(data, offset),
        "nextBlock": u16(data, offset + 2),
        "fileType": u16(data, offset + 4),
        "name": pascal_name(data, offset + 6, 16),
        "bytesUsedLastBlock": u16(data, offset + 0x16),
        "date": u16(data, offset + 0x18),
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--disk", default="SUNDOG1.DSK")
    parser.add_argument("--offset", default="0x9100")
    parser.add_argument("--entries", type=int, default=6)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    offset = int(args.offset, 0)
    with zipfile.ZipFile(args.archive) as archive:
        data = archive.read(args.disk)
    header = parse_header(data, offset)
    entries = [
        parse_entry(data, offset + ENTRY_SIZE * (index + 1), index + 1)
        for index in range(args.entries)
    ]
    errors: list[str] = []
    if header["fileType"] != 0:
        errors.append("header file type is not zero")
    if header["filesInDirectory"] != len(entries):
        errors.append("header file count does not match requested entries")
    previous_next = None
    for entry in entries:
        if entry["startBlock"] >= entry["nextBlock"]:
            errors.append(f"non-positive block span: {entry['name']}")
        if previous_next is not None and entry["startBlock"] != previous_next:
            errors.append(f"non-contiguous span before: {entry['name']}")
        if entry["bytesUsedLastBlock"] not in (0, 512):
            errors.append(f"unexpected bytes-used field: {entry['name']}")
        previous_next = entry["nextBlock"]
    if previous_next != header["blocksInVolume"]:
        errors.append("last entry does not end at volume block count")

    result = {
        "status": "pass" if not errors else "fail",
        "disk": args.disk,
        "diskSha256": hashlib.sha256(data).hexdigest(),
        "rawOffset": offset,
        "entrySize": ENTRY_SIZE,
        "header": header,
        "entries": entries,
        "errors": errors,
        "interpretation": {
            "proven": [
                "26-byte field alignment、little-endian numeric values、Pascal-length names 與 raw bytes",
                "entries 的 block span 在候選表內連續覆蓋至 volume block count",
            ],
            "strongInference": [
                "此結構像 Apple Pascal directory entry，且 file type／欄位形狀相符",
                "它可能是 SunDog 的 compact／embedded file table，而不是標準 Pascal volume directory",
            ],
            "unknown": [
                "raw offset 與磁碟 logical block 的真正關係",
                "boot／loader 是否以此 table 讀檔，以及 nextBlock=3 的非標準差異原因",
            ],
        },
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
