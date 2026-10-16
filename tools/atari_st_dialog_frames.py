#!/usr/bin/env python3
"""解碼 Atari ST SunDog 的對話 frame——原版對話 VM 的一筆一筆資料。

輸入是唯讀的外層 Atari ST archive；工具只讀 nested raw ``.st``，不改寫原始媒體，
也不把磁碟 bytes 原封輸出。輸出是含 raw offset、來源雜湊與逐 frame 欄位的 JSON。

版面來自原版自己的 p-code（`XDOINTER:0x1f new_frame`、`0x16 set_frame`、
`0x0e yank`、`0x21 set_local_tokens`），每一條在 ``docs/atari-st/ship-rules.json``
都有位元組簽章：

* frame 編號 16 以上住在磁碟：logical block = ``20 + (編號 / 16 - 1)``，
  block 內索引 = ``編號 mod 16``。編號 16 以下由程式當場組出來，不在磁碟上。
* block 的前 16 個位元組是偏移表，第 16、17 個位元組是 local token 的數量與起點
  （起點乘 2），token 區的起點同時就是 frame 記錄區的結束。
* frame 的欄位是**串流讀出來的**：一個游標一次取一個位元組。因此 record 沒有固定
  長度，長度由它自己的旗標與按鈕數決定。

record 的文法（欄位順序自 p-code 讀出，旗標各個 bit 的作用由下述 tiling 判定）：

```
位元組 0   旗標 = 高 4 bit；按鈕數 = bit 2–3；顯示時間檔位 = bit 0–1
位元組 1   precode（四個子欄位的位元組，見下）
若 旗標 bit1 且 按鈕數 == 0：
    1 個位元組  接續的 frame 編號，其後接 pick_one 的候選位元組
否則：
    1 個位元組  訊息編號；小於 30 時其後接 pick_one 的候選位元組
    2 組        每組 1 個位元組，旗標 bit0 時另接 1–2 個位元組的替代值
    按鈕數 組   每組 2 個位元組，旗標 bit1 時另接 1–2 個位元組的替代值
若 旗標 bit3：
    1 個位元組  postcode 操作碼，其後接該操作碼固定數量的運算元
```

**tiling 是這份解碼的驗收條件**：99 筆 frame 逐筆解析之後的結束位置，必須恰好等於
下一筆的起始位移；每個 block 的最後一筆之後到 token 區之間必須全為零。任何一個欄位
長度記錯都會讓後面整串錯位，所以 99/99 對齊不是巧合。

**未命名的部分**保留原值：precode 與 postcode 的四個子欄位在此只做位元拆解與操作碼
記錄，不替它們的效果命名；pick_one 的候選位元組、訊息編號與按鈕標籤所指的文字，
都屬於另一份已解出的對話文字表，本工具不重複宣稱。
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
# laanwj/sundog doc/disklayout.md：`0x04800 0x00 P-system "logical addressing" base
# block`。logical block L 的檔案位移因此是 0x04800 + L * 512，與世界表（logical 8 在
# raw 0x05800）一致。
LOGICAL_BASE_OFFSET = 0x04800

# `new_frame` 以 `20 + (frame / 16 - 1)` 取 block；磁碟上這段連續 14 個 block。
FRAME_FIRST_BLOCK = 20
FRAME_BLOCK_COUNT = 14

OFFSET_TABLE_SIZE = 16
LOCAL_TOKEN_COUNT_BYTE = 16
LOCAL_TOKEN_START_BYTE = 17
LOCAL_TOKEN_START_SCALE = 2

# 訊息編號小於這個值時要先跑一次 pick_one 挑候選。
PICK_ONE_THRESHOLD = 30

# postcode 操作碼各自吃幾個運算元位元組，讀自 `XDOINTER:0x1b call_postcode` 的分派表。
POSTCODE_OPERANDS = {0: 0, 1: 1, 2: 2, 3: 4, 4: 0, 5: 1, 6: 1, 7: 1}
# 操作碼 3 的四個運算元裡，後兩個是店家存貨的第一個與最後一個物品編號。
STOCK_OPCODE = 3
STOCK_FIRST_OPERAND = 2

FLAG_ALTERNATE_SLOTS = 0x1
FLAG_ALTERNATE_BUTTONS = 0x2
FLAG_POSTCODE = 0x8

ALTERNATE_SLOTS = 2


class FrameFormatError(ValueError):
    """frame 沒有照著已解出的版面排列。"""


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            return choose_disk(nested)


def block_offset(logical_block: int) -> int:
    return LOGICAL_BASE_OFFSET + logical_block * BLOCK_SIZE


class _Cursor:
    """`XDOINTER:0x0e yank` 的游標：一次一個位元組，只往前走。"""

    def __init__(self, block: bytes, start: int) -> None:
        self.block = block
        self.at = start

    def yank(self) -> int:
        if self.at >= len(self.block):
            raise FrameFormatError(f"frame 讀過 block 結尾（位移 {self.at}）")
        value = self.block[self.at]
        self.at += 1
        return value


def _pick_one(cursor: _Cursor, value: int) -> list[int]:
    """`XDOINTER:0x17 pick_one` 會吃掉的候選位元組。

    候選要吃幾個由 value 決定，與亂數無關——亂數只影響挑中哪一個，不影響游標。
    """
    # p-code：L6 = (L5 != 22) AND (L5 != 23)，然後 `tjp` 在 L6 為真時**跳過**減法
    # （`XDOINTER:0x17` 0x0835–0x083b）——所以只有 22 與 23 會減 2。
    selector = value
    shifted = selector not in (22, 23)
    if not shifted:
        selector -= 2
    taken: list[int] = []
    if selector in (20, 21):
        if shifted:
            if selector == 21:
                taken.append(cursor.yank())
            span = 2
        else:
            span = 3 if selector == 21 else 2
        for _ in range(span + 1):
            taken.append(cursor.yank())
    elif selector <= 10:
        for _ in range(selector):
            taken.append(cursor.yank())
    else:
        # 這一支吃的是「指標、條件」交替的清單：`pick_one` 每讀一個候選就再讀一個
        # 條件碼交給 `XDOINTER:0x06 passed` 判，通過就用前一個候選；最後一個沒有條件，
        # 是預設值（`XDOINTER:0x17` 0x086a–0x0885）。
        selector -= 10
        while True:
            taken.append(cursor.yank())
            selector -= 1
            if selector > 0:
                taken.append(cursor.yank())
            if selector <= 0:
                break
    return taken


def _get_alternate(cursor: _Cursor) -> list[int]:
    """`XDOINTER:0x1c get_alternate`：條件位元組為 0 就只吃一個，否則再吃替代值。"""
    condition = cursor.yank()
    if condition == 0:
        return [condition]
    return [condition, cursor.yank()]


def parse_frame(block: bytes, start: int) -> dict[str, object]:
    cursor = _Cursor(block, start)
    header = cursor.yank()
    flags = header >> 4
    buttons = (header >> 2) & 0x3
    frame: dict[str, object] = {
        "start": start,
        "header": header,
        "flags": flags,
        "buttons": buttons,
        "displayStep": header & 0x3,
        "precode": cursor.yank(),
    }
    if (flags & FLAG_ALTERNATE_BUTTONS) and buttons == 0:
        target = cursor.yank()
        frame["kind"] = "chain"
        frame["target"] = target
        frame["choices"] = _pick_one(cursor, target)
    else:
        message = cursor.yank()
        frame["kind"] = "message"
        frame["message"] = message
        frame["choices"] = _pick_one(cursor, message) if message < PICK_ONE_THRESHOLD else []
        # 候選清單有兩種形狀。`message > 10` 那一支是「指標、條件」交替，最後一個是預設；
        # 其餘是清一色的指標，由亂數挑一個。分開記，消費端才不會把條件碼當成文字指標。
        if message < PICK_ONE_THRESHOLD and message > 10 and message not in (20, 21, 22, 23):
            frame["choiceKinds"] = [
                "pointer" if index % 2 == 0 else "condition"
                for index in range(len(frame["choices"]))
            ]
        elif frame["choices"]:
            frame["choiceKinds"] = ["pointer"] * len(frame["choices"])
        slots = []
        for _ in range(ALTERNATE_SLOTS):
            slot: dict[str, object] = {"value": cursor.yank()}
            if flags & FLAG_ALTERNATE_SLOTS:
                slot["alternate"] = _get_alternate(cursor)
            slots.append(slot)
        frame["slots"] = slots
        button_list = []
        for _ in range(buttons):
            button: dict[str, object] = {"label": cursor.yank(), "target": cursor.yank()}
            if flags & FLAG_ALTERNATE_BUTTONS:
                button["alternate"] = _get_alternate(cursor)
            button_list.append(button)
        frame["buttonList"] = button_list
    if flags & FLAG_POSTCODE:
        opcode = cursor.yank()
        operands = [cursor.yank() for _ in range(POSTCODE_OPERANDS.get(opcode, 0))]
        frame["postcode"] = {"opcode": opcode, "operands": operands}
    frame["end"] = cursor.at
    frame["length"] = cursor.at - start
    return frame


def parse_block(block: bytes, logical_block: int) -> dict[str, object]:
    if len(block) != BLOCK_SIZE:
        raise FrameFormatError(f"block {logical_block} 不是 {BLOCK_SIZE} bytes")
    table = list(block[:OFFSET_TABLE_SIZE])
    starts = [entry for entry in table if entry]
    token_count = block[LOCAL_TOKEN_COUNT_BYTE]
    token_start = block[LOCAL_TOKEN_START_BYTE] * LOCAL_TOKEN_START_SCALE

    frames = []
    for index, start in enumerate(starts):
        frame = parse_frame(block, start)
        frame["block"] = logical_block
        frame["index"] = index
        frame["id"] = OFFSET_TABLE_SIZE * (logical_block - FRAME_FIRST_BLOCK + 1) + index
        last = index + 1 == len(starts)
        expected = token_start if last else starts[index + 1]
        end = int(frame["end"])
        if last:
            # 最後一筆之後到 token 區之間允許補零，但只能是零。
            if end > expected or any(block[end:expected]):
                raise FrameFormatError(
                    f"block {logical_block} 最後一筆 frame 結束於 0x{end:02x}，"
                    f"token 區在 0x{expected:02x}，中間不是全零"
                )
        elif end != expected:
            raise FrameFormatError(
                f"block {logical_block} frame {index} 結束於 0x{end:02x}，"
                f"下一筆從 0x{expected:02x} 開始"
            )
        frames.append(frame)

    return {
        "logicalBlock": logical_block,
        "rawOffset": f"0x{block_offset(logical_block):05x}",
        "offsetTable": table,
        "localTokenCount": token_count,
        "localTokenStart": token_start,
        "frames": frames,
    }


def store_stock(frames: list[dict[str, object]]) -> list[dict[str, object]]:
    """postcode 操作碼 3 帶的兩個位元組就是店家存貨的第一個與最後一個物品編號。"""
    stock = []
    for frame in frames:
        postcode = frame.get("postcode")
        if not isinstance(postcode, dict) or postcode.get("opcode") != STOCK_OPCODE:
            continue
        operands = list(postcode.get("operands", []))
        first, last = operands[STOCK_FIRST_OPERAND], operands[STOCK_FIRST_OPERAND + 1]
        entry: dict[str, object] = {
            "frameId": frame["id"],
            "firstItem": first,
            "lastItemByte": last,
        }
        if last >= first:
            entry["items"] = list(range(first, last + 1))
        else:
            # 第二個數小於第一個時，原版先拿它去查一張執行期表再當終點。那張表由
            # `initialize()` 填，內容尚未解出，所以這裡不猜終點。
            entry["lastItemFromRuntimeTable"] = True
        stock.append(entry)
    return stock


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)
    blocks = []
    for offset in range(FRAME_BLOCK_COUNT):
        logical_block = FRAME_FIRST_BLOCK + offset
        start = block_offset(logical_block)
        blocks.append(parse_block(disk[start:start + BLOCK_SIZE], logical_block))

    frames = [frame for block in blocks for frame in block["frames"]]
    kinds: dict[str, int] = {}
    opcodes: dict[str, int] = {}
    for frame in frames:
        kinds[str(frame["kind"])] = kinds.get(str(frame["kind"]), 0) + 1
        postcode = frame.get("postcode")
        if isinstance(postcode, dict):
            key = str(postcode["opcode"])
            opcodes[key] = opcodes.get(key, 0) + 1

    return {
        "schema": 1,
        "tool": "tools/atari_st_dialog_frames.py",
        "evidenceLevel": "proven",
        "source": {
            "archive": archive_path.name,
            "archiveSha256": sha256(archive_path.read_bytes()),
            "variant": variant,
            "disk": disk_name,
            "diskSha256": sha256(disk),
        },
        "layout": {
            "logicalBaseOffset": f"0x{LOGICAL_BASE_OFFSET:05x}",
            "firstLogicalBlock": FRAME_FIRST_BLOCK,
            "logicalBlockCount": FRAME_BLOCK_COUNT,
            "blockSize": BLOCK_SIZE,
            "offsetTableSize": OFFSET_TABLE_SIZE,
            "localTokenCountByte": LOCAL_TOKEN_COUNT_BYTE,
            "localTokenStartByte": LOCAL_TOKEN_START_BYTE,
            "localTokenStartScale": LOCAL_TOKEN_START_SCALE,
            "postcodeOperands": {str(k): v for k, v in POSTCODE_OPERANDS.items()},
        },
        "summary": {
            "frameCount": len(frames),
            "kinds": kinds,
            "postcodeOpcodes": opcodes,
            "tilingChecked": True,
        },
        "storeStock": store_stock(frames),
        "blocks": blocks,
        "unresolved": [
            "存貨終點小於起點時查的那張執行期表，內容由 initialize() 填，尚未解出。",
        ],
        "resolvedElsewhere": [
            "precode 的四個子欄位與 postcode 八個操作碼的效果：見 docs/atari-st/dialog-rendered.md"
            "與 dialog-vm.json。",
            "pick_one 的候選位元組指向哪一段文字：見 docs/atari-st/dialog-rendered.json，"
            "183 個文字指標全部落在字串邊界上。",
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
