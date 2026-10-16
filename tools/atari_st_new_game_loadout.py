#!/usr/bin/env python3
"""從 Atari ST SunDog 的 p-code 解出新遊戲的隨船配備。

分流器與船艦儲物的初始內容不在資料表裡，而是 ``XSTARTUP:0x32 set_up_lockers`` 直接寫進
遊戲狀態的常數。本工具在 ``SYSTEM.STARTUP`` 的位元組流裡找那一段連續的寫入，把偏移量、
數量與物品編號讀出來。

## 三種寫入的位元組形狀

所有寫入都以 ``9b 04 5b``（``lae 0x4,0x5b``，系統狀態陣列的基底）加上 ``80 <off>``
（``ldcb``，基底起算的 byte 偏移）開頭，接著是三種之一：

- ``<n> <val> 70 15``：``fillchar``，把 ``n`` 個 byte 填成 ``val``。
- ``<val> 80 <hi> a2 c8``：把 ``val + hi`` 寫進一個 byte。高位不是物品編號的一部分——
  物品編號只佔低 5 bits——但它的用途未解，因此原值保留。
- ``<val> c8``：把 ``val`` 寫進一個 byte。
- ``<val> 00 73 0b <n> 8f a2 c8``：把 ``val + random() mod n`` 寫進一個 byte。這一格是
  船體（見 ``docs/atari-st/combat-rules.md``），新遊戲因此開在 ``val`` 到 ``val + n - 1``
  之間。

``<n>``／``<val>`` 是 ``sldc``（``0x00``–``0x1f``，opcode 的值即常數）。

## 為什麼要求連續

單一個寫入的形狀在別處也可能出現。``set_up_lockers`` 的寫入首尾相接，工具因此要求它們
構成一段連續的區塊，並且第一個必須是填分流器的那一次；找不到這樣的區塊就 fail-closed，
不退而求其次挑最像的那個。

形狀不符的寫入會被記成 ``unrecognised`` 並保留原始位元組，而不是被略過——略過會讓「連續」
這個條件失去意義，也會讓那一格從紀錄裡消失。續接只在下一個基底寫入就在附近時才允許。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


# lae 0x4,0x5b 之後接 ldcb：系統狀態陣列的基底加一個 byte 偏移。
BASE_PREFIX = bytes([0x9B, 0x04, 0x5B, 0x80])
FILLCHAR_SUFFIX = bytes([0x70, 0x15])
STORE_OPCODE = 0xC8
LDCB_OPCODE = 0x80
ADD_OPCODE = 0xA2
MOD_OPCODE = 0x8F
# scxg4 0x0b：呼叫 MAINLIB:0x0b random()。
RANDOM_CALL = bytes([0x73, 0x0B])
SLDC_LIMIT = 0x20

# 分流器：第一個寫入必須是它，否則這段不是 set_up_lockers。
SHUNT_ITEM = 0x17
# 物品編號只佔低 5 bits。
ITEM_CODE_MASK = 0x1F
# 兩個可辨識的寫入之間最多容許這麼多 bytes 的未辨識寫入。
UNRECOGNISED_LIMIT = 24


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return disk_name, disk_bytes


def decode_write(disk: bytes, offset: int) -> dict[str, object] | None:
    """讀出一次寫入；形狀不符就回 None。"""
    if disk[offset : offset + len(BASE_PREFIX)] != BASE_PREFIX:
        return None
    target = disk[offset + 4]
    body = offset + 5

    if disk[body] < SLDC_LIMIT and disk[body + 1] < SLDC_LIMIT and disk[body + 2 : body + 4] == FILLCHAR_SUFFIX:
        return {
            "rawOffset": f"0x{offset:05x}",
            "kind": "fillchar",
            "stateOffset": target,
            "count": disk[body],
            "value": disk[body + 1],
            "size": body + 4 - offset,
        }

    if (
        disk[body] < SLDC_LIMIT
        and disk[body + 1] == LDCB_OPCODE
        and disk[body + 3] == ADD_OPCODE
        and disk[body + 4] == STORE_OPCODE
    ):
        return {
            "rawOffset": f"0x{offset:05x}",
            "kind": "store",
            "stateOffset": target,
            "count": 1,
            "value": disk[body] + disk[body + 2],
            "size": body + 5 - offset,
        }

    if disk[body] < SLDC_LIMIT and disk[body + 1] == STORE_OPCODE:
        return {
            "rawOffset": f"0x{offset:05x}",
            "kind": "store",
            "stateOffset": target,
            "count": 1,
            "value": disk[body],
            "size": body + 2 - offset,
        }

    if (
        disk[body] < SLDC_LIMIT
        and disk[body + 1] == 0x00
        and disk[body + 2 : body + 4] == RANDOM_CALL
        and disk[body + 4] < SLDC_LIMIT
        and disk[body + 5] == MOD_OPCODE
        and disk[body + 6] == ADD_OPCODE
        and disk[body + 7] == STORE_OPCODE
    ):
        return {
            "rawOffset": f"0x{offset:05x}",
            "kind": "randomRange",
            "stateOffset": target,
            "count": 1,
            "value": disk[body],
            "span": disk[body + 4],
            "size": body + 8 - offset,
        }

    return None


def find_loadout(disk: bytes) -> list[dict[str, object]]:
    """找出 set_up_lockers 那一段連續寫入。"""
    candidates: list[list[dict[str, object]]] = []
    position = -1
    while True:
        position = disk.find(BASE_PREFIX, position + 1)
        if position < 0:
            break
        first = decode_write(disk, position)
        if first is None or first["kind"] != "fillchar" or first["value"] != SHUNT_ITEM:
            continue
        run = [first]
        cursor = position + int(first["size"])
        while True:
            write = decode_write(disk, cursor)
            if write is not None:
                run.append(write)
                cursor += int(write["size"])
                continue
            # 這一段裡有一次寫入不是那三種形狀（它的值由 random() 決定）。跳過它會讓
            # 「連續」這個條件失去意義，所以它被記下來而不是被忽略，並且只允許在下一個
            # 基底寫入就在附近時才續接。
            following = disk.find(BASE_PREFIX, cursor + 1, cursor + 1 + UNRECOGNISED_LIMIT)
            if following < 0:
                break
            run.append(
                {
                    "rawOffset": f"0x{cursor:05x}",
                    "kind": "unrecognised",
                    "stateOffset": disk[cursor + 4],
                    "count": 1,
                    "value": None,
                    "bytes": disk[cursor:following].hex(),
                    "size": following - cursor,
                }
            )
            cursor = following
        if len(run) > 1:
            candidates.append(run)

    if len(candidates) != 1:
        raise ValueError(f"found {len(candidates)} candidate loadout blocks, want exactly 1")
    return candidates[0]


def build_document(archive_path: Path, variant: str, item_data_path: Path) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)
    document = json.loads(item_data_path.read_text(encoding="utf-8"))
    names = {int(str(record["index"]), 0): record["name"] for record in document["items"]["records"]}

    writes = []
    for write in find_loadout(disk):
        if write["kind"] == "unrecognised":
            writes.append(
                {
                    "rawOffset": write["rawOffset"],
                    "kind": "unrecognised",
                    "stateOffset": f"0x{int(write['stateOffset']):02x}",
                    "bytes": write["bytes"],
                    "note": "這一次寫入的值由 random() 決定，形狀不是常數寫入；原始位元組保留，remake 不套用。",
                }
            )
            continue
        if write["kind"] == "randomRange":
            minimum = int(write["value"])
            span = int(write["span"])
            writes.append(
                {
                    "rawOffset": write["rawOffset"],
                    "kind": "randomRange",
                    "stateOffset": f"0x{int(write['stateOffset']):02x}",
                    "minimum": minimum,
                    "span": span,
                    "maximum": minimum + span - 1,
                    "meaning": "船體",
                    "meaningEvidence": (
                        "SHIPLIB:0x21 update_attack 以同一個 byte 當船體，每次打中扣 random() mod 3 + 1；"
                        "XDOUNITE:0x13 do_depot:repair_ship 也讀它。見 docs/atari-st/ship-rules.md 的「打船體」一節。"
                    ),
                }
            )
            continue
        code = int(write["value"]) & ITEM_CODE_MASK
        if code not in names:
            raise ValueError(f"loadout writes item 0x{code:02x}, absent from the item table")
        entry = {
            "rawOffset": write["rawOffset"],
            "kind": write["kind"],
            "stateOffset": f"0x{int(write['stateOffset']):02x}",
            "count": write["count"],
            "rawValue": f"0x{int(write['value']):02x}",
            "itemIndex": f"0x{code:02x}",
            "itemName": names[code],
        }
        if int(write["value"]) != code:
            entry["highBits"] = f"0x{int(write['value']) & ~ITEM_CODE_MASK:02x}"
            entry["highBitsNote"] = "高 3 位是剩餘充能次數（0xe0 = 7 次），低 5 位才是物品編號；見本檔 openQuestions 的 high bits 與 ground-rules.md。"
        writes.append(entry)

    return {
        "schema": 1,
        "tool": "tools/atari_st_new_game_loadout.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
            "itemData": {"path": str(item_data_path), "sha256": sha256(item_data_path.read_bytes())},
        },
        "procedure": "XSTARTUP:0x32 utility:set_new_game:new_game:set_up_lockers",
        "arrayBaseGlobal": "MAINLIB word 0x5b",
        "writes": writes,
        "evidence": "proven",
        "notes": [
            "偏移量是「系統狀態陣列基底起算的 byte 數」，不是 doc/gamestate.md 的 word offset。",
            (
                "次級文件把船艦 locker 記為 word 0x0af（基底起算 168 bytes）、stores 記為 word 0x0bd"
                "（196 bytes），各 20 bytes。填分流器那次寫在 183–187，落在 locker 尾端；"
                "0xc0（192）落在兩者之間，兩份記載都涵蓋不到。"
            ),
            "反組譯使用 laanwj/sundog 的 tools/list_pcode.py；位元組與結論出自本專案。",
        ],
        "openQuestions": [
            {
                "topic": "byte 0xc0",
                "status": "proven",
                "detail": (
                    "該 byte 是船體：SHIPLIB:0x21 update_attack 打中它時扣 random() mod 3 + 1，"
                    "XDOUNITE:0x13 do_depot:repair_ship 修它。新遊戲開在 13–16。"
                ),
            },
            {
                "topic": "high bits",
                "status": "proven",
                "detail": (
                    "口袋位元組的低 5 位是物品編號、高 3 位是剩餘充能次數，因此 0xe0 代表 7 次。"
                    "第二個獨立證據是對話後置碼操作碼 1：對 shield 與 scattergun 擲 rand(1,7) × 32。"
                    "護盾起始的 7 次與遊戲內文字「absorbs up to seven direct weapon hits!」相符。"
                    "見 docs/atari-st/ground-rules.md 與 dialog-rendered.md。"
                ),
            },
        ],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path, help="外層 Atari ST archive（唯讀）")
    parser.add_argument("--variant", default="[a2]", help="nested archive 變體標記")
    parser.add_argument("--item-data", type=Path, default=Path("docs/atari-st/item-data.json"))
    parser.add_argument("--output", type=Path, help="輸出 JSON 路徑；未指定則印到 stdout")
    args = parser.parse_args()

    document = build_document(args.archive, args.variant, args.item_data)
    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
