#!/usr/bin/env python3
"""檢查手冊用語與遊戲內 UI 標籤的對照表沒有漂移。

手冊是原版文件的翻譯，用語照原文；遊戲畫面上的標籤是重製版 UI catalog 的譯法。
兩者不一定逐字相同，因此手冊裡有一張對照表。這個工具**直接讀那張表**當唯一真相，
逐列檢查三件事：

1. catalog 裡確實有那個 key；
2. catalog 的譯法就是表上的「遊戲內標籤」——UI 改字而沒同步改表就會被擋下；
3. 手冊正文裡找得到表上的「手冊用語」——手冊改寫而沒同步改表也會被擋下。

表本身在手冊裡，所以維護只有一個地方；工具不另外持有一份會腐化的清單。
"""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path


TABLE_HEADER = "| 手冊用語 | 遊戲內標籤 | catalog key |"
ROW = re.compile(r"^\|\s*(?P<manual>[^|]+?)\s*\|\s*(?P<label>[^|]+?)\s*\|\s*`(?P<key>[^`]+)`\s*\|$")


def strip_code(value: str) -> str:
    return value.strip().strip("`").strip()


def parse_table(manual: str) -> list[dict[str, str]]:
    lines = manual.splitlines()
    try:
        start = lines.index(TABLE_HEADER)
    except ValueError as error:
        raise ValueError("手冊裡找不到用語對照表的表頭") from error
    rows = []
    # 表頭下一行是分隔列，再往下讀到第一個非表格列為止。
    for line in lines[start + 2:]:
        if not line.startswith("|"):
            break
        match = ROW.match(line)
        if not match:
            raise ValueError(f"對照表有一列不符合格式：{line}")
        rows.append({
            "manualTerm": strip_code(match.group("manual")),
            "uiLabel": strip_code(match.group("label")),
            "key": match.group("key"),
        })
    if not rows:
        raise ValueError("對照表是空的")
    return rows


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manual", type=Path, help="繁中手冊 Markdown")
    parser.add_argument("catalog", type=Path, help="繁中 UI catalog JSON")
    args = parser.parse_args()

    manual = args.manual.read_text(encoding="utf-8")
    catalog = json.loads(args.catalog.read_text(encoding="utf-8"))
    translations = {entry["key"]: entry["translation"] for entry in catalog["entries"]}

    rows = parse_table(manual)
    errors: list[str] = []
    for row in rows:
        key, label, term = row["key"], row["uiLabel"], row["manualTerm"]
        if key not in translations:
            errors.append(f"catalog 沒有 key {key}")
            continue
        if translations[key] != label:
            errors.append(f"{key} 的譯法是「{translations[key]}」，對照表寫的是「{label}」")
        if term not in manual:
            errors.append(f"手冊正文找不到用語「{term}」（{key}）")

    result = {
        "status": "pass" if not errors else "fail",
        "errors": errors,
        "rowCount": len(rows),
        "manual": str(args.manual),
        "catalog": str(args.catalog),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if errors:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
