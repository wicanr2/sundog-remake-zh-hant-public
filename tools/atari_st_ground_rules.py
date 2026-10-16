#!/usr/bin/env python3
"""在 Atari ST SunDog 的 p-code 裡定位「城市地面」那一組規則的位置。

這是 ``tools/atari_st_ship_rules.py`` 的姊妹工具，做法完全一樣，管的是另一塊：走在城市
地面時的通行判定、街頭遭遇、身上的攜物格、以及地面圖塊的外觀。這幾項先前在 README 與
PLAN 裡被標成 remake-owned，理由是「原版沒解出來」；實際反組譯之後，四項裡有三項原版
都寫得很清楚，其中「身上帶四格」根本就是原版的數字。

**本工具不宣稱自動讀懂那些流程**——語意是人讀反組譯讀出來的，寫在
``docs/atari-st/ground-rules.md``。工具做的是另一件事：把每條規則對應的那一小段位元組
**釘在磁碟上**，要求它在整張磁碟映像裡恰好出現一次；簽章對不上或出現多次就 fail-closed。
另外，同一個 segment 的所有規則必須解析到同一個段基底，任何一條位址記錯都會讓它對不齊。
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

# 每條規則：識別碼、來源程序、段內位址（供人對照反組譯）、位元組簽章、語意摘要。
RULES: list[dict[str, object]] = [
    # ── 通行判定 ────────────────────────────────────────────────────────────
    {
        "id": "passability.colour-flags",
        "procedure": "GEMBIND:0x1b set_coll_color",
        "segmentOffset": "0x0208",
        "signature": "6184 0aab 0800 6820 0fb2 d40f 8682 3020 6120 a780 30a3 c820 ed8a eb",
        "summary": (
            "能不能走是逐像素判顏色，不是逐格。GEMBIND_G230 是 16 個旗標，一個硬體色一格；"
            "本程序把一個 16 字元字串逐字減 0x30 填進去。迴圈上界的 sldc15 就是 16 這個數字的出處。"
            "**旗標為 1 表示該顏色不能走**，0 才是可通行。"
        ),
    },
    {
        "id": "passability.city-street",
        "procedure": "XMOVEONG:0x23 init_city",
        "segmentOffset": "0x1a54",
        "signature": "8280 f008 9d72 1b",
        "summary": (
            "進城市地面時套用的通行字串是 0111110001011111（常數池 0x2ac6），"
            "也就是可通行的色號是 0、6、7、8、10，其餘十一色擋住。"
        ),
        "detail": (
            "旗標的極性由實跑釘死（Hatari，Drahew 城市地面）：黑色鋪面 100% 是色 0，"
            "地表植被是色 0（86.7%）＋色 6（10%）＋色 10（3.3%），角色在兩種地面上都走得動；"
            "字串索引 0 是 '0'，所以 '1' 只能是「擋住」——反過來讀的話色 0 不能走，角色會無處可站。"
        ),
    },
    # ── 捲動：畫面邊緣的死區與捲一格的後果 ──────────────────────────────────
    {
        "id": "scroll.margin-chain",
        "procedure": "XMOVEONG:0x14 update_position",
        "segmentOffset": "0x0aeb",
        "signature": (
            "ad38 7805 b3f1 0501 e16d 8a21 ad38 7880 22b2 f104 016d 8a15 "
            "ad37 7805 b3f1 0501 e16c 8a09 ad37 7813 b2f1 0201 6c"
        ),
        "summary": (
            "捲動由走路人在**畫面上**的位置決定，不是由格座標決定。sprite 座標各除以 8 之後："
            "x < 5 → 往左捲、x > 34 → 往右捲、y < 5 → 往上捲、y > 19 → 往下捲。"
            "以 8 像素為單位換算回像素，死區是 x ∈ [40, 279]、y ∈ [40, 159]。"
        ),
        "detail": (
            "四條判定是 if／else 串成的一條鏈，x 先於 y，所以**一次呼叫最多只捲一個方向**。"
            "走路人只在左鍵按住時走（`xmove_on_ground` 的主迴圈），所以捲動也只在那時發生。"
        ),
    },
    {
        "id": "scroll.direction-code",
        "procedure": "XMOVEONG:0x14 update_position",
        "segmentOffset": "0x0b2c",
        "signature": "2504 a224 038c a3a6 0110",
        "summary": (
            "方向碼 = (dx + 4) − dy × 3，寫進 `wmap` 讀的那個區域變數；`wmap` 以 `xjp` 從 2 起算"
            "分派到 scroll_up／scroll_left／scroll_down／scroll_right 四支巢狀程序。"
        ),
    },
    {
        "id": "scroll.origin-wraps",
        "procedure": "XMOVEONG:0x14 update_position",
        "segmentOffset": "0x0b80",
        "signature": (
            "ad3a ad3a 7880 28a2 25a2 8028 e200 bdcb 8fc4 "
            "ad39 ad39 7818 a224 a218 e200 bdcb 8fc4 ef12"
        ),
        "summary": (
            "捲動原點是**格單位**且環繞：scrollX := (scrollX + 40 + dx) mod 40、"
            "scrollY := (scrollY + 24 + dy) mod 24，然後呼叫 `redraw_map`。"
            "40 × 24 與 `check_exit` 換算格座標時用的模數是同一組。"
        ),
    },
    {
        "id": "scroll.sprite-steps-back",
        "procedure": "XMOVEONG:0x14 update_position",
        "segmentOffset": "0x0ba4",
        "signature": (
            "26e7 0226 7a80 c7ed a2ad 1cad 0fa2 248c a380 c7ed e200 bdcb 8fc4 "
            "26e7 0126 7981 3f01 eda2 8020 ad0f a225 8ca3 813f 01ed e200 bdcb 8fc4"
        ),
        "summary": (
            "捲完之後走路人在畫面上**往回退一格**："
            "y := (y + 200 − (L1c + Lf) × dy) mod 200、x := (x + 320 − (0x20 + Lf) × dx) mod 320。"
            "城市地面 L1c + Lf ＝ 0x20 + Lf ＝ 40，所以退的正好是一個格距。"
            "這就是實跑看到的「畫面往下捲、走路人在畫面上反而往上移」。"
        ),
    },
    {
        "id": "scroll.city-bounds",
        "procedure": "XMOVEONG:0x14 update_position",
        "segmentOffset": "0x0b3f",
        "signature": (
            "9b04 810b 0500 8027 cba7 6921 ed02 8d68 ad3a 7825 a26b ad39 7824 a26a"
        ),
        "summary": (
            "城市不是整張 40 × 24：捲動之後的原點要落在 x ∈ [11 − w, 21 + w]、y ∈ [6 − h, 13 + h]，"
            "其中 w 是 `G10b[5]`（城市尺寸位元組）、h ＝ (w + 1) / 2。出界就改呼叫 `exit_to_world`，"
            "畫面換成行星表面。"
        ),
    },
    {
        "id": "passability.city-extra-colours",
        "procedure": "XMOVEONG:0x23 init_city",
        "segmentOffset": "0x1ac4",
        "signature": "9b03 8230 0700 0fcb 0100 80ff cbc8 9b03 8230 0800 0fcb 0100 80ff cbc8",
        "summary": (
            "init_city 之後依 MAINLIB_G32 的 packed 欄位 (1,13) 條件把 G230[7] 與 G230[8] 改成 1，"
            "把兩種灰（色 7 亮灰、色 8 暗灰）從可通行改成擋住。"
        ),
        "detail": (
            "這兩色正是建築圖塊的外框與牆面：實跑量到 Drahew 交易所那一格 32×32 裡有 331 個色 7、"
            "153 個色 8。關掉它們之後，建築格只剩下少數色 0 的缺口可以進去——那就是門。"
        ),
    },
    # ── 街頭遭遇（搶匪）─────────────────────────────────────────────────────
    {
        "id": "encounter.trigger-roll",
        "procedure": "XMOVEONG:0x01 xmove_on_ground",
        "segmentOffset": "0x289b",
        "signature": "730b 81e8 03e2 00bd cb8f 15b0 0073 0b81 e803 e200 bdcb 8f80 39b0 9a04 17a1 a0a1 d402 9033",
        "summary": (
            "在城市地面（MAINLIB_G32 欄位 (4,8) == 3 且欄位 (1,13)）時，每個移動迴圈迭代擲一次："
            "random() mod 1000 == 21，或 random() mod 1000 == 57 且 MAINLIB_G17。命中才叫 little_window。"
        ),
    },
    {
        "id": "encounter.type-weights",
        "procedure": "XMOVEONG:0x35 little_window:set_itype:which",
        "segmentOffset": "0x247a",
        "signature": (
            "0073 0b80 64e2 00bd cb8f 6823 9a04 3302 8da2 6b22 23a2 6a21 22a2 6920 23b3 f106"
            "10a6 0204 8a1a 2022 b3f1 0612 a602 048a 0f20 21b3 f106 16a6 0204 8a04 00a6 0204"
        ),
        "summary": (
            "r = random() mod 100；a += MAINLIB_G33 div 2；b += a；c += b。"
            "r < a → 型別 16，r < b → 18，r < c → 22，否則 0。"
            "MAINLIB_G33 是現金的高位字，而金額一律走 set_money 的 1000 為底拆法（world-data.json 的 money.base-1000），"
            "所以它是現金的千位，帶著一萬 Cr 走路等於替型別 16 加上 5 的權重。"
        ),
    },
    {
        "id": "encounter.type-confirm",
        "procedure": "XMOVEONG:0x36 little_window:set_itype:confirm",
        "segmentOffset": "0x24c0",
        "signature": "ae04 21b0 0073 0b14 e200 bdcb 8f20 b39f a1d4 0400 a602 04",
        "summary": "選出的型別要再過一關：型別相符且 random() mod 20 < 門檻就取消。三個門檻分別給型別 16、18、22。",
    },
    {
        "id": "encounter.g17-forces-thugs",
        "procedure": "XMOVEONG:0x34 little_window:set_itype",
        "segmentOffset": "0x25e0",
        "signature": "9a04 17ad 0400 b1a1 0073 0b80 63e2 00bd cb8f 07b1 a1d4 0410 a601 04",
        "summary": "MAINLIB_G17 開著時，只要型別非 0 且 random() mod 99 != 7，一律改成型別 16。",
    },
    {
        "id": "encounter.exit-to-combat",
        "procedure": "XMOVEONG:0x33 little_window",
        "segmentOffset": "0x2795",
        "signature": "2c23 00b1 a1d4 189b 0432 0408 05ca 9b04 3204 0403 ca23 d904 80db ad36 80ff c4",
        "summary": (
            "沒能脫身且型別非 0 → MAINLIB_G32 欄位 (4,8) = 5、欄位 (4,4) = 3、MAINLIB_Gdb = 型別、"
            "回傳碼 = 0xff。SUNDOG:0x04 do_ground 收到 0xff 就轉進 GRNDLIB:0x02 do_combat。"
        ),
    },
    {
        "id": "encounter.combat-setup",
        "procedure": "XDOCOMBA:0x0a initialize",
        "segmentOffset": "0x03cb",
        "signature": (
            "9a04 80db 10d2 149b 0453 8032 d810 0100 ca00 0206 730c a601 1a8a 1b9a 0480 db0f"
            "d310 9b04 5380 32d8 1001 01ca 02a6 011a 8a04 02a6 011a"
        ),
        "summary": (
            "MAINLIB_Gdb == 16 → 對手無槍、人數 rand(2,6)；== 15 → 有槍、人數 2；其餘 → 人數 2。"
            "整條搶匪路徑沒有任何一處寫 MAINLIB_G33／G34，也就是不直接扣現金。"
        ),
    },
    # ── 違停與竊盜（真正會自動扣錢的那個事件）───────────────────────────────
    {
        "id": "parking.tow-roll",
        "procedure": "XMOVEONG:0x19 re_enter_pod",
        "segmentOffset": "0x0f4e",
        "signature": (
            "0004 10ad 09a3 148c 730c 20b3 9f00 730b 05e2 00bd cb8f 02b0 9a04 17a1 a0a1 d410"
            "9b04 5380 4300 807f cbd8 1001 01ca 8a6a"
        ),
        "summary": (
            "把載具停在街道格上、離開城市再回來時：rand(0, (16 − L9) × 20) < 停放分鐘數，"
            "或 random() mod 5 == 2 且 MAINLIB_G17 → 拖吊／攔查，顯示四段台詞其中一段。"
        ),
    },
    {
        "id": "parking.theft-roll",
        "procedure": "XMOVEONG:0x19 re_enter_pod",
        "segmentOffset": "0x0f7e",
        "signature": (
            "0004 ad09 8078 8c73 0c20 b3f1 5d00 730b 1ee2 00bd cb8f 10ad 09a3 b39f 9a04 17a0"
            "d41d 80d8 6a22 80e1 b2d4 129b 045b 2200 80e1 cb00 0080 ffcb c822 ed8a e78a 2b00"
            "730b 0ae2 00bd cb8f 6a9b 045b 80d8 0073 0b0a e200 bdcb 8fa2 0080 e1cb 0000 80ff"
            "cbc8 22ee 6a22 00b3 f1df"
        ),
        "summary": (
            "沒被拖吊就擲竊盜：rand(0, L9 × 0x78) < 停放分鐘數才成立。成立後若 random() mod 30 < 16 − L9"
            "或 MAINLIB_G17 → MAINLIB_G5b[0xd8..0xe1] 十格全清 0；否則清 (random() mod 10) + 1 格，位置每次重擲。"
        ),
    },
    # ── 攜物格 ──────────────────────────────────────────────────────────────
    {
        "id": "pockets.set-membership",
        "procedure": "XDOINTER:0x28 move_item:item_check",
        "segmentOffset": "0x1396",
        "signature": "ad02 9a05 03a3 ee68 200f 01da d407 9b04 3f20 ad03 c8",
        "summary": (
            "身上的物品放在 MAINLIB_G3f 這個 byte 陣列。sldc15; sldc1; inn 是集合常數 0x000f（長度 1 word），"
            "即 {0,1,2,3}；通過才寫入。四格是原版的數字。"
        ),
    },
    {
        "id": "pockets.range-check",
        "procedure": "WINDOWLI:0x04 swap_items",
        "segmentOffset": "0x00d5",
        "signature": "0020 b220 03b2 a1d4 069b 043f 2029 c8",
        "summary": "另一個彼此獨立的寫入點：0 <= i AND i <= 3 才寫 MAINLIB_G3f[i]。兩處一致，上界 3 不是巧合。",
    },
    # ── 圖塊外觀 ────────────────────────────────────────────────────────────
    {
        "id": "tile.look-roll",
        "procedure": "XMOVEONG:0x22 draw_city_map",
        "segmentOffset": "0x188f",
        "signature": (
            "0073 0bd4 0424 10a2 6c8a 2d00 730b 04e2 00bd cb8f 8a10 056c 8a0f 0b6c 8a0b 156c"
            "8a07 1b6c 8a03 d680 da8a 0d80 206c 8a08 8021 6c8a 03d6 80e0"
        ),
        "summary": (
            "地形碼走 0x02..0x0f 的跳表：碼 5 固定畫 0x21、碼 15 固定畫 0x20、碼 11 從 {5,11,21,27} 擲"
            "random() mod 4 選一、其餘的碼擲 random() 取最低位元，為真就 +16 換另一種樣子。"
            "原版是每次重畫都擲，同一格前後兩次可以不一樣。"
        ),
    },
    {
        "id": "tile.blit-pitch",
        "procedure": "XMOVEONG:0x0b wmap",
        "segmentOffset": "0x07db",
        "signature": (
            "8717 8020 8ca4 1687 1b01 e1d2 0500 a40d 8a11 871a ad0f a2ed 871b 8020 ad0f a28c a2a4 0d"
        ),
        "summary": (
            "圖塊在畫面上的幾何：來源 x = 條內編號 × 32，落點 x = 邊距 + 欄 × (32 + Lf)——"
            "圖塊本身 32×32，欄距是 32 加上格間隙 Lf（實跑量得 8，畫面起點 Lf/2 = 4）。"
            "間隙露出的是平鋪的地表背景圖樣（影像記錄 0x1b166 取左上 40×40），"
            "地面因此看起來連續。實跑對拍：背景層在純地表區逐像素 100% 吻合，"
            "建築圖塊格逐像素全等（hatari-live-run.md 第三輪）。"
        ),
    },
    {
        "id": "tile.strip-partition",
        "procedure": "XMOVEONG:0x0b wmap",
        "segmentOffset": "0x0793",
        "signature": (
            "8718 8020 b3d4 1087 1880 20a3 a417 8428 8422 c500 028a 3187 1824 b3d4 0f87 1824"
            "a3a4 1784 2884 24c5 0002"
        ),
        "summary": (
            "圖塊編號拆到三條圖形帶：編號 ≥ 0x20 用第一條（減 0x20 當條內編號）、"
            "≥ 條界（城市 18）用第二條、其餘用第三條。三條帶的指標在 GEMBIND 全域"
            "（城市 G5a/G5c/G5e、世界圖 G60/G62/G64），draw_city_map 填的 24×40 編號"
            "緩衝就吃這個分段。"
        ),
    },
    {
        "id": "tile.blit-call",
        "procedure": "XMOVEONG:0x0b wmap",
        "segmentOffset": "0x08af",
        "signature": "8428 9b03 5203 8711 2f2e 2d2c 2b2a 2972 1c",
        "summary": (
            "實際落筆是 GEMBIND:0x1c copy_opaque(來源帶, 螢幕 G52, vrmode 3, 來源與目的矩形)"
            "——不透明矩形搬運，沒有透明色，蓋住該格背景的 32×32 區域。"
        ),
    },
    {
        "id": "tile.map-addressing",
        "procedure": "XMOVEONG:0x22 draw_city_map",
        "segmentOffset": "0x1874",
        "signature": "8781 0ae7 0426 0a1d cb0a a3d7 0325 0611 cb06 a3d8 0404 c96c",
        "summary": (
            "城市地圖的定址：`map[欄 − 10][列 − 6]`，第一維步距 3 words、第二維以 "
            "`ixp 4,4` 取 4-bit 格子——**一個 word 裝四格，欄位 0 在 word 的最低位**。"
            "68000 的 word 是大端，最低位在第二個 byte，所以一個 word 的四格依序是 "
            "byte1 低、byte1 高、byte0 低、byte0 高。逐 byte 讀會把每四格對調成 "
            "(2,3,0,1)，讓地圖的列兩兩互換（0↔2、1↔3、4↔6…）。"
        ),
    },
    {
        "id": "tile.background-blit",
        "procedure": "XMOVEONG:0x0b wmap",
        "segmentOffset": "0x0886",
        "signature": "9b03 589b 0352 0300 2100 202c 2bad 0fa2 2a29 ad0f a272 1c",
        "summary": (
            "地形碼 0 的格子不畫圖塊，改從 `GEMBIND_G58` 搬一塊 (32+Lf)×(32+Lf) 的"
            "**地表背景圖樣**（城市 Lf=8 → 40×40），而且蓋滿整格含格間隙。"
            "G58 由 `XSTARTUP:0x03 init_gem` 指到影像記錄 raw `0x1b166`（48×40）的"
            "左上角；48 寬是為了容納最大的格間隙，實際只用左邊 32+Lf 欄。"
        ),
    },
    {
        "id": "tile.solid-cell",
        "procedure": "XMOVEONG:0x0b wmap",
        "segmentOffset": "0x089f",
        "signature": "8718 01e1 d20a 8720 2c2b 2a29 7338",
        "summary": (
            "地形碼 1 的格子也不畫圖塊，改用 `color_rect` 塗單色（城市塗 0＝黑，"
            "世界圖塗 2）——城市畫面上那片黑色鋪面就是它，不是背景也不是圖塊。"
        ),
    },
    {
        "id": "tile.city-gap",
        "procedure": "XMOVEONG:0x28 init_movement",
        "segmentOffset": "0x2019",
        "signature": "9029 08a6 010d 08a6 010f",
        "summary": (
            "城市地面的格間隙 `Lf = 8`：欄距 32+8 = 40、畫面起點 Lf/2 = 4。"
            "世界地圖那條路徑把 Lf 設成 0（`init_world` 前），所以行星地表是 32 "
            "間距、沒有間隙。實跑量到的正是 40 間距、起點 (4,4)。"
        ),
    },
    # ── collide：native 68000 ───────────────────────────────────────────────
    {
        "id": "passability.collide-native",
        "procedure": "GEMBIND:0x28 collide",
        "segmentOffset": "0x0636",
        "signature": (
            "205f 3c1f 5946 3a1f 5945 3e1f 2f08 2f0b 3f3c 0003 4e4e 548f 2440 43ee 0468 47fa"
            "00a8 e34f 3033 70e2 e24f 1e33 70f6 d6c0 7000 101b dc40 3006 e548 d046 eb48 d4c0"
            "3c05 600a 101b 6706 d4fc 00a0 3c05 7000 101b dc40 3206 3001 e848 e748 0241 000f"
            "b23c 0008 6504 5200 5101 0a01 0007 204a d0c0 7400 0310 6702 5242 0328 0002 6702"
            "5442 0328 0004 6702 5842 0328 0006 6702 5042 4a31 2800 660e 51cf ffaa 7000 265f"
            "3f40 0004 4e75 7001 60f4"
        ),
        "summary": (
            "170 bytes 的 68000：取螢幕基底（XBIOS 3 Physbase），沿取樣點逐點算出像素的 4 平面顏色"
            "（btst 平面 0/1/2/3，權重 1/2/4/8），拿它去查 0x468(a6) 這張旗標表（= GEMBIND_G230）。"
            "任何一點的旗標非 0 就 moveq #1 提早返回，全部走完才回 0。旗標 1 = 不可站。"
        ),
    },
    {
        "id": "passability.collide-tables",
        "procedure": "GEMBIND:0x28 collide",
        "segmentOffset": "0x06e0",
        "signature": "0000 0020 0040 0062 0082 008a 00a2 00ce 00fa 0122 0f0f 100f 030b 1515 1313",
        "summary": (
            "十組取樣圖樣：偏移表（word）在 0x06e0，筆數表（byte）在 0x06f4，點資料從 0x06fe 起。"
            "每組的長度是 1（起始 y）+ 1（第一個 dx）+ 2 × 筆數，十組全部對得上偏移表，格式自我驗證。"
        ),
    },
    # ── MAINLIB_G17 = 玩家在第 3 號星系 ────────────────────────────────────
    {
        "id": "system.g17-is-system-three",
        "procedure": "XSTARTUP:0x0a load_game",
        "segmentOffset": "0x05c0",
        "signature": "9b04 2a04 00c9 03b0 d904 17",
        "summary": (
            "MAINLIB_G17 不是難度或除錯旗標，是「玩家在第 3 號星系」。載入存檔時由位置字 "
            "MAINLIB_G2a 的系統欄位 (4,0) 與 3 比較得出。系統欄位是 1-based（別處都是 chk 1,12 後 deci 再索引），"
            "所以 3 = Enlie。"
        ),
    },
    {
        "id": "system.g17-after-jump",
        "procedure": "XSHOWMOV:0x09 do_jump",
        "segmentOffset": "0x03b5",
        "signature": "9b04 4a04 00c9 03b0 d904 17",
        "summary": "曲速跳躍結束時同樣重算一次，只是改讀船艦位置字 MAINLIB_G4a 的同一個欄位。",
    },
    # ── 星系記錄的 4-bit 性格欄位 ──────────────────────────────────────────
    {
        "id": "system.character-nibbles",
        "procedure": "XMOVEONG:0x28 init_movement",
        "segmentOffset": "0x1f8a",
        "signature": (
            "9b04 811f 9b04 2a04 00c9 010c cbee d70a 6921 e708 0300 07cb d804 04c9 a601 0921"
            "e708 0400 07cb d804 04c9 a601 0821 e708 0500 07cb d804"
        ),
        "summary": (
            "星系記錄（MAINLIB_G11f，步距 10 words）的 word +8／+9 是八個 4-bit 欄位。"
            "init_movement 取 nibble 3、4、5 當三種街頭遭遇的抑制門檻（分別對應型別 16、18、22）。"
            "nibble 3 與 4 是 world-data.json 已命名的 encounterRating 與 priceLevel；"
            "這一條證明它們在地面還有第二個消費端。"
        ),
    },
    {
        "id": "system.character-nibbles-prices",
        "procedure": "XDOINTER:0x24 initialize()",
        "segmentOffset": "0x11e3",
        "signature": (
            "9b04 811f 9b04 2a04 00c9 eed7 0a6c 1024 e708 03d8 0404 c9a3 6924 e708 04d8 0404"
            "c9a6 011a 24e7 0805 d804 04c9 6824 e708 07d8 0404 c9a6 0119"
        ),
        "summary": (
            "對話系統讀同一筆記錄的同一組 nibble：nibble 4 當價格倍率（prices.md 的物價等級），"
            "另取 nibble 5 與 7（nibble 7 在十二筆裡全是 0）。此處算的 16 − nibble 3 是死碼——"
            "start_interaction 一進來就把同一個 local 覆寫成價格公式的定值 30。"
        ),
    },
    {
        "id": "parking.message-from-law-level",
        "procedure": "XMOVEONG:0x19 re_enter_pod",
        "segmentOffset": "0x101b",
        "signature": "9b04 811f 9b04 2a04 00c9 010c cbee d70a e708 0300 07cb d804 04c9 048d 8a4e",
        "summary": (
            "四段違停台詞不是隨機，是 nibble 3（治安等級）div 4。0–3 幫派搶錢、4–7 善意警告、"
            "8–11 扣押載具、12–15 開罰單——台詞隨治安等級遞增，這條反過來釘住了 nibble 的排列順序。"
        ),
    },
    # ── 遭遇型別是對話種類，不是對手種類 ────────────────────────────────────
    {
        "id": "encounter.dialogue-first",
        "procedure": "XDOCOMBA:0x01 xdo_combat",
        "segmentOffset": "0x0acf",
        "signature": "9a04 17e5 9a04 80db 10b1 a000 730b 0a8f 04b0 a0d4 0290 0f",
        "summary": (
            "街頭遭遇預設先開一段對話：只有「人在第 3 號星系且型別為 16」才直接開打，"
            "而且還有 random() mod 10 == 4 的十分之一機率照樣先講話。"
        ),
    },
    {
        "id": "encounter.window-is-interaction",
        "procedure": "XDOCOMBA:0x0f bring_up_window",
        "segmentOffset": "0x0783",
        "signature": "9b04 3204 0809 caad 5a9a 0480 dbc4 ad5a 780f b0a6 0105 0072 2aad 5c78 ad5b 78ad 5a78 7603",
        "summary": (
            "把 MAINLIB_G32 欄位 (4,8) 設成 9 後呼叫 DONESOFA:0x03 do_interaction，第三個參數就是"
            "MAINLIB_Gdb。所以 16／18／22 是**互動種類**，與建築物的門用的是同一套編號。"
        ),
    },
    {
        "id": "encounter.fight-over-is-kind-15",
        "procedure": "XDOCOMBA:0x0e fire_gun",
        "segmentOffset": "0x073f",
        "signature": "ad1a 01b0 a601 02ad 02d4 050f d904 80db",
        "summary": "最後一名對手倒下時把 MAINLIB_Gdb 改成 15，那既是戰鬥迴圈的結束條件，也是善後那段對話的種類。",
    },
    {
        "id": "encounter.kind-18-is-begging",
        "procedure": "XDOINTER:0x26 initialize:start_interaction",
        "segmentOffset": "0x1120",
        "signature": (
            "9b04 810b 01a7 01b2 9f00 8065 f006 a19b 0453 807d d810 01c9 e5a1 d416 8110 2701"
            "f002 9b04 5380 7dd8 1001 01ca 80e0 f01f 8a0d 0005 802d 730c 01f0 0280 31f0 1f"
        ),
        "summary": (
            "種類 18 是街頭乞討：set_up_price(rand(5,45)) 是他開口要的金額，走 frame 0x31；"
            "整場遊戲只有一次，若 MAINLIB_G10b[1] > 1、passed(0,101) 通過且一次性旗標 G53 bit 0x7d 未設，"
            "改成 set_up_price(10000) 走 frame 0xe0，並把旗標設起來。"
        ),
    },
    {
        "id": "encounter.kind-15-amount",
        "procedure": "XDOINTER:0x26 initialize:start_interaction",
        "segmentOffset": "0x10e4",
        "signature": (
            "ae1a 803c 0000 730b 8100 0c8f 7309 a38c a602 20ae 209b 0481 0b0b a7e2 8c8c a602"
            "20ae 2001 f002 00a6 0221 8033 f01f"
        ),
        "summary": (
            "打完之後那段的金額：價格倍率 × (60 − isqrt(random() mod 3072)) × MAINLIB_G10b[11]²，"
            "而 G10b[11] 是 init_people 寫下的「對手數 − 1」。金額隨對手數平方成長。"
        ),
    },
    # ── 星系記錄 nibble 7：執行時的熱度 ─────────────────────────────────────
    {
        "id": "system.heat-raised-by-firing",
        "procedure": "XDOFIGHT:0x06 fire_it",
        "segmentOffset": "0x02f4",
        "signature": "27e7 0807 d804 04c9 ed69 0161 0f73 0a",
        "summary": (
            "星系記錄 word +9 的 nibble 7 不是靜態資料，是執行時的「熱度」："
            "太空戰鬥每開一次火就 +1，再由 MAINLIB:0x0a condition 夾在 15 以下。"
            "十二筆在磁碟上全是 0，因為那是新遊戲的初值。"
        ),
    },
    {
        "id": "system.heat-cleared-by-jettison",
        "procedure": "XDOFIGHT:0x17 do_jettison",
        "segmentOffset": "0x0e0a",
        "signature": "d70a e708 07d8 0404 00ca",
        "summary": "拋棄貨物把該星系的熱度直接寫回 0——花錢消災的機制。",
    },
    {
        "id": "system.heat-raises-encounters",
        "procedure": "SHIPLIB:0x1f update_movement",
        "segmentOffset": "0x0b4d",
        "signature": "28e7 0807 d804 04c9 0073 0b0a 8fb2 d4",
        "summary": "熱度 <= random() mod 10 才放過這一刻；值越高越常遇襲。",
    },
    # ── 口袋位元組的高位是剩餘次數 ──────────────────────────────────────────
    {
        "id": "pockets.charges-in-high-bits",
        "procedure": "XDOCOMBA:0x05 check_shield",
        "segmentOffset": "0x00be",
        "signature": (
            "9b04 3fad 0ba7 8020 8f0b b1ad 0b03 b39f a1d4 08ad 0bed a601 0b8a e59b 043f"
            "ad0b a780 208f 0bb0 a601 04ad 04d4 0e9b 043f ad0b a780 208d a601 0a8a 04"
        ),
        "summary": (
            "MAINLIB_G3f 的每個位元組：**mod 32 是物品編號、div 32 是剩餘次數**。"
            "check_shield 掃四格找 mod 32 == 11（shield），拿 div 32 當剩餘次數。"
            "GEMBIND:0x20 draw_icon 也是先 AND 31 才畫圖示，兩邊一致。"
        ),
    },
    {
        "id": "pockets.shield-starts-with-seven",
        "procedure": "XSTARTUP:0x30 utility:set_new_game:new_game:set_up_lockers",
        "segmentOffset": "0x1b58",
        "signature": "9b04 5b80 ce0b 80e0 a2c8 9b04 5b80 cf0c c8",
        "summary": (
            "新遊戲把護盾寫成 11 + 0xe0，也就是編號 11、次數 7——"
            "遊戲裡的情報櫃檯正好說「absorbs up to seven direct weapon hits!」。"
            "同一段接著寫 12（stinger），那一格沒有高位，代表不計次。"
        ),
    },
    # ── 亂數 ────────────────────────────────────────────────────────────────
    {
        "id": "rng.random",
        "procedure": "MAINLIB:0x0b random",
        "segmentOffset": "0x0614",
        "signature": (
            "8681 0101 d701 8681 0101 d701 7886 8101 00d7 0178 a20b 8cee c486 8101 00d7"
            "0181 b97a 8681 0100 d701 788c 0b86 8101 01d7 0178 8ca2 c486 8101 00d7 0178"
            "048d e068"
        ),
        "summary": (
            "兩個字的 LCG，狀態在 MAINLIB_G101／G102（存檔裡也有這兩個字）："
            "s1 = (s1 + s0) × 11 − 1；s0 = 0x7ab9 × s0 + 11 × s1；回傳 |s0 div 4|。"
            "全部 16 位元環繞。"
        ),
    },
    {
        "id": "rng.rand",
        "procedure": "MAINLIB:0x0c rand",
        "segmentOffset": "0x065a",
        "signature": "2100 910b 2021 a3ed 8fa2 6a",
        "summary": "rand(low, high) = low + random() mod (high − low + 1)。兩端都含。",
    },
    {
        "id": "rng.seed-new-game",
        "procedure": "XSTARTUP:0x30 utility:set_new_game:new_game",
        "segmentOffset": "0x1b93",
        "signature": "7015 9b04 8101 01d7 019b 0481 0100 d701 7014 9b04 8101 01d7 019b 0481 0101 d701",
        "summary": (
            "LCG 的種子來自 p-system 的系統時鐘：`KERNEL:0x14 time(hiword, loword)` 是 VAR 參數，"
            "這裡直接把 MAINLIB_G101［1］（即 G102）與 G101［0］兩個位址餵給它，"
            "所以時鐘一次寫滿 LCG 的兩個狀態字。之後每個字再各自加上一個本地兩元素陣列的對應值，"
            "混入第二份熵。`KERNEL` 不在 SYSTEM.STARTUP 裡，它是 SYSTEM.PASCAL 的執行期庫段"
            "（見 docs/atari-st/pcode-runtime-support.md）。"
        ),
    },
    {
        "id": "rng.seed-load-fresh",
        "procedure": "XSTARTUP:0x0a load_game",
        "segmentOffset": "0x0567",
        "signature": "9b04 8101 01d7 019b 0481 0100 d701 7014 8a66 8801",
        "summary": (
            "load_game 有兩條互斥分支。走這一條時（前面 `L6 OR L4` 為真）會清掉三個計數器、"
            "把 G53 的 packed 欄位 0x7a 設成 1，再以同一個 time(&G102, &G101) 重新播種；"
            "另一條分支則用 GEMBIND:0x04 moveram 把 0xc00 bytes 的存檔區塊搬回來，"
            "連同存在裡面的 G101／G102 一起還原。所以「讀檔會不會重播種」取決於走哪一條。"
        ),
    },
    # ── 順帶解出的現金欄位 ──────────────────────────────────────────────────
    {
        "id": "cash.new-game",
        "procedure": "XSTARTUP:0x30 utility:set_new_game:new_game",
        "segmentOffset": "0x1c37",
        "signature": "9b04 3300 0073 0b81 bc02 8f80 c8a2 730d",
        "summary": (
            "新遊戲的現金是 random() mod 700 + 200，即 200..899 Cr。"
            "set_money(&G33, 0, value) 的第二個引數恆為 0、第三個是金額，因此 G33 是高位字、G34 是低位字；"
            "set_money 以 1000 為底拆數，所以現金 = G33 × 1000 + G34。"
        ),
    },
    # ── 順帶解出的結算分數 ──────────────────────────────────────────────────
    {
        "id": "score.formula",
        "procedure": "MAINLIB:0x11 game_score",
        "segmentOffset": "0x076e",
        "signature": "8542 ee0a 8c68 2000 b3f1 0200 6885 1380 36a3 078d 038c 6921 00b3 f102 0069 8541 20a3 21a3 6a",
        "summary": (
            "結算分數 = G41 −（G42 − 1）× 10 −（(G13 − 54) / 7）× 3，兩個罰分各自下限為 0。"
            "G41 是送到 Banville 的貨物累計，**G42 是死亡次數**"
            "（XSTARTUP:0x0c change_deaths 把它加 1 存檔再減回來），G13 是經過時間。"
        ),
    },
    {
        "id": "score.no-death-bonus-is-cosmetic",
        "procedure": "XSTARTUP:0x0d show_score",
        "segmentOffset": "0x06f1",
        "signature": "9a04 42 00 d209 8280 3908 9d73 1b8a 11 8280 4408 9d73 1b9a 0442 ee0a e18c 0590 0e",
        "summary": (
            "死亡 0 次印 `No-death bonus:    25`，否則印 `Restart penalty:` 加上 (G42 − 1) × (−10)。"
            "**那 25 分從來沒有進總分**：G41 全域只有兩個寫入端，都在 XDOTRADI:0x1b do_selling，"
            "都不加 25；game_score 也不加。獎勵只存在於那一行字串裡。"
        ),
    },
    {
        "id": "score.banville-gate",
        "procedure": "XDOTRADI:0x07 get_data",
        "segmentOffset": "0x0246",
        "signature": "9b04 2a06 09c9 803c b0a6 010d",
        "summary": (
            "只有在 Banville 賣貨才算分：閘門是「目前城市編號（MAINLIB_G2a 的 bits 9–14，1-based）"
            "等於 0x3c」，也就是城市表第 60 筆——world-data.json 裡的 Banville。"
        ),
    },
    {
        "id": "condition.decay",
        "procedure": "MAINLIB:0x52 the_time:update_condition",
        "segmentOffset": "0x0b66",
        "signature": "863d 25a7 6825 01b1 8653 803b d810 01c9 e5a0 d40c 2061 2080 408d a780 30a3 a368",
        "summary": (
            "角色狀態的衰減。四項住在 `MAINLIB_G3d` 的四個位元組（0–255），"
            "`the_time` 每次推進時更新。衰減量不是固定值，是查一張四段表："
            "`目前值 -= 表[目前值 / 0x40] - '0'`——表的四格對應值落在 0–63／64–127／"
            "128–191／192–255 哪一段，所以越滿掉得越快或越慢由該項自己決定。"
            "索引 1 有例外：`G53` 的第 0x3b 個位元欄位為真（睡眠中）時整段跳過，"
            "那個位元由 `XDOUSERM:0x10 go_to_sleep` 設起來。"
        ),
    },
    {
        "id": "condition.decay-tables",
        "procedure": "MAINLIB:0x35 the_time（常數池）",
        "segmentOffset": "0x13f8",
        "signature": "3432 3334 3130 3030 3131 3233",
        "summary": (
            "三張四段衰減表，各四個 ASCII 數字，由 `the_time` 依序餵給 "
            "`update_condition(1, …)`／`(2, …)`／`(3, …)`："
            "索引 1 是 `4234`、索引 2 是 `1000`、索引 3 是 `1123`。"
            "索引 2 那張只有最低一段會掉，而且值超過 0x80 時反而回升——"
            "對照手冊 p6 的四項讀數，這是 Health 自癒的形狀；索引 1 睡覺才回，是 Rest；"
            "索引 3 越滿掉越快，是 Nourish。"
        ),
    },
    {
        "id": "condition.vigor-collapse",
        "procedure": "MAINLIB:0x35 the_time",
        "segmentOffset": "0x0c8a",
        "signature": (
            "2300 b2d4 1386 3d00 00c8 8632 0408 0cca 01a5 0300 a518 8a0e 2380 ffb2 f103 80ff 6b86 3d00 23c8"
        ),
        "summary": (
            "第四項（`G3d[0]`）不走 `update_condition`，`the_time` 自己算，而且它歸零有後果："
            "把 `G3d[0]` 設 0、`MAINLIB_G32` 的欄位 (4,8) 設成 12、`G3` 設 1、`G18` 清 0。"
            "沒歸零就夾在 0xff 以內寫回。手冊 p10 的「長時間不睡可能因疲勞而倒下」"
            "指的就是這條——四項裡只有這一項歸零會中止玩家正在做的事，所以它是 Vigor。"
        ),
    },
    {
        "id": "score.cargo-accumulation",
        "procedure": "XDOTRADI:0x1b do_selling",
        "segmentOffset": "0x11ba",
        "signature": "6405 00c9 1fd2 099a 0441 edd9 0441 8a0b 9a04 4164 0305 c9a2 d904 41",
        "summary": (
            "貨物編號 31 只加 1 分，其餘貨物加上該筆的 5-bit 數量欄位。"
            "整段由上一條的 Banville 閘門擋著，在別的城市賣貨不計分。"
        ),
    },
]


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return disk_name, disk_bytes


def signature_bytes(signature: str) -> bytes:
    return bytes.fromhex(signature.replace(" ", ""))


def locate(disk: bytes, signature: str) -> list[int]:
    needle = signature_bytes(signature)
    if not needle:
        raise ValueError("rule signature is empty")
    found: list[int] = []
    position = -1
    while True:
        position = disk.find(needle, position + 1)
        if position < 0:
            return found
        found.append(position)


# ── 地面 sprite 圖樣 ────────────────────────────────────────────────────────
# birth_sprite／do_sprite 的 pattern 參數是**編號不是指標**（get_marks 寫進 G100 與
# 精靈表的都是 2、4、5 這類小整數）。編號對到的點陣在 GEMBIND 的 native 資料區：
# raw 0x30802 起 10 個圖樣、每樣 8 bytes（7 列 1-bit 點陣 + 1 byte 零填充）。
# 0–3 與 6–9 是不同圖樣族群，各自依方向選圖；不是加 6 輪替動畫幀。
# GEMBIND native 0xcd4／0xd2c 由 IDA 定位，Talos 各八方向執行通過；
# 舊解釋與勘誤保留於 docs/atari-st/walker-timing-spec.md。4／5 的用途沿用既有證據。
SPRITE_PATTERNS_RAW = 0x30802
SPRITE_PATTERN_COUNT = 10
SPRITE_PATTERN_STRIDE = 8
SPRITE_PATTERN_ROWS = 7


def decode_sprite_patterns(disk: bytes) -> dict[str, object]:
    span = disk[SPRITE_PATTERNS_RAW : SPRITE_PATTERNS_RAW + SPRITE_PATTERN_COUNT * SPRITE_PATTERN_STRIDE]
    patterns: list[list[int]] = []
    for index in range(SPRITE_PATTERN_COUNT):
        record = span[index * SPRITE_PATTERN_STRIDE : (index + 1) * SPRITE_PATTERN_STRIDE]
        if record[SPRITE_PATTERN_ROWS] != 0:
            raise ValueError(f"sprite pattern {index} 的填充 byte 是 {record[7]}，預期 0")
        patterns.append(list(record[:SPRITE_PATTERN_ROWS]))
    if patterns[0] != [0x00, 0x38, 0x38, 0x38, 0x38, 0x38, 0x38]:
        raise ValueError("sprite pattern 0 不是直向的人形棒——位址讀錯了")
    return {
        "rawOffset": f"0x{SPRITE_PATTERNS_RAW:05x}",
        "stride": SPRITE_PATTERN_STRIDE,
        "rows": SPRITE_PATTERN_ROWS,
        "sha256": sha256(span),
        "patterns": patterns,
        "directionMapping": {
            "evidence": "proven（GEMBIND native 0xcd4／0xd2c；IDA 9.4 定位，Talos 兩族群各八方向通過；見 walker-timing-spec.md）",
            "walker": "族群基底 0 或 6；dx==0 → 基底、dy==0 → 基底+2、斜向異號 → 基底+1、同號 → 基底+3；選圖不跨族群",
            "marks": "4 = 小點、5 = 方塊",
        },
    }


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)

    rules = []
    for rule in RULES:
        matches = locate(disk, str(rule["signature"]))
        if len(matches) != EXPECTED_MATCHES:
            raise ValueError(
                f"rule {rule['id']} signature matched {len(matches)} times, want {EXPECTED_MATCHES}"
            )
        entry = {
            "id": rule["id"],
            "procedure": rule["procedure"],
            "segmentOffset": rule["segmentOffset"],
            "rawOffset": f"0x{matches[0]:05x}",
            "signature": rule["signature"],
            "summary": rule["summary"],
        }
        if rule.get("detail"):
            entry["detail"] = rule["detail"]
        rules.append(entry)

    sprite_patterns = decode_sprite_patterns(disk)

    # 同一個 segment 內的所有規則，raw offset 減去段內位址必須是同一個值——那就是這個
    # segment 在磁碟上的起點。任何一條位址記錯都會讓它對不齊。
    bases: dict[str, int] = {}
    for rule in rules:
        segment = str(rule["procedure"]).split(":")[0]
        base = int(str(rule["rawOffset"]), 16) - int(str(rule["segmentOffset"]), 16)
        if segment in bases and bases[segment] != base:
            raise ValueError(
                f"segment {segment} resolves to base 0x{bases[segment]:05x} and 0x{base:05x}"
            )
        bases[segment] = base

    return {
        "schema": 1,
        "tool": "tools/atari_st_ground_rules.py",
        "segmentBases": {segment: f"0x{base:05x}" for segment, base in sorted(bases.items())},
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "whatThisProves": (
            "每條規則對應的位元組確實存在於這張磁碟上，且在整張映像裡唯一。"
            "規則的語意是人讀反組譯得到的，寫在 docs/atari-st/ground-rules.md；本檔只把它們釘住。"
        ),
        "evidence": "proven（位置與唯一性）",
        "semanticsEvidence": "proven（自原版 p-code 靜態讀出，未經執行對拍）",
        "rules": rules,
        "spritePatterns": sprite_patterns,
        "openQuestions": [
            {
                "topic": "system record nibble 6",
                "status": "unknown",
                "detail": (
                    "星系記錄 word +9 的 nibble 2（欄位索引 6）在整份 p-code 裡找不到任何讀寫端。"
                    "其餘七個都有：0–2 是座標、3 是遭遇設定兼治安、4 是物價、5 是型別 22 的門檻、"
                    "7 是執行時的熱度。world-data.json 記的 tail bytes 2–5 全零同樣未解。"
                ),
            },
            {
                "topic": "XDOINTER:0x06 passed",
                "status": "strong inference",
                "detail": (
                    "種類 18 那條一次性分支的閘門之一。passed(x) 把 x 拆成「百位選一項檢定、"
                    "餘數當門檻」（x >= 200 → 減 200、換 20；x >= 100 → 減 100、換 19；依此類推），"
                    "但每一項檢定實際比的是哪個屬性尚未逐條命名。另一個閘門 MAINLIB_G10b[1] > 1 也未解。"
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
