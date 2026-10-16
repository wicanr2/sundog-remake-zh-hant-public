#!/usr/bin/env python3
"""拿執行時的 p-system 記憶體 dump 驗證靜態解出的城市地圖。

`docs/atari-st/city-maps.json` 是從磁碟靜態讀出來的：60 座城市，每座 20×12 的地形碼加上
最多六個建築槽位。那份資料只證明「磁碟上是這樣寫的」，不證明「遊戲跑起來真的照著畫」。
這支補上後半段：把 laanwj port 在城市地面停住時 dump 的整份 p-system 記憶體讀進來，找出
`MAINLIB_G3a1` 那塊 40×24 的繪圖緩衝，逐格對回靜態地圖。

**緩衝位置是掃出來的，不是給的。** 40×24 的形狀、值域上界、以及「城市只佔中間一塊、
四周全 0」這三個條件合起來在 870,400 bytes 裡只剩一個候選，所以不必先知道 `G3a1` 的值。
掃描先於指標，是因為 `ldo MAINLIB 0x3a1` 只有在對的時機取才有意義，而掃描隨時都成立。

**地形碼到繪圖碼不是恆等。** `draw_city_map` 會重映射三個碼，並在建築那一格蓋上槽位編號，
另外隨機給某些格加上 16 當視覺變體（`check_exit` 讀出來之後會減掉）。映射寫在 `TERRAIN_TO_TILE`，
是這支的判準本體——改了它就等於改了對原版的宣稱。

證據等級：映射表是 **proven**——`draw_city_map` 的兩張跳表逐條對過碼，Drahew 的 240 格
執行時記憶體也全部對上。建築槽位落點與「城市矩形外全 0」是 **proven-by-oracle**。
還沒做的是第二座城市的實跑 dump。
"""

from __future__ import annotations

import argparse
import collections
import json
import pathlib
import sys

# 繪圖緩衝的幾何（MAINLIB_G3a1）。
ROWS, COLUMNS = 24, 40
# draw_city_map 只畫中間這塊；城市地圖的 20×12 就攤在這裡。
FIRST_ROW, FIRST_COLUMN = 6, 10
GRID_COLUMNS, GRID_ROWS = 20, 12
# 繪圖時加上的偏移；`check_exit` 讀出來之後「大於 15 就減 16」。
#
# 對正規化後 0–15 的碼，這個偏移是選用的視覺變體（同一種地物的第二張圖塊）。對正規化後
# 大於 15 的碼（碼 16 以上的門與建築）它是必然的：緩衝若直接存 17，`check_exit` 會把它
# 正規化成 1，與一般地面撞號。所以那些碼在緩衝裡一定以「碼 + 16」的形式出現。
VARIANT = 16
# 建築槽位的編號同時當圖塊碼用（city-maps.json 的 layout.buildingFirstCode）。
FIRST_BUILDING_CODE = 34

# 靜態地形碼 → 允許的繪圖碼。沒有列出的碼是恆等。
#
# 三條都對過 `draw_city_map` 的跳表（見 ground-rules.md）：碼 5 → 33、碼 15 → 32，減掉必帶
# 的 16 之後就是這裡的 17 與 16；碼 11 走 `random() mod 4`，從 5／11／21／27 挑一張，
# 正規化之後收斂成 5 或 11，所以這一條列兩個值不是判準放寬，是原版真的有兩種。
# 兩種在分派時等價——`check_building` 開頭就是 `if 碼 = 5 then L1 := 11`，而 `check_exit`
# 的閘門也特地寫成「碼 > 9，或碼 = 5」。
#
# 輸出的 observedMapping 把每個地形碼實際畫成什麼攤開，這樣列多個值也不會掩蓋掉異常。
TERRAIN_TO_TILE = {5: (17,), 11: (5, 11), 15: (16,)}


def tiles_for(terrain: int) -> tuple[int, ...]:
    return TERRAIN_TO_TILE.get(terrain, (terrain,))


def locate_buffer(memory: bytes) -> list[int]:
    """回傳所有符合 40×24 城市繪圖緩衝形狀的起點。

    判準只用結構，不用任何一座城市的內容：城市矩形外必須全 0、矩形內必須有非零格、
    值域不得超過建築碼加變體偏移的上界。
    """
    limit = FIRST_BUILDING_CODE + 6 + VARIANT
    found = []
    for base in range(0, len(memory) - ROWS * COLUMNS + 1, 2):
        block = memory[base:base + ROWS * COLUMNS]
        if max(block) > limit:
            continue
        inside = outside = 0
        for row in range(ROWS):
            for column in range(COLUMNS):
                value = block[row * COLUMNS + column]
                if not value:
                    continue
                if (FIRST_ROW <= row < FIRST_ROW + GRID_ROWS
                        and FIRST_COLUMN <= column < FIRST_COLUMN + GRID_COLUMNS):
                    inside += 1
                else:
                    outside += 1
                    break
            if outside:
                break
        if outside or inside < GRID_COLUMNS:
            continue
        found.append(base)
    return found


def compare(memory: bytes, base: int, city: dict) -> dict:
    """逐格比對，回傳分類統計與每一個對不上的格。"""
    grid = city["grid"]
    buildings = {(b["row"], b["column"]): b["code"] for b in city["buildings"]}
    counts = collections.Counter()
    observed = collections.defaultdict(collections.Counter)
    mismatches = []
    for column in range(GRID_COLUMNS):        # grid[i] 的 i 是畫面上的欄
        for row in range(GRID_ROWS):          # grid[i][j] 的 j 是畫面上的列
            terrain = grid[column][row]
            got = memory[base + (FIRST_ROW + row) * COLUMNS + FIRST_COLUMN + column]
            plain = got - VARIANT if got >= VARIANT else got
            want = buildings.get((column, row))
            if want is not None:
                kind = "building" if got == want else "bad"
            elif plain in tiles_for(terrain):
                kind = "variant" if got >= VARIANT else "exact"
            else:
                kind = "bad"
            counts[kind] += 1
            if want is None:
                observed[terrain][plain] += 1
            if kind == "bad":
                mismatches.append({
                    "column": column, "row": row, "terrain": terrain,
                    "expected": want if want is not None else list(tiles_for(terrain)),
                    "got": got,
                })
    return {
        "counts": dict(counts),
        "observedMapping": {str(k): dict(sorted(v.items())) for k, v in sorted(observed.items())},
        "mismatches": mismatches,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("dump", type=pathlib.Path,
                        help="port 的 p-system 記憶體 dump（除錯器的 `dm`）")
    parser.add_argument("--city", type=int, default=1,
                        help="停在哪一座城市的地面（city-maps.json 的 cityNumber，預設 1＝Drahew）")
    parser.add_argument("--maps", type=pathlib.Path,
                        default=pathlib.Path(__file__).parent.parent / "docs/atari-st/city-maps.json")
    parser.add_argument("--base", type=lambda v: int(v, 0),
                        help="直接指定緩衝位址，跳過掃描（例如 0x746c）")
    parser.add_argument("--print-map", action="store_true", help="把 40×24 的繪圖碼印出來")
    args = parser.parse_args(argv)

    memory = args.dump.read_bytes()
    cities = json.loads(args.maps.read_text())["cities"]
    matching = [c for c in cities if c["cityNumber"] == args.city]
    if not matching:
        print(f"FAIL：city-maps.json 裡沒有城市編號 {args.city}")
        return 1
    city = matching[0]

    if args.base is not None:
        bases = [args.base]
    else:
        bases = locate_buffer(memory)
        if not bases:
            print("FAIL：dump 裡找不到符合城市繪圖緩衝形狀的區塊——這份 dump 大概不是在城市地面取的")
            return 1

    # 掃描是逐 word 前進的，同一塊緩衝會連出一段候選；取比對結果最好的那一個。
    best = min(
        (dict(compare(memory, base, city), base=base) for base in bases),
        key=lambda r: (len(r["mismatches"]), r["base"]),
    )
    result = {
        "cityNumber": args.city,
        "cityName": city.get("cityName"),
        "bufferAddress": hex(best["base"]),
        "candidateCount": len(bases),
        "cells": GRID_COLUMNS * GRID_ROWS,
        "counts": best["counts"],
        "observedMapping": best["observedMapping"],
        "mismatches": best["mismatches"],
        "terrainToTile": {str(k): list(v) for k, v in sorted(TERRAIN_TO_TILE.items())},
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))

    if args.print_map:
        base = best["base"]
        print("\n40×24 繪圖緩衝（. 是 0）：")
        for row in range(ROWS):
            cells = memory[base + row * COLUMNS: base + (row + 1) * COLUMNS]
            print(f"{row:2d}  " + " ".join("  ." if v == 0 else f"{v:3d}" for v in cells))

    if best["mismatches"]:
        print(f"FAIL：{len(best['mismatches'])} 格對不上靜態地圖")
        return 1
    print(f"PASS：{GRID_COLUMNS * GRID_ROWS} 格全部對上；"
          f"建築 {best['counts'].get('building', 0)} 格落在靜態記載的位置，城市矩形外沒有非零格")
    return 0


if __name__ == "__main__":
    sys.exit(main())
