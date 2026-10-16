#!/usr/bin/env python3
"""Validate the Apple II word-boundary evidence catalog against raw bytes."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from apple2_word_boundary_candidates import decode_candidates, load_disk


def parse_int(value: str) -> int:
    return int(value, 0)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("disk_or_archive", type=Path)
    parser.add_argument("catalog", type=Path)
    args = parser.parse_args()

    catalog = json.loads(args.catalog.read_text(encoding="utf-8"))
    display_name, raw, archive_name, archive_sha256 = load_disk(args.disk_or_archive)
    errors: list[str] = []

    if catalog.get("schema") != 1:
        errors.append("catalog schema must be 1")
    if catalog.get("disk", {}).get("name") != display_name:
        errors.append(f"disk name mismatch: {catalog.get('disk', {}).get('name')} != {display_name}")
    if catalog.get("disk", {}).get("size") != len(raw):
        errors.append("disk size mismatch")
    if catalog.get("disk", {}).get("sha256") != __import__("hashlib").sha256(raw).hexdigest():
        errors.append("disk SHA-256 mismatch")
    if archive_name is not None and catalog.get("sourceArchive", {}).get("name") != archive_name:
        errors.append("archive name mismatch")
    if archive_sha256 is not None and catalog.get("sourceArchive", {}).get("sha256") != archive_sha256:
        errors.append("archive SHA-256 mismatch")

    region = catalog.get("region", {})
    try:
        start = parse_int(region["start"])
        end = parse_int(region["endExclusive"])
    except (KeyError, TypeError, ValueError) as exc:
        errors.append(f"invalid region: {exc}")
        start = end = 0

    expected = decode_candidates(raw, start, end) if not errors or end else []
    actual = catalog.get("entries", [])
    if len(actual) != len(expected):
        errors.append(f"entry count mismatch: {len(actual)} != {len(expected)}")

    for index, (candidate, evidence) in enumerate(zip(expected, actual, strict=False)):
        for field in ("offset", "endOffsetExclusive", "length", "source", "rawHex"):
            if evidence.get(field) != candidate.get(field):
                errors.append(
                    f"entry {index} {field} mismatch: {evidence.get(field)!r} != {candidate.get(field)!r}"
                )
        if not evidence.get("translation"):
            errors.append(f"entry {index} translation is empty")
        if evidence.get("translationStatus") not in {"draft", "final"}:
            errors.append(f"entry {index} translationStatus must be draft or final")

    result = {
        "catalog": args.catalog.name,
        "disk": display_name,
        "entryCount": len(actual),
        "errors": errors,
        "status": "pass" if not errors else "fail",
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    raise SystemExit(main())
