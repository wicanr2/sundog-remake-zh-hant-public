#!/usr/bin/env python3
"""驗證 remake-owned 繁中 UI overlay 的結構、唯一 key 與中文內容。"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def has_cjk(value: str) -> bool:
    return any("\u3400" <= char <= "\u9fff" for char in value)


# 少數幾筆的正確譯法就是不翻。列成明表而不是放寬規則，讓新的漏譯照樣會被擋下來。
DELIBERATELY_NOT_CHINESE = {
    # 角色狀態視窗把運氣的數值印成 `??`——原版就是這樣藏著它（PLAN §0.55）。
    # 換成中文等於把原版沒說的事說出來。
    "ui.attribute.hidden",
    # 語系選單採各語言自稱，英文不能翻成中文，否則切錯語系時更難找回。
    "ui.locale.en",
    # Atari ST 銀行終端的來源抬頭，保留原版產品字樣與箭頭。
    "ui.teller.header",
    # 緊縮按鍵帶只有鍵名，中文說明位於同頁操作列；貨幣縮寫沿原版Cr。
    "ui.hand_key",
    "ui.stores_short_keys",
    "ui.tube.credits",
}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("catalog", type=Path)
    args = parser.parse_args()

    document = json.loads(args.catalog.read_text(encoding="utf-8"))
    errors: list[str] = []
    entries = document.get("entries")
    if document.get("locale") != "zh-Hant":
        errors.append(f"locale is not zh-Hant: {document.get('locale')!r}")
    if not isinstance(entries, list) or not entries:
        errors.append("entries is empty or not a list")
        entries = []

    keys: set[str] = set()
    for index, entry in enumerate(entries):
        key = entry.get("key") if isinstance(entry, dict) else None
        translation = entry.get("translation") if isinstance(entry, dict) else None
        if not key:
            errors.append(f"entry {index} has an empty key")
        elif key in keys:
            errors.append(f"duplicate key: {key}")
        else:
            keys.add(key)
        if not isinstance(translation, str) or not translation.strip():
            errors.append(f"entry {index} has an empty translation")
        elif not has_cjk(translation) and key not in DELIBERATELY_NOT_CHINESE:
            errors.append(f"entry {index} has no CJK character: {key}")

    result = {
        "status": "pass" if not errors else "fail",
        "entryCount": len(entries),
        "errors": errors,
        "locale": document.get("locale"),
        "translationStatus": document.get("translationStatus"),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
