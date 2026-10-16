#!/usr/bin/env python3
"""驗證 Go runtime 直接引用的 player-visible catalog key 都有繁中翻譯。"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


KEY_RE = re.compile(r"[\"']((?:ui|apple2)\.[A-Za-z0-9_.]+)[\"']")


def load_keys(path: Path) -> set[str]:
    document = json.loads(path.read_text(encoding="utf-8"))
    return {
        entry["key"]
        for entry in document.get("entries", [])
        if isinstance(entry, dict) and isinstance(entry.get("key"), str)
    }


def source_files(root: Path) -> list[Path]:
    files: list[Path] = []
    for directory in (root / "cmd", root / "internal"):
        if directory.exists():
            files.extend(
                path
                for path in directory.rglob("*.go")
                if path.is_file() and not path.name.endswith("_test.go")
            )
    return sorted(files)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path)
    parser.add_argument("ui_catalog", type=Path)
    parser.add_argument("source_catalog", type=Path)
    args = parser.parse_args()

    ui_keys = load_keys(args.ui_catalog)
    source_keys = load_keys(args.source_catalog)
    all_keys = ui_keys | source_keys
    references: dict[str, list[str]] = {}
    for path in source_files(args.root):
        text = path.read_text(encoding="utf-8")
        for key in KEY_RE.findall(text):
            if key.endswith("."):
                # Dynamic families such as ui.destination.<id> are checked by
                # their concrete catalog entries and localization tests.
                continue
            references.setdefault(key, []).append(str(path.relative_to(args.root)))

    errors: list[str] = []
    for key in sorted(references):
        if key not in all_keys:
            errors.append(f"missing catalog key {key}: {', '.join(references[key])}")
    result = {
        "status": "pass" if not errors else "fail",
        "referencedKeyCount": len(references),
        "uiCatalogKeyCount": len(ui_keys),
        "apple2SourceKeyCount": len(source_keys),
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
