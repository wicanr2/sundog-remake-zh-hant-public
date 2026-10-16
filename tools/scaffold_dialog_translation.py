#!/usr/bin/env python3
"""由對話 evidence 產生／更新繁中翻譯檔的骨架。

翻譯檔 `docs/i18n/atari-st-dialog-zh-Hant.json` 是**唯一可編輯處**：每一筆同時保存
來源字串與譯文，來源由本工具從 evidence 重新產生。既有譯文會依「來源字串」比對後保留，
所以 evidence 重跑造成的順序變動不會弄丟翻譯；來源字串本身變了才會退回未翻。

引用在來源與譯文裡都寫成 `{hh}`（例如 `{2b}` 代表 stock phrase 0x2b）。譯文必須保留
來源出現過的每一個引用，這條由 `tools/validate_dialog_translation.py` 把關。
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

PENDING = ""


def render(tokens: list[dict]) -> str:
    parts: list[str] = []
    for token in tokens:
        if token["kind"] == "text":
            parts.append(str(token["text"]))
        elif token["kind"] == "reference":
            parts.append("{%s}" % str(token["index"])[2:])
    return "".join(parts)


def existing_by_source(document: dict) -> dict[str, str]:
    """把既有譯文攤成「來源字串 → 譯文」，供重新產生時沿用。"""
    table: dict[str, str] = {}

    def take(entry: dict) -> None:
        source = entry.get("source")
        translation = entry.get("translation")
        if isinstance(source, str) and isinstance(translation, str) and translation:
            table.setdefault(source, translation)

    for record in document.get("stockPhrases", {}).values():
        for entry in record.get("alternatives", []):
            take(entry)
    for block in document.get("blocks", {}).values():
        for record in block.get("phrases", {}).values():
            for entry in record.get("alternatives", []):
                take(entry)
        for entry in block.get("lines", []):
            take(entry)
    return table


def build(evidence: dict, previous: dict) -> dict:
    known = existing_by_source(previous)

    def entry(source: str) -> dict:
        return {"source": source, "translation": known.get(source, PENDING)}

    stock: dict[str, dict] = {}
    for record in evidence["stockPhrases"]["records"]:
        alternatives = [alt for alt in record["alternatives"] if alt]
        if not alternatives:
            continue
        stock[str(record["index"])] = {
            "rawOffset": record["rawOffset"],
            "alternatives": [entry(render(alt)) for alt in alternatives],
        }

    blocks: dict[str, dict] = {}
    for block in evidence["dialogBlocks"]:
        phrases: dict[str, dict] = {}
        lines: list[dict] = []
        for record in block["strings"]:
            if record["role"] == "block-phrase":
                alternatives = [alt for alt in record["alternatives"] if alt]
                phrases[str(record["referenceIndex"])] = {
                    "rawOffset": record["rawOffset"],
                    "alternatives": [entry(render(alt)) for alt in alternatives],
                }
            else:
                line = entry(render(record["tokens"]))
                line["rawOffset"] = record["rawOffset"]
                lines.append(line)
        blocks[str(block["blockNumber"])] = {"phrases": phrases, "lines": lines}

    return {
        "schema": 1,
        "locale": "zh-Hant",
        "translationStatus": "draft; requires user review",
        "policy": [
            "對話是組合出來的，不是完成句：譯文保留來源的引用 `{hh}` 與變體結構，執行時才展開。",
            "同一個 stock phrase 的多個變體要譯成語氣或用詞不同、但語意等價的說法，那是原版語句變化的來源。",
            "引用在句中的位置可依中文語序移動，但不得增減；缺一個引用就是漏掉一段會被代入的文字。",
            "空白與標點依中文習慣調整；來源的英文前後空格不必逐字保留。",
            "來源字串由 tools/scaffold_dialog_translation.py 從 evidence 重新產生，不要手改。",
        ],
        "stockPhrases": stock,
        "blocks": blocks,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, default=Path("docs/atari-st/dialog-text.json"))
    parser.add_argument("--output", type=Path, default=Path("docs/i18n/atari-st-dialog-zh-Hant.json"))
    args = parser.parse_args()

    evidence = json.loads(args.evidence.read_text(encoding="utf-8"))
    previous: dict = {}
    if args.output.exists():
        previous = json.loads(args.output.read_text(encoding="utf-8"))

    document = build(evidence, previous)
    args.output.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    total = 0
    translated = 0
    for record in document["stockPhrases"].values():
        for item in record["alternatives"]:
            total += 1
            translated += bool(item["translation"])
    for block in document["blocks"].values():
        for record in block["phrases"].values():
            for item in record["alternatives"]:
                total += 1
                translated += bool(item["translation"])
        for item in block["lines"]:
            total += 1
            translated += bool(item["translation"])
    print(json.dumps({"entries": total, "translated": translated, "pending": total - translated}, indent=2))


if __name__ == "__main__":
    main()
