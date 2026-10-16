#!/usr/bin/env python3
"""驗證 Apple II Disk 2 工程艙標籤的 raw segment／offset／雜湊證據。"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path
from typing import Any


def parse_offset(value: str | int) -> int:
    if isinstance(value, int):
        return value
    return int(value, 0)


def validate(disk: bytes, document: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    disk_info = document.get("input", {}).get("disk2", {})
    if disk_info.get("size") != len(disk):
        errors.append("disk size mismatch")
    if disk_info.get("sha256") != hashlib.sha256(disk).hexdigest():
        errors.append("disk sha256 mismatch")

    segment_info = document.get("rawTableSegment", {})
    try:
        offset = parse_offset(segment_info["offset"])
        length = int(segment_info["length"])
        expected_hex = str(segment_info["rawHex"])
    except (KeyError, TypeError, ValueError) as exc:
        return [*errors, f"invalid rawTableSegment metadata: {exc}"]
    if offset < 0 or length <= 0 or offset + length > len(disk):
        return [*errors, "rawTableSegment outside disk"]

    segment = disk[offset : offset + length]
    if segment.hex() != expected_hex:
        errors.append("rawTableSegment rawHex mismatch")
    if segment_info.get("sha256") != hashlib.sha256(segment).hexdigest():
        errors.append("rawTableSegment sha256 mismatch")

    label_info = segment_info.get("label", {})
    try:
        value = str(label_info["value"])
        label_offset = int(label_info["offsetWithinSegment"])
        raw_offset = parse_offset(label_info["rawOffset"])
        expected_label_hex = str(label_info["rawHex"])
        encoded = value.encode("ascii")
    except (KeyError, TypeError, ValueError, UnicodeEncodeError) as exc:
        return [*errors, f"invalid label metadata: {exc}"]
    if raw_offset != offset + label_offset:
        errors.append("label rawOffset does not match segment offset")
    if label_offset < 0 or label_offset + len(encoded) > len(segment):
        errors.append("label outside rawTableSegment")
    elif segment[label_offset : label_offset + len(encoded)] != encoded:
        errors.append("label bytes do not match rawTableSegment")
    if encoded.hex() != expected_label_hex:
        errors.append("label rawHex mismatch")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("disk", type=Path)
    parser.add_argument("evidence", type=Path)
    args = parser.parse_args()

    disk = args.disk.read_bytes()
    document = json.loads(args.evidence.read_text(encoding="utf-8"))
    errors = validate(disk, document)
    result = {
        "status": "pass" if not errors else "fail",
        "diskBytes": len(disk),
        "errors": errors,
        "label": document.get("rawTableSegment", {}).get("label", {}).get("value"),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
