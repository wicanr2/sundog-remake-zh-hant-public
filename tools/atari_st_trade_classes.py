#!/usr/bin/env python3
"""解碼交易資料庫的商品類別表（logical block 7）。

交易所的價格已經全部解出來了（`docs/atari-st/screen-geometry.md` 與
`internal/game/exchange.go`），只差**每個城市的供給與需求從哪來**。這支工具補上那一塊。

## 表在哪

`XDOTRADI:0x07 get_data` 開頭做三件事：

```text
moveram(G433, &L7c0, 0xc00)            ; 交易資料庫（每城的倉庫等等）搬到區域緩衝
moveblock(1, 0xff, 0, &La5, 1, 17, 1)  ; logical block 17
moveblock(1, 0xff, 0, &L3,  1,  7, 1)  ; logical block 7 ← 商品類別表
```

logical block L 的檔案位移是 `(27 + L + 9) × 512`（同 `atari_st_sine_table.py`），
所以 block 7 是 raw `0x5600`。

## 表的形狀

`get_data` 接著：

```text
t := G2a 的 (寬 5, 起始位元 4) 欄位      ; 行星編號，chk 到 1..18
索引 := (t − 1 + 遊戲分類位移) mod 18
copy 14 words ← 表[索引]                ; 進 L2b（7 words）與 L32（7 words）
home := 表[t − 1].word6
if home <> 0 then
    L2b[home − 1] := 7                  ; 供給拉滿
    L32[home − 1] := 0                  ; 需求歸零
want := 表[t − 1].word13
if want <> 0 then
    L2b[want − 1] := 0
    L32[want − 1] := 7
```

**18 筆正好對上 18 顆行星**。每筆 14 個 word：

| word | 內容 |
|---:|---|
| 0–5 | 供給類別，UCSD packed array（`ixp 5,3`：一個 word 五格、每格 3 bits），商品 1–30 |
| 6 | 這顆行星的本地產物編號（0 = 沒有）|
| 7–12 | 需求類別，同樣的打包法 |
| 13 | 行星需要的商品編號（0 = 沒有），供給0、需求7 |

分類位移是G10b byte3，不是城市圖塊header。XSTARTUP更新block7
byte0x1f8的計數為(counter+5)%18，再加RND%3-1後mod18。
原始寫入端與原生收據見docs/atari-st/banville-restoration-spec.md。

打包方向是**低位在前**（`ixp` 算出的 rightbit ＝ 寬度 × (索引 mod 每字格數)），
而 5 × 3 ＝ 15 bits 用不到最高位——供給與需求各108個類別word **每一個都 ≤ 0x7fff**，
這是版面的獨立驗證，工具因此 fail-closed。

## 類別怎麼變成供給與需求

`get_data:init_sd(table, goods)`（第一次到訪）：

```text
v := random() mod (類別 + 1)
if v < 類別 and random() 的最低位元 then v := v + random() mod 4
condition(0, &v, 類別)                  ; 夾回 [0, 類別]
table[goods − 1] := v
```

`get_data:adjust(table, 類別表, 每期步進)`（隔一段時間再來）：

```text
for goods := 1 to 30:
    v := table[goods − 1] + 步進
    if v > 類別 then v := 類別
    table[goods − 1] := v
```

也就是**供給與需求會隨時間回升到類別值**，而不是回到某個固定中位數。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


BLOCK_SIZE = 512
SECTORS_PER_TRACK = 9
FILE_TRACK_SHIFT = 3 * SECTORS_PER_TRACK
DISK_TRACK_OFFSET = SECTORS_PER_TRACK
# get_data 的 moveblock 呼叫點寫死了這個 block 編號。
TRADE_CLASS_BLOCK = 7
TRADE_CLASS_OFFSET = (FILE_TRACK_SHIFT + TRADE_CLASS_BLOCK + DISK_TRACK_OFFSET) * BLOCK_SIZE

# 一筆 14 words，18 筆（每顆行星一筆）。
RECORD_WORDS = 14
RECORD_COUNT = 18
# 打包：一個 word 五格、每格 3 bits，所以 6 words 覆蓋商品 1–30。
FIELDS_PER_WORD = 5
FIELD_BITS = 3
CLASS_WORDS = 6
GOODS_COUNT = CLASS_WORDS * FIELDS_PER_WORD
MAX_CLASS = (1 << FIELD_BITS) - 1
# 15 bits 用得到，最高位用不到——這是版面的獨立驗證。
CLASS_WORD_MAX = (1 << (FIELDS_PER_WORD * FIELD_BITS)) - 1

# 位元組簽章：語意是人讀反組譯讀出來的，這些位址把它釘回磁碟。
SEGMENT_BASES = {"XDOTRADI": 0x16200, "XMOVEONG": 0x12000, "XSTARTUP": 0xbc00}
PSYSTEM_STARTUP_RAW = 0x2E000

RULES: list[dict[str, object]] = [
    {
        "id": "trade.class-block-read",
        "procedure": "XDOTRADI:0x07 get_data",
        "segment": "XDOTRADI",
        "segmentOffset": "0x0288",
        "signature": "0180 ff00 6201 0701 7349",
        "summary": (
            "`moveblock(1, 0xff, 0, &L3, 1, 7, 1)`：商品類別表是 **logical block 7**，"
            "整塊讀進一個區域緩衝。同一支程序前一行讀的是 block 17。"
        ),
    },
    {
        "id": "trade.class-record-pick",
        "procedure": "XDOTRADI:0x07 get_data",
        "segment": "XDOTRADI",
        "segmentOffset": "0x0296",
        "signature": (
            "9b04 2a05 04c9 ee9b 0481 0b03 0080 27cb a7a2 12e2 00bd cb8f ed01 12cb ee d70e"
        ),
        "summary": (
            "挑哪一筆：`((行星 − 1 + G10b byte3分類位移) mod 18) + 1`，再 `ixa 0x0e` 取 14 words。"
            "行星是 `G2a` 的 (寬 5, 起始位元 4) 欄位，`chk` 到 1..18——**18 筆正好一顆行星一筆**。"
        ),
    },
    {
        "id": "trade.class-demand-goods",
        "procedure": "XDOTRADI:0x07 get_data",
        "segment": "XDOTRADI", "segmentOffset": "0x02ff",
        "signature": "8801 2b21 011e cbee d805 0300 0007 cbca 8801 3221 011e cbee d805 0307 0007 cbca",
        "summary": "行星自己記錄word13非零時，指定商品供給0、需求7；在word6覆寫之後。",
    },
    {
        "id": "trade.class-shift-init",
        "procedure": "XSTARTUP:0x2a", "segment": "XSTARTUP", "segmentOffset": "0x171d",
        "signature": "9b04 810b 0312 21a2 0073 0b03 8fa2 ee12 8fc8",
        "summary": "G10b byte3=(18+nextCounter+RND%3-1)%18；nextCounter由block7 byte0x1f8加5後mod18。不是城市圖塊header。",
    },
    {
        "id": "trade.class-home-goods",
        "procedure": "XDOTRADI:0x07 get_data",
        "segment": "XDOTRADI",
        "segmentOffset": "0x02cb",
        "signature": (
            "8801 2b21 011e cbee d805 0307 0007 cbca 8801 3221 011e cbee d805 0300 0007 cbca"
        ),
        "summary": (
            "本地產物：行星自己那一筆的 word 6 若不為 0，就把該商品的**供給類別設成 7、"
            "需求類別設成 0**。這也認出兩個半邊——被設成 7 的那半是供給。"
        ),
    },
    {
        "id": "trade.class-seed",
        "procedure": "XDOTRADI:0x09 get_data:init_sd",
        "segment": "XDOTRADI",
        "segmentOffset": "0x01ee",
        "signature": (
            "0001 0073 0b22 2101 1fcb eed8 0503 c9ed e200 bdcb 8f68"
        ),
        "summary": (
            "第一次到訪：`v := random() mod (類別 + 1)`，若 `v < 類別` 且 `random()` 的最低位元"
            "為真再加 `random() mod 4`，最後 `condition(0, &v, 類別)` 夾回 [0, 類別]。"
        ),
    },
    {
        "id": "trade.class-ageing",
        "procedure": "XDOTRADI:0x08 get_data:adjust",
        "segment": "XDOTRADI",
        "segmentOffset": "0x01b0",
        "signature": (
            "2821 011f cbee d805 03c9 ad01 a268 2062 2101 1ecb eed8 0503 c9b2 f10b"
        ),
        "summary": (
            "隔期再來：每項加上步進之後**夾在類別值**，所以供給與需求會隨時間回升到類別，"
            "不是回到某個固定中位數。"
        ),
    },
]


def read_disk(archive: Path, variant: str) -> bytes:
    with zipfile.ZipFile(archive) as outer:
        _, nested_bytes = choose_nested(outer, variant)
    with zipfile.ZipFile(io.BytesIO(nested_bytes)) as inner:
        _, disk_bytes = choose_disk(inner)
    return disk_bytes


def unpack_classes(words: list[int]) -> list[int]:
    """把 6 個 word 攤成 30 個 3-bit 類別，低位在前。"""
    out: list[int] = []
    for word in words:
        for field in range(FIELDS_PER_WORD):
            out.append((word >> (field * FIELD_BITS)) & MAX_CLASS)
    return out


def decode_records(block: bytes) -> list[dict[str, object]]:
    words = [int.from_bytes(block[i : i + 2], "big") for i in range(0, len(block), 2)]
    records: list[dict[str, object]] = []
    for index in range(RECORD_COUNT):
        record = words[index * RECORD_WORDS : (index + 1) * RECORD_WORDS]
        if len(record) != RECORD_WORDS:
            raise SystemExit(f"block 7 只放得下 {len(records)} 筆記錄，需要 {RECORD_COUNT} 筆")
        supply_words, demand_words = record[:CLASS_WORDS], record[7 : 7 + CLASS_WORDS]
        for word in supply_words + demand_words:
            if word > CLASS_WORD_MAX:
                raise SystemExit(
                    f"記錄 {index} 有一個類別 word 是 0x{word:04x}，超過 5 × 3 bits 的 0x{CLASS_WORD_MAX:04x}"
                    "——版面讀錯了"
                )
        supply_home, demand_home = record[6], record[13]
        for home in (supply_home, demand_home):
            if home > GOODS_COUNT:
                raise SystemExit(f"記錄 {index} 的本地產物編號 {home} 超出商品 1..{GOODS_COUNT}")
        records.append(
            {
                "planetIndex": index,
                "supply": unpack_classes(supply_words),
                "demand": unpack_classes(demand_words),
                "supplyHomeGoods": supply_home,
                "demandHomeGoods": demand_home,
                "rawWords": [f"0x{word:04x}" for word in record],
            }
        )
    return records


def pin_rules(image: bytes) -> list[dict[str, object]]:
    pinned: list[dict[str, object]] = []
    for rule in RULES:
        signature = bytes.fromhex(rule["signature"].replace(" ", ""))
        matches = []
        start = 0
        while True:
            found = image.find(signature, start)
            if found < 0:
                break
            matches.append(found)
            start = found + 1
        if len(matches) != 1:
            raise SystemExit(f"簽章 {rule['id']} 在磁碟上出現 {len(matches)} 次，必須恰好一次")
        # 段基底是相對 SYSTEM.STARTUP 的，落到磁碟上要加那個檔案自己的位移。
        want = PSYSTEM_STARTUP_RAW + SEGMENT_BASES[rule["segment"]] + int(rule["segmentOffset"], 16)
        if matches[0] != want:
            raise SystemExit(
                f"簽章 {rule['id']} 落在 0x{matches[0]:05x}，段基底加段內位址算出來是 0x{want:05x}"
            )
        entry = dict(rule)
        entry["rawOffset"] = f"0x{matches[0]:05x}"
        pinned.append(entry)
    return pinned


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("archive", type=Path, nargs="?", help="外層 Atari ST archive（唯讀）")
    parser.add_argument("--disk", type=Path, help="改用玩家自備原始磁碟（唯讀）")
    parser.add_argument("--variant", default="[a2]", help="nested archive 變體標記")
    parser.add_argument("--output", type=Path, help="輸出 JSON 路徑；未指定則印到 stdout")
    args = parser.parse_args()

    if bool(args.archive) == bool(args.disk):
        parser.error("請選archive或--disk其中一個輸入")
    image = args.disk.read_bytes() if args.disk else read_disk(args.archive, args.variant)
    block = image[TRADE_CLASS_OFFSET : TRADE_CLASS_OFFSET + BLOCK_SIZE]
    if len(block) != BLOCK_SIZE:
        raise SystemExit("磁碟太短，讀不到 logical block 7")
    records = decode_records(block)
    document = {
        "schema": 1,
        "tool": "tools/atari_st_trade_classes.py",
        "evidenceLevel": "proven",
        "source": {
            "input": (args.disk or args.archive).name,
            "sha256": hashlib.sha256(image).hexdigest(),
            "logicalBlock": TRADE_CLASS_BLOCK,
            "rawOffset": f"0x{TRADE_CLASS_OFFSET:05x}",
            "blockSha256": hashlib.sha256(block).hexdigest(),
            "startupCounter": block[0x1f8],
            "startupCounterRawOffset": f"0x{TRADE_CLASS_OFFSET+0x1f8:05x}",
        },
        "layout": {
            "recordCount": RECORD_COUNT,
            "recordWords": RECORD_WORDS,
            "goodsCount": GOODS_COUNT,
            "fieldsPerWord": FIELDS_PER_WORD,
            "fieldBits": FIELD_BITS,
            "note": (
                "一筆一顆行星：word 0–5 供給類別、word 6 本地產物、word 7–12 需求類別、"
                "word 13 本地需求商品。類別是 UCSD packed array，低位在前。"
            ),
        },
        "records": records,
        "rules": pin_rules(image),
        "unresolved": [],
        "corrections": [{
            "date": "2026-10-03",
            "source": "docs/atari-st/banville-restoration-spec.md",
            "summary": "原始171d分類寫入端及02eb讀取端推翻城市header位移與word13未讀的舊結論；歷史證據保留於Git及banville-state-evidence-20260908.json。",
        }, {
            "date": "2026-10-03",
            "source": "docs/atari-st/banville-restoration-spec.md",
            "summary": "logical block17是raw0x6a00貨物表；XDOTRADI027b..0287載入outera5，0927..0933依商品碼讀record byte15。其用途已核定，舊未知紀錄保留於Git。",
        }],
    }
    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
