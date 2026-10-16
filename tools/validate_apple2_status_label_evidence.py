#!/usr/bin/env python3
"""驗證 Apple II 角色狀態標籤的 raw offset／bytes／雙磁碟雜湊證據。"""

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


def has_cjk(value: str) -> bool:
    return any("\u3400" <= char <= "\u9fff" for char in value)


def validate(disk1: bytes, disk2: bytes, document: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    inputs = document.get("input", {})
    for key, disk in (("disk1", disk1), ("disk2", disk2)):
        info = inputs.get(key, {})
        if info.get("size") != len(disk):
            errors.append(f"{key} size mismatch")
        if info.get("sha256") != hashlib.sha256(disk).hexdigest():
            errors.append(f"{key} sha256 mismatch")

    anchors = document.get("rawLabelAnchors")
    if not isinstance(anchors, list) or not anchors:
        return [*errors, "rawLabelAnchors is empty or not a list"]

    keys: set[str] = set()
    for index, anchor in enumerate(anchors):
        if not isinstance(anchor, dict):
            errors.append(f"anchor {index} is not an object")
            continue
        key = anchor.get("key")
        if not isinstance(key, str) or not key or key in keys:
            errors.append(f"anchor {index} has missing or duplicate key: {key!r}")
            continue
        keys.add(key)
        try:
            value = str(anchor["value"])
            translation = str(anchor["translation"])
            offset = parse_offset(anchor["rawOffset"])
            expected_hex = str(anchor["rawHex"])
            encoded = value.encode("ascii")
        except (KeyError, TypeError, ValueError, UnicodeEncodeError) as exc:
            errors.append(f"anchor {key} has invalid metadata: {exc}")
            continue
        if not translation or not has_cjk(translation):
            errors.append(f"anchor {key} has empty or non-CJK translation")
        if encoded.hex() != expected_hex:
            errors.append(f"anchor {key} rawHex mismatch")
        for disk_key, disk in (("disk1", disk1), ("disk2", disk2)):
            if offset < 0 or offset + len(encoded) > len(disk):
                errors.append(f"anchor {key} is outside {disk_key}")
            elif disk[offset : offset + len(encoded)] != encoded:
                errors.append(f"anchor {key} {disk_key} raw bytes mismatch")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("disk1", type=Path)
    parser.add_argument("disk2", type=Path)
    parser.add_argument("evidence", type=Path)
    args = parser.parse_args()

    document = json.loads(args.evidence.read_text(encoding="utf-8"))
    errors = validate(args.disk1.read_bytes(), args.disk2.read_bytes(), document)
    result = {
        "status": "pass" if not errors else "fail",
        "anchorCount": len(document.get("rawLabelAnchors", [])),
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
