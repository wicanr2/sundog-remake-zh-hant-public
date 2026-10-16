#!/usr/bin/env python3
"""讀取 Atari ST raw .st nested archive 的 FAT12 root／cluster inventory。"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import struct
import zipfile
from pathlib import Path


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def u16(data: bytes, offset: int) -> int:
    return struct.unpack_from("<H", data, offset)[0]


def u32(data: bytes, offset: int) -> int:
    return struct.unpack_from("<I", data, offset)[0]


def choose_nested(outer: zipfile.ZipFile, variant: str) -> tuple[str, bytes]:
    candidates = [info for info in outer.infolist() if info.filename.lower().endswith(".zip")]
    variant_lower = variant.lower()
    matches = [info for info in candidates if variant_lower in info.filename.lower()]
    if not matches:
        raise ValueError(f"no nested archive matches variant {variant!r}")
    if len(matches) != 1:
        raise ValueError(f"variant {variant!r} matches {len(matches)} nested archives")
    info = matches[0]
    return info.filename, outer.read(info)


def choose_disk(nested: zipfile.ZipFile) -> tuple[str, bytes]:
    candidates = [info for info in nested.infolist() if info.filename.lower().endswith(".st")]
    if len(candidates) != 1:
        raise ValueError(f"nested archive contains {len(candidates)} raw .st members")
    info = candidates[0]
    return info.filename, nested.read(info)


def decode_fat12(fat: bytes, cluster: int) -> int:
    offset = cluster + cluster // 2
    if offset + 1 >= len(fat):
        return 0xFFF
    value = fat[offset] | (fat[offset + 1] << 8)
    if cluster & 1:
        return (value >> 4) & 0xFFF
    return value & 0xFFF


def cluster_chain(fat: bytes, start: int, limit: int) -> tuple[list[int], str | None]:
    if start < 2:
        return [], "no data cluster"
    chain: list[int] = []
    seen: set[int] = set()
    current = start
    while len(chain) < limit:
        if current in seen:
            return chain, f"cluster loop at {current}"
        seen.add(current)
        chain.append(current)
        next_cluster = decode_fat12(fat, current)
        if 0xFF8 <= next_cluster <= 0xFFF:
            return chain, None
        if next_cluster < 2 or next_cluster >= 0xFF0:
            return chain, f"invalid next cluster {next_cluster:#x}"
        current = next_cluster
    return chain, f"chain exceeded limit {limit}"


def inventory(path: Path, variant: str) -> dict[str, object]:
    archive_bytes = path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        nested_name, nested_bytes = choose_nested(outer, variant)
    with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
        disk_name, disk = choose_disk(nested)

    if len(disk) < 512:
        raise ValueError("raw .st image is smaller than one boot sector")
    bytes_per_sector = u16(disk, 11)
    sectors_per_cluster = disk[13]
    reserved_sectors = u16(disk, 14)
    fat_count = disk[16]
    root_entries = u16(disk, 17)
    total16 = u16(disk, 19)
    sectors_per_fat = u16(disk, 22)
    sectors_per_track = u16(disk, 24)
    heads = u16(disk, 26)
    total_sectors = total16 or u32(disk, 32)
    root_sectors = (root_entries * 32 + bytes_per_sector - 1) // bytes_per_sector
    fat_offset = reserved_sectors * bytes_per_sector
    root_offset = (reserved_sectors + fat_count * sectors_per_fat) * bytes_per_sector
    data_offset = root_offset + root_sectors * bytes_per_sector
    fat_size = sectors_per_fat * bytes_per_sector
    fat = disk[fat_offset : fat_offset + fat_size]

    warnings: list[str] = []
    actual_sectors = len(disk) // bytes_per_sector if bytes_per_sector else 0
    if bytes_per_sector == 0 or len(disk) % bytes_per_sector:
        warnings.append("image size is not an exact number of BPB sectors")
    if total_sectors != actual_sectors:
        warnings.append(f"BPB total sectors {total_sectors} differs from image sectors {actual_sectors}")

    files: list[dict[str, object]] = []
    ordinary_file_count = 0
    for index in range(root_entries):
        entry_offset = root_offset + index * 32
        entry = disk[entry_offset : entry_offset + 32]
        if len(entry) < 32 or entry[0] == 0x00:
            break
        if entry[0] == 0xE5 or entry[11] == 0x0F:
            continue
        raw_name = entry[0:8].decode("ascii", errors="replace").rstrip(" ")
        raw_ext = entry[8:11].decode("ascii", errors="replace").rstrip(" ")
        name = f"{raw_name}.{raw_ext}" if raw_ext else raw_name
        start_cluster = u16(entry, 26)
        size = u32(entry, 28)
        if entry[11] & 0x08:
            files.append(
                {
                    "directoryIndex": index,
                    "name": name,
                    "type": "volume-label",
                    "attributes": entry[11],
                    "size": size,
                }
            )
            continue
        ordinary_file_count += 1
        chain, chain_error = cluster_chain(fat, start_cluster, max(1, actual_sectors))
        file_record: dict[str, object] = {
            "directoryIndex": index,
            "name": name,
            "attributes": entry[11],
            "startCluster": start_cluster,
            "size": size,
            "clusterCount": len(chain),
            "clusters": chain,
        }
        if chain_error:
            file_record["chainWarning"] = chain_error
            warnings.append(f"{name}: {chain_error}")
        if start_cluster >= 2:
            file_record["firstDataRawOffset"] = data_offset + (start_cluster - 2) * sectors_per_cluster * bytes_per_sector
        files.append(file_record)

    if ordinary_file_count == 0 and any(fat[3:]):
        warnings.append("root directory has no ordinary files although FAT contains allocated entries")

    return {
        "archiveSha256": sha256(archive_bytes),
        "nestedArchive": nested_name,
        "nestedArchiveSha256": sha256(nested_bytes),
        "disk": disk_name,
        "diskSha256": sha256(disk),
        "diskSize": len(disk),
        "bpb": {
            "bytesPerSector": bytes_per_sector,
            "sectorsPerCluster": sectors_per_cluster,
            "reservedSectors": reserved_sectors,
            "fatCount": fat_count,
            "rootEntries": root_entries,
            "totalSectors": total_sectors,
            "sectorsPerFat": sectors_per_fat,
            "sectorsPerTrack": sectors_per_track,
            "heads": heads,
        },
        "layout": {
            "fatRawOffset": fat_offset,
            "rootRawOffset": root_offset,
            "dataRawOffset": data_offset,
            "rootSectors": root_sectors,
            "actualSectors": actual_sectors,
            "rootHeadHex": disk[root_offset : root_offset + min(64, root_entries * 32)].hex(),
        },
        "files": files,
        "warnings": warnings,
        "evidence": {
            "proven": "BPB fields、root entries、FAT12 cluster links 與 raw offsets 能重現解析",
            "strongInference": "檔案內容的用途可由檔名／大小作研究線索",
            "unknown": "SunDog runtime 是否消費這些檔案，以及 graphics／font／rule schema",
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--variant", choices=("[a2]", "[a]"), required=True)
    args = parser.parse_args()
    print(json.dumps(inventory(args.archive, args.variant), ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
