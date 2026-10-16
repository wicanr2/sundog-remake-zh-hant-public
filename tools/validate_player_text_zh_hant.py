#!/usr/bin/env python3
"""檢查玩家可見 UI overlay 是否仍殘留不必要的英文實作術語。"""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path


FORBIDDEN_TERMS = {"remake", "prototype", "tileset", "slot", "writable"}

# 少數幾筆的正確譯法就是不翻，與 validate_ui_catalog.py 的同名清單保持一致。
# 角色狀態視窗把運氣印成 `??`——原版就是這樣藏著它，換成中文等於把原版沒說的事說出來。
DELIBERATELY_NOT_CHINESE = {
    "ui.attribute.hidden",
    "ui.locale.en",
    "ui.teller.header",
}

# Remake Modern 是玩家可見的正式圖塊集名稱，不是洩漏的實作術語。
DELIBERATELY_ALLOWED_TERMS = {
    "ui.help.tileset": {"remake"},
    "ui.tileset.remake_modern": {"remake"},
}

HAN_RE = re.compile(r"[\u3400-\u9fff]")
WORD_RE = re.compile(r"[A-Za-z][A-Za-z0-9./_-]*")


def main() -> int:
    parser = argparse.ArgumentParser(description="驗證玩家可見繁中 UI 不殘留不必要的英文實作術語。")
    parser.add_argument("catalog", type=Path)
    args = parser.parse_args()

    document = json.loads(args.catalog.read_text(encoding="utf-8"))
    errors: list[str] = []
    entries = document.get("entries", [])
    for index, entry in enumerate(entries):
        key = entry.get("key", f"entry-{index}")
        translation = entry.get("translation", "")
        if not HAN_RE.search(translation) and key not in DELIBERATELY_NOT_CHINESE:
            errors.append(f"{key}: translation has no Han character")
        allowed = DELIBERATELY_ALLOWED_TERMS.get(key, set())
        forbidden = sorted(
            word.lower()
            for word in WORD_RE.findall(translation)
            if word.lower() in FORBIDDEN_TERMS and word.lower() not in allowed
        )
        if forbidden:
            errors.append(f"{key}: unnecessary English terms: {', '.join(forbidden)}")

    result = {
        "catalog": str(args.catalog),
        "entryCount": len(entries),
        "forbiddenTerms": sorted(FORBIDDEN_TERMS),
        "errors": errors,
        "status": "pass" if not errors else "fail",
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    raise SystemExit(main())
