#!/usr/bin/env python3
"""把本專案的對話解碼結果與 `laanwj/sundog` 的 dump 輸出逐筆對拍。

用途是驗證**解碼器**，不是取得資料：本專案的 `docs/atari-st/dialog-text.json` 由
`tools/atari_st_dialog_text.py` 自行從原始 bytes 解出，這支工具只回答「另一個獨立實作
從同一張磁碟讀出來的字串是否一模一樣」。相符即可把解碼正確性標成 `proven-by-oracle`；
不相符時，要先查清楚是誰錯，不得直接改本專案的輸出去迎合 oracle。

oracle 的 dump 以文字檔傳入（`--oracle-dump`），可以是先前保存的輸出，因此重跑不需要
oracle 的原始碼樹在場。oracle 的原始碼、磁碟與 dump 都不進 repository。

兩邊的標記法不同，比對前正規化：oracle 印 `[hh]` 表示 stock phrase 引用，本專案印
`<0xhh>`；兩邊都用 `|` 表示變體分隔。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path

ORACLE_REFERENCE = re.compile(r"\[([0-9a-f]{2})\]")
LOCAL_REFERENCE = re.compile(r"<0x([0-9a-f]{2})>")
# oracle 的 stock phrase 行：'<index> <offset> <text>'
ORACLE_STOCK_LINE = re.compile(r"^([0-9a-f]{2}) ([0-9a-f]{4,5}) (.*)$")
# oracle 的區塊字串行：'<offset> [=<ref>] <text>' 或 '<offset> <text>'
ORACLE_STRING_LINE = re.compile(r"^([0-9a-f]{4,5}) (?:\[=[0-9a-f]{2}\] )?(.*)$")
DIALOG_BLOCK_START = 0x07000


def normalize(text: str, pattern: re.Pattern[str]) -> str:
    return pattern.sub(lambda match: f"@{match.group(1)}", text).rstrip()


def parse_oracle_dump(dump: str) -> tuple[dict[int, str], dict[int, str]]:
    stock: dict[int, str] = {}
    strings: dict[int, str] = {}
    for line in dump.splitlines():
        match = ORACLE_STOCK_LINE.match(line)
        if match:
            stock[int(match.group(1), 16)] = normalize(match.group(3), ORACLE_REFERENCE)
            continue
        match = ORACLE_STRING_LINE.match(line)
        if match and int(match.group(1), 16) >= DIALOG_BLOCK_START:
            strings[int(match.group(1), 16)] = normalize(match.group(2), ORACLE_REFERENCE)
    return stock, strings


def compare(document: dict, stock: dict[int, str], strings: dict[int, str]) -> tuple[int, int, list[dict]]:
    mismatches: list[dict] = []
    checked_stock = 0
    for record in document["stockPhrases"]["records"]:
        index = int(str(record["index"]), 16)
        mine = normalize(str(record["preview"]), LOCAL_REFERENCE)
        theirs = stock.get(index)
        if theirs is None:
            mismatches.append({"kind": "stock-absent-from-oracle", "index": record["index"], "mine": mine})
            continue
        checked_stock += 1
        if mine != theirs:
            mismatches.append({"kind": "stock", "index": record["index"], "mine": mine, "oracle": theirs})

    checked_strings = 0
    for block in document["dialogBlocks"]:
        for record in block["strings"]:
            offset = int(str(record["rawOffset"]), 16)
            mine = normalize(str(record["preview"]), LOCAL_REFERENCE)
            theirs = strings.get(offset)
            if theirs is None:
                mismatches.append(
                    {"kind": "string-absent-from-oracle", "rawOffset": record["rawOffset"], "mine": mine}
                )
                continue
            checked_strings += 1
            if mine != theirs:
                mismatches.append(
                    {
                        "kind": "string",
                        "rawOffset": record["rawOffset"],
                        "mine": mine,
                        "oracle": theirs,
                    }
                )
    return checked_stock, checked_strings, mismatches


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("oracle_dump", type=Path, help="oracle dump 的文字輸出")
    parser.add_argument("--evidence", type=Path, default=Path("docs/atari-st/dialog-text.json"))
    parser.add_argument("--oracle-commit", default="e56d8e2532134f0741a7f82b80f9357776779f27")
    parser.add_argument("--output", type=Path, help="輸出對拍證據 JSON")
    args = parser.parse_args()

    dump_bytes = args.oracle_dump.read_bytes()
    stock, strings = parse_oracle_dump(dump_bytes.decode("utf-8"))
    document = json.loads(args.evidence.read_text(encoding="utf-8"))
    checked_stock, checked_strings, mismatches = compare(document, stock, strings)

    result = {
        "schema": 1,
        "tool": "tools/crosscheck_dialog_against_oracle.py",
        "status": "pass" if not mismatches else "fail",
        "evidenceLevel": "proven-by-oracle",
        "oracle": {
            "project": "laanwj/sundog",
            "commit": args.oracle_commit,
            "dumpSha256": hashlib.sha256(dump_bytes).hexdigest(),
            "dumpLineCount": len(dump_bytes.decode("utf-8").splitlines()),
        },
        "localEvidence": {
            "path": str(args.evidence),
            "sha256": hashlib.sha256(args.evidence.read_bytes()).hexdigest(),
        },
        "checkedStockPhrases": checked_stock,
        "checkedBlockStrings": checked_strings,
        "mismatchCount": len(mismatches),
        "mismatches": mismatches[:40],
        "note": (
            "相符只證明兩個獨立實作從同一張磁碟讀出相同字串，也就是解碼正確；"
            "它不證明原版讀取路徑、變體選取規則或 Apple II parity。"
        ),
    }
    text = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    print(text, end="")
    return 0 if not mismatches else 1


if __name__ == "__main__":
    sys.exit(main())
