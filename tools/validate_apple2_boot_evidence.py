#!/usr/bin/env python3
"""驗證 Apple II SYSTEM.APPLE 啟動提示 evidence manifest 的 hash、offset 與 rawHex。"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import zipfile
from pathlib import Path


def parse_hex(value: str) -> int:
    return int(value, 0)


def has_cjk(value: str) -> bool:
    return any("\u3400" <= char <= "\u9fff" for char in value)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("evidence", type=Path)
    args = parser.parse_args()

    document = json.loads(args.evidence.read_text(encoding="utf-8"))
    errors: list[str] = []
    archive_bytes = args.archive.read_bytes()
    with zipfile.ZipFile(args.archive) as archive:
        disk_name = document.get("disk", {}).get("name", "SUNDOG1.DSK")
        disk = archive.read(disk_name)

    if hashlib.sha256(archive_bytes).hexdigest() != document.get("sourceArchive", {}).get("sha256"):
        errors.append("source archive sha256 mismatch")
    disk_metadata = document.get("disk", {})
    if len(disk) != disk_metadata.get("size"):
        errors.append("disk size mismatch")
    if hashlib.sha256(disk).hexdigest() != disk_metadata.get("sha256"):
        errors.append("disk sha256 mismatch")

    span = document.get("declaredSpan", {})
    try:
        start = parse_hex(span["rawStart"])
        end = parse_hex(span["rawEndExclusive"])
        start_block = int(span["startBlock"])
        next_block = int(span["nextBlock"])
        block_size = int(span["blockSize"])
        expected_start = parse_hex(span["baseOffset"]) + start_block * block_size
        expected_end = parse_hex(span["baseOffset"]) + next_block * block_size
        if (start, end) != (expected_start, expected_end):
            errors.append("declared raw span does not match block arithmetic")
    except (KeyError, TypeError, ValueError) as exc:
        errors.append(f"invalid span metadata: {exc}")
        start = end = 0

    if not (0 <= start < end <= len(disk)):
        errors.append(f"span outside disk: 0x{start:x}..0x{end:x}")
    else:
        raw_span = disk[start:end]
        if len(raw_span) != span.get("size"):
            errors.append("span size mismatch")
        if hashlib.sha256(raw_span).hexdigest() != span.get("sha256"):
            errors.append("span sha256 mismatch")

    entries = document.get("entries")
    if not isinstance(entries, list) or not entries:
        errors.append("entries is empty or not a list")
        entries = []
    for index, entry in enumerate(entries):
        if not isinstance(entry, dict):
            errors.append(f"entry {index} is not an object")
            continue
        label = entry.get("rawOffset", index)
        source = entry.get("sourceText")
        translation = entry.get("translation")
        if not isinstance(source, str) or not source:
            errors.append(f"entry {label} has empty sourceText")
            continue
        if not isinstance(translation, str) or not translation or not has_cjk(translation):
            errors.append(f"entry {label} has empty or non-CJK translation")
        if entry.get("evidenceLevel") != "proven-bytes":
            errors.append(f"entry {label} is not proven-bytes")
        if entry.get("consumerStatus") != "unknown":
            errors.append(f"entry {label} consumerStatus must remain unknown")
        if entry.get("translationStatus") != "draft":
            errors.append(f"entry {label} translationStatus must remain draft")
        try:
            raw_offset = parse_hex(entry["rawOffset"])
            length = int(entry["length"])
            raw = disk[raw_offset : raw_offset + length]
            if raw_offset < start or raw_offset + length > end:
                errors.append(f"entry {label} is outside declared span")
            if raw_offset - start != parse_hex(entry["spanOffset"]):
                errors.append(f"entry {label} spanOffset mismatch")
            if raw != source.encode("ascii"):
                errors.append(f"entry {label} sourceText does not match raw bytes")
            if raw.hex(" ") != entry.get("rawHex"):
                errors.append(f"entry {label} rawHex mismatch")
        except (KeyError, TypeError, ValueError, UnicodeEncodeError) as exc:
            errors.append(f"entry {label} has invalid raw metadata: {exc}")

    result = {
        "status": "pass" if not errors else "fail",
        "entryCount": len(entries),
        "errors": errors,
        "disk": disk_name,
        "artifact": document.get("artifact"),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())

