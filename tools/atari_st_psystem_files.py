#!/usr/bin/env python3
"""解出 Atari ST SunDog 磁碟的 UCSD p-system 卷冊目錄，並可抽出其中的檔案。

這支工具存在的理由：遊戲規則不在資料表裡，在 ``SYSTEM.STARTUP`` 的 p-code 裡。要靜態
回答「這個欄位是什麼意思」「這條規則怎麼算」，第一步就是把那個檔案從磁碟取出來。

卷冊目錄位於 p-system logical block 2（raw ``0x04c00``），欄位是 **big-endian**——
68000 的位元組序，與 Apple II 版不同。目錄本身是本工具自行從 bytes 解出並驗證的。

抽出的檔案是原版程式碼，**只能寫到明確指定的輸出目錄**（預設 ``/tmp``），不得進入
repository 或公開封包。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import struct
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


# p-system 的 logical block 0 在磁碟上的位置。
LOGICAL_BASE = 0x04800
BLOCK_SIZE = 512
DIRECTORY_BLOCK = 2
VOLUME_ENTRY_SIZE = 26
# 卷冊名稱最多 7 字元、檔名最多 15，兩者的長度前綴欄位因此不同大小。
VOLUME_NAME_FIELD_SIZE = 8
FILE_NAME_FIELD_SIZE = 16
MAX_FILES = 77


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return disk_name, disk_bytes


def decode_name(field: bytes) -> str:
    length = field[0]
    if length > len(field) - 1:
        raise ValueError(f"directory name declares length {length}")
    body = field[1 : 1 + length]
    for byte in body:
        if byte < 0x20 or byte > 0x7E:
            raise ValueError(f"directory name has a non-printable byte 0x{byte:02x}")
    return body.decode("ascii")


def read_directory(disk: bytes) -> dict[str, object]:
    offset = LOGICAL_BASE + DIRECTORY_BLOCK * BLOCK_SIZE
    block = disk[offset : offset + BLOCK_SIZE * 2]
    if len(block) < BLOCK_SIZE:
        raise ValueError("directory block is truncated")

    first, last, kind = struct.unpack(">HHH", block[0:6])
    volume = decode_name(block[6 : 6 + VOLUME_NAME_FIELD_SIZE])
    eov, file_count = struct.unpack(">HH", block[14:18])
    if not 0 < file_count <= MAX_FILES:
        raise ValueError(f"directory declares {file_count} files")
    if first != 0:
        raise ValueError(f"volume first block is {first}, want 0")

    files: list[dict[str, object]] = []
    for index in range(1, file_count + 1):
        entry = block[index * VOLUME_ENTRY_SIZE : (index + 1) * VOLUME_ENTRY_SIZE]
        if len(entry) != VOLUME_ENTRY_SIZE:
            raise ValueError(f"directory entry {index} is truncated")
        entry_first, entry_last, entry_kind = struct.unpack(">HHH", entry[0:6])
        if entry_last <= entry_first or entry_last > eov:
            raise ValueError(f"directory entry {index} spans blocks {entry_first}..{entry_last}")
        name = decode_name(entry[6 : 6 + FILE_NAME_FIELD_SIZE])
        last_byte, = struct.unpack(">H", entry[22:24])
        raw_offset = LOGICAL_BASE + entry_first * BLOCK_SIZE
        size = (entry_last - entry_first) * BLOCK_SIZE
        files.append(
            {
                "name": name,
                "firstBlock": entry_first,
                "lastBlock": entry_last,
                "blockCount": entry_last - entry_first,
                "rawOffset": f"0x{raw_offset:05x}",
                "size": size,
                "lastByte": last_byte,
                # kind 是 p-system 的檔案型別欄位，這裡不解讀，只保留原值。
                "kindRaw": f"0x{entry_kind:04x}",
                "sha256": sha256(disk[raw_offset : raw_offset + size]),
            }
        )

    return {
        "logicalBase": f"0x{LOGICAL_BASE:05x}",
        "directoryBlock": DIRECTORY_BLOCK,
        "directoryRawOffset": f"0x{offset:05x}",
        "byteOrder": "big-endian",
        "volumeName": volume,
        "volumeKindRaw": f"0x{kind:04x}",
        "lastBlock": last,
        "endOfVolumeBlock": eov,
        "fileCount": file_count,
        "files": files,
    }


def build_document(archive_path: Path, variant: str) -> tuple[dict[str, object], bytes]:
    disk_name, disk = load_disk(archive_path, variant)
    directory = read_directory(disk)
    return {
        "schema": 1,
        "tool": "tools/atari_st_psystem_files.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "directory": directory,
        "evidence": "proven",
        "notes": [
            "卷冊目錄的欄位是 big-endian；以 little-endian 解會得到荒謬的檔案數。",
            (
                "次級文件 laanwj/sundog doc/disklayout.md 把 raw 0x2e000 標為 SYSTEM.PASCAL，"
                "但卷冊目錄顯示該處是 SYSTEM.STARTUP，SYSTEM.PASCAL 在其後。以目錄為準。"
            ),
            "抽出的檔案是原版程式碼，只寫到指定輸出目錄，不進 repository 或公開封包。",
        ],
    }, disk


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path, help="外層 Atari ST archive（唯讀）")
    parser.add_argument("--variant", default="[a2]", help="nested archive 變體標記")
    parser.add_argument("--output", type=Path, help="輸出目錄 JSON 路徑；未指定則印到 stdout")
    parser.add_argument(
        "--extract-to",
        type=Path,
        help="把目錄中的檔案寫到這個目錄；必須明確指定，且不得是 repository 內的路徑",
    )
    args = parser.parse_args()

    document, disk = build_document(args.archive, args.variant)
    if args.extract_to:
        args.extract_to.mkdir(parents=True, exist_ok=True)
        for entry in document["directory"]["files"]:
            offset = int(str(entry["rawOffset"]), 16)
            payload = disk[offset : offset + int(entry["size"])]
            (args.extract_to / str(entry["name"])).write_bytes(payload)
        document["extractedTo"] = str(args.extract_to)

    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
