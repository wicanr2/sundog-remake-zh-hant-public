#!/usr/bin/env python3
"""從執行時的 p-system 記憶體算出「要站到哪一格、要寫哪些值」。

`port_oracle_building_probe.sh` 在 `check_exit` 停下時讀到一組執行時的量（捲動偏移的指標、
兩個步距分項、`&Gec[0]`），這支把那組量兌現成具體的注入計畫。

換算來自 `check_exit`（XMOVEONG:0x20，見該腳本的註解）：

    欄 = (捲動x + Gec[0].w1 / 步距x) mod 40      步距x = 32 + lex1[0xf]
    列 = (捲動y + Gec[0].w2 / 步距y) mod 24      步距y = lex1[0x1c] + lex1[0xf]

式子可逆，所以「要站到某一格」有唯一解。**先用當下的實值驗算一次**：把記憶體裡的 w1／w2
代進去，算出來的 (列, 欄) 必須等於除錯器讀到的 L1／L2。對不上就中止——那表示對式子的解讀
或某個 local 的編號錯了，這時候繼續注入只會做出自洽但錯的結論。

建築格不是給定的，是從執行時緩衝掃出來的：`check_city_map_runtime.locate_buffer` 用結構
（40×24、城市矩形外全 0、值域上界）找到那塊緩衝，再取值 >= 34 的格。那個值同時是建築的
槽位編號（`city-maps.json` 的 `layout.buildingFirstCode`）。
"""

from __future__ import annotations

import argparse
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).parent))

from check_city_map_runtime import COLUMNS, FIRST_BUILDING_CODE, ROWS, VARIANT, locate_buffer

# check_exit 寫死在碼裡的兩個常數（0x14d8 的 ldcb 0x20、0x14df/0x14f3 的模數）。
STRIDE_X_BASE = 0x20
MOD_COLUMNS, MOD_ROWS = 40, 24

# 門在格子裡的位置，取自 `check_building` 的每個分支（見 ground-rules.md 的門表）。
# 站進格子還不夠——`check_loc` 另外要求格內偏移落在門的 ±4 以內，所以目標是門，不是格中央。
# 建築槽位的碼由 34 起算，正規化（減 16）之後是 18 起算，因此只有前三個槽有門：
# `check_building` 的跳表範圍是 10–20，碼 21 以上不跳、直接落地。
DOOR_OFFSETS = {
    16: [(13, 21)],            # do_teller：Uniteller 提款機
    18: [(14, 26)],            # do_tube_station：接駁站
    19: [(15, 16), (19, 21)],  # do_depot：倉庫
    20: [(24, 21), (15, 16)],  # do_exchange：交易所
}
# 設施碼與它們的來源。18–20 是建築槽位（緩衝值 34–36），16 來自城市標頭欄位 0，
# 在緩衝裡以地形碼 15 加變體偏移的形式出現（值 32）——不是槽位，所以掃描不能只看
# 「值 >= 34」，那會漏掉 Uniteller。
FACILITY_NAMES = {
    16: "uniteller",
    18: "tube-station",
    19: "depot",
    20: "exchange",
}
# 碼 18 那條在 check_loc 裡多一個 `G17 為真就進不去` 的條件，能避開就避開。
DOOR_HAS_EXTRA_GATE = {18}

# 視埠就是整個 320×200 畫面，而 sprite 座標是視埠內像素——超出去的建築在畫面上點不到，
# 得先把地圖捲過去。放大四倍加上視窗在 Xvfb 的 y=80，就是 xdotool 要用的螢幕座標。
VIEWPORT = (320, 200)
SCALE, WINDOW_Y = 4, 80


def word(memory: bytes, addr: int) -> int:
    """p-system 記憶體裡的 word 是 big-endian 存放的。"""
    return int.from_bytes(memory[addr:addr + 2], "big")


def cell_of(memory: bytes, gec: int, scroll_x: int, scroll_y: int,
            stride_x: int, stride_y: int) -> tuple[int, int]:
    w1, w2 = word(memory, gec + 2), word(memory, gec + 4)
    return ((scroll_y + w2 // stride_y) % MOD_ROWS,
            (scroll_x + w1 // stride_x) % MOD_COLUMNS)


def buildings(memory: bytes) -> tuple[int, list[tuple[int, int, int]]]:
    """回傳緩衝位址與每個設施格：(正規化碼, 列, 欄)。"""
    bases = locate_buffer(memory)
    if len(bases) != 1:
        raise SystemExit(f"FAIL：城市繪圖緩衝的候選不是一個，而是 {len(bases)}")
    base = bases[0]
    found = []
    for row in range(ROWS):
        for column in range(COLUMNS):
            value = memory[base + row * COLUMNS + column]
            code = value - VARIANT if value > 15 else value
            if code in FACILITY_NAMES:
                found.append((code, row, column))
    return base, sorted(found)


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--memory", type=pathlib.Path, required=True)
    p.add_argument("--gec", required=True, help="&Gec[0]（除錯器的 ldl 4）")
    p.add_argument("--scroll-x-ptr", required=True, help="lex1 local 0x3a")
    p.add_argument("--scroll-y-ptr", required=True, help="lex1 local 0x39")
    p.add_argument("--stride-common", required=True, help="lex1 local 0xf")
    p.add_argument("--stride-y-extra", required=True, help="lex1 local 0x1c")
    p.add_argument("--row", required=True, help="除錯器讀到的 L1")
    p.add_argument("--column", required=True, help="除錯器讀到的 L2")
    p.add_argument("--slot", type=int, help="目標建築槽位（0–2）；預設取編號最小的")
    p.add_argument("--facility", choices=sorted(set(FACILITY_NAMES.values())),
                   help="用設施名稱指定目標，Uniteller 沒有槽位編號只能這樣指")
    p.add_argument(
        "--scroll",
        action="store_true",
        help="視埠外的建築也算：連捲動偏移一起算出來，把地圖捲到目標附近",
    )
    args = p.parse_args(argv)

    n = lambda v: int(str(v).strip(), 0)
    memory = args.memory.read_bytes()
    gec = n(args.gec)
    stride_common = n(args.stride_common)
    stride_x = STRIDE_X_BASE + stride_common
    stride_y = n(args.stride_y_extra) + stride_common
    if stride_x <= 0 or stride_y <= 0:
        raise SystemExit(f"FAIL：步距不是正數（x={stride_x} y={stride_y}）")
    scroll_x = word(memory, n(args.scroll_x_ptr))
    scroll_y = word(memory, n(args.scroll_y_ptr))

    # 正對照：用記憶體裡的實值重算一次，必須等於除錯器讀到的 L1／L2。
    got = cell_of(memory, gec, scroll_x, scroll_y, stride_x, stride_y)
    want = (n(args.row), n(args.column))
    print(f"   驗算 步距 {stride_x}×{stride_y}  捲動 ({scroll_x},{scroll_y})  "
          f"重算 {got}  除錯器 {want}")
    if got != want:
        raise SystemExit("FAIL：換算對不上除錯器讀到的格座標，不繼續注入")

    base, found = buildings(memory)
    print(f"   緩衝 {hex(base)}  設施 " +
          "、".join(f"{FACILITY_NAMES[k]}@(列{r},欄{c})" for k, r, c in found))
    if not found:
        raise SystemExit("FAIL：緩衝裡找不到設施格")
    # 逐一算門的像素座標，挑一個真的點得到的。
    reachable = []
    for code, row, column in found:
        raw = memory[base + row * COLUMNS + column]
        slot = code - 18 if code >= 18 else None
        for door_x, door_y in DOOR_OFFSETS.get(code, []):
            w1 = (column - scroll_x) * stride_x + door_x
            w2 = (row - scroll_y) * stride_y + door_y
            inside = 0 <= w1 < VIEWPORT[0] and 0 <= w2 < VIEWPORT[1]
            reachable.append({
                "slot": slot, "name": FACILITY_NAMES[code],
                "row": row, "column": column, "raw": raw, "code": code,
                "w1": w1, "w2": w2, "inside": inside,
                "extra_gate": code in DOOR_HAS_EXTRA_GATE,
            })
    for r in reachable:
        mark = "可點" if r["inside"] else "在視埠外"
        gate = "（碼 18 另有 G17 閘門）" if r["extra_gate"] else ""
        print(f"   {r['name']} (列{r['row']},欄{r['column']}) 碼 {r['raw']}→{r['code']} "
              f"門 sprite ({r['w1']},{r['w2']}) {mark}{gate}")

    def wanted(rows):
        if args.facility is not None:
            return [r for r in rows if r["name"] == args.facility]
        if args.slot is not None:
            return [r for r in rows if r["slot"] == args.slot]
        return rows

    candidates = wanted([r for r in reachable if r["inside"]])
    scroll_target = None
    if args.scroll:
        # 捲動偏移也是可寫的 word（`check_exit` 讀 lex1[0x3a]／[0x39] 指向的位置），
        # 所以視埠外的建築不必用走的過去——把視埠移過去就行。目標放在視埠中間，
        # 免得門落在邊界上被整數除法算到鄰格。
        columns_in_view = VIEWPORT[0] // stride_x
        rows_in_view = VIEWPORT[1] // stride_y
        outside = wanted([r for r in reachable if not r["inside"]])
        outside.sort(key=lambda r: (r["extra_gate"], r["code"]))
        if not candidates and outside:
            pick = outside[0]
            new_x = (pick["column"] - columns_in_view // 2) % MOD_COLUMNS
            new_y = (pick["row"] - rows_in_view // 2) % MOD_ROWS
            door_x, door_y = pick["w1"] % stride_x, pick["w2"] % stride_y
            pick = {
                **pick,
                "w1": (pick["column"] - new_x) * stride_x + door_x,
                "w2": (pick["row"] - new_y) * stride_y + door_y,
                "inside": True,
            }
            if not (0 <= pick["w1"] < VIEWPORT[0] and 0 <= pick["w2"] < VIEWPORT[1]):
                raise SystemExit(
                    f"FAIL：捲到 ({new_x},{new_y}) 之後門仍在視埠外"
                    f"（{pick['w1']},{pick['w2']}）")
            scroll_target = (new_x, new_y)
            candidates = [pick]
            print(f"   捲動 ({scroll_x},{scroll_y}) → ({new_x},{new_y})，"
                  f"門移到 sprite ({pick['w1']},{pick['w2']})")
    # 沒有額外閘門的優先——碼 18 進不進得去還要看 G17。
    candidates.sort(key=lambda r: (r["extra_gate"], r["code"]))
    if not candidates:
        raise SystemExit(
            "FAIL：視埠內沒有可點的建築門"
            + (f"（限定 {args.facility}）" if args.facility
               else f"（限定槽位 {args.slot}）" if args.slot is not None else "")
            + "。走路人得先把地圖捲到建築附近。")
    pick = candidates[0]

    print(f"   目標 {pick['name']} (列{pick['row']},欄{pick['column']}) "
          f"門 sprite ({pick['w1']},{pick['w2']})")
    print(f"SET GEC_W1_ADDR={hex(gec + 2)} GEC_W2_ADDR={hex(gec + 4)} "
          f"TARGET_W1={pick['w1']} TARGET_W2={pick['w2']} "
          f"TARGET_ROW={pick['row']} TARGET_COL={pick['column']} "
          f"TARGET_CODE={pick['code']} TARGET_SLOT={pick['slot']} "
          f"TARGET_FACILITY={pick['name']} "
          f"CLICK_X={pick['w1'] * SCALE} CLICK_Y={pick['w2'] * SCALE + WINDOW_Y}"
          + (f" SCROLL_X_ADDR={args.scroll_x_ptr} SCROLL_Y_ADDR={args.scroll_y_ptr}"
             f" TARGET_SCROLL_X={scroll_target[0]} TARGET_SCROLL_Y={scroll_target[1]}"
             if scroll_target else ""))
    return 0


if __name__ == "__main__":
    sys.exit(main())
