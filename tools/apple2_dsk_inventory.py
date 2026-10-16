#!/usr/bin/env python3
"""建立 Apple II 5.25 吋 .DSK 的幾何與 sector-order 候選盤點。

這個工具不把檔案大小或字串直接升格成 filesystem 判定。輸出會同時保留：

* raw disk offset；
* 35 tracks × 16 sectors × 256 bytes 的幾何候選；
* DOS 3.3 16-sector interleave 的 logical-sector 候選；
* 啟動／檔名標記的位置；
* Track 17 的 VTOC 候選 bytes，供後續人工或 parser 交叉驗證。

輸入 archive、disk 與 directory 只讀；結果輸出到 stdout，適合由 Docker 工作產生並重跑。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import zipfile
from pathlib import Path


TRACKS = 35
SECTORS_PER_TRACK = 16
SECTOR_SIZE = 256
TRACK_SIZE = SECTORS_PER_TRACK * SECTOR_SIZE
EXPECTED_SIZE = TRACKS * TRACK_SIZE

# Apple II DOS 3.3 16-sector logical-to-physical order candidate.  This is
# deliberately labeled as a candidate: a .DSK byte stream can use another
# order, and the source must be validated by a real directory/boot consumer.
DOS33_PHYSICAL_ORDER = (0, 7, 14, 6, 13, 5, 12, 4, 11, 3, 10, 2, 9, 1, 8, 15)
DOS33_PHYSICAL_TO_LOGICAL = {
    physical: logical for logical, physical in enumerate(DOS33_PHYSICAL_ORDER)
}

MARKERS = (
    b"MUST BOOT FROM SLOT 6",
    b"SYSTEM.DATA",
    b"SUNDOG.LIBRARY",
    b"SYSTEM.STARTUP",
    b"SYSTEM.APPLE",
    b"SYSTEM.PASCAL",
)


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def locations(data: bytes, marker: bytes, limit: int = 16) -> list[dict[str, int]]:
    result: list[dict[str, int]] = []
    start = 0
    while len(result) < limit:
        offset = data.find(marker, start)
        if offset < 0:
            break
        track, remainder = divmod(offset, TRACK_SIZE)
        physical_sector, sector_offset = divmod(remainder, SECTOR_SIZE)
        result.append(
            {
                "rawOffset": offset,
                "track": track,
                "physicalSector": physical_sector,
                "sectorOffset": sector_offset,
                "dos33LogicalSectorCandidate": DOS33_PHYSICAL_TO_LOGICAL.get(
                    physical_sector
                ),
            }
        )
        start = offset + 1
    return result


def track_sector(data: bytes, track: int, physical_sector: int) -> bytes:
    start = (track * SECTORS_PER_TRACK + physical_sector) * SECTOR_SIZE
    return data[start : start + SECTOR_SIZE]


def optional_byte(data: bytes, offset: int) -> int | None:
    """Return a candidate field without turning a truncated image into a crash."""

    if 0 <= offset < len(data):
        return data[offset]
    return None


def vtoc_candidate(data: bytes) -> dict[str, object]:
    sector = track_sector(data, 17, DOS33_PHYSICAL_ORDER[0])
    return {
        "mapping": "DOS 3.3 16-sector logical sector 0 candidate",
        "track": 17,
        "logicalSector": 0,
        "physicalSector": DOS33_PHYSICAL_ORDER[0],
        "headHex": sector[:16].hex(),
        "candidateFields": {
            "catalogTrackByte01": optional_byte(sector, 0x01),
            "catalogSectorByte02": optional_byte(sector, 0x02),
            "dosReleaseByte03": optional_byte(sector, 0x03),
            "maxTrackByte27": optional_byte(sector, 0x27),
            "volumeByte30": optional_byte(sector, 0x30),
        },
    }


def disk_record(name: str, data: bytes) -> dict[str, object]:
    return {
        "name": name,
        "size": len(data),
        "sha256": sha256(data),
        "geometryCandidate": {
            "tracks": TRACKS,
            "sectorsPerTrack": SECTORS_PER_TRACK,
            "sectorSize": SECTOR_SIZE,
            "sizeMatches": len(data) == EXPECTED_SIZE,
        },
        "sectorOrderCandidates": {
            "rawSequential": "每 4096 bytes 視為一軌、每 256 bytes 視為一個 raw sector",
            "dos33LogicalToPhysical": list(DOS33_PHYSICAL_ORDER),
            "dos33PhysicalToLogical": DOS33_PHYSICAL_TO_LOGICAL,
        },
        "sector0HeadHex": data[:16].hex(),
        "track17PhysicalSectorHeadsHex": [
            track_sector(data, 17, physical)[:8].hex()
            for physical in range(SECTORS_PER_TRACK)
        ],
        "dos33VtocCandidate": vtoc_candidate(data),
        "markers": {
            marker.decode("ascii"): locations(data, marker) for marker in MARKERS
        },
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "input_path",
        type=Path,
        help="包含 .DSK 的 zip、單張 .dsk，或只含目標 .dsk 的資料夾",
    )
    return parser.parse_args()


def manifest_sha256(source_files: list[dict[str, object]]) -> str:
    digest = hashlib.sha256()
    for source in source_files:
        digest.update(str(source["name"]).encode("utf-8"))
        digest.update(b"\x00")
        digest.update(str(source["sha256"]).encode("ascii"))
        digest.update(b"\n")
    return digest.hexdigest()


def load_input(
    input_path: Path,
) -> tuple[str, str | None, list[dict[str, object]], list[tuple[str, bytes]]]:
    """Read an input without changing its byte order or writing a copy.

    The returned disk tuples retain source-relative names and raw bytes.  The
    optional input SHA is the container hash only; each disk always receives
    its own SHA in ``source_files`` and ``disk_record``.
    """

    if not input_path.exists():
        raise ValueError(f"input does not exist: {input_path}")

    if input_path.is_dir():
        paths = sorted(
            path
            for path in input_path.iterdir()
            if path.is_file() and path.suffix.lower() == ".dsk"
        )
        if not paths:
            raise ValueError(f"input directory contains no .dsk files: {input_path}")
        disk_items = [
            (path.relative_to(input_path).as_posix(), path.read_bytes()) for path in paths
        ]
        input_kind = "directory"
        input_sha = None
    elif input_path.is_file() and input_path.suffix.lower() == ".dsk":
        disk_items = [(input_path.name, input_path.read_bytes())]
        input_kind = "disk"
        input_sha = sha256(disk_items[0][1])
    elif input_path.is_file() and zipfile.is_zipfile(input_path):
        disk_items = []
        with zipfile.ZipFile(input_path) as archive:
            for name in archive.namelist():
                if name.lower().endswith(".dsk"):
                    disk_items.append((name, archive.read(name)))
        if not disk_items:
            raise ValueError(f"zip contains no .dsk files: {input_path}")
        input_kind = "zip"
        input_sha = sha256(input_path.read_bytes())
    else:
        raise ValueError(
            "input must be a .dsk file, a directory containing .dsk files, or a zip archive"
        )

    source_files = [
        {"name": name, "size": len(data), "sha256": sha256(data)}
        for name, data in disk_items
    ]
    return input_kind, input_sha, source_files, disk_items


def main() -> None:
    args = parse_args()
    try:
        input_kind, input_sha, source_files, disk_items = load_input(args.input_path)
    except ValueError as error:
        raise SystemExit(f"error: {error}") from error
    records = [disk_record(name, data) for name, data in disk_items]

    output = {
        "schema": 2,
        "tool": "tools/apple2_dsk_inventory.py",
        "input": str(args.input_path),
        "inputKind": input_kind,
        "inputSha256": input_sha,
        "archive": str(args.input_path) if input_kind == "zip" else None,
        "archiveSha256": input_sha if input_kind == "zip" else None,
        "sourceManifestSha256": manifest_sha256(source_files),
        "sourceFiles": source_files,
        "diskCount": len(records),
        "disks": records,
        "interpretation": {
            "proven": [
                "source bytes、hash、大小與 raw offset",
                "143360 bytes 可由 35×16×256 幾何表示",
            ],
            "strongInference": [
                "DOS 3.3 16-sector interleave 僅作 logical-sector 候選",
                "啟動／檔名 marker 可能屬於 boot 或程式內資料，尚未證明是 filesystem directory",
            ],
            "unknown": [
                "真正 filesystem、sector order、檔案目錄 consumer 與啟動後 loader 邊界",
            ],
        },
    }
    print(json.dumps(output, ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
