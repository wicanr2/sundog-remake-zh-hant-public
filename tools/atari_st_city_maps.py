#!/usr/bin/env python3
"""解碼 Atari ST SunDog 的 60 張城市地表地圖。

輸入是唯讀的外層 Atari ST archive；工具只讀 nested raw ``.st``，不改寫原始媒體。
輸出是含 raw offset、來源雜湊、逐格地形與建築座標的 JSON。

位置與版面都來自原版自己的 p-code（`XMOVEONG:0x22 draw_city_map`），每一條在
``docs/atari-st/ship-rules.json`` 都有位元組簽章：

* 城市編號 1–60。地圖所在的 logical block = ``0x22 + (編號 - 1) / 4``，
  block 內第 ``(編號 - 1) mod 4`` 個 128-byte 區段——**一個 block 裝四座城市**。
* 一張地圖固定 128 bytes：

  | 位移 | 內容 |
  |---|---|
  | 0–1 | 四個 4-bit 欄位；其中三個在繪圖時各加一個偏移後當成圖塊碼 |
  | 2–7 | 六個建築位置，每個一個 byte：``行 = b mod 20``、``列 = b / 20``，`0xff` 表示沒有 |
  | 8–127 | 20 列 × 12 格地形，每格 4 bit（每列 6 bytes）|

繪圖迴圈的列走 10–29、行走 6–17，都是螢幕座標；換算回地圖座標就是 20 × 12。

**驗收條件**：六個建築位置只能是 `0xff` 或小於 240（= 20 × 12）。60 座城市共 360 個
欄位全部通過；版面若記錯，這個範圍檢查會大量失敗。

**未命名的部分**保留原值：前兩個 byte 的四個 4-bit 欄位只記數值與繪圖時加上的偏移，
不替它們的用途命名；地形 0–15 的每一格代表什麼地物也不在此宣稱。
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
LOGICAL_BASE_OFFSET = 0x04800

CITY_COUNT = 60
CITY_MAP_FIRST_BLOCK = 0x22
CITY_MAP_SIZE = 128
CITY_MAPS_PER_BLOCK = 4

GRID_ROWS = 20
GRID_COLS = 12
GRID_OFFSET = 8
ROW_BYTES = GRID_COLS // 2

BUILDING_SLOTS = 6
BUILDING_OFFSET = 2
BUILDING_ABSENT = 0xFF
# 建築槽位在繪圖迴圈裡的編號是 0x22–0x27，同時當成畫在該格的圖塊碼。
BUILDING_FIRST_CODE = 0x22

# 前兩個 byte 的四個 4-bit 欄位。繪圖時欄位 0 加 31、欄位 1 與 2 各加 34 後當圖塊碼用；
# 欄位 3 在 draw_city_map 裡沒有被讀到。
HEADER_FIELD_BIASES = {0: 31, 1: 34, 2: 34}


class CityMapFormatError(ValueError):
    """城市地圖沒有照著已解出的版面排列。"""


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            return choose_disk(nested)


def city_map_offset(city_index: int) -> int:
    """city_index 是 0-based；原版的城市編號是它加 1。"""
    if not 0 <= city_index < CITY_COUNT:
        raise CityMapFormatError(f"城市索引 {city_index} 不在 0–{CITY_COUNT - 1}")
    block = CITY_MAP_FIRST_BLOCK + city_index // CITY_MAPS_PER_BLOCK
    within = (city_index % CITY_MAPS_PER_BLOCK) * CITY_MAP_SIZE
    return LOGICAL_BASE_OFFSET + block * BLOCK_SIZE + within


def decode_header(data: bytes) -> dict[str, object]:
    # 原版以 `ixp 4,4` 取這四個欄位——那是「一個 16-bit word 裝四個 4-bit 欄位、
    # 欄位 0 在最低位」。68000 是大端，word 的最低位在**第二個 byte**，所以
    # 讀 word 必須大端，欄位再從低位往上取。
    word = (data[0] << 8) | data[1]
    fields = [(word >> (index * 4)) & 0xF for index in range(4)]
    return {
        "fields": fields,
        "tileCodes": {str(index): fields[index] + bias for index, bias in HEADER_FIELD_BIASES.items()},
    }


def decode_buildings(data: bytes) -> list[dict[str, int]]:
    buildings = []
    for slot in range(BUILDING_SLOTS):
        value = data[BUILDING_OFFSET + slot]
        if value == BUILDING_ABSENT:
            continue
        if value >= GRID_ROWS * GRID_COLS:
            raise CityMapFormatError(
                f"建築槽位 {slot} 的位置 {value} 超出 {GRID_ROWS} × {GRID_COLS} 的地圖"
            )
        buildings.append({
            "slot": slot,
            "code": BUILDING_FIRST_CODE + slot,
            "row": value % GRID_ROWS,
            "column": value // GRID_ROWS,
        })
    return buildings


def decode_grid(data: bytes) -> list[list[int]]:
    """地圖是 20 筆 × 每筆 3 個 word、每個 word 四個 4-bit 欄位。

    `XMOVEONG:0x22 draw_city_map` 以 `ixp 0x4,0x4` 取格子，也就是「欄位 0 在
    word 的最低位」。68000 的 word 是大端，最低位落在**第二個 byte**，因此
    一個 word 的四個格子依序是 byte1 低、byte1 高、byte0 低、byte0 高。
    逐 byte 讀會把每四格對調成 (2,3,0,1)，讓地圖的列兩兩互換。
    """
    rows = []
    for row in range(GRID_ROWS):
        start = GRID_OFFSET + row * ROW_BYTES
        cells: list[int] = []
        for word_start in range(start, start + ROW_BYTES, 2):
            word = (data[word_start] << 8) | data[word_start + 1]
            cells.extend((word >> (index * 4)) & 0xF for index in range(4))
        rows.append(cells)
    return rows


def decode_city(disk: bytes, city_index: int) -> dict[str, object]:
    offset = city_map_offset(city_index)
    data = disk[offset:offset + CITY_MAP_SIZE]
    if len(data) != CITY_MAP_SIZE:
        raise CityMapFormatError(f"城市 {city_index} 的地圖讀不滿 {CITY_MAP_SIZE} bytes")
    return {
        "index": city_index,
        "cityNumber": city_index + 1,
        "rawOffset": f"0x{offset:05x}",
        "header": decode_header(data),
        "buildings": decode_buildings(data),
        "grid": decode_grid(data),
        "mapSha256": sha256(data),
    }


def load_city_names(world_data_path: Path | None) -> list[str]:
    if world_data_path is None or not world_data_path.exists():
        return []
    document = json.loads(world_data_path.read_text(encoding="utf-8"))
    records = document["tables"]["cities"]["records"]
    return [str(record["name"]) for record in records]


def build_document(archive_path: Path, variant: str, world_data_path: Path | None) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)
    names = load_city_names(world_data_path)
    cities = []
    for index in range(CITY_COUNT):
        city = decode_city(disk, index)
        if index < len(names):
            city["cityName"] = names[index]
        cities.append(city)

    terrain_histogram: dict[str, int] = {}
    for city in cities:
        for row in city["grid"]:
            for cell in row:
                key = str(cell)
                terrain_histogram[key] = terrain_histogram.get(key, 0) + 1

    return {
        "schema": 1,
        "tool": "tools/atari_st_city_maps.py",
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
            "firstLogicalBlock": CITY_MAP_FIRST_BLOCK,
            "mapsPerBlock": CITY_MAPS_PER_BLOCK,
            "mapSize": CITY_MAP_SIZE,
            "gridRows": GRID_ROWS,
            "gridColumns": GRID_COLS,
            "gridOrientation": (
                "grid[i][j]：i 是畫面上的**行**（0–19，對到繪圖緩衝的第 10–29 欄）、"
                "j 是畫面上的**列**（0–11，對到第 6–17 列）。"
                "`XMOVEONG:0x22 draw_city_map` 讀的是 map[欄 − 10][列 − 6]。"
            ),
            "gridPacking": (
                "一列 3 個大端 word，每個 word 由最低位往上放四格（`ixp 4,4`）——"
                "一個 word 的四格依序是 byte1 低、byte1 高、byte0 低、byte0 高。"
            ),
            "gridOffset": GRID_OFFSET,
            "buildingSlots": BUILDING_SLOTS,
            "buildingOffset": BUILDING_OFFSET,
            "buildingFirstCode": BUILDING_FIRST_CODE,
        },
        "summary": {
            "cityCount": len(cities),
            "buildingsPerCity": {
                str(count): sum(1 for city in cities if len(city["buildings"]) == count)
                for count in sorted({len(city["buildings"]) for city in cities})
            },
            "terrainHistogram": {key: terrain_histogram[key] for key in sorted(terrain_histogram, key=int)},
        },
        "cities": cities,
        "evidenceNotes": [
            "城市名稱是依索引與 world-data.json 的 60 筆城市 record 對接的；"
            "兩張表都是 60 筆且原版以同一個 1–60 的編號索引，屬強推論而非逐筆證實。",
            "格子的裝箱順序與方向已由原版實跑對拍證實：Drahew 的可見畫面 36 格"
            "逐格與本表預測的地形碼相符（另 4 格被 pod 與角色 sprite 覆蓋），"
            "見 docs/atari-st/hatari-live-run.md 第四輪。",
        ],
        "unresolved": [
            "地形 0–15 每一格代表什麼地物。碼怎麼變成圖塊已經解出（見 resolvedElsewhere），"
            "沒解的是每一塊圖案在遊戲世界裡是草地還是水還是路——那要看圖塊本身。",
            "前兩個 byte 四個 4-bit 欄位裡的欄位 3：另外三個是圖塊條索引，這一個在繪圖時"
            "完全沒被讀到。",
            "前兩個 byte 的欄位 3 是這份檔唯一還沒解的東西；建築槽位已全部解出，見下。",
        ],
        "resolvedElsewhere": [
            "地形碼怎麼變成畫面上的圖塊：docs/atari-st/ship-rules.json 的 city.terrain-code、"
            "city.tile-pick、city.tile-threshold、city.tile-sets。",
            "建築槽位 0／1／2（地形碼 18／19／20）分別是電車站、補給站、交易所："
            "同檔的 city.facility-tube-station、city.facility-depot、city.facility-trading。",
            "地形碼 11–14 是四種建築的門，對到互動種類 12／13／14／23"
            "（酒館櫃檯、隨身裝備店、船艦零件店、旅館）：同檔的 city.facility-door。"
            "碼 16 是 Uniteller 提款機：city.facility-teller。",
            "建築槽位 3／4／5（地形碼 21–23）沒有在用：分派表的上界是 20，這三個碼完全在"
            "表外（city.facility-dispatch-table）；磁碟上它們也只出現在城市表尾那兩筆 "
            "dummy 填充記錄裡。",
        ],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path, help="外層 Atari ST archive（唯讀）")
    parser.add_argument("--variant", default="[a2]", help="nested archive 變體標記")
    parser.add_argument(
        "--world-data",
        type=Path,
        default=Path("docs/atari-st/world-data.json"),
        help="世界表 JSON；用來把城市名稱依索引對上，找不到就只輸出索引",
    )
    parser.add_argument("--output", type=Path, help="輸出 JSON 路徑；未指定則印到 stdout")
    args = parser.parse_args()

    document = build_document(args.archive, args.variant, args.world_data)
    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
