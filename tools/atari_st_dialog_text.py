#!/usr/bin/env python3
"""解碼 Atari ST SunDog 的對話文字系統。

輸入是唯讀的外層 Atari ST archive；工具只讀 nested raw ``.st``，輸出是保留原始
offset、雜湊與**結構**的 JSON。輸出刻意不把對話攤平成完成句：原版的對話是組合出來
的，攤平會讓中文化失去它真正要翻的東西。

區段位置取自公開 secondary oracle ``laanwj/sundog`` 的 ``doc/disklayout.md`` 與
``doc/sundog_dialog_system.md``；**編碼、record 邊界與 token 模型是本工具自行從
bytes 解出並驗證的**，兩者證據等級分開記。

## 編碼

每個字串是「1 byte 長度 + 內容」。內容的每個 byte：

- bit 7 設起來表示這個字元後面接一個空格（原版的空格壓縮）。
- 低 7 位元：
  - ``0x20 0x21 0x27 0x2c 0x2e 0x7e`` 直接輸出對應標點。
  - ``0x5c`` 與 ``0x7c`` 都輸出 ``I``。
  - bit 5 設起來時：``0x60`` 是變體分隔符，``0x61``–``0x7a`` 是小寫字母。
  - bit 5 沒設、落在 ``0x41``–``0x5a`` 時，兩種字串類型的意義不同：
    - **line**（一般對話行）：大寫字母字面值。
    - **phrase**（stock phrase）：小寫字母**且該變體到此結束**。這是原版把
      「變體最後一個字元」用清掉 bit 5 來標記的壓縮法。
  - 其餘 ``0x00``–``0x3f`` 是 stock phrase 引用。

## 動態引用

``0x20``–``0x2f`` 之中有數個在 stock phrase 表裡是空的，因為它們由程式產生
（金額、物品名等）。本工具把它們標成 ``dynamic`` 並附上次級文件記載的語意，
證據等級為 hypothesis；表裡有內容的一律以實際 bytes 為準。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


# laanwj/sundog doc/disklayout.md：0x06c00 起是壓縮對話文字，0x08c00 附近結束。
TEXT_START = 0x06C00
TEXT_END = 0x08C00
BLOCK_SIZE = 512

STOCK_PHRASE_COUNT = 64
STOCK_TABLE_ENTRIES = STOCK_PHRASE_COUNT + 1

# 對話區塊從文字區的第 0x400 byte 起，每 512 bytes 一個。
DIALOG_SECTION_START = 0x400
SECTOR_NUM_STOCK_OFFSET = 0x10
SECTOR_DATA_START_OFFSET = 0x11
BLOCK_LOCAL_STOCK_BASE = 0x34

PUNCTUATION = {0x20: " ", 0x21: "!", 0x27: "'", 0x2C: ",", 0x2E: ".", 0x7E: "~"}
ALTERNATIVE_SEPARATOR = 0x60
# 0x00–0x3f 是 stock phrase 引用區；標點是其中先被攔下的例外。
REFERENCE_LIMIT = 0x40
LETTER_I_BYTES = (0x5C, 0x7C)

# 次級文件記載、在 stock phrase 表中為空的程式產生引用。語意是 hypothesis。
DYNAMIC_REFERENCES = {
    0x21: "secondary doc: nop, expected '!'",
    0x22: "secondary doc: literal O.K.",
    0x23: "secondary doc: value produced by XDOINTER:0x0f",
    0x24: "secondary doc: formatted money amount",
    0x25: "secondary doc: item name",
    0x27: "secondary doc: literal apostrophe",
    0x2C: "secondary doc: literal comma",
    0x2E: "secondary doc: nop, expected '.'",
    0x3F: "secondary doc: end-of-text marker",
}


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        nested_name, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return nested_name, disk_name, disk_bytes


def decode_tokens(raw: bytes, kind: str) -> list[dict[str, object]]:
    """把壓縮 bytes 解成 token 串；未識別的 byte 保留為 ``unknown``，不猜。"""
    if kind not in ("line", "phrase"):
        raise ValueError(f"unknown string kind {kind!r}")
    tokens: list[dict[str, object]] = []
    pending = ""

    def flush() -> None:
        nonlocal pending
        if pending:
            tokens.append({"kind": "text", "text": pending})
            pending = ""

    for byte in raw:
        trailing_space = bool(byte & 0x80)
        value = byte & 0x7F
        if value in PUNCTUATION:
            pending += PUNCTUATION[value]
        elif value < REFERENCE_LIMIT:
            # 引用佔滿 0x00–0x3f，其中一部分的 bit 5 是 1，所以這一支必須排在
            # 字母判斷之前，否則 0x22–0x3f 會被誤判成字母區的未知位元組。
            flush()
            token: dict[str, object] = {"kind": "reference", "index": f"0x{value:02x}"}
            if value in DYNAMIC_REFERENCES:
                token["dynamic"] = True
                token["note"] = DYNAMIC_REFERENCES[value]
            tokens.append(token)
        elif value in LETTER_I_BYTES:
            pending += "I"
        elif value == ALTERNATIVE_SEPARATOR:
            flush()
            tokens.append({"kind": "alternative"})
        elif 0x61 <= value <= 0x7A:
            pending += chr(value)
        elif 0x41 <= value <= 0x5A:
            if kind == "line":
                pending += chr(value)
            else:
                pending += chr(value | 0x20)
                flush()
                tokens.append({"kind": "alternative"})
        else:
            flush()
            tokens.append({"kind": "unknown", "byte": f"0x{value:02x}"})
        if trailing_space:
            pending += " "
    flush()
    return tokens


def split_alternatives(tokens: list[dict[str, object]]) -> list[list[dict[str, object]]]:
    """把 token 串以變體分隔符切開。空變體會被保留，那是原版資料的一部分。"""
    alternatives: list[list[dict[str, object]]] = [[]]
    for token in tokens:
        if token["kind"] == "alternative":
            alternatives.append([])
            continue
        alternatives[-1].append(token)
    # 最後一個變體以分隔符結尾時會留下空尾巴，那是編碼副作用而非資料。
    if len(alternatives) > 1 and not alternatives[-1]:
        alternatives.pop()
    return alternatives


def preview(tokens: list[dict[str, object]]) -> str:
    """給人眼比對用的單行還原；不是 runtime 格式。"""
    parts: list[str] = []
    for token in tokens:
        if token["kind"] == "text":
            parts.append(str(token["text"]))
        elif token["kind"] == "reference":
            parts.append(f"<{token['index']}>")
        elif token["kind"] == "alternative":
            parts.append("|")
        else:
            parts.append(f"<?{token['byte']}>")
    return "".join(parts)


def decode_stock_table(data: bytes) -> tuple[list[dict[str, object]], int]:
    """解首段的 stock phrase 表。offset 是單 byte，遞減即代表跨過 0x100。"""
    offsets: list[int] = []
    base = 0
    last = 0
    for index in range(STOCK_TABLE_ENTRIES):
        value = data[index]
        if value < last:
            base += 0x100
        offsets.append(base + value)
        last = value
    for index in range(STOCK_PHRASE_COUNT):
        if offsets[index] > offsets[index + 1]:
            raise ValueError(f"stock phrase offset table is not monotonic at {index}")
    if offsets[0] < STOCK_TABLE_ENTRIES:
        raise ValueError("stock phrase payload overlaps its own offset table")

    phrases: list[dict[str, object]] = []
    for index in range(STOCK_PHRASE_COUNT):
        start, end = offsets[index], offsets[index + 1]
        raw = data[start:end]
        tokens = decode_tokens(raw, "phrase")
        alternatives = split_alternatives(tokens)
        record: dict[str, object] = {
            "index": f"0x{index:02x}",
            "rawOffset": f"0x{TEXT_START + start:05x}",
            "byteLength": len(raw),
            "alternatives": alternatives,
            "preview": preview(tokens),
        }
        if not raw:
            record["empty"] = True
            if index in DYNAMIC_REFERENCES:
                record["dynamic"] = True
                record["note"] = DYNAMIC_REFERENCES[index]
        phrases.append(record)
    return phrases, offsets[STOCK_PHRASE_COUNT]


def decode_dialog_block(data: bytes, sector_base: int) -> dict[str, object]:
    """解一個 512-byte 對話區塊；字串長度前綴、偶數對齊、零長度視為結束。"""
    num_stock = data[sector_base + SECTOR_NUM_STOCK_OFFSET]
    data_start = data[sector_base + SECTOR_DATA_START_OFFSET] * 2
    # data_start 是 byte 值乘二，上限 510，永遠落在區塊內；真正會失效的是它指回
    # 自己的標頭，那會把 offset 表當成字串解。
    if data_start <= SECTOR_DATA_START_OFFSET:
        raise ValueError(f"sector 0x{sector_base:04x} points its data start into its own header")

    strings: list[dict[str, object]] = []
    pointer = sector_base + data_start
    limit = sector_base + BLOCK_SIZE
    index = 0
    terminator: str | None = None
    while pointer < limit:
        length = data[pointer]
        if length == 0:
            terminator = f"0x{TEXT_START + pointer:05x}"
            break
        end = pointer + 1 + length
        if end > limit:
            raise ValueError(f"string at 0x{TEXT_START + pointer:05x} runs past its block")
        raw = data[pointer + 1 : end]
        is_stock = index < num_stock
        tokens = decode_tokens(raw, "phrase" if is_stock else "line")
        record: dict[str, object] = {
            "index": index,
            "rawOffset": f"0x{TEXT_START + pointer:05x}",
            "byteLength": length,
            "role": "block-phrase" if is_stock else "line",
            "preview": preview(tokens),
        }
        if is_stock:
            record["referenceIndex"] = f"0x{BLOCK_LOCAL_STOCK_BASE + index:02x}"
            record["alternatives"] = split_alternatives(tokens)
        else:
            record["tokens"] = tokens
        strings.append(record)
        pointer = end + (end & 1)
        index += 1

    trailing = data[pointer:limit] if terminator else b""
    return {
        "rawOffset": f"0x{TEXT_START + sector_base:05x}",
        "blockNumber": f"0x{(sector_base - DIALOG_SECTION_START) // BLOCK_SIZE * 0x10 + 0x10:02x}",
        "declaredBlockPhraseCount": num_stock,
        "dataStartBytes": data_start,
        "blockPhraseCount": sum(1 for record in strings if record["role"] == "block-phrase"),
        "lineCount": sum(1 for record in strings if record["role"] == "line"),
        "headerHex": data[sector_base : sector_base + SECTOR_DATA_START_OFFSET + 1].hex(),
        "terminatorOffset": terminator,
        "trailingBytes": len(trailing),
        "trailingIsZero": not any(trailing),
        "sha256": sha256(data[sector_base:limit]),
        "strings": strings,
    }


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    nested_name, disk_name, disk = load_disk(archive_path, variant)
    data = disk[TEXT_START:TEXT_END]
    if len(data) != TEXT_END - TEXT_START:
        raise ValueError("dialog text region is truncated")

    phrases, payload_end = decode_stock_table(data)
    blocks = [
        decode_dialog_block(data, sector_base)
        for sector_base in range(DIALOG_SECTION_START, len(data), BLOCK_SIZE)
    ]

    gap = data[payload_end:DIALOG_SECTION_START]
    return {
        "schema": 1,
        "tool": "tools/atari_st_dialog_text.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "nestedArchive": nested_name,
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "region": {
            "rawStart": f"0x{TEXT_START:05x}",
            "rawEnd": f"0x{TEXT_END:05x}",
            "sha256": sha256(data),
            "locationSource": "laanwj/sundog doc/disklayout.md + doc/sundog_dialog_system.md (secondary oracle, commit e56d8e2532134f0741a7f82b80f9357776779f27)",
            "locationEvidence": "proven（次級文件所載）",
            "encodingEvidence": "proven",
        },
        "stockPhrases": {
            "rawOffset": f"0x{TEXT_START:05x}",
            "tableEntries": STOCK_TABLE_ENTRIES,
            "phraseCount": STOCK_PHRASE_COUNT,
            "payloadEndOffset": f"0x{TEXT_START + payload_end:05x}",
            "records": phrases,
        },
        "gapAfterStockPhrases": {
            "rawStart": f"0x{TEXT_START + payload_end:05x}",
            "rawEnd": f"0x{TEXT_START + DIALOG_SECTION_START:05x}",
            "byteLength": len(gap),
            "isZero": not any(gap),
            "sha256": sha256(gap),
            "status": "padding" if not any(gap) else "unknown",
            "note": (
                "stock phrase payload 與第一個對話區塊之間的區段。實測全為零，"
                "屬對齊填補；非零時要改標 unknown 並重新追查。"
            ),
        },
        "dialogBlocks": blocks,
        "totals": {
            "stockPhrases": STOCK_PHRASE_COUNT,
            "dialogBlocks": len(blocks),
            "blockPhrases": sum(int(block["blockPhraseCount"]) for block in blocks),
            "lines": sum(int(block["lineCount"]) for block in blocks),
        },
        "openQuestions": [
            {
                "topic": "dialog block header",
                "status": "unknown",
                "detail": "每個區塊前 0x10 bytes 是十六個 frame 的起點（見 atari_st_dialog_frames.py 與 dialog-frames.json 的 offsetTable）；這一支不解 frame，只保留 headerHex。",
            },
            {
                "topic": "block entry points",
                "status": "proven",
                "detail": (
                    "互動類別對應到起始 frame 的表已自行解出，不再倚賴次級文件："
                    "12 酒館櫃檯、13 隨身裝備店、14 船艦零件店、15 搜身、16 搶匪、17／22 街頭交易者、"
                    "18 乞丐、19 海盜、21 酒館搭話、23 旅館。見 docs/atari-st/dialog-rendered.md。"
                ),
            },
            {
                "topic": "alternative selection",
                "status": "unknown",
                "detail": "變體預設隨機選取、腳本可指定特定變體；選取規則與 RNG 未追。",
            },
            {
                "topic": "original consumer",
                "status": "proven",
                "detail": (
                    "原版的對話直譯器已逐支讀出：XDOINTER 的 new_frame／pick_one／passed／"
                    "made_roll／made_attr 與前置碼、後置碼。見 docs/atari-st/dialog-vm.json（11 條簽章）。"
                ),
            },
        ],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path, help="外層 Atari ST archive（唯讀）")
    parser.add_argument("--variant", default="[a2]", help="nested archive 變體標記")
    parser.add_argument("--output", type=Path, help="輸出 JSON 路徑；未指定則印到 stdout")
    args = parser.parse_args()

    document = build_document(args.archive, args.variant)
    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
