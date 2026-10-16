#!/usr/bin/env python3
"""驗證 SYSTEM.APPLE 磁碟查表排除證據的 hash、offset 與消費端 bytes。"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
import zipfile
from pathlib import Path


def parse_hex(value: str) -> int:
    return int(value, 0)


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
    disk_meta = document.get("disk", {})
    if len(disk) != disk_meta.get("size"):
        errors.append("disk size mismatch")
    if hashlib.sha256(disk).hexdigest() != disk_meta.get("sha256"):
        errors.append("disk sha256 mismatch")

    file_span = document.get("fileSpan", {})
    try:
        span_start = parse_hex(file_span["rawStart"])
        span_end = parse_hex(file_span["rawEndExclusive"])
    except (KeyError, TypeError, ValueError) as exc:
        errors.append(f"invalid file span: {exc}")
        span_start = span_end = 0
    if not (0 <= span_start < span_end <= len(disk)):
        errors.append("file span outside disk")
    else:
        span = disk[span_start:span_end]
        if len(span) != file_span.get("size"):
            errors.append("file span size mismatch")
        if hashlib.sha256(span).hexdigest() != file_span.get("sha256"):
            errors.append("file span sha256 mismatch")

    table = document.get("candidateTable", {})
    try:
        table_start = parse_hex(table["rawOffsetStart"])
        table_end = parse_hex(table["rawOffsetEndExclusive"])
        file_start = parse_hex(table["fileOffsetStart"])
        file_end = parse_hex(table["fileOffsetEndExclusive"])
    except (KeyError, TypeError, ValueError) as exc:
        errors.append(f"invalid candidate table offsets: {exc}")
        table_start = table_end = file_start = file_end = 0
    if table_start != span_start + file_start or table_end != span_start + file_end:
        errors.append("candidate table file/raw offset mapping mismatch")
    if not (span_start <= table_start < table_end <= span_end):
        errors.append("candidate table outside SYSTEM.APPLE span")
    else:
        table_bytes = disk[table_start:table_end]
        if len(table_bytes) != table.get("size"):
            errors.append("candidate table size mismatch")
        if hashlib.sha256(table_bytes).hexdigest() != table.get("sha256"):
            errors.append("candidate table sha256 mismatch")

    for index, row in enumerate(document.get("consumerObservations", [])):
        try:
            offset = span_start + parse_hex(row["fileOffset"])
            expected = bytes.fromhex(row["rawBytes"])
            raw = disk[offset : offset + len(expected)]
        except (KeyError, TypeError, ValueError) as exc:
            errors.append(f"consumer row {index} invalid: {exc}")
            continue
        if raw != expected:
            errors.append(f"consumer row {index} raw bytes mismatch")
        if row.get("evidenceLevel") != "proven-bytes":
            errors.append(f"consumer row {index} must remain proven-bytes")

    interpretation = document.get("interpretation", {})
    if interpretation.get("status") != "strong-inference":
        errors.append("interpretation must remain strong-inference")
    if interpretation.get("classification") != "disk-nibble-or-controller-lookup":
        errors.append("interpretation classification changed")
    if interpretation.get("notGraphicsAsset") is not True:
        errors.append("candidate must remain excluded from graphics assets")

    result = {
        "status": "pass" if not errors else "fail",
        "errors": errors,
        "artifact": document.get("artifact"),
        "consumerCount": len(document.get("consumerObservations", [])),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
