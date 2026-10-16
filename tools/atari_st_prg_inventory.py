#!/usr/bin/env python3
"""從 Atari ST raw .st 的 FAT12 chain 重建 SUNDOG.PRG 的證據摘要。

本工具只在記憶體中重建指定檔案，不把原始程式或解壓資料寫入 repository。輸出保留
disk／檔案 hash、cluster chain、每個 cluster 的 raw offset、TOS executable header
與可讀 ASCII 字串，供後續 68000 consumer 研究使用。檔案內容本身不會輸出。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import re
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested, cluster_chain, u16, u32


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def find_root_file(disk: bytes, root_offset: int, root_entries: int, wanted: str) -> dict[str, object]:
    wanted_upper = wanted.upper()
    for index in range(root_entries):
        offset = root_offset + index * 32
        entry = disk[offset : offset + 32]
        if len(entry) < 32 or entry[0] == 0x00:
            break
        if entry[0] in (0xE5,) or entry[11] == 0x0F or entry[11] & 0x08:
            continue
        raw_name = entry[0:8].decode("ascii", errors="replace").rstrip(" ")
        raw_ext = entry[8:11].decode("ascii", errors="replace").rstrip(" ")
        name = f"{raw_name}.{raw_ext}" if raw_ext else raw_name
        if name.upper() == wanted_upper:
            return {
                "directoryIndex": index,
                "name": name,
                "entryRawOffset": offset,
                "attributes": entry[11],
                "startCluster": u16(entry, 26),
                "size": u32(entry, 28),
            }
    raise ValueError(f"file {wanted!r} is not present in the FAT12 root directory")


def reconstruct(disk: bytes, file_record: dict[str, object], bpb: dict[str, int]) -> tuple[bytes, list[int], list[int]]:
    bytes_per_sector = bpb["bytesPerSector"]
    sectors_per_cluster = bpb["sectorsPerCluster"]
    fat_offset = bpb["reservedSectors"] * bytes_per_sector
    fat_size = bpb["sectorsPerFat"] * bytes_per_sector
    fat = disk[fat_offset : fat_offset + fat_size]
    actual_sectors = len(disk) // bytes_per_sector
    clusters, chain_error = cluster_chain(fat, int(file_record["startCluster"]), max(1, actual_sectors))
    if chain_error:
        raise ValueError(f"invalid file chain: {chain_error}")
    root_sectors = (bpb["rootEntries"] * 32 + bytes_per_sector - 1) // bytes_per_sector
    data_offset = (
        bpb["reservedSectors"]
        + bpb["fatCount"] * bpb["sectorsPerFat"]
        + root_sectors
    ) * bytes_per_sector
    raw_offsets = [data_offset + (cluster - 2) * sectors_per_cluster * bytes_per_sector for cluster in clusters]
    chunks = [
        disk[offset : offset + sectors_per_cluster * bytes_per_sector]
        for offset in raw_offsets
    ]
    payload = b"".join(chunks)[: int(file_record["size"])]
    if len(payload) != int(file_record["size"]):
        raise ValueError("cluster chain is shorter than the directory file size")
    return payload, clusters, raw_offsets


def tos_header(payload: bytes) -> dict[str, object]:
    result: dict[str, object] = {"headerBytes": payload[:28].hex(), "format": "unknown"}
    if len(payload) < 28:
        return result
    branch = int.from_bytes(payload[0:2], "big")
    result.update(
        {
            "branchWord": f"0x{branch:04x}",
            "textLength": int.from_bytes(payload[2:6], "big"),
            "dataLength": int.from_bytes(payload[6:10], "big"),
            "bssLength": int.from_bytes(payload[10:14], "big"),
            "symbolLength": int.from_bytes(payload[14:18], "big"),
            "reserved1": int.from_bytes(payload[18:22], "big"),
            "programFlags": int.from_bytes(payload[22:26], "big"),
            "absoluteFlag": int.from_bytes(payload[26:28], "big"),
        }
    )
    if branch == 0x601A:
        result["format"] = "Atari TOS executable header candidate"
    return result


def ascii_strings(payload: bytes, minimum: int = 6, limit: int = 64) -> list[dict[str, object]]:
    rows: list[dict[str, object]] = []
    for match in re.finditer(rb"[ -~]{%d,}" % minimum, payload):
        rows.append({"rawOffset": match.start(), "text": match.group().decode("ascii")[:160]})
        if len(rows) >= limit:
            break
    return rows


def inventory(path: Path, variant: str, wanted: str) -> dict[str, object]:
    archive_bytes = path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        nested_name, nested_bytes = choose_nested(outer, variant)
    with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
        disk_name, disk = choose_disk(nested)

    if len(disk) < 512:
        raise ValueError("raw .st image is smaller than one boot sector")
    bpb = {
        "bytesPerSector": u16(disk, 11),
        "sectorsPerCluster": disk[13],
        "reservedSectors": u16(disk, 14),
        "fatCount": disk[16],
        "rootEntries": u16(disk, 17),
        "sectorsPerFat": u16(disk, 22),
    }
    root_sectors = (bpb["rootEntries"] * 32 + bpb["bytesPerSector"] - 1) // bpb["bytesPerSector"]
    root_offset = (
        bpb["reservedSectors"] + bpb["fatCount"] * bpb["sectorsPerFat"]
    ) * bpb["bytesPerSector"]
    file_record = find_root_file(disk, root_offset, bpb["rootEntries"], wanted)
    payload, clusters, raw_offsets = reconstruct(disk, file_record, bpb)
    file_record["clusterCount"] = len(clusters)
    file_record["clusters"] = clusters
    file_record["clusterRawOffsets"] = [f"0x{offset:x}" for offset in raw_offsets]
    file_record["payloadSha256"] = sha256(payload)
    return {
        "tool": "tools/atari_st_prg_inventory.py",
        "archiveSha256": sha256(archive_bytes),
        "nestedArchive": nested_name,
        "nestedArchiveSha256": sha256(nested_bytes),
        "disk": disk_name,
        "diskSha256": sha256(disk),
        "diskSize": len(disk),
        "bpb": bpb,
        "file": file_record,
        "tosHeader": tos_header(payload),
        "asciiStrings": ascii_strings(payload),
        "evidence": {
            "proven": [
                "FAT12 root entry、cluster chain、每個 cluster 的 raw offset 與 payload hash",
                "payload 開頭 28 bytes 與可重現的 big-endian header 欄位",
            ],
            "strongInference": [
                "0x601a branch word 代表 Atari TOS executable header candidate",
                "可讀字串可能是程式錯誤／提示文字，但未接到 runtime consumer",
            ],
            "unknown": [
                "68000 code 的函式邊界、graphics／font／rule consumer 與 Apple II 對應關係",
            ],
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--variant", choices=("[a2]", "[a]"), required=True)
    parser.add_argument("--file", default="SUNDOG.PRG")
    args = parser.parse_args()
    print(json.dumps(inventory(args.archive, args.variant, args.file), ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
