#!/usr/bin/env python3
"""驗證對話文字 evidence 檔的自洽性。

只讀已提交的 JSON，不碰原始磁碟。檢查項目：

- 三層計數（stock phrase／區塊／行）與各區塊實際 record 數相符。
- 每個字串的 raw offset 落在自己的區塊內，且 token 型別合法。
- 每個引用都指向存在的 stock phrase：``0x00``–``0x33`` 指全域表，
  ``0x34`` 起指該區塊自己的 phrase；引用超出該區塊宣告的數量即失敗。
- 未識別位元組數為零。目前整份語料沒有任何 unknown token，這條把它固定住：
  日後若編碼理解退步，會在這裡直接紅掉，而不是靜靜寫進玩家看得到的文字。
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

STOCK_PHRASE_COUNT = 64
BLOCK_LOCAL_STOCK_BASE = 0x34
BLOCK_SIZE = 512
TOKEN_KINDS = {"text", "reference", "alternative", "unknown"}


def iter_token_lists(record: dict):
    if "tokens" in record:
        yield record["tokens"]
    for alternative in record.get("alternatives", []):
        yield alternative


def validate(document: dict) -> list[str]:
    errors: list[str] = []
    if document.get("schema") != 1:
        errors.append(f"unexpected schema: {document.get('schema')!r}")

    stock = document.get("stockPhrases", {})
    stock_records = stock.get("records", [])
    if len(stock_records) != STOCK_PHRASE_COUNT:
        errors.append(f"stock phrase count is {len(stock_records)}, want {STOCK_PHRASE_COUNT}")

    unknown_tokens = 0

    def check_tokens(where: str, tokens, local_stock: int | None) -> None:
        nonlocal unknown_tokens
        for token in tokens:
            kind = token.get("kind")
            if kind not in TOKEN_KINDS:
                errors.append(f"{where} has token of unknown kind {kind!r}")
                continue
            if kind == "unknown":
                unknown_tokens += 1
                continue
            if kind != "reference":
                continue
            index = int(str(token.get("index")), 16)
            if index < BLOCK_LOCAL_STOCK_BASE:
                if index >= len(stock_records):
                    errors.append(f"{where} references global phrase 0x{index:02x} beyond the table")
                continue
            if local_stock is None:
                errors.append(f"{where} references block phrase 0x{index:02x} outside a block")
                continue
            slot = index - BLOCK_LOCAL_STOCK_BASE
            # 0x3f 是文件記載的結束標記，不是第 12 個區塊 phrase。
            if token.get("dynamic"):
                continue
            if slot >= local_stock:
                errors.append(
                    f"{where} references block phrase 0x{index:02x} but the block declares {local_stock}"
                )

    for record in stock_records:
        where = f"stock phrase {record.get('index')}"
        for tokens in iter_token_lists(record):
            check_tokens(where, tokens, None)

    blocks = document.get("dialogBlocks", [])
    total_block_phrases = 0
    total_lines = 0
    for block in blocks:
        base = int(str(block.get("rawOffset")), 16)
        declared = block.get("declaredBlockPhraseCount")
        phrases = 0
        lines = 0
        for record in block.get("strings", []):
            where = f"block {block.get('blockNumber')} string {record.get('index')}"
            offset = int(str(record.get("rawOffset")), 16)
            if not base <= offset < base + BLOCK_SIZE:
                errors.append(f"{where} at 0x{offset:05x} is outside its own block")
            if record.get("role") == "block-phrase":
                phrases += 1
                expected = f"0x{BLOCK_LOCAL_STOCK_BASE + int(record['index']):02x}"
                if record.get("referenceIndex") != expected:
                    errors.append(f"{where} declares reference {record.get('referenceIndex')}, want {expected}")
            else:
                lines += 1
            for tokens in iter_token_lists(record):
                check_tokens(where, tokens, declared)
        if phrases != declared:
            errors.append(
                f"block {block.get('blockNumber')} declares {declared} phrases and carries {phrases}"
            )
        if phrases != block.get("blockPhraseCount") or lines != block.get("lineCount"):
            errors.append(f"block {block.get('blockNumber')} counts do not match its records")
        if not block.get("trailingIsZero"):
            errors.append(f"block {block.get('blockNumber')} has non-zero trailing bytes")
        total_block_phrases += phrases
        total_lines += lines

    totals = document.get("totals", {})
    if totals.get("dialogBlocks") != len(blocks):
        errors.append("totals.dialogBlocks does not match the block list")
    if totals.get("blockPhrases") != total_block_phrases:
        errors.append(f"totals.blockPhrases is {totals.get('blockPhrases')}, counted {total_block_phrases}")
    if totals.get("lines") != total_lines:
        errors.append(f"totals.lines is {totals.get('lines')}, counted {total_lines}")

    if unknown_tokens:
        errors.append(f"corpus carries {unknown_tokens} unrecognised bytes, want 0")

    gap = document.get("gapAfterStockPhrases", {})
    if gap.get("isZero") and gap.get("status") != "padding":
        errors.append("zero-filled gap is not recorded as padding")

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence", type=Path, nargs="?", default=Path("docs/atari-st/dialog-text.json"))
    args = parser.parse_args()

    document = json.loads(args.evidence.read_text(encoding="utf-8"))
    errors = validate(document)
    result = {
        "status": "pass" if not errors else "fail",
        "evidence": str(args.evidence),
        "stockPhrases": len(document.get("stockPhrases", {}).get("records", [])),
        "dialogBlocks": len(document.get("dialogBlocks", [])),
        "lines": document.get("totals", {}).get("lines"),
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
