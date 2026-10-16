#!/usr/bin/env python3
"""釘住四個艦上畫面的繪製幾何常數（WORKLIST P7）。

戰術（XDOFIGHT）、三張導航圖（XREADMAP）、系統內航行（XMOVEONS）與駕駛台
（XPILOTAG）的資料層早已解出（世界表、敵艦表、極座標），這支工具釘的是
「畫在哪、多大」的常數：掃描儀圓心與半徑、星圖投影係數、行星圖的城市格點、
儀表線的位置算法。這些常數是 D4（與原版實跑畫面對拍）的座標基準。

做法與 ``tools/atari_st_city_subsystems.py`` 相同：語意是人讀反組譯讀出來的，
寫在 ``docs/atari-st/screen-geometry.md``；工具把每條規則對應的位元組釘在磁碟
上，要求全碟唯一且落點等於段基底加段內位址。對不上就 fail-closed。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


EXPECTED_MATCHES = 1
MINIMUM_SIGNATURE_BYTES = 10

PSYSTEM_STARTUP_RAW = 0x2E000

SEGMENT_BASES = {
    "SHIPLIB": 0x3E00,
    "XPILOTAG": 0x7200,
    "XREADMAP": 0x7A00,
    "XMOVEONS": 0x9E00,
    "XDOFIGHT": 0xA600,
    "MAINLIB": 0x800,
    "WINDOWLI": 0x3200,
    "XDOUSERM": 0x6800,
    "XDOCOMBA": 0x11400,
    "XDOUNITE": 0x14E00,
    "XDOTRADI": 0x16200,
    "XMOVEINB": 0x17E00,
}

RULES: list[dict[str, object]] = [
    # ── 戰術畫面（XDOFIGHT） ────────────────────────────────────────────────
    {
        "id": "tactical.scanner-arcs",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x08 init_fight",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x049b",
        "signature": (
            "01 72 18 00 81 10 0e 80 25 80 a4 1f 1f 72 15 00 81 10 0e 80 66 80 a4 1f 1f 72 15"
        ),
        "summary": (
            "兩個圓形掃描儀：`ell_arc(0, 3600, x, y=164, 31, 31)`，圓心 x 分別是 "
            "37（方位儀）與 102（俯仰儀），半徑 31，線色 1。角度單位是 0.1 度，"
            "0–3600 畫滿整圈。"
        ),
    },
    {
        "id": "tactical.scanner-vectors",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x08 init_fight",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x04b6",
        "signature": (
            "0a 72 18 80 25 80 a4 80 25 15 a3 80 a4 15 a3 72 11 80 25 80 a4 80 25 15 a2 "
            "80 a4 15 a3 72 11 0c 72 18 80 66 15 a3 80 a4 15 a3 80 66 80 a4 72 11 "
            "80 66 15 a3 80 a4 15 a2 80 66 80 a4 72 11"
        ),
        "summary": (
            "掃描儀裡的固定指標線：方位儀（色 10）從圓心 (37,164) 往左上與右上各拉 "
            "21 像素成 V 形；俯仰儀（色 12）從 (102−21, 164∓21) 收斂到圓心成 < 形。"
        ),
    },
    {
        "id": "tactical.scanner-dot",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x07 plot_dot",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x03aa",
        "signature": (
            "24 1c b3 d4 06 00 a6 01 02 8a 30 25 04 8d 6d 25 00 b3 f1 08 81 00 01 25 a2 6d "
            "8a 05 25 80 ff 8f 6d 25 24 21 20 71 0c 21 23 21 78 a2 c4 20 22 20 78 a2 c4 "
            "04 03 21 78 20 78 72 21"
        ),
        "summary": (
            "掃描點：半徑取距離/4，≥28 就出圈不畫（並清掉鎖定旗標）；角度除 4 後"
            "正規化到 0–255（負的加 0x100、正的 mod 0xff），`polar_to_rect` 轉直角"
            "座標加圓心，`draw_char(4, 3, x, y)` 畫字元 4、色 3 的點。方位儀吃 "
            "`Gd0 + 0x100`、俯仰儀吃 `−Gcf`，半徑是 `Gdb / 4`。已於原版實跑對拍：六個"
            "同幀樣本（畫面自快照 VRAM 解出）的點位與 RAM 算出的位置差 ≤1.5 px。"
        ),
    },
    {
        "id": "tactical.viewport-window",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x0a replot_screen",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x06fb",
        "signature": "ad 1a e0 80 4b b3 9f ad 19 e0 80 33 b3 9f a1 9a 04 80 db 80 6c b3 9f a1 d4 33",
        "summary": (
            "主視窗看得到敵艦的條件：|Δx| < 75 且 |Δy| < 51（以視窗中心為原點），"
            "且距離 < 108。三個條件有一個不成立就不畫敵艦字圖。"
        ),
    },
    {
        "id": "tactical.enemy-center",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x0a replot_screen",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x06c3",
        "signature": "9a 04 80 d0 02 8d 81 00 01 a3 a6 01 1a 9a 04 80 cf 02 8d 81 00 01 a3 a6 01 19",
        "summary": (
            "敵艦的畫面偏移：兩軸角度（`MAINLIB_Gd0`／`Gcf`，0–0x3ff）各除 2 再減 "
            "0x100，得到以視窗中心為原點的 Δx／Δy。角度本身每拍以 mod 0x400 更新。"
        ),
    },
    {
        "id": "tactical.enemy-erase-box",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x0a replot_screen",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x06e1",
        "signature": "01 72 16 00 72 1a ad 16 03 a3 ad 16 03 a2 ad 15 04 a3 ad 15 04 a2 72 14",
        "summary": (
            "擦掉上一拍的敵艦：以舊位置為中心 `fill_rect(x−3, x+3, y−4, y+4)` 塗回"
            "背景色 0——敵艦字圖的佔格是 7×9 像素。"
        ),
    },
    {
        "id": "tactical.status-outlines",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x03 show_status",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x0098",
        "signature": (
            "01 80 67 00 20 e7 81 58 08 73 40 8a 51 01 80 79 00 20 e7 81 8c 08 73 40 8a 44 "
            "01 80 9d 00 20 e7 81 c9 08 73 40 8a 37 01 80 65 00 20 e7 82 18 08"
        ),
        "summary": (
            "狀態視窗的敵艦輪廓：七種艦型各一段折線，`draw_poly(1, n, 0, 緩衝+偏移, 8)`。"
            "第二個參數依艦型是 0x67/0x79/0x9d/0x65/0x71/0x93/0x63——那不是 x 座標，是各段"
            "折線的位元組數減一（104−1、122−1、158−1、102−1、114−1、148−1、100−1，與 "
            "enemy-ships.json 的 outlines 逐一相等）；折線在戰鬥緩衝 word 偏移 "
            "0x158/0x18c/0x1c9/0x218/0x24b/0x284/0x2ce——正是 enemy-ships.json 的 "
            "outlineTable（raw 0x40b0），這裡是它的繪製端。實跑對拍：Voton Mk IV 的 51 點"
            "折線以狀態視窗原點 (137,133) 平移後每一段都落在畫出的像素上（0 漏）。"
        ),
    },
    {
        "id": "tactical.status-damage-label",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x03 show_status",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x0084",
        "signature": "82 80 05 08 9d 80 94 80 c0 72 12",
        "summary": "狀態視窗的 \"Damage:\" 標籤以 `wstring` 畫在絕對座標 (148, 192)。",
    },
    {
        "id": "tactical.slew-tables",
        "screen": "tactical",
        "procedure": "XDOFIGHT:0x0b slew",
        "segment": "XDOFIGHT",
        "segmentOffset": "0x0762",
        "signature": (
            "86 0e 86 0d 72 1e 3c 04 a2 a5 0d 3c 80 bf b2 f1 04 80 bf a5 0d 9b 04 87 37 "
            "3c 06 8d d7 01 78 a5 0d 9b 04 87 37 1f 3d 0a 8d a3 d7 01 78 a5 0e"
        ),
        "summary": (
            "轉向速度：滑鼠 x 加 4 夾到 ≤191 後查 `G737[x/6]`，y 查 `G737[31 − y/10]`"
            "——查的是 sine-table.json 的滑鼠斜移表（raw 0xd444），這裡是戰術畫面的"
            "讀取端。"
        ),
    },
    # ── 導航圖（XREADMAP） ──────────────────────────────────────────────────
    {
        "id": "map.sector-projection",
        "screen": "map-sector",
        "procedure": "XREADMAP:0x03 disp_map:dispsect",
        "segment": "XREADMAP",
        "segmentOffset": "0x005c",
        "signature": (
            "c9 0c 8c ae 03 a2 08 a2 6c 26 e7 08 01 d8 04 04 c9 0a 8c ae 02 a2 08 a2 6b "
            "06 26 e7 08 02 d8 04 04 c9 02 8d a3 69"
        ),
        "summary": (
            "3D 星圖的投影：星系 4-bit 座標 (X,Y,Z) 畫成 `x = X×12 + 原點 + 8`、"
            "`y = Y×10 + 原點 + 8`，圓半徑 `r = 6 − Z/2`（Z 軸只以大小表現距離）。"
        ),
    },
    {
        "id": "map.sector-star-glyphs",
        "screen": "map-sector",
        "procedure": "XREADMAP:0x03 disp_map:dispsect",
        "segment": "XREADMAP",
        "segmentOffset": "0x0091",
        "signature": "00 81 10 0e 24 23 21 21 72 15 8a 08 80 40 24 ee 23 ed 72 13",
        "summary": (
            "星系畫成滿圈 `ell_arc(0,3600,x,y,r,r)`；r ≤ 0（最遠的 Z）退化成在 "
            "(x−1, y+1) 畫字元 0x40 的一點。目前／目的星系另加方框：五點 `poly_line`"
            "走 (x±r', y±r')，r' = r+1 且至少 2；目前所在色 10（`G4a` 的星系欄位且在規劃模式），目的地（`Gdf`）"
            "以色 12↔0 閃爍。原版實跑：藍框在目前星系、框比圓大一圈（第十二輪）。"
        ),
    },
    {
        "id": "map.system-orbit-ring",
        "screen": "map-system",
        "procedure": "XREADMAP:0x05 disp_map:disp_system",
        "segment": "XREADMAP",
        "segmentOffset": "0x01ea",
        "signature": (
            "23 80 2b 66 65 71 0c ad 15 d4 0c 15 02 ae 05 26 a2 ae 04 25 a2 72 21 23 15 "
            "ae 05 26 a2 ae 03 a3 ae 04 25 a2 ae 02 a3 74 0d 23 10 a2 6b 23 81 00 01"
        ),
        "summary": (
            "星系圖的基準圈：半徑 43、角度 0 起每 16/256 圈一點共 16 點，"
            "`draw_char(21, 2, …)` 畫點並登記點擊格。行星本身按 record 的極座標"
            "（半徑＋角度）經 `polar_to_rect` 擺，字元 6 色 2。"
        ),
    },
    {
        "id": "map.system-backdrop",
        "screen": "map-system",
        "procedure": "XREADMAP:0x05 disp_map:disp_system",
        "segment": "XREADMAP",
        "segmentOffset": "0x013f",
        "signature": "00 06 73 37 9b 03 66 0c 80 60 06 73 4c 01 72 19",
        "summary": (
            "星系圖的底圖：視窗 6 先塗色 0，再 `fetch_at` 鋪影像 12（引數 0x60 是"
            "影像高度 96 行的裁切上限）。"
        ),
    },
    {
        "id": "map.planet-backdrop",
        "screen": "map-planet",
        "procedure": "XREADMAP:0x06 disp_map:disp_planet",
        "segment": "XREADMAP",
        "segmentOffset": "0x02b2",
        "signature": "00 06 73 37 9b 03 66 0b 80 60 06 73 4c",
        "summary": "行星圖的底圖同樣鋪進視窗 6，用的是影像 11。",
    },
    {
        "id": "map.planet-grid",
        "screen": "map-planet",
        "procedure": "XREADMAP:0x07 disp_map:disp_planet:change_xy",
        "segment": "XREADMAP",
        "segmentOffset": "0x0274",
        "signature": "21 07 23 ed 02 8d 8c 04 a2 c4 23 d4 06 21 21 78 03 a2 c4 20 22 04 8c 02 a2 c4",
        "summary": (
            "行星圖的格點換算：城市 record 的 (col,row) 畫成 "
            "`x = 7×((col+1)/2) + 4`（col 為奇數再 +3）、`y = 4×row + 2`——"
            "奇偶欄交錯半格，60 座城市全用同一條式子。"
        ),
    },
    {
        "id": "map.planet-orbit-ship",
        "screen": "map-planet",
        "procedure": "XREADMAP:0x06 disp_map:disp_planet",
        "segment": "XREADMAP",
        "segmentOffset": "0x030b",
        "signature": (
            "80 28 03 62 61 90 07 ad 15 d4 21 22 ae 03 a2 6a 21 ae 02 a2 69 9b 04 4a 06 09 c9 "
            "00 b0 27 a1 d4 0b 9a 04 83 8f 03 22 07 a2 21 72 21"
        ),
        "summary": (
            "軌道上的船：擺在格點 (40, 3) 換算出的位置再右移 7 像素，字圖用全域的"
            "船字元（`MAINLIB_G38f`）、色 3，只在檢視自己所在的行星且不在地面時畫。"
        ),
    },
    {
        "id": "map.planet-city-glyphs",
        "screen": "map-planet",
        "procedure": "XREADMAP:0x06 disp_map:disp_planet",
        "segment": "XREADMAP",
        "segmentOffset": "0x0396",
        "signature": "20 84 10 06 09 c9 d2 05 01 72 19 8a 03 05 72 19 14 02 ae 03 22 a2 ae 02 21 a2 72 21",
        "summary": (
            "城市點：字元 20 色 2；目前所在城市文字色 1、其他 5，船所在城另以色 8 疊"
            "船字元。降落點（選定的落點城市）把字元換成 0x3c。"
        ),
    },
    # ── 系統內航行（XMOVEONS） ──────────────────────────────────────────────
    {
        "id": "move.gauge-line",
        "screen": "move-on-ship",
        "procedure": "XMOVEONS:0x02 show_stat",
        "segment": "XMOVEONS",
        "segmentOffset": "0x0024",
        "signature": (
            "9b 04 87 73 26 ee d7 05 e7 01 69 21 7b 24 04 8d a3 68 23 72 18 21 78 20 21 79 20 72 11"
        ),
        "summary": (
            "駕駛台儀表是一條橫線：在視窗表 G773 該視窗的 `y = y1 − 值/4` 高度，"
            "從 x0 畫到 x1，線色由呼叫端給（0 就只更新數值不畫線）。"
        ),
    },
    {
        "id": "move.window-swap",
        "screen": "move-on-ship",
        "procedure": "XMOVEONS:0x03 pilot_disp",
        "segment": "XMOVEONS",
        "segmentOffset": "0x0056",
        "signature": (
            "9b 04 87 73 0a ee d7 05 9b 04 87 73 80 4e ee d7 05 c5 00 05 9b 04 87 73 0f ee d7 05 "
            "9b 04 87 73 80 4f ee d7 05 c5 00 05 9b 04 87 73 10 ee d7 05 9b 04 87 73 80 50 ee d7 05 c5 00 05"
        ),
        "summary": (
            "進系統內航行時把視窗表的備援 record 0x4e/0x4f/0x50 整筆蓋到視窗 "
            "10/15/16——window-menu-tables.json 尾端那三筆「沒有被誰引用」的 record "
            "就是這裡的駕駛台版面。"
        ),
    },
    {
        "id": "move.viewport-color",
        "screen": "move-on-ship",
        "procedure": "XMOVEONS:0x03 pilot_disp",
        "segment": "XMOVEONS",
        "segmentOffset": "0x00bc",
        "signature": "33 d4 06 0c 10 73 37 8a 04 0b 10 73 37 0b 9a 04 80 e5 9a 04 80 e5 00 0a ef 02",
        "summary": (
            "視窗 16 的底色：有敵艦接觸塗 12、平時塗 11。這一格是 window-swap 拿 "
            "window 80 的 record（CONDITION 讀數框，16–72 × 6–16）蓋出來的，所以變色的"
            "是畫面左上那個框——手冊 p8 的 COMBAT STATUS INDICATOR 指的就是它。"
            "四條儀表接著畫在視窗 11–14，其中 13 與 14 帶線色 1，11 與 12 只更新數值。"
        ),
    },
    {
        "id": "move.gauge-sources",
        "screen": "move-on-ship",
        "procedure": "XMOVEONS:0x03 pilot_disp",
        "segment": "XMOVEONS",
        "segmentOffset": "0x00c9",
        "signature": (
            "0b 9a 04 80 e5 9a 04 80 e5 00 0a ef 02 0c 9a 04 80 e4 9a 04 80 e4 00 0c ef 02 "
            "0d 9a 04 80 e3 9a 04 80 e2 01 0b ef 02 0e 9a 04 80 e1 9a 04 80 e0 01 0f ef 02"
        ),
        "summary": (
            "四根量表各讀哪個全域：`show_stat(11, Ge5, Ge5, 0, 10)`、"
            "`show_stat(12, Ge4, Ge4, 0, 12)`、`show_stat(13, Ge3, Ge2, 1, 11)`、"
            "`show_stat(14, Ge1, Ge0, 1, 15)`。視窗 11–14 是量表框裡 x 273/283/293/303 "
            "那四根，標籤由上而下拼成 FUEL／GUNS／SHLD／WARP（original-layout.json），"
            "所以 Ge5 是燃料、Ge4 是武器充能、Ge2 是護盾電力、Ge0 是躍遷充能。"
            "第四個引數是線色：11 與 12 給 0，只更新數值不畫線；13 與 14 給 1。"
            "不畫線的兩根把同一個全域傳兩次，畫線的兩根傳兩個不同的——後者是要擦掉上一條線。"
            "Ge5 另有 `planetary.fuel-gate` 佐證，Ge4 有 "
            "`combat.fire-requires-guns-and-charge`，Ge2 有 `planetary.shields-off`。"
        ),
    },
    {
        "id": "move.warp-probability",
        "screen": "move-on-ship",
        "procedure": "XMOVEONS:0x03 pilot_disp",
        "segment": "XMOVEONS",
        "segmentOffset": "0x00fd",
        "signature": (
            "9b 04 53 01 d8 10 01 c9 f1 30 9b 04 4a 05 04 c9 00 b2 f1 04 00 68 8a 0b "
            "9b 04 49 08 08 c9 07 a2 02 8c 68"
        ),
        "summary": (
            "手冊 p8 的 PROBABILITY WARP SUCCESS 怎麼算。三段：`G53` 的第 1 個位元欄位為真"
            "就整個跳過不畫；否則看 `G4a` 的欄位 (5,4)（行星），不大於 0 就顯示 0；"
            "其餘情況顯示 **(`G49` 的欄位 (8,8) + 7) × 2**。"
            "`G49` 是船的極座標——`SHIPLIB:0x1f update_movement` 把它的 (8,0) 與 (8,8) "
            "兩個欄位直接餵給 `SHIPLIB:0x0c polar_to_rect`，高位元組是半徑。"
            "所以這個機率就是**離恆星的距離**：星系圖的基準圈半徑是 43"
            "（`map.system-orbit-ring`），(43+7)×2 剛好是 100，"
            "也就是飛到系統邊緣時躍遷必定成功。畫出來的是三位數前導零，"
            "與手冊圖上的 `000` 一致。"
        ),
    },
    {
        "id": "move.warp-readout",
        "screen": "move-on-ship",
        "procedure": "XMOVEONS:0x03 pilot_disp",
        "segment": "XMOVEONS",
        "segmentOffset": "0x0123",
        "signature": "9b 04 87 73 0f ee d7 05 e7 01 6b 20 03 23 78 23 7b 01 73 1e",
        "summary": (
            "畫面右上那個獨立讀數框。整段照 IV.2.1 的 opcode 讀出來是："
            "`9b 04 87 73` 取全域 `G773`（視窗表）的位址、`0f` 推 15、`ee` 減一、"
            "`d7 05` 以每筆 5 個 word 索引——所以視窗表是 1-based，每筆 5 word，"
            "與 window-menu-tables.json 的 recordWords 一致；`e7 01` 再加一個 word "
            "取該筆的第 2 個欄位，`6b` 存進區域變數 4。接著推五個引數"
            "（`20` 區域變數 1、`03` 常數 3、`23 78` 取 [區域變數4]、"
            "`23 7b` 取 [區域變數4+3]、`01` 常數 1），最後 `73 1e` 是短跨段呼叫"
            "（`SCXG`，段號編在 opcode 裡、程序號 0x1e = 30 在下一個位元組）。"
            "視窗 15 是 window-swap 拿 window 79 的 record 蓋出來的，那筆 record "
            "是一個點（281, 15），落在 original-layout.json 量到的 `warp-readout`"
            "（273, 5, 42×13）框內。手冊 p8 把這個框標成 PROBABILITY WARP SUCCESS，"
            "手冊圖上它顯示三位數前導零（`000`），與量表框最右那根 WARP（躍遷充能）"
            "是兩回事。**值怎麼算還沒解出**：要先讀那支被呼叫的程序。"
        ),
    },
    {
        "id": "move.bay-lights-loop",
        "screen": "move-on-ship",
        "procedure": "XMOVEONS:0x03 pilot_disp",
        "segment": "XMOVEONS",
        "segmentOffset": "0x0137",
        "signature": "00 68 20 06 b2 d4 07 20 71 11 20 ed 8a f3 73 2e",
        "summary": (
            "進畫面時 `bay_lights(0..6)` 連畫七格量表條——bay_lights 以視窗 115"
            "（P1 選單列旁那格）為畫布，是儀表的填格端。"
        ),
    },
    # ── 地面上的 zoom 視窗（WINDOWLI make_zoom 與各呼叫端） ─────────────────
    {
        "id": "zoom.make-zoom-size",
        "screen": "zoom-window",
        "procedure": "WINDOWLI:0x15 make_zoom(style,gx,gy)",
        "segment": "WINDOWLI",
        "segmentOffset": "0x0939",
        "signature": "21 24 08 8c ed c4 21 e7 02 23 08 8c ed c4 25 0c b3 f1 24 21 e7 01 21 78 80 a0 a2 03 a3 c4 21 e7 03 21 7a 80 64 a2 03 a3 c4",
        "summary": (
            "視窗 3 的內容區：x0 = gx×8+1、y0 = gy×8+1；style < 12 時 x1 = x0+157、y1 = y0+97"
            "（160×100 含外框一圈），style ≥ 12 時 x1 = x0+77、y1 = y0+47（80×50）。"
            "style 0–4 再走 make_man 放角色卡的格位。"
        ),
    },
    {
        "id": "zoom.make-zoom-small",
        "screen": "zoom-window",
        "procedure": "WINDOWLI:0x15 make_zoom(style,gx,gy)",
        "segment": "WINDOWLI",
        "segmentOffset": "0x0970",
        "signature": "21 e7 01 21 78 80 4d a2 c4 21 e7 03 21 7a 80 2f a2 c4",
        "summary": "style ≥ 12 的小視窗：寬 78、高 48 的內容區（little_window 的訊息窗）。",
    },
    {
        "id": "zoom.condition-clamp",
        "screen": "zoom-window",
        "procedure": "MAINLIB:0x0a condition(lo,&v,hi)",
        "segment": "MAINLIB",
        "segmentOffset": "0x066c",
        "signature": "21 78 22 b3 f1 05 21 22 c4 8a 09 21 78 20 b2 f1 03 21 20 c4",
        "summary": "v < lo 就設成 lo，否則 v > hi 就設成 hi——各呼叫端用它把 gx／gy 夾在畫面裡。",
    },
    {
        "id": "zoom.trade-origin",
        "screen": "trade",
        "procedure": "XDOTRADI:0x22 init_trade",
        "segment": "XDOTRADI",
        "segmentOffset": "0x161b",
        "signature": "9a 04 2d 08 8d 69 9b 04 2e 08 00 c9 08 8d 68 01 61 13 73 0a 01 60 0a 73 0a 0b 21 20 74 15",
        "summary": (
            "交易所：gx = 走路人 x（G2d）÷8 夾到 1–19，gy = 走路人 y（G2e 低位元組）÷8 夾到 1–10，"
            "make_zoom(11, gx, gy)。走路人固定在畫面第 (4,2) 格時就是 (19,10) → 內容區 (153,81)。"
        ),
    },
    {
        "id": "zoom.status-origin",
        "screen": "character-status",
        "procedure": "XDOUSERM:0x08 initialize",
        "segment": "XDOUSERM",
        "segmentOffset": "0x0157",
        "signature": "9a 04 2d 08 8d a6 01 13 9b 04 2e 08 00 c9 08 8d a6 01 12 0b 88 01 13 13 73 0a 01 88 01 12 09 73 0a 00 ad 13 ad 12 74 15",
        "summary": "角色卡：同交易所的算法，gx 夾到 11–19、gy 夾到 1–9，make_zoom(0, gx, gy) → (153,73)。",
    },
    {
        "id": "zoom.marquee-origin",
        "screen": "uniteller",
        "procedure": "XDOUNITE:0x06 set_marquee",
        "segment": "XDOUNITE",
        "segmentOffset": "0x0211",
        "signature": "0b 00 08 0c 73 0c 00 03 09 73 0c 74 15",
        "summary": "補給站／Uniteller／接駁站：make_zoom(11, rand(8,12), rand(3,9))，位置每次隨機；Drahew 補給站量到 (9,3)。",
    },
    {
        "id": "zoom.interior-origin",
        "screen": "interior",
        "procedure": "XMOVEINB:0x12 get_building",
        "segment": "XMOVEINB",
        "segmentOffset": "0x0a5a",
        "signature": "05 ad 80 a6 78 ad 80 a5 78 74 15",
        "summary": (
            "建築內部：make_zoom(樣式 5／8／6／7, 門格 x÷8, 門格 y÷8)，樣式 5–9 的 gy 夾到 0–18 後再加 1；"
            "門在畫面第 (4,2) 格時是 (20,11) → (161,89)。"
        ),
    },
    {
        "id": "zoom.arena-origin",
        "screen": "arena",
        "procedure": "XDOCOMBA:0x09 init_arena",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0336",
        "signature": "03 ad 5c 12 73 0a 03 ad 5b 09 73 0a 0b ad 5c 78 ad 5b 78 74 15",
        "summary": "競技場：gx 夾到 3–18、gy 夾到 3–9，make_zoom(11, gx, gy) → (145,73)。",
    },
    # ── 駕駛台（XPILOTAG） ─────────────────────────────────────────────────
    {
        "id": "pilot.ground-line",
        "screen": "pilotage",
        "procedure": "XPILOTAG:0x06 do_move",
        "segment": "XPILOTAG",
        "segmentOffset": "0x02e1",
        "signature": "9b 04 87 73 0e ee d7 05 e7 01 a4 0f 01 72 18 2e 78 2e 7b 2e 79 2e 7b 72 11",
        "summary": (
            "移動畫面的地平線：取視窗表 G773 視窗 14 的 record，以線色 1 沿 "
            "`(x0,y1)–(x1,y1)` 畫一條底線。駕駛台其餘版面全部走 P1 的視窗／選單表"
            "（put_menu），沒有另外的幾何常數。"
        ),
    },
    # ── SHIPLIB 的 native 特效資料 ─────────────────────────────────────────
    {
        "id": "fx.explosion-palette",
        "screen": "effects",
        "procedure": "SHIPLIB:0x18 explosion()（native 68000）",
        "segment": "SHIPLIB",
        "segmentOffset": "0x1568",
        "signature": "00 00 00 08 00 04 00 0c 00 0c 00 0d 00 0d 00 01 00 00 00 00 00 00",
        "summary": (
            "爆炸的色序表：native 程式尾端的 8 個 word（0,8,4,12,12,13,13,1），"
            "逐拍取一色塗爆炸像素。explosion/stars/laser_fx/wowzo 四支都是 native "
            "68000，逐指令語意未展開，此處釘的是資料表本體。"
        ),
    },
    {
        "id": "fx.stars-pattern",
        "screen": "effects",
        "procedure": "SHIPLIB:0x16 stars()（native 68000）",
        "segment": "SHIPLIB",
        "segmentOffset": "0x13d6",
        "signature": (
            "41 fa 00 60 c3 71 00 00 4a 30 30 00 67 04 85 71 00 00 c3 71 00 02 4a 30 30 10 "
            "67 04 85 71 00 02 c3 71 00 04 4a 30 30 20 67 04 85 71 00 04 c3 71 00 06 4a 30 "
            "30 30 67 04 85 71 00 06 4e 75 22 4d 70 00 30 02 e5 48 d0 42 eb 48 d2 c0 30 01 "
            "e8 48 e7 48 02 41 00 0f 34 3c 80 00 e2 6a 32 02 46 41 4e 75 00 01 01 00 00 00 "
            "01 01 00 01 01 00 00 00 01 01 00 01 00 01"
        ),
        "summary": (
            "星空的 0/1 圖樣表：段內 0x1438 起 64 bytes，native 星空程式逐格查表決定"
            "亮暗。同一份表在 SHIPLIB 段內 0x24c6 還有一份完全相同的副本（warp 特效"
            "自帶一份），簽章因此含前導指令以保持唯一。"
        ),
    },
]


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, str, bytes]:
    archive = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive)) as outer:
        nested_name, nested = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested)) as inner:
            disk_name, disk = choose_disk(inner)
    return nested_name, disk_name, disk


def pin_rules(disk: bytes) -> list[dict[str, object]]:
    """把每條規則釘在磁碟上；簽章不唯一或落點對不上就 fail-closed。"""
    pinned: list[dict[str, object]] = []
    for rule in RULES:
        signature = bytes.fromhex(str(rule["signature"]).replace(" ", ""))
        if len(signature) < MINIMUM_SIGNATURE_BYTES:
            raise ValueError(f"{rule['id']}: 簽章只有 {len(signature)} bytes，太短不足以釘住位置")
        matches: list[int] = []
        start = disk.find(signature)
        while start != -1:
            matches.append(start)
            start = disk.find(signature, start + 1)
        if len(matches) != EXPECTED_MATCHES:
            raise ValueError(
                f"{rule['id']}: 簽章在磁碟上出現 {len(matches)} 次，預期 {EXPECTED_MATCHES} 次"
            )
        raw = matches[0]
        base = SEGMENT_BASES[str(rule["segment"])]
        expected = PSYSTEM_STARTUP_RAW + base + int(str(rule["segmentOffset"]), 16)
        if raw != expected:
            raise ValueError(
                f"{rule['id']}: 簽章落在 0x{raw:05x}，但段基底與段內位址算出 0x{expected:05x}"
            )
        entry = dict(rule)
        entry["rawOffset"] = f"0x{raw:05x}"
        entry["signatureBytes"] = len(signature)
        pinned.append(entry)
    return pinned


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    nested_name, disk_name, disk = load_disk(archive_path, variant)
    pinned = pin_rules(disk)
    screens: dict[str, int] = {}
    for rule in pinned:
        key = str(rule["screen"])
        screens[key] = screens.get(key, 0) + 1
    return {
        "schema": 1,
        "tool": "tools/atari_st_screen_geometry.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "nestedArchive": nested_name,
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "evidenceLevel": "proven（位置與唯一性）／語意為自原版 p-code 靜態讀出，未經執行對拍",
        "screens": screens,
        "rules": pinned,
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
