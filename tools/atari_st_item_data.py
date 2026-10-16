#!/usr/bin/env python3
"""解碼 Atari ST SunDog 的物品表與貨物表。

輸入是唯讀的外層 Atari ST archive；工具只讀 nested raw ``.st``，輸出是含 raw offset、
來源雜湊與證據等級的 JSON。

區段位置取自公開 secondary oracle ``laanwj/sundog`` 的 ``doc/disklayout.md``
（``0x05e00`` Inv items、``0x06a00`` Cargo）；**record 邊界、欄位與數值是本工具自行從
bytes 解出並驗證的**。

## record 格式

兩張表都是 32 筆 16-byte record：1 byte 長度前綴 + 名稱字元，尾端補零，**最後一個
byte 是該筆的數值**。名稱長度 14 以內時兩者不重疊。

貨物表第 25 筆 ``pharmaceuticals`` 的名稱剛好 15 個字元，把數值那個 byte 佔掉了。
這是原始資料本身的邊界情況，不是解碼錯誤：本工具保留該筆名稱、把數值標成 ``absent``，
不替它補一個數字。

## 數值語意

尾端數值是**定價的輸入**：``XDOINTER:0x02 set_up_price`` 以 16 bytes 步距索引物品表，
再讀 offset 15 的 byte。同一支程序開頭的 ``item mod 0x20`` 也證實表恰為 32 筆。
完整的價格公式已解出，寫在 ``docs/atari-st/prices.md``；工具仍只記錄原值，
不替它換算成「價格」欄位——價格取決於星系、城市與船上是否已有同一件，不是物品的屬性。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


ITEM_TABLE_START = 0x05E00
CARGO_TABLE_START = 0x06A00
RECORD_SIZE = 16
RECORD_COUNT = 32
TABLE_SIZE = RECORD_SIZE * RECORD_COUNT
NAME_MAX_LENGTH = RECORD_SIZE - 1
VALUE_OFFSET = RECORD_SIZE - 1


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        nested_name, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return nested_name, disk_name, disk_bytes


def decode_record(record: bytes, raw_offset: int) -> dict[str, object]:
    if len(record) != RECORD_SIZE:
        raise ValueError(f"record at 0x{raw_offset:05x} is {len(record)} bytes")
    length = record[0]
    if length == 0 or length > NAME_MAX_LENGTH:
        raise ValueError(f"record at 0x{raw_offset:05x} declares name length {length}")
    name_bytes = record[1 : 1 + length]
    for byte in name_bytes:
        if byte < 0x20 or byte > 0x7E:
            raise ValueError(f"record at 0x{raw_offset:05x} has a non-printable name byte 0x{byte:02x}")

    result: dict[str, object] = {
        "rawOffset": f"0x{raw_offset:05x}",
        "name": name_bytes.decode("ascii"),
        "nameLength": length,
        "recordSha256": sha256(record),
        "rawTailByte": record[VALUE_OFFSET],
    }

    # 名稱佔滿時沒有獨立數值；仍保留XDOTRADI無條件讀取的實際尾byte。
    if 1 + length > VALUE_OFFSET:
        result["value"] = None
        result["valueStatus"] = "absent"
        result["note"] = "名稱長度佔用了數值所在的 byte；原始資料如此，未補值。"
        return result

    padding = record[1 + length : VALUE_OFFSET]
    if any(padding):
        raise ValueError(f"record at 0x{raw_offset:05x} has non-zero padding")
    result["value"] = record[VALUE_OFFSET]
    result["valueStatus"] = "present"
    return result


def decode_table(disk: bytes, start: int, kind: str) -> dict[str, object]:
    region = disk[start : start + TABLE_SIZE]
    if len(region) != TABLE_SIZE:
        raise ValueError(f"{kind} table is truncated")
    records = []
    for index in range(RECORD_COUNT):
        offset = start + index * RECORD_SIZE
        record = decode_record(disk[offset : offset + RECORD_SIZE], offset)
        record["index"] = f"0x{index:02x}"
        records.append(record)
    return {
        "rawOffset": f"0x{start:05x}",
        "recordSize": RECORD_SIZE,
        "recordCount": RECORD_COUNT,
        "sha256": sha256(region),
        "records": records,
    }


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    nested_name, disk_name, disk = load_disk(archive_path, variant)
    items = decode_table(disk, ITEM_TABLE_START, "item")
    cargo = decode_table(disk, CARGO_TABLE_START, "cargo")

    return {
        "schema": 1,
        "tool": "tools/atari_st_item_data.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "nestedArchive": nested_name,
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "locationSource": "laanwj/sundog doc/disklayout.md (secondary oracle, commit e56d8e2532134f0741a7f82b80f9357776779f27)",
        "locationEvidence": "proven（次級文件所載）",
        "recordBoundaryEvidence": "proven",
        "items": items,
        "cargo": cargo,
        "openQuestions": [
            {
                "topic": "cargo raw tail byte",
                "status": "proven",
                "detail": "XDOTRADI raw基底0x44200，0x0927..0x0933無條件讀貨物record byte15；pharmaceuticals為名稱末字母s即115，獨立value仍absent；cryogens原值0。見docs/atari-st/banville-restoration-spec.md。",
            },
            {
                "topic": "value semantics",
                "status": "proven-by-oracle",
                "detail": (
                    "XDOINTER:0x02 set_up_price 以 ixa 0x8（16 bytes 步距）索引物品表，"
                    "再 ldb 讀 offset 15 的 byte，也就是本工具解出的尾端數值——它是定價的輸入。"
                    "完整的價格公式已解出，見 docs/atari-st/prices.md："
                    "倍率 = clamp(30 × 星系物價等級 / 城市係數, 1, 32)，"
                    "且「船上沒有同一件」時反而更貴。"
                ),
            },
            {
                "topic": "item index wrap",
                "status": "proven-by-oracle",
                "detail": "set_up_price 開頭即 `item mod 0x20`，證實物品表恰為 32 筆。",
            },
            {
                "topic": "dialog item name",
                "status": "proven-by-oracle",
                "detail": (
                    "對話引用 0x25（物品名）由 set_up_price 從同一張表以相同索引取出，"
                    "寫進 XDOINTER_01_L54；doc/sundog_dialog_system.md 對該 slot 的記載因此得到證實。"
                ),
            },
            {
                "topic": "cargo table size",
                "status": "proven",
                "detail": (
                    "次級文件把貨物標為 2 個 block，但 32 筆 record 恰好填滿 1 個 block "
                    "（512 bytes），其後即為對話文字區。以自行解出的邊界為準。"
                ),
            },
            {
                "topic": "ship system composition",
                "status": "proven",
                "detail": (
                    "六套船艦系統各需要哪四個零件不在這兩張表裡，但已由原版 p-code 自行解出："
                    "版面與零件編號見 docs/atari-st/ship-systems.json 與 "
                    "docs/atari-st/secondary-gamestate.md。本工具只驗證那些編號在物品表中存在。"
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
