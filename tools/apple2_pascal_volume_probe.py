#!/usr/bin/env python3
"""以明確 sector skew 探測 Apple Pascal 的標準 block-2 目錄。

這個工具只做唯讀格式探測。它不會把 SunDog 的 embedded Pascal-like table
命名成 filesystem，也不會替來源 image 重排或寫回任何 bytes。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import struct
from pathlib import Path

try:
    from tools.apple2_dsk_inventory import EXPECTED_SIZE, load_input
except ModuleNotFoundError:  # direct ``python tools/...`` invocation
    from apple2_dsk_inventory import EXPECTED_SIZE, load_input


SECTOR_SIZE = 256
SECTORS_PER_TRACK = 16
BLOCK_SIZE = 512
BLOCKS_PER_TRACK = 8
DIRECTORY_FIRST_BLOCK = 2
DIRECTORY_LAST_BLOCK = 5
DIRECTORY_ENTRY_SIZE = 26
MAX_DIRECTORY_FILES = 77

# A .dsk image stores the physical sectors in ascending sector-number order;
# the operating system's logical sector number is translated by a skew table.
# These are named views, not claims about the source image's live consumer.
SECTOR_ORDER_CANDIDATES: dict[str, tuple[int, ...]] = {
    "raw-sequential": tuple(range(SECTORS_PER_TRACK)),
    "apple-pascal-2-ascending": (0, 8, 1, 9, 2, 10, 3, 11, 4, 12, 5, 13, 6, 14, 7, 15),
    "dos33-2-descending": (0, 7, 14, 6, 13, 5, 12, 4, 11, 3, 10, 2, 9, 1, 8, 15),
}


def u16(data: bytes, offset: int) -> int:
    return struct.unpack_from("<H", data, offset)[0]


def raw_sector_offset(track: int, physical_sector: int) -> int:
    return (track * SECTORS_PER_TRACK + physical_sector) * SECTOR_SIZE


def read_logical_sector(
    data: bytes, track: int, logical_sector: int, sector_order: tuple[int, ...]
) -> bytes:
    if not 0 <= track < len(data) // (SECTORS_PER_TRACK * SECTOR_SIZE):
        raise ValueError(f"track outside image: {track}")
    if not 0 <= logical_sector < SECTORS_PER_TRACK:
        raise ValueError(f"logical sector outside track: {logical_sector}")
    physical_sector = sector_order[logical_sector]
    start = raw_sector_offset(track, physical_sector)
    return data[start : start + SECTOR_SIZE]


def read_block(data: bytes, block: int, sector_order: tuple[int, ...]) -> bytes:
    if block < 0 or block >= len(data) // BLOCK_SIZE:
        raise ValueError(f"block outside image: {block}")
    track, block_in_track = divmod(block, BLOCKS_PER_TRACK)
    first_logical_sector = block_in_track * 2
    return b"".join(
        read_logical_sector(data, track, logical_sector, sector_order)
        for logical_sector in (first_logical_sector, first_logical_sector + 1)
    )


def block_segments(block: int, sector_order: tuple[int, ...]) -> list[dict[str, int]]:
    track, block_in_track = divmod(block, BLOCKS_PER_TRACK)
    first_logical_sector = block_in_track * 2
    result = []
    for logical_sector in (first_logical_sector, first_logical_sector + 1):
        physical_sector = sector_order[logical_sector]
        raw_offset = raw_sector_offset(track, physical_sector)
        result.append(
            {
                "block": block,
                "track": track,
                "logicalSector": logical_sector,
                "physicalSector": physical_sector,
                "rawOffset": raw_offset,
                "rawEndExclusive": raw_offset + SECTOR_SIZE,
            }
        )
    return result


def pascal_name(data: bytes, offset: int, field_size: int) -> str:
    if offset < 0 or offset + field_size > len(data):
        raise ValueError("name field is outside directory")
    length = data[offset]
    if length == 0 or length > field_size - 1:
        raise ValueError(f"invalid Pascal name length {length}")
    raw = data[offset + 1 : offset + 1 + length]
    if not all(0x20 <= value <= 0x7E for value in raw):
        raise ValueError("name contains non-printable ASCII")
    return raw.decode("ascii")


def parse_pascal_directory(
    data: bytes, sector_order: tuple[int, ...]
) -> tuple[bool, dict[str, object]]:
    directory = b"".join(
        read_block(data, block, sector_order)
        for block in range(DIRECTORY_FIRST_BLOCK, DIRECTORY_LAST_BLOCK + 1)
    )
    errors: list[str] = []
    header: dict[str, object] = {
        "systemAreaStartBlock": u16(directory, 0),
        "nextBlock": u16(directory, 2),
        "fileType": u16(directory, 4),
        "blocksInVolume": u16(directory, 0x0E),
        "filesInDirectory": u16(directory, 0x10),
        "lastAccess": u16(directory, 0x12),
        "date": u16(directory, 0x14),
    }
    try:
        header["volumeName"] = pascal_name(directory, 6, 8)
    except ValueError as error:
        header["volumeName"] = None
        errors.append(f"volume name: {error}")

    expected_blocks = len(data) // BLOCK_SIZE
    if header["systemAreaStartBlock"] != 0:
        errors.append("volume header system area does not start at block 0")
    if header["nextBlock"] != DIRECTORY_LAST_BLOCK + 1:
        errors.append("volume header next block is not 6")
    if header["fileType"] != 0:
        errors.append("volume header file type is not 0")
    if header["blocksInVolume"] != expected_blocks:
        errors.append(
            f"volume block count {header['blocksInVolume']} differs from image {expected_blocks}"
        )
    file_count = int(header["filesInDirectory"])
    if not 0 <= file_count <= MAX_DIRECTORY_FILES:
        errors.append(f"file count outside 0..{MAX_DIRECTORY_FILES}: {file_count}")

    entries: list[dict[str, object]] = []
    previous_next: int | None = None
    if 0 <= file_count <= MAX_DIRECTORY_FILES:
        for index in range(file_count):
            offset = DIRECTORY_ENTRY_SIZE * (index + 1)
            entry: dict[str, object] = {
                "index": index + 1,
                "directoryLogicalOffset": offset,
                "startBlock": u16(directory, offset),
                "nextBlock": u16(directory, offset + 2),
                "fileType": u16(directory, offset + 4),
                "bytesUsedLastBlock": u16(directory, offset + 0x16),
                "date": u16(directory, offset + 0x18),
            }
            try:
                entry["name"] = pascal_name(directory, offset + 6, 16)
            except ValueError as error:
                entry["name"] = None
                errors.append(f"entry {index + 1} name: {error}")

            start = int(entry["startBlock"])
            end = int(entry["nextBlock"])
            file_type = int(entry["fileType"]) & 0x000F
            bytes_used = int(entry["bytesUsedLastBlock"])
            if start < DIRECTORY_LAST_BLOCK + 1:
                errors.append(f"entry {index + 1} starts before block 6")
            if start >= end:
                errors.append(f"entry {index + 1} has non-positive block span")
            if end > expected_blocks:
                errors.append(f"entry {index + 1} ends after image")
            if previous_next is not None and start < previous_next:
                errors.append(f"entry {index + 1} is not sorted by start block")
            if file_type > 8:
                errors.append(f"entry {index + 1} has unknown file type {file_type}")
            if bytes_used == 0 or bytes_used > BLOCK_SIZE:
                errors.append(f"entry {index + 1} has invalid last-block byte count")
            previous_next = end
            entry["fileTypeLowNibble"] = file_type
            entries.append(entry)

    return not errors, {
        "directoryBlocks": [DIRECTORY_FIRST_BLOCK, DIRECTORY_LAST_BLOCK],
        "directoryRawSegments": [
            segment
            for block in range(DIRECTORY_FIRST_BLOCK, DIRECTORY_LAST_BLOCK + 1)
            for segment in block_segments(block, sector_order)
        ],
        "header": header,
        "entries": entries,
        "errors": errors,
    }


def probe_disk(name: str, data: bytes) -> dict[str, object]:
    views: list[dict[str, object]] = []
    for view_name, sector_order in SECTOR_ORDER_CANDIDATES.items():
        valid, result = parse_pascal_directory(data, sector_order)
        views.append(
            {
                "view": view_name,
                "logicalToPhysicalSector": list(sector_order),
                "validStandardPascalDirectory": valid,
                **result,
            }
        )
    return {
        "name": name,
        "size": len(data),
        "sha256": hashlib.sha256(data).hexdigest(),
        "geometry": {
            "tracks": len(data) // (SECTORS_PER_TRACK * SECTOR_SIZE),
            "sectorsPerTrack": SECTORS_PER_TRACK,
            "sectorSize": SECTOR_SIZE,
            "blockSize": BLOCK_SIZE,
            "sizeMatches140K": len(data) == EXPECTED_SIZE,
        },
        "directoryBlockProbe": views,
        "interpretation": {
            "proven": [
                "每個 view 都以 block 2–5 的 26-byte Pascal directory 形狀重播",
                "每個欄位保留 logical block／sector 與 raw byte offset 對照",
            ],
            "strongInference": [
                "Apple Pascal 的 2 ascending view 是一個格式背景候選",
                "若 view 通過所有結構條件，可稱為標準 Pascal directory structural match",
            ],
            "unknown": [
                "SunDog boot／RWTS 是否真的使用通過的 directory view",
                "未通過 view 時，image 是否為自訂 loader、embedded table 或不同 sector order",
            ],
        },
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input_path", type=Path)
    parser.add_argument(
        "--disk",
        help="只輸出名稱完全相同或 basename 相同的 disk；未指定時輸出全部 disk",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    input_kind, input_sha, source_files, disks = load_input(args.input_path)
    if args.disk:
        selected = [
            item
            for item in disks
            if item[0] == args.disk or Path(item[0]).name == args.disk
        ]
        if not selected:
            raise SystemExit(f"disk not found: {args.disk}")
        disks = selected
    print(
        json.dumps(
            {
                "schema": 1,
                "tool": "tools/apple2_pascal_volume_probe.py",
                "input": str(args.input_path),
                "inputKind": input_kind,
                "inputSha256": input_sha,
                "sourceFiles": source_files,
                "disks": [probe_disk(name, data) for name, data in disks],
            },
            ensure_ascii=False,
            indent=2,
            sort_keys=True,
        )
    )


if __name__ == "__main__":
    main()
