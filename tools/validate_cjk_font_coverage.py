#!/usr/bin/env python3
"""以 fc-query 驗證繁中 catalog、手冊與 runtime 文字所需 CJK glyph。"""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
from pathlib import Path


def collect_text(root: Path) -> str:
    paths: list[Path] = []
    paths.extend(sorted((root / "docs/i18n").glob("*.json")))
    paths.extend(sorted((root / "docs/manual").glob("*zh-Hant.md")))
    for directory in (root / "cmd", root / "internal"):
        if directory.exists():
            paths.extend(sorted(path for path in directory.rglob("*.go") if path.is_file()))
    chunks: list[str] = []
    for path in paths:
        chunks.append(path.read_text(encoding="utf-8"))
    return "\n".join(chunks)


def parse_charset(value: str) -> set[int]:
    supported: set[int] = set()
    for token in value.split():
        if "-" in token:
            start_text, end_text = token.split("-", 1)
            start, end = int(start_text, 16), int(end_text, 16)
            supported.update(range(start, end + 1))
        else:
            supported.add(int(token, 16))
    return supported


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("font", type=Path)
    parser.add_argument("root", type=Path)
    args = parser.parse_args()

    charset = subprocess.run(
        ["fc-query", "--format=%{charset}\n", str(args.font)],
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    supported = parse_charset(charset)
    text = collect_text(args.root)
    required = {
        ord(char)
        for char in text
        if "\u3400" <= char <= "\u4dbf" or "\u4e00" <= char <= "\u9fff"
        or "\u3040" <= char <= "\u30ff" or "\uac00" <= char <= "\ud7a3"
        or "\u1100" <= char <= "\u11ff" or "\u3130" <= char <= "\u318f"
    }
    missing = sorted(required - supported)
    result = {
        "status": "pass" if not missing else "fail",
        "font": str(args.font),
        "fontSha256": hashlib.sha256(args.font.read_bytes()).hexdigest(),
        "requiredCjkCodepointCount": len(required),
        "missing": [f"U+{codepoint:04X}" for codepoint in missing],
        "scannedInputs": [
            "docs/i18n/*.json",
            "docs/manual/*zh-Hant.md",
            "cmd/**/*.go",
            "internal/**/*.go",
        ],
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not missing else 1


if __name__ == "__main__":
    sys.exit(main())
