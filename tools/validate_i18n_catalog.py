#!/usr/bin/env python3
"""驗證 Apple II source catalog 的 hash、offset、長度與繁中翻譯欄位。"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import zipfile
from pathlib import Path


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def parse_int(value: str | int) -> int:
    if isinstance(value, int):
        return value
    return int(value, 0)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("catalog", type=Path)
    args = parser.parse_args()

    catalog = json.loads(args.catalog.read_text(encoding="utf-8"))
    source = catalog["source"]
    errors: list[str] = []
    checks: list[str] = []

    archive_bytes = args.archive.read_bytes()
    archive_hash = sha256(archive_bytes)
    if archive_hash != source["archiveSha256"]:
        errors.append(f"archive hash mismatch: {archive_hash}")
    else:
        checks.append("archive hash")

    with zipfile.ZipFile(args.archive) as archive:
        try:
            disk = archive.read(source["disk"])
        except KeyError:
            errors.append(f"missing disk member: {source['disk']}")
            disk = b""

    disk_hash = sha256(disk)
    if disk_hash != source["diskSha256"]:
        errors.append(f"disk hash mismatch: {disk_hash}")
    else:
        checks.append("disk hash")

    region = source["region"]
    start = parse_int(region["start"])
    end = parse_int(region["end"])
    stride = int(region["stride"])
    record_size = int(region["recordSize"])
    entries = catalog["entries"]
    seen_keys: set[str] = set()
    seen_offsets: set[int] = set()
    draft_count = 0

    for entry in entries:
        key = entry["key"]
        offset = parse_int(entry["offset"])
        length = int(entry["length"])
        if key in seen_keys:
            errors.append(f"duplicate key: {key}")
        seen_keys.add(key)
        if offset in seen_offsets:
            errors.append(f"duplicate offset: {entry['offset']}")
        seen_offsets.add(offset)
        if offset < start or offset + record_size > end:
            errors.append(f"entry outside region: {key} at {entry['offset']}")
            continue
        record = disk[offset : offset + record_size]
        if not record:
            errors.append(f"entry outside disk: {key}")
            continue
        if record[0] != length:
            errors.append(f"length prefix mismatch: {key}: {record[0]} != {length}")
        try:
            decoded = record[1 : 1 + length].decode("ascii")
        except UnicodeDecodeError:
            errors.append(f"non-ASCII source bytes: {key}")
            decoded = ""
        if decoded != entry["source"]:
            errors.append(f"source mismatch: {key}: {decoded!r} != {entry['source']!r}")
        if not entry.get("translation", "").strip():
            errors.append(f"empty translation: {key}")
        if entry.get("translationStatus") == "draft":
            draft_count += 1

    for excluded in catalog.get("excludedRecords", []):
        offset = parse_int(excluded["offset"])
        if offset < start or offset + record_size > end:
            errors.append(f"excluded record outside region: {excluded['offset']}")
        elif disk[offset] != 0:
            errors.append(f"excluded record is not zero-prefixed: {excluded['offset']}")

    expected_count = (end - start) // stride
    if len(entries) != expected_count - len(catalog.get("excludedRecords", [])):
        errors.append(
            "entry count does not cover fixed stride region after exclusions: "
            f"{len(entries)} != {expected_count - len(catalog.get('excludedRecords', []))}"
        )
    else:
        checks.append("fixed stride coverage")

    result = {
        "status": "pass" if not errors else "fail",
        "checks": checks,
        "errors": errors,
        "entryCount": len(entries),
        "draftCount": draft_count,
        "excludedCount": len(catalog.get("excludedRecords", [])),
        "sourceRegion": region,
        "archiveSha256": archive_hash,
        "diskSha256": disk_hash,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
