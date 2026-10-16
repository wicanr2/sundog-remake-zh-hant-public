#!/usr/bin/env python3
"""驗證對話繁中翻譯檔與 evidence 的一致性。

檢查項目：

- **來源漂移**：翻譯檔記錄的來源字串必須與 evidence 重新算出的完全相同。
- **引用完整**：譯文用到的引用集合必須與來源一致。位置可依中文語序移動，
  但少一個代表有一段會被代入的文字被漏掉，多一個代表憑空多出一段。
- **中文內容**：已翻的條目要含中文字，純引用組成的條目除外。
- `--require-complete` 時，未翻條目視為失敗；預設只回報數量，方便分批翻譯。
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

REFERENCE = re.compile(r"\{([0-9a-f]{2})\}")
LATIN_LETTER = re.compile(r"[A-Za-z]")
# 0x3f 是結束標記，本身不產生文字。譯文可留可不留，不列入引用比對。
MARKER_REFERENCES = {"3f"}


def has_cjk(value: str) -> bool:
    return any("㐀" <= char <= "鿿" for char in value)


def render(tokens: list[dict]) -> str:
    parts: list[str] = []
    for token in tokens:
        if token["kind"] == "text":
            parts.append(str(token["text"]))
        elif token["kind"] == "reference":
            parts.append("{%s}" % str(token["index"])[2:])
    return "".join(parts)


def expected_sources(evidence: dict) -> tuple[dict[str, list[str]], dict[str, dict]]:
    stock: dict[str, list[str]] = {}
    for record in evidence["stockPhrases"]["records"]:
        alternatives = [render(alt) for alt in record["alternatives"] if alt]
        if alternatives:
            stock[str(record["index"])] = alternatives

    blocks: dict[str, dict] = {}
    for block in evidence["dialogBlocks"]:
        phrases: dict[str, list[str]] = {}
        lines: list[str] = []
        for record in block["strings"]:
            if record["role"] == "block-phrase":
                phrases[str(record["referenceIndex"])] = [
                    render(alt) for alt in record["alternatives"] if alt
                ]
            else:
                lines.append(render(record["tokens"]))
        blocks[str(block["blockNumber"])] = {"phrases": phrases, "lines": lines}
    return stock, blocks


def check_entry(where: str, entry: dict, source: str, errors: list[str], pending: list[str]) -> None:
    if entry.get("source") != source:
        errors.append(f"{where} source drift: {entry.get('source')!r} versus {source!r}")
        return
    translation = entry.get("translation", "")
    if not translation:
        pending.append(where)
        return
    want = sorted(set(REFERENCE.findall(source)) - MARKER_REFERENCES)
    got = sorted(set(REFERENCE.findall(translation)) - MARKER_REFERENCES)
    if want != got:
        errors.append(f"{where} references {got} but the source has {want}")
        return
    # 只有「譯文裡還留著英文字母」才算沒翻；純標點或純引用的條目是合法的。
    remainder = REFERENCE.sub("", translation)
    if LATIN_LETTER.search(remainder) and not has_cjk(translation):
        errors.append(f"{where} translation is still in English: {translation!r}")


def validate(evidence: dict, document: dict, require_complete: bool) -> tuple[list[str], list[str], int]:
    errors: list[str] = []
    pending: list[str] = []
    total = 0

    stock, blocks = expected_sources(evidence)

    if sorted(document.get("stockPhrases", {})) != sorted(stock):
        errors.append("stock phrase index set differs from the evidence")
    for index, sources in stock.items():
        record = document.get("stockPhrases", {}).get(index)
        if record is None:
            errors.append(f"stock phrase {index} is missing from the translation")
            continue
        entries = record.get("alternatives", [])
        if len(entries) != len(sources):
            errors.append(f"stock phrase {index} has {len(entries)} alternatives, want {len(sources)}")
            continue
        for slot, source in enumerate(sources):
            total += 1
            check_entry(f"stock {index}[{slot}]", entries[slot], source, errors, pending)

    if sorted(document.get("blocks", {})) != sorted(blocks):
        errors.append("dialog block set differs from the evidence")
    for number, expected in blocks.items():
        block = document.get("blocks", {}).get(number)
        if block is None:
            errors.append(f"dialog block {number} is missing from the translation")
            continue
        for reference, sources in expected["phrases"].items():
            record = block.get("phrases", {}).get(reference)
            if record is None:
                errors.append(f"block {number} phrase {reference} is missing")
                continue
            entries = record.get("alternatives", [])
            if len(entries) != len(sources):
                errors.append(f"block {number} phrase {reference} has {len(entries)} alternatives, want {len(sources)}")
                continue
            for slot, source in enumerate(sources):
                total += 1
                check_entry(f"block {number} phrase {reference}[{slot}]", entries[slot], source, errors, pending)
        entries = block.get("lines", [])
        if len(entries) != len(expected["lines"]):
            errors.append(f"block {number} has {len(entries)} lines, want {len(expected['lines'])}")
            continue
        for slot, source in enumerate(expected["lines"]):
            total += 1
            check_entry(f"block {number} line {slot}", entries[slot], source, errors, pending)

    if require_complete and pending:
        errors.append(f"{len(pending)} entries are still untranslated")
    return errors, pending, total


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, default=Path("docs/atari-st/dialog-text.json"))
    parser.add_argument("--translation", type=Path, default=Path("docs/i18n/atari-st-dialog-zh-Hant.json"))
    parser.add_argument("--require-complete", action="store_true")
    args = parser.parse_args()

    evidence = json.loads(args.evidence.read_text(encoding="utf-8"))
    document = json.loads(args.translation.read_text(encoding="utf-8"))
    errors, pending, total = validate(evidence, document, args.require_complete)

    result = {
        "status": "pass" if not errors else "fail",
        "translation": str(args.translation),
        "entryCount": total,
        "translatedCount": total - len(pending),
        "pendingCount": len(pending),
        "errors": errors[:40],
        "pendingSample": pending[:10],
    }
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
