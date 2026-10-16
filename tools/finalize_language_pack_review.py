#!/usr/bin/env python3
"""把通過一次正式覆核的四語檔標成 production language pack。"""

from __future__ import annotations

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
I18N = ROOT / "docs/i18n"
STATUS = "reviewed once; official AI language pack; 2026-08-31"
FILES = [
    I18N / "sundog-ui-zh-Hant.json",
    I18N / "sundog-ui-zh-Hans.json",
    I18N / "sundog-ui-ja.json",
    I18N / "sundog-ui-en.json",
    I18N / "apple2-text-catalog.json",
    I18N / "apple2-text-zh-Hans.json",
    I18N / "apple2-text-ja.json",
    I18N / "apple2-text-en.json",
    I18N / "atari-st-dialog-zh-Hant.json",
    I18N / "atari-st-dialog-zh-Hans.json",
    I18N / "atari-st-dialog-ja.json",
    I18N / "atari-st-dialog-en.json",
]


def main() -> None:
    for path in FILES:
        document = json.loads(path.read_text(encoding="utf-8"))
        document["translationStatus"] = STATUS
        if isinstance(document.get("source"), dict) and "translationStatus" in document["source"]:
            document["source"]["translationStatus"] = STATUS
        for entry in document.get("entries", []):
            entry["translationStatus"] = STATUS
        path.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
