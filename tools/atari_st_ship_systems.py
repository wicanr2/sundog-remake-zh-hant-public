#!/usr/bin/env python3
"""從 Atari ST SunDog 的 p-code 解出六套船艦系統的零件組成。

這些數字不在任何資料表裡，而是新遊戲初始化程序 ``XSTARTUP:0x31`` 的六次呼叫參數。
本工具因此不是掃資料區，而是**在 `SYSTEM.STARTUP` 的位元組流裡找那六個呼叫點**，
把參數讀出來。

## 呼叫點的形狀

UCSD p-code 的 ``0x00``–``0x1f`` 是 ``sldc0``–``sldc31``（推入常數，opcode 值即常數），
``90 nn`` 是 ``clp 0xnn``（呼叫同層 procedure）。六個參數的呼叫點因此是「六個小於
``0x20`` 的 byte 接著 ``90 31``」。

參數順序為 ``sys(index, col1, col2, col3, altItem, altColumn)``：

- ``index`` 是系統編號（``0``–``5``）。
- ``col1``–``col3`` 是第 1–3 欄的零件 item 編號；第 0 欄由程序自己寫死為 ``0x1f``
  control node，不經參數。
- ``altItem``／``altColumn`` 是該系統的替代零件與它可以裝的欄位；``0`` 表示沒有替代零件。

**第一個呼叫（系統 0）的 index 不是常數**，而是 ``lde 0x4,0xe1`` 讀一個全域值，因此不符合
上面的形狀。工具的處理方式是：五個常數呼叫必須恰好給出 ``1``–``5``，且第六個呼叫點必須
緊接在它們之前、參數 1–5 同樣是常數；此時把它認定為系統 ``0``。這是消去法，不是猜測——
六套系統與五個已知編號之間只剩一個空位。

## record 版面（由 ``XSTARTUP:0x31`` 與 ``SHIPLIB:0x0e`` 的 p-code 證實）

系統陣列的基底是 MAINLIB 全域 word ``0x5b``，以 byte 定址，**每套系統 28 bytes**：

- ``[row*5 + col]``（row 0–4、col 0–3）：該格裝的零件；``[row*5 + 4]``：該列狀態。
- **第 4 列是樣板列**，初始化時一律寫成完好的 ``[0x1f, col1, col2, col3, 4]``，
  ``SHIPLIB:0x0e`` 就是拿 ``[20 + col]``（即樣板列）當「這一欄應該是什麼」的依據。
- ``[24]``：系統總狀態＝四個實際列的狀態和，最大 ``0x10``；``[25]``：初始化為 0。
- ``[26]``／``[27]``：替代零件的編號與欄位。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested

# clp 0x31：呼叫 XSTARTUP:0x31（新遊戲的系統初始化）。
CALL_OPCODE = 0x90
CALL_PROCEDURE = 0x31
# 0x00–0x1f 是 sldc0–sldc31，opcode 的值就是被推入的常數。
SLDC_LIMIT = 0x20
PARAM_COUNT = 6
SYSTEM_COUNT = 6
# 第 0 欄由程序寫死，不經參數。
CONTROL_NODE_ITEM = 0x1F
# 零件在物品表中的編號範圍；解出的每個編號都必須落在這裡並在表中存在。
PART_ITEM_MIN = 0x13
PART_ITEM_MAX = 0x1F

SYSTEM_STRIDE = 28
ROW_STRIDE = 5
ROW_COUNT = 5
TEMPLATE_ROW = 4
PART_COLUMNS = 4
STATUS_OFFSET = 24
RESERVED_OFFSET = 25
ALTERNATE_ITEM_OFFSET = 26
ALTERNATE_COLUMN_OFFSET = 27
MAX_SYSTEM_STATUS = 0x10


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return disk_name, disk_bytes


def find_constant_call_sites(disk: bytes) -> list[int]:
    """找出六個參數全為常數的 ``clp 0x31`` 呼叫點，回傳 opcode 的位置。"""
    sites: list[int] = []
    for offset in range(PARAM_COUNT, len(disk) - 1):
        if disk[offset] != CALL_OPCODE or disk[offset + 1] != CALL_PROCEDURE:
            continue
        params = disk[offset - PARAM_COUNT : offset]
        if all(byte < SLDC_LIMIT for byte in params):
            sites.append(offset)
    return sites


def decode_call_sites(disk: bytes) -> list[dict[str, object]]:
    sites = find_constant_call_sites(disk)
    if len(sites) != SYSTEM_COUNT - 1:
        raise ValueError(f"found {len(sites)} constant-parameter call sites, want {SYSTEM_COUNT - 1}")

    # 五個常數呼叫必須連續、每個 8 bytes（6 個 sldc + 2 bytes 的 clp）。
    for previous, current in zip(sites, sites[1:]):
        if current - previous != PARAM_COUNT + 2:
            raise ValueError(f"call sites 0x{previous:05x} and 0x{current:05x} are not adjacent")

    calls: list[dict[str, object]] = []
    for site in sites:
        params = list(disk[site - PARAM_COUNT : site])
        calls.append({"site": site, "params": params})

    indices = [call["params"][0] for call in calls]
    if indices != list(range(1, SYSTEM_COUNT)):
        raise ValueError(f"constant call sites declare system indices {indices}, want 1..{SYSTEM_COUNT - 1}")

    # 系統 0 的呼叫緊接在前，index 由全域值供給，其餘五個參數仍是常數。
    first_site = sites[0] - (PARAM_COUNT + 2)
    if disk[first_site] != CALL_OPCODE or disk[first_site + 1] != CALL_PROCEDURE:
        raise ValueError(f"no call to XSTARTUP:0x31 immediately before 0x{sites[0]:05x}")
    leading = list(disk[first_site - (PARAM_COUNT - 1) : first_site])
    if not all(byte < SLDC_LIMIT for byte in leading):
        raise ValueError(f"call site 0x{first_site:05x} does not push five constant parameters")
    calls.insert(0, {"site": first_site, "params": [0] + leading, "indexFromGlobal": True})
    return calls


def build_systems(calls: list[dict[str, object]], item_names: dict[int, str]) -> list[dict[str, object]]:
    systems: list[dict[str, object]] = []
    for call in calls:
        params = call["params"]
        index, column_items, alternate_item, alternate_column = params[0], params[1:4], params[4], params[5]

        columns = []
        for column, item in enumerate([CONTROL_NODE_ITEM, *column_items]):
            if not PART_ITEM_MIN <= item <= PART_ITEM_MAX:
                raise ValueError(f"system {index} column {column} wants item 0x{item:02x}, outside the part range")
            if item not in item_names:
                raise ValueError(f"system {index} column {column} wants item 0x{item:02x}, absent from the item table")
            columns.append({"column": column, "itemIndex": f"0x{item:02x}", "itemName": item_names[item]})

        entry: dict[str, object] = {
            "index": index,
            "rawOffset": f"0x{call['site']:05x}",
            "columns": columns,
        }
        if call.get("indexFromGlobal"):
            entry["indexEvidence"] = (
                "此呼叫的系統編號由全域值供給，不是常數；其餘五個呼叫恰為 1..5，"
                "六套系統中只剩 0，因此以消去法認定。"
            )
        if alternate_item == 0:
            entry["alternate"] = None
        else:
            if not PART_ITEM_MIN <= alternate_item <= PART_ITEM_MAX:
                raise ValueError(f"system {index} alternate item 0x{alternate_item:02x} is outside the part range")
            if alternate_item not in item_names:
                raise ValueError(f"system {index} alternate item 0x{alternate_item:02x} is absent from the item table")
            if not 0 < alternate_column < PART_COLUMNS:
                raise ValueError(f"system {index} alternate column {alternate_column} is not a part column above 0")
            entry["alternate"] = {
                "itemIndex": f"0x{alternate_item:02x}",
                "itemName": item_names[alternate_item],
                "column": alternate_column,
            }
        systems.append(entry)
    return systems


def load_item_names(item_data_path: Path) -> dict[int, str]:
    document = json.loads(item_data_path.read_text(encoding="utf-8"))
    names: dict[int, str] = {}
    for record in document["items"]["records"]:
        names[int(str(record["index"]), 0)] = record["name"]
    return names


def build_document(archive_path: Path, variant: str, item_data_path: Path) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)
    item_names = load_item_names(item_data_path)
    calls = decode_call_sites(disk)
    systems = build_systems(calls, item_names)

    return {
        "schema": 1,
        "tool": "tools/atari_st_ship_systems.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
            "itemData": {"path": str(item_data_path), "sha256": sha256(item_data_path.read_bytes())},
        },
        "recordLayout": {
            "arrayBaseGlobal": "MAINLIB word 0x5b",
            "systemStride": SYSTEM_STRIDE,
            "rowCount": ROW_COUNT,
            "rowStride": ROW_STRIDE,
            "partColumns": PART_COLUMNS,
            "templateRow": TEMPLATE_ROW,
            "statusOffset": STATUS_OFFSET,
            "reservedOffset": RESERVED_OFFSET,
            "alternateItemOffset": ALTERNATE_ITEM_OFFSET,
            "alternateColumnOffset": ALTERNATE_COLUMN_OFFSET,
            "maxSystemStatus": MAX_SYSTEM_STATUS,
        },
        "rules": [
            {
                "topic": "row status",
                "status": "proven",
                "detail": (
                    "SHIPLIB:0x0e update_system 由 4 起算逐欄檢查：裝了 shunt（0x17）且不是第 0 欄，"
                    "狀態減 1；裝的不是樣板列該欄的零件（或替代零件）則整列歸 0。"
                ),
            },
            {
                "topic": "system status",
                "status": "proven",
                "detail": (
                    "系統總狀態寫在 [24]，等於四個實際列的狀態和，最大 0x10。"
                    "XSTARTUP:0x31 初始化時先把五列狀態加總再減 4，扣掉的正是樣板列那一列。"
                ),
            },
            {
                "topic": "shunt placement",
                "status": "proven",
                "detail": "XDOREPAI:0x0a fits 允許 shunt 裝進第 0 欄以外的任一欄；第 0 欄（control node）不能被 shunt 取代。",
            },
            {
                "topic": "alternate part",
                "status": "proven",
                "detail": (
                    "fits 與 update_system 都額外檢查 [26]／[27] 這組「替代零件＋欄位」。"
                    "替代零件裝上時該列仍算完好，並另外設一個旗標。"
                ),
            },
            {
                "topic": "new game damage",
                "status": "proven",
                "detail": (
                    "新遊戲每一實際列有 1/3 機率帶缺陷：隨機挑一欄，若挑到第 0 欄或再一次亂數為真，"
                    "該格變成空（0）或壞掉的零件（0x07）且該列狀態 0；否則裝上 shunt（0x17）、狀態 3。"
                ),
            },
        ],
        "systems": systems,
        "evidence": "proven",
        "notes": [
            "本表的數字是 p-code 的呼叫參數，不是資料區的 record；工具因此比對呼叫點的位元組形狀而非表格版面。",
            (
                "次級文件 laanwj/sundog doc/gamestate.md 把 Ship's Guns 第 2 欄記成 concentrator（0x13）；"
                "呼叫參數顯示該欄的樣板零件是 photon bridge（0x1a），concentrator 是它的替代零件。"
                "Force Shields 的 cloaker（0x15）與 Pilotage 的 ground scanner（0x16）兩組替代零件則是次級文件沒有的。"
            ),
            "反組譯使用 laanwj/sundog 的 tools/list_pcode.py；位元組與結論出自本專案。",
        ],
        "openQuestions": [
            {
                "topic": "system names",
                "status": "proven",
                "detail": (
                    "六套系統的顯示名稱在 XDOREPAI 的常數池（段內 0x06a2 起，長度前綴字串）："
                    "Warp Drives、Sub-c Engines、Ship's Guns、Tactical Disply、Force Shields、Pilotage，"
                    "後面接著 Ship's Locker 與 Ship's Stores。「Tactical Disply」是原版自己的拼寫。"
                ),
            },
            {
                "topic": "damage sources",
                "status": "proven",
                "detail": (
                    "SHIPLIB:0x21 update_attack 是唯一會打壞零件的來源，見 docs/atari-st/ship-rules.md。"
                    "XDOFIGHT 對 MAINLIB_G5b 的寫入只有 0xbf（戰鬥暫存）與 0xc1（敵艦護盾），"
                    "不碰六套系統那一段；航行本身也沒有損耗零件的寫入點。"
                    "這是掃過全段 p-code 的**有界否定觀察**：零件只會被戰鬥打壞。"
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
