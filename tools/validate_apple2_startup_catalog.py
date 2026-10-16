#!/usr/bin/env python3
"""驗證 Apple II SYSTEM.STARTUP raw string evidence catalog 的 hash、offset 與繁中 draft。"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import zipfile
from pathlib import Path


STATUS_WINDOW_KEYS = {
    "apple2.startup.condition",
    "apple2.startup.vigor",
    "apple2.startup.rest",
    "apple2.startup.health",
    "apple2.startup.sleep",
}
STATUS_WINDOW_EVIDENCE = "docs/apple2/mame-status-label-trace-evidence.json"


def parse_hex(value: str) -> int:
    return int(value, 0)


def has_cjk(value: str) -> bool:
    return any("\u3400" <= char <= "\u9fff" for char in value)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("catalog", type=Path)
    args = parser.parse_args()

    document = json.loads(args.catalog.read_text(encoding="utf-8"))
    errors: list[str] = []
    archive_bytes = args.archive.read_bytes()
    with zipfile.ZipFile(args.archive) as archive:
        disk_name = document.get("disk", {}).get("name", "SUNDOG1.DSK")
        disk = archive.read(disk_name)

    expected_archive = document.get("sourceArchive", {}).get("sha256")
    if hashlib.sha256(archive_bytes).hexdigest() != expected_archive:
        errors.append("source archive sha256 mismatch")
    expected_disk = document.get("disk", {}).get("sha256")
    if hashlib.sha256(disk).hexdigest() != expected_disk:
        errors.append("disk sha256 mismatch")

    span = document.get("span", {})
    try:
        base = parse_hex(span["baseOffset"])
        block_size = int(span["blockSize"])
        start_block = int(span["startBlock"])
        next_block = int(span["nextBlock"])
        start = base + start_block * block_size
        end = base + next_block * block_size
    except (KeyError, TypeError, ValueError) as exc:
        errors.append(f"invalid span metadata: {exc}")
        start = end = 0

    if not (0 <= start < end <= len(disk)):
        errors.append(f"span outside disk: 0x{start:x}..0x{end:x}")
    else:
        raw_span = disk[start:end]
        if len(raw_span) != span.get("byteLength"):
            errors.append("span byteLength mismatch")
        if hashlib.sha256(raw_span).hexdigest() != span.get("sha256"):
            errors.append("span sha256 mismatch")

    entries = document.get("entries")
    if not isinstance(entries, list) or not entries:
        errors.append("entries is empty or not a list")
        entries = []
    keys: set[str] = set()
    for index, entry in enumerate(entries):
        key = entry.get("key") if isinstance(entry, dict) else None
        if not key or key in keys:
            errors.append(f"entry {index} has missing or duplicate key: {key!r}")
        keys.add(key)
        source = entry.get("source") if isinstance(entry, dict) else None
        translation = entry.get("translation") if isinstance(entry, dict) else None
        if not isinstance(source, str) or not source:
            errors.append(f"entry {index} has empty source")
            continue
        if not isinstance(translation, str) or not translation or not has_cjk(translation):
            errors.append(f"entry {index} has empty or non-CJK translation")
        if entry.get("evidenceLevel") != "proven-bytes":
            errors.append(f"entry {key} is not proven-bytes")
        consumer_status = entry.get("consumerStatus")
        if consumer_status not in {"unknown", "status-window-proven"}:
            errors.append(f"entry {key} has invalid consumerStatus: {consumer_status!r}")
        elif consumer_status == "status-window-proven":
            if key not in STATUS_WINDOW_KEYS:
                errors.append(f"entry {key} is not an approved status-window key")
            if entry.get("consumerEvidence") != STATUS_WINDOW_EVIDENCE:
                errors.append(f"entry {key} is missing status-window evidence path")
        elif "consumerEvidence" in entry:
            errors.append(f"entry {key} has consumerEvidence without a proven consumer")
        if entry.get("translationStatus") != "draft":
            errors.append(f"entry {key} translationStatus must remain draft")
        try:
            raw_offset = parse_hex(entry["rawOffset"])
            length = int(entry["length"])
            raw = disk[raw_offset : raw_offset + length]
            if raw_offset < start or raw_offset + length > end:
                errors.append(f"entry {key} is outside declared span")
            if raw != source.encode("ascii"):
                errors.append(f"entry {key} source does not match raw bytes")
            if raw.hex() != entry.get("rawHex"):
                errors.append(f"entry {key} rawHex mismatch")
        except (KeyError, TypeError, ValueError, UnicodeEncodeError) as exc:
            errors.append(f"entry {key} has invalid raw metadata: {exc}")

    result = {
        "status": "pass" if not errors else "fail",
        "entryCount": len(entries),
        "errors": errors,
        "disk": disk_name,
        "span": span.get("name"),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
