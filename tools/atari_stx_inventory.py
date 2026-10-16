#!/usr/bin/env python3
"""盤點 Pasti STX header／track／sector descriptors，不解碼 protected payload。"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import struct
import zipfile
from pathlib import Path


TRACK_DESCRIPTOR_SIZE = 16
SECTOR_DESCRIPTOR_SIZE = 16


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def u16(data: bytes, offset: int) -> int:
    return struct.unpack_from("<H", data, offset)[0]


def u32(data: bytes, offset: int) -> int:
    return struct.unpack_from("<I", data, offset)[0]


def choose_stx(archive: Path) -> tuple[str, bytes, str]:
    outer_bytes = archive.read_bytes()
    with zipfile.ZipFile(io.BytesIO(outer_bytes)) as outer:
        nested_infos = [info for info in outer.infolist() if info.filename.lower().endswith(".zip")]
        for nested_info in nested_infos:
            nested_bytes = outer.read(nested_info)
            with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
                stx_infos = [info for info in nested.infolist() if info.filename.lower().endswith(".stx")]
                if stx_infos:
                    if len(stx_infos) != 1:
                        raise ValueError(f"nested archive {nested_info.filename!r} has multiple STX members")
                    info = stx_infos[0]
                    return info.filename, nested.read(info), nested_info.filename
    raise ValueError("no nested STX member found")


def parse_track(data: bytes, offset: int) -> tuple[dict[str, object], list[str]]:
    warnings: list[str] = []
    if offset + TRACK_DESCRIPTOR_SIZE > len(data):
        return {}, [f"track descriptor at {offset:#x} is truncated"]
    record_size = u32(data, offset)
    fuzzy_count = u32(data, offset + 4)
    sector_count = u16(data, offset + 8)
    track_flags = u16(data, offset + 10)
    track_length = u16(data, offset + 12)
    track_number = data[offset + 14]
    track_type = data[offset + 15]
    if record_size < TRACK_DESCRIPTOR_SIZE:
        warnings.append(f"record size {record_size} is smaller than descriptor")
    record_end = offset + record_size
    if record_end > len(data):
        warnings.append(f"record ends at {record_end:#x}, beyond file size {len(data):#x}")
        record_end = len(data)

    sector_descriptors: list[dict[str, object]] = []
    descriptor_start = offset + TRACK_DESCRIPTOR_SIZE
    has_sector_descriptors = bool(track_flags & 0x0001)
    if has_sector_descriptors:
        expected_end = descriptor_start + sector_count * SECTOR_DESCRIPTOR_SIZE
        if expected_end > record_end:
            warnings.append("sector descriptor area exceeds track record")
        for index in range(sector_count):
            descriptor_offset = descriptor_start + index * SECTOR_DESCRIPTOR_SIZE
            if descriptor_offset + SECTOR_DESCRIPTOR_SIZE > record_end:
                break
            sector_descriptors.append(
                {
                    "rawOffset": descriptor_offset,
                    "dataOffset": u32(data, descriptor_offset),
                    "bitPosition": u16(data, descriptor_offset + 4),
                    "readTime": u16(data, descriptor_offset + 6),
                    "id": {
                        "track": data[descriptor_offset + 8],
                        "head": data[descriptor_offset + 9],
                        "number": data[descriptor_offset + 10],
                        "sizeCode": data[descriptor_offset + 11],
                        "crc": u16(data, descriptor_offset + 12),
                    },
                    "fdcFlags": data[descriptor_offset + 14],
                    "reserved": data[descriptor_offset + 15],
                }
            )

    track = {
        "rawOffset": offset,
        "recordSize": record_size,
        "recordEnd": record_end,
        "fuzzyCount": fuzzy_count,
        "sectorCount": sector_count,
        "trackFlags": track_flags,
        "trackLength": track_length,
        "trackNumberRaw": track_number,
        "side": (track_number >> 7) & 1,
        "trackNumber": track_number & 0x7F,
        "trackType": track_type,
        "hasSectorDescriptors": has_sector_descriptors,
        "sectorDescriptors": sector_descriptors,
    }
    if not has_sector_descriptors and sector_count:
        track["implicitSectorSize"] = 512
        track["implicitSectorNumbering"] = "1..sectorCount"
    return track, warnings


def inventory(archive: Path) -> dict[str, object]:
    member, data, nested_archive = choose_stx(archive)
    errors: list[str] = []
    warnings: list[str] = []
    if len(data) < 16:
        raise ValueError("STX member is smaller than file descriptor")
    magic = data[:4]
    version = u16(data, 4)
    tool = u16(data, 6)
    reserved_1 = u16(data, 8)
    track_count = data[10]
    revision = data[11]
    reserved_2 = u32(data, 12)
    if magic != b"RSY\x00":
        errors.append(f"unexpected magic {magic!r}")
    if version != 3:
        errors.append(f"unsupported STX version {version}")

    tracks: list[dict[str, object]] = []
    offset = 16
    for index in range(track_count):
        track, track_warnings = parse_track(data, offset)
        warnings.extend(f"track {index}: {warning}" for warning in track_warnings)
        if not track:
            errors.append(f"track {index} descriptor is truncated")
            break
        tracks.append(track)
        next_offset = int(track["recordEnd"])
        if next_offset <= offset:
            errors.append(f"track {index} does not advance: {offset:#x} → {next_offset:#x}")
            break
        offset = next_offset
    if len(tracks) != track_count:
        warnings.append(f"parsed {len(tracks)} track records, header declares {track_count}")
    if offset != len(data):
        warnings.append(f"track records end at {offset:#x}, file size is {len(data):#x}")

    descriptor_count = sum(len(track["sectorDescriptors"]) for track in tracks)
    return {
        "archiveSha256": sha256(archive.read_bytes()),
        "nestedArchive": nested_archive,
        "stxMember": member,
        "stxSha256": sha256(data),
        "stxSize": len(data),
        "fileDescriptor": {
            "magic": magic.decode("latin-1"),
            "version": version,
            "tool": tool,
            "reserved1": reserved_1,
            "trackCount": track_count,
            "revision": revision,
            "reserved2": reserved_2,
        },
        "parsedTrackCount": len(tracks),
        "sectorDescriptorCount": descriptor_count,
        "tracks": tracks,
        "errors": errors,
        "warnings": warnings,
        "evidence": {
            "proven": "RSY header、little-endian descriptor欄位、recordSize邊界與 sector descriptor raw offsets 能重現解析",
            "strongInference": "sector descriptor 的 dataOffset 可作 payload 取址候選",
            "unknown": "payload 是否為正常檔案 sector、保護機制、68000 consumer 與 tileset 語意",
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    args = parser.parse_args()
    print(json.dumps(inventory(args.archive), ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
