#!/usr/bin/env python3
"""定位原版四個「走進去才看得到」的子系統的規則位置。

這四個子系統是 `WORKLIST.md` C 區列的功能落差：吃角子老虎（`XSLOTS`）、地面戰鬥
競技場（`XDOCOMBA`）、建築物內部（`XMOVEINB`）與降落起飛動畫（`XLANDFX`）。它們的
共同點是**都在城市地面之下一層**——玩家走進一扇門或按下一個按鈕才進得去，所以先前
解城市地面時沒有跟著解。

做法與 ``tools/atari_st_ground_rules.py`` 相同：語意是人讀反組譯讀出來的，寫在
``docs/atari-st/city-subsystems.md``；工具負責把每條規則對應的那一小段位元組**釘在
磁碟上**，要求它在整張映像裡恰好出現一次，且落點要與宣告的段基底加段內位址相符。
對不上就 fail-closed。
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

# SYSTEM.STARTUP 在磁碟映像上的位置；段基底加上這個值才是 raw offset。
PSYSTEM_STARTUP_RAW = 0x2E000

SEGMENT_BASES = {
    "XSLOTS": 0x19600,
    "XDOCOMBA": 0x11400,
    "XMOVEINB": 0x17E00,
    "XLANDFX": 0x8E00,
    "WINDOWLI": 0x3200,
    "XMOVEONG": 0x12000,
}

RULES: list[dict[str, object]] = [
    # ── 吃角子老虎 ─────────────────────────────────────────────────────────
    {
        "id": "slots.luck-clamp",
        "subsystem": "slots",
        "procedure": "XSLOTS:0x07 init_slots",
        "segment": "XSLOTS",
        "segmentOffset": "0x0144",
        "signature": "9b04 3504 000e cba7 a601 0305 8801 0380 6473 0a",
        "summary": (
            "機台會讀玩家的運氣：`MAINLIB_G35[4]`（LK，畫面上印成 `??` 的那一項）"
            "先夾在 5–100，之後每一轉都用它偏移骰值。"
        ),
    },
    {
        "id": "slots.bet",
        "subsystem": "slots",
        "procedure": "XSLOTS:0x07 init_slots",
        "segment": "XSLOTS",
        "segmentOffset": "0x01ba",
        "signature": (
            "8032 8782 4de7 0804 0007 cbd8 0404 c911 8782 4de7 0803 0007 cbd8 0404 c9a3 a206 8ded 8c"
        ),
        "summary": (
            "每一轉的注碼由所在星系決定：`50 × ((物價等級 + 17 − 治安等級) / 6 + 1)`（整數除法）。"
            "兩個欄位就是星系 record 第 9 個 word 的 4-bit 欄位 4 與 3，"
            "也就是 world-data.json 的 `priceLevel` 與 `encounterRating`。"
            "**物價高、治安差的星系賭注大。**"
        ),
    },
    {
        "id": "slots.odds-record",
        "subsystem": "slots",
        "procedure": "XSLOTS:0x08 init_slots:set_odds",
        "segment": "XSLOTS",
        "segmentOffset": "0x0122",
        "signature": "8802 0725 0007 cbd7 0468 20e7 0324 c420 e702 23c4 20e7 0122 c420 21c4",
        "summary": (
            "賠率表是 8 筆、每筆 4 個 word 的陣列。`set_odds(i, a, b, c, d)` 寫成 "
            "`rec[3]=a`、`rec[2]=b`、`rec[1]=c`、`rec[0]=d`——**參數順序與存放順序是相反的**。"
        ),
    },
    {
        "id": "slots.odds-table",
        "subsystem": "slots",
        "procedure": "XSLOTS:0x07 init_slots",
        "segment": "XSLOTS",
        "segmentOffset": "0x02d4",
        "signature": (
            "0700 8064 0a80 6490 0806 8064 80c8 0a80 6490 0805 80c8 8190 0102 8032 9008 0481 "
            "9001 8158 0202 8032 9008 0381 5802 8120 0302 8032 9008 0281 2003 81b0 0401 0590 "
            "0801 81b0 0481 4006 0105 9008 0081 4006 81d0 0701 0590 08"
        ),
        "summary": (
            "八個圖案的完整賠率表，從索引 7 倒著寫到 0。`a`／`b` 是該圖案在 1–2000 這條"
            "數線上佔的區間，`c` 是兩個相同的倍數、`d` 是三個相同的倍數："
            "7:(0,100,×10,×100)、6:(100,200,×10,×100)、5:(200,400,×2,×50)、"
            "4:(400,600,×2,×50)、3:(600,800,×2,×50)、2:(800,1200,×1,×5)、"
            "1:(1200,1600,×1,×5)、0:(1600,2000,×1,×5)。"
            "**區間越窄的圖案越罕見、賠得越多**，八段加起來正好 2000。"
        ),
    },
    {
        "id": "slots.reel-roll",
        "subsystem": "slots",
        "procedure": "XSLOTS:0x0b roll_one",
        "segment": "XSLOTS",
        "segmentOffset": "0x0426",
        "signature": "0073 0b81 d007 e200 bdcb 8f00 730b ad03 e200 bdcb 8fa3 6801 6081 d007 730a 01e1",
        "summary": (
            "一個轉輪的骰值是 `random() mod 2000 − random() mod 運氣`，再夾在 1–2000。"
            "**運氣只會把骰值往下壓**，而數線的低端住的正是賠率最高的圖案 7 與 6。"
            "運氣因此是真的有用，不是裝飾。"
        ),
    },
    {
        "id": "slots.reel-band",
        "subsystem": "slots",
        "procedure": "XSLOTS:0x0b roll_one",
        "segment": "XSLOTS",
        "segmentOffset": "0x0447",
        "signature": (
            "23ed 6b88 0107 2300 07cb d704 7b20 b220 8801 0723 0007 cbd7 047a b2a1 d4e2 8801 "
            "2724 0002 cbd7 0123 c4"
        ),
        "summary": (
            "骰值落在哪一段就是哪一個圖案：從索引 0 往上掃，"
            "第一個滿足 `rec[3] <= 骰值 <= rec[2]` 的就是答案，寫進該轉輪。"
        ),
    },
    {
        "id": "slots.three-of-a-kind",
        "subsystem": "slots",
        "procedure": "XSLOTS:0x0c roll_slots",
        "segment": "XSLOTS",
        "segmentOffset": "0x04f5",
        "signature": (
            "8801 2700 0002 cbd7 0178 8801 2701 0002 cbd7 0178 b088 0127 0100 02cb d701 7888 "
            "0127 0200 02cb d701 78b0 a1d4 0603 0090 0d"
        ),
        "summary": (
            "三個轉輪全部相同就記成「相同數 3」，中獎的圖案取第一輪；"
            "否則依序比 0–1、1–2、2–0，中任一對就記成「相同數 2」。三者皆異則不賠。"
        ),
    },
    {
        "id": "slots.payout",
        "subsystem": "slots",
        "procedure": "XSLOTS:0x0a get_pay",
        "segment": "XSLOTS",
        "segmentOffset": "0x0404",
        "signature": "8801 0724 0007 cbd7 046a 2303 d207 2522 788c 698a 0525 2279 8c69 216e",
        "summary": (
            "彩金 = 注碼 × （相同數 == 3 ? `rec[0]` : `rec[1]`），"
            "`rec` 取自**中獎圖案**那一筆賠率，不是注碼那一筆。"
        ),
    },
    # ── 地面戰鬥競技場 ─────────────────────────────────────────────────────
    {
        "id": "arena.player-stats",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x08 init_people",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x01ec",
        "signature": (
            "24e7 029b 043d 02a7 9b04 3500 a78c 805a 8dc4 24e7 019b 0435 02a7 058d 03a2 c424 "
            "7a80 20b2 d407 24e7 0280 20ed c424 e704 01"
        ),
        "summary": (
            "玩家進場時的兩個數值：血量 = `健康 × ST / 90`（`MAINLIB_G3d[2] × G35[0] / 90`），"
            "速度 = `DX / 5 + 3`（`G35[2] / 5 + 3`）。血量若不到 33 會被提到 33——"
            "因為 32 以下就算倒地，不墊高的話虛弱的角色一進場就結束。"
        ),
    },
    {
        "id": "arena.size",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x09 init_arena",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0336",
        "signature": "03ad 5c12 730a 03ad 5b09 730a",
        "summary": (
            "與室內同一組傳址參數：夾住的是**玩家在地面上的格子座標**（3–18 與 3–9），"
            "`make_zoom(11, x, y)` 拿它決定競技場視窗開在哪。場地本身的大小是樣式 11。"
        ),
    },
    {
        "id": "arena.shield-slot",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x05 check_shield",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x00be",
        "signature": (
            "9b04 3fad 0ba7 8020 8f0b b1ad 0b03 b39f a1d4 08ad 0bed a601 0b8a e59b 043f ad0b "
            "a780 208f 0bb0 a601 04ad 04d4 0e9b 043f ad0b a780 208d a601 0a8a 0400 a601 0a"
        ),
        "summary": (
            "護盾從身上四格裡找：掃 `MAINLIB_G3f[0..3]`，第一個 `byte mod 32 == 11` 的就是"
            "（物品 11 = shield）。剩餘次數是同一個 byte 的 `div 32`——與口袋位元組的"
            "「低 5 位編號、高 3 位次數」是同一條規則的第三個獨立證據。"
        ),
    },
    {
        "id": "arena.gun-to-hand",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x06 check_guns",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0119",
        "signature": (
            "9b04 3f22 a780 208f 6921 0cb0 210d b0a0 d446 2200 d31f 9b04 3f00 a768 9b04 3f00 "
            "9b04 3f22 a7c8 9b04 3f22 20c8 ad0b 00d2 0422 a601 0b05 6a88 0121 01d7 05e7 0401"
        ),
        "summary": (
            "進場時自動掏槍：掃四格找物品 12（stinger）或 13（scattergun），"
            "找到就把它與**第 0 格對調**。第 0 格是手上那一格，所以「手上拿著什麼」"
            "在戰鬥開始的瞬間會被機器改寫。scattergun 另外讀 `div 32` 當剩餘發數。"
        ),
    },
    {
        "id": "arena.shield-absorb",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x0e fire_gun",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x06c4",
        "signature": (
            "8801 2121 d705 6aad 0421 01b0 a1d4 1cad 0aee a601 0a05 7303 ad0a 00d2 0c07 7303 "
            "9b04 3fad 0b07 c8ef 05"
        ),
        "summary": (
            "護盾擋下的那一發**完全不扣血**，只扣一次充能。充能歸零時那一格就地換成"
            "dead part（值 7），再重掃一次護盾——與船艦零件被打壞的處理是同一套做法。"
        ),
    },
    {
        "id": "arena.shot-damage",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x0e fire_gun",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x06ef",
        "signature": "2501 d20c ad03 d404 0c68 8a02 0568 8a02 0368 22e7 0222 7a20 0900 730b 048f a28c a3",
        "summary": (
            "一發的傷害是 `基數 × (9 + random() mod 4)`，也就是基數的 9–12 倍。"
            "基數：玩家用 scattergun 是 12、用 stinger 是 5、暴徒一律 3。"
            "**scattergun 的期望傷害是 stinger 的 2.4 倍。**"
        ),
    },
    {
        "id": "arena.incapacitated",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x0e fire_gun",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0711",
        "signature": (
            "227a 00b3 f105 22e7 0200 c422 7a80 20b2 d42c 8801 2121 d705 7872 2f21 ef02 2101 "
            "d204 ef04"
        ),
        "summary": (
            "血量扣到負值會先夾回 0。**32 以下就倒地**（不是 0）：倒地就把 sprite 收掉，"
            "是玩家就走 `remove_player`，是暴徒且血量恰為 0 才從場上移除。"
        ),
    },
    # ── 競技場的即時制（2026-09-06） ────────────────────────────────────────
    {
        "id": "arena.record-count",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x0a initialize",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x03cb",
        "signature": "9a04 80db 10d2 149b 0453 8032 d810 0100 ca00 0206 730c a601 1a8a 1b",
        "summary": (
            "遭遇型別 16（打劫）把 `rand(2,6)` 存進 local `0x1a`，其餘型別存 2。"
            "**那個數字連玩家一起算**——記錄 1 是玩家，走對手的迴圈都寫成 "
            "`for i := 2 to 0x1a`，所以一場打劫是 1 到 5 個對手，不是 2 到 6 個。"
        ),
    },
    {
        "id": "arena.records-include-player",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x08 init_people",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0278",
        "signature": "0269 21ad 1ab2 d5ad 0088 0121 21d7 056c 24e7 0200 8046 80fa 730c c4",
        "summary": (
            "對手的迴圈從記錄 **2** 開始數到 local `0x1a`，每筆記錄 5 word，"
            "血量先擲 `rand(70, 250)`。這條是上一條「記錄數含玩家」的另一半證據。"
        ),
    },
    {
        "id": "arena.thug-numbers",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x08 init_people",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x02a5",
        "signature": (
            "24e7 0100 730b 088f 06a2 c49a 0417 d407 24e7 0124 79ed c424 e703 000e 03a3 0e05 "
            "a273 0c24 79a3 c4"
        ),
        "summary": (
            "記錄 word 1 是 `random() mod 8 + 6`，在 Enlie 再加一；word 3 的初值是 "
            "`rand(11, 19) − word1`。**word 1 不是走路速度，是開火速度**——它只出現在"
            "冷卻長度與開火機率兩處。"
        ),
    },
    {
        "id": "arena.walk-speed",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x08 init_people",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0308",
        "signature": "2322 0600 730b 048f a20e 9b03 812c 2478 d720 2478 722b",
        "summary": (
            "走路速度是 `create_sprite` 的第三個參數：對手 `6 + random() mod 4`、"
            "玩家固定 6（`0x01d3`）。sprite 一圈實際走幾個像素在 `do_sprite` 裡，沒有量過。"
        ),
    },
    {
        "id": "arena.field-inset",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x09 init_arena",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x034b",
        "signature": (
            "0388 0116 8801 1588 0114 8801 1373 3cad 1607 a2a6 0110 ad15 07a3 a601 0fad 1407 "
            "a2a6 010e ad13 07a3 a601 0d03 8801 1288 0111 733d"
        ),
        "summary": (
            "取視窗 3 的四個界與中心，四個界各往內縮 **7** 就是行動範圍。"
            "走出這個框的：玩家算逃離戰鬥，對手直接從場上撤掉。"
        ),
    },
    {
        "id": "arena.aim-history",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x10 handle_player",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x090c",
        "signature": (
            "ad18 ed07 8fa6 0118 8801 44ad 18d7 026b 8801 2101 d705 6c23 e701 2279 c423 227a "
            "c4ad 18ed 078f a601 17"
        ),
        "summary": (
            "每一圈把玩家的 sprite 座標寫進 local `0x44` 起、七筆兩個 word 的環，"
            "索引 `(i + 1) mod 7`。對手瞄的就是這個環裡隨機一筆——**原版射不準的來源**"
            "是它瞄的是玩家前幾圈的位置。"
        ),
    },
    {
        "id": "arena.thug-target-roll",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x11 handle_thugs",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0963",
        "signature": "0073 0b07 8f6b 8801 2124 d705 6d88 0144 23d7 026e 9b03 80ec 2578 d708 6f25",
        "summary": (
            "每個對手每一圈重擲 `random() mod 7`，用它從位置環裡挑一筆當目標，"
            "再取自己的記錄與 sprite。距離、移動與開火的落點全都用這一筆。"
        ),
    },
    {
        "id": "arena.close-range",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x11 handle_thugs",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x09ea",
        "signature": (
            "ad1b 8023 b29f ad0c 01b0 a0d4 3200 730b 148f 8a26 2578 2679 0007 028c e107 028c "
            "730c a226 7800 0702 8ce1 0702 8c73 0ca2 9012 8a09 2578 722e 8a03 d680 128a 26"
        ),
        "summary": (
            "`|dx| + |dy| > 0x23`（35）才走遠距離那一支，而遠距離是一顆二十面骰："
            "0 與 1 朝目標 `± rand(−14, 14)` 移動、2 停下、**其餘 17 面什麼都不做**，"
            "sprite 照著上一次給的目標繼續飄。"
        ),
    },
    {
        "id": "arena.retreat-step",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x11 handle_thugs",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0a29",
        "signature": (
            "2578 2779 0027 7926 79a3 7308 0073 0b8c 078f a227 7a00 277a 2678 a373 0800 730b "
            "8c07 8fa2 9012"
        ),
        "summary": (
            "距離在 35 以內時走的是**後退**：`x + (sign(x − 目標x) × random()) mod 7`，"
            "兩軸各一次。`sign` 指向遠離，`mod` 保留被除數的符號，所以對手不是貼上來，"
            "是繞著玩家維持射程。"
        ),
    },
    {
        "id": "arena.thug-bounds",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x11 handle_thugs",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0a4f",
        "signature": (
            "2779 ad10 b39f 2779 ad0f b29f a027 7aad 0eb3 9fa0 277a ad0d b29f a0d4 1188 0121 "
            "24d7 0578 722f 24ef 0324 ee6c 8a33"
        ),
        "summary": (
            "走出內縮框的對手直接 `kill_sprite` 並從記錄陣列裡壓掉，迴圈索引跟著退一格。"
            "所以逃跑的對手不是跑到邊上停住，是真的離場。"
        ),
    },
    {
        "id": "arena.fire-odds",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x11 handle_thugs",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0a7d",
        "signature": (
            "257b 00b2 ad0c 00b2 a19b 0453 8032 d810 01c9 e5a1 2579 257b a300 730b 8096 8fb2 "
            "9fa1 d40f 2426 7926 78ef 0e25 e703 0e25 79a3 c4"
        ),
        "summary": (
            "開火要四個條件同時成立：冷卻 `word3 <= 0`、模式 `<= 0`、旗標 `0x32` 沒設，"
            "以及 `word1 − word3 > random() mod 150`。`word3` 過期後會一直往負的走，"
            "所以**分子隨等待時間線性成長，最多 150 圈必定開火**。開完把 `word3` 設成 "
            "`14 − word1`。"
        ),
    },
    {
        "id": "arena.hit-scan",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x0d hit_scan",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0593",
        "signature": (
            "872e 872b 7aa3 e002 b3f1 0400 698a 0f87 2e87 2b79 b2f1 0402 698a 0302 e169 872e "
            "872b 79a3 e06a 872d 872b 7aa3 e002 b3f1 0400 688a 0f87 2d87 2b7a b2f1 0402 688a "
            "0302 e168 872d 872b 7aa3 e06b 2100 d20a 2000 d206 00a4 308b c700 2387 2a79 b29f "
            "2287 2a79 b29f a0d4 0a23 028d 6b22 028d 6a8a e723 6d22 6c"
        ),
        "summary": (
            "一發子彈是一條線：兩軸各走 ±2，`|dx|` 與 `|dy|` 反覆折半直到都不超過開火者的 "
            "`word1`（那決定線畫得多細），再互相計時走 Bresenham。**x 那一軸的死區比的是"
            "目標的 x 與開火者的 y**（`0x0593` 的 `sind2`）——看起來像手滑，但原版就是這樣，"
            "所以照抄。"
        ),
    },
    {
        "id": "arena.trigger-latch",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x10 handle_player",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x08ac",
        "signature": "0001 721f ad01 a121 7ca1 ad0c 02b3 9fa1 d444 8801 1e88 011d 721e 01ad 1ead 1def 0e",
        "summary": (
            "右鍵開火，落點就是滑鼠當下的座標。條件裡的 local `0x01` 是一個閂——"
            "開完火設成假、右鍵放開才設回真，所以**按住右鍵只會出一發**。"
        ),
    },
    {
        "id": "arena.scattergun-pulls",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x10 handle_player",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x08d1",
        "signature": (
            "ad19 ed04 8fa6 0119 ad19 00d2 1aad 09ee a601 09ad 0900 d20f 00a6 0103 9b04 3f00 "
            "07c8 0773 03ef 06"
        ),
        "summary": (
            "scattergun **每四次扣扳機才耗一發**（`(i+1) mod 4 == 0`）。耗到 0 時手上那格"
            "換成 dead part（值 7）、放 `part_dies` 音、再跑一次 `check_guns` 看身上還有沒有槍。"
        ),
    },
    {
        "id": "arena.down-countdown",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x04",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x008d",
        "signature": "9b04 5380 30d8 1001 c9d4 0720 e703 02c4 8a05 20e7 0313 c401 a601 0c",
        "summary": (
            "玩家倒下時把記錄 word 3 設成 **19**（旗標 `0x30` 已設則設 2），模式設成 1。"
            "那是被搶之前的倒數圈數。"
        ),
    },
    {
        "id": "arena.rob-carry-slots",
        "subsystem": "arena",
        "procedure": "XDOCOMBA:0x10 handle_player",
        "segment": "XDOCOMBA",
        "segmentOffset": "0x0819",
        "signature": (
            "9b04 5380 30d8 1001 c9f1 1100 6820 03b2 d40a 9b04 3f20 00c8 20ed 8af0 9b04 3300 "
            "0400 7015 00a6 0103 00a6 0104 02a6 010c 9b04 5380 30d8 1001 00ca 8a"
        ),
        "summary": (
            "倒數走完：`MAINLIB_G3f[0..3]` 四格**全部清零**（身上的東西被拿光），"
            "`MAINLIB_G33` 的 4 個 byte 也被清掉，然後模式設成 2 讓對手往最近的邊跑。"
            "地面戰鬥**不會殺死玩家**——它用的血量是自己的 `健康 × ST / 90`，"
            "從頭到尾沒有寫回角色。"
        ),
    },
    # ── 建築物內部 ─────────────────────────────────────────────────────────
    {
        "id": "interior.kind-dispatch",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x12 get_building",
        "segment": "XMOVEINB",
        "segmentOffset": "0x0a44",
        "signature": "01a6 0116 01a6 0104 00a6 0112 04a6 0103 ad80 a3",
        "summary": (
            "室內的樣子由互動種類決定：先設一組預設（房間 1 格深、4 個櫃位、沒有卡座），"
            "再以種類跳表覆寫。每個分支各給房間高度、兩個子視窗編號（餵給 screen_pos 取絕對矩形）、櫃位數與一個金額。"
            "種類 12／13／14／23 就是地形碼 11–14 那四扇門（酒館櫃檯、隨身裝備店、"
            "船艦零件店、旅館），見 ship-rules.json 的 `city.facility-door`。"
        ),
    },
    {
        "id": "interior.layout-bar",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x12 get_building",
        "segment": "XMOVEINB",
        "segmentOffset": "0x0a5a",
        "signature": (
            "05ad 80a6 78ad 80a5 7874 1507 a601 031d 0080 7acb 8801 808f 7336 1e00 807a cb88 "
            "0180 8b73 3680 32a6 010a 8028 a601 0401 a601 12"
        ),
        "summary": (
            "地形碼 11（酒館）：視窗樣式 5、子視窗編號 29 與 30、客人上限 7、"
            "客人上門的除數 50、老虎機在視窗區 40 且旗標開著。"
            "**四種建築裡只有酒館有老虎機**，這是原版的，不是重製版的選擇。"
        ),
    },
    {
        "id": "interior.layout-equipment",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x12 get_building",
        "segment": "XMOVEINB",
        "segmentOffset": "0x0ae7",
        "signature": (
            "07ad 80a6 78ad 80a5 7874 1519 0080 7acb 8801 808f 7336 1a00 807a cb88 0180 8b73 "
            "3680 c8a6 010a"
        ),
        "summary": "地形碼 12（隨身裝備店）：視窗樣式 7、子視窗編號 25 與 26、客人除數 200。其餘沿用預設。",
    },
    {
        "id": "interior.layout-parts",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x12 get_building",
        "segment": "XMOVEINB",
        "segmentOffset": "0x0abb",
        "signature": (
            "06ad 80a6 78ad 80a5 7874 151b 0080 7acb 8801 808f 7336 1c00 807a cb88 0180 8b73 "
            "3600 a601 1680 64a6 010a"
        ),
        "summary": (
            "地形碼 13（船艦零件店）：視窗樣式 6、子視窗編號 27 與 28、客人除數 100，"
            "而且把方向旗標設成 0——**四種建築裡只有它的櫃檯是直的**，其餘三種是橫的。"
        ),
    },
    {
        "id": "interior.layout-hotel",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x12 get_building",
        "segment": "XMOVEINB",
        "segmentOffset": "0x0a90",
        "signature": (
            "08ad 80a6 78ad 80a5 7874 1580 2900 807a cb88 0180 8f73 3680 2a00 807a cb88 0180 "
            "8b73 3681 2c01 a601 0a"
        ),
        "summary": "地形碼 14（旅館）：視窗樣式 8、子視窗編號 41 與 42、客人除數 300——四種裡最冷清的。",
    },
    {
        "id": "interior.customer-count",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x12 get_building",
        "segment": "XMOVEINB",
        "segmentOffset": "0x0b12",
        "signature": "0003 ad03 730c a601 0e17 0080 7acb 8801 0788 0106",
        "summary": "開場的客人數是 `rand(3, 上限)`：酒館 3–7 人，其餘三種都是 3–4 人。",
    },
    {
        "id": "interior.customer-rate",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x1c handle_customers",
        "segment": "XMOVEINB",
        "segmentOffset": "0x14ff",
        "signature": "ad0e ad03 b39f 0073 0bad 0ae2 00bd cb8f 0ab3 9fa1 d417 ad0e ed68 6000 ef0f",
        "summary": (
            "每次更新，只要人數還沒到上限且 `random() mod 除數 < 10`，就 `create_person` 進一個客人。"
            "除數因此是機率的分母：酒館 10/50（20%）、零件店 10/100、裝備店 10/200、旅館 10/300。"
            "**酒館最熱鬧，旅館最冷清。**"
        ),
    },
    {
        "id": "interior.slots-gate",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x1b handle_player",
        "segment": "XMOVEINB",
        "segmentOffset": "0x12e3",
        "signature": "0025 7925 7aad 0400 807a cb73 3fad 12a1 d402 ef1a",
        "summary": (
            "站進視窗區 `L4` 且旗標 `L12` 開著才進 `do_slots`。兩者都只有酒館設成 40 與 1，"
            "所以吃角子老虎只出現在酒館的角落。"
        ),
    },
    {
        "id": "interior.window-anchor",
        "subsystem": "interior",
        "procedure": "XMOVEINB:0x14 init_building",
        "segment": "XMOVEINB",
        "segmentOffset": "0x0b5e",
        "signature": "02ad 80a6 1173 0a02 ad80 a507 730a",
        "summary": (
            "前兩個參數**不是房間尺寸，是玩家在地面上的格子座標**——"
            "`XMOVEONG:0x14 update_position` 把精靈座標除以 8 寫回這兩個傳址參數，"
            "`SUNDOG:0x04 do_ground` 再原樣轉給 `move_in_building`。"
            "這裡把它們夾成 2–17 與 2–7，`get_building` 拿去當 `make_zoom` 的位置。"
            "**室內視窗因此開在玩家走進門的那個位置上**；房間本身的大小是 `make_zoom` "
            "的第一個參數，也就是四種建築各自的樣式常數 5／7／6／8。"
        ),
    },
    {
        "id": "interior.backdrop-case",
        "subsystem": "interior",
        "procedure": "WINDOWLI:0x1b draw_zoom",
        "segment": "WINDOWLI",
        "segmentOffset": "0x0830",
        "signature": "0e06 901c 8a19 1404 901c 8a13 1803 901c 8a0d 1b03 901c 8a07 0d68 8a03 d680 0b",
        "summary": (
            "室內背景圖的 case 表：視窗樣式 5（酒館）→ pick_pic(14, 6)、6（零件店）→ "
            "pick_pic(20, 4)、7（裝備店）→ pick_pic(24, 3)、8（旅館）→ pick_pic(27, 3)、"
            "9（太空船內部）固定用影像 13。影像 13–29 全是 160×100——正是 zoom 視窗的大小。"
            "選定後由 fetch_at 直接畫在視窗原點。"
        ),
    },
    {
        "id": "interior.backdrop-pick",
        "subsystem": "interior",
        "procedure": "WINDOWLI:0x1c draw_zoom:pick_pic",
        "segment": "WINDOWLI",
        "segmentOffset": "0x07d2",
        "signature": "9a04 17d4 0621 a601 018a 0d21 ed9a 0483 9520 ee8f a2a6 0101",
        "summary": (
            "選哪一張：一般是 `基底 + 1 + (G395 mod (張數 − 1))`——同一座城市固定同一張；"
            "**在 Enlie（G17）一律用基底那張**。可能值因此是酒館 15–19（Enlie 14）、"
            "零件店 21–23（Enlie 20）、裝備店 25–26（Enlie 24）、旅館 28–29（Enlie 27）。"
        ),
    },
    {
        "id": "interior.city-hash",
        "subsystem": "interior",
        "procedure": "XMOVEONG:0x20 check_exit",
        "segment": "XMOVEONG",
        "segmentOffset": "0x14fa",
        "signature": (
            "9b04 8227 9b04 2a06 09c9 eed7 0600 9b04 8395 0002 700f 9a04 8395 21a2 20a2 d904 8395"
        ),
        "summary": (
            "G395（室內圖的城市散列）在走出城界進下一座城時更新："
            "`城市記錄的第 0 個 word + 進城座標 x + 進城座標 y`。"
            "城市索引取自 G2a 的 packed 欄位 (6,9)，城市表基底是 MAINLIB_G227。"
        ),
    },
    # ── 降落與起飛動畫 ─────────────────────────────────────────────────────
    {
        "id": "landing.descent",
        "subsystem": "landing",
        "procedure": "XLANDFX:0x0b do_landing",
        "segment": "XLANDFX",
        "segmentOffset": "0x022c",
        "signature": "8075 02a3 a601 0301 a601 050e 0aef 03ad 20ef 0200 6820 8032 b2d4 09ef 0601 732b 20ed 8af0",
        "summary": (
            "降落分兩段：先跑 51 幀純星空（`do_stars` 加一格延遲），"
            "再把地平線從 y=117 一路往上掃到 y=6，每掃一列畫一次星空與地平線。"
            "城市輪廓在開始前由 `init_city` 依該城市的地圖生成。"
        ),
    },
    {
        "id": "landing.ascent",
        "subsystem": "landing",
        "procedure": "XLANDFX:0x0d do_take_off",
        "segment": "XLANDFX",
        "segmentOffset": "0x0310",
        "signature": "ad20 ef0c 0e0a ef03 06a6 0103 06a6 0104 06a6 0102 01e1 a601 05",
        "summary": (
            "起飛是降落的反向：地平線從 y=6 掃回 117，方向旗標設成 −1，"
            "城市輪廓改由 `city_up` 生成。沒有降落那段 51 幀的純星空。"
        ),
    },
    {
        "id": "landing.starfield",
        "subsystem": "landing",
        "procedure": "XLANDFX:0x06 do_stars",
        "segment": "XLANDFX",
        "segmentOffset": "0x00c2",
        "signature": "ad03 06b2 9fad 0380 75b2 a1d4 0c3f eda5 10ad 0306 a3a5 1271 16",
        "summary": (
            "星空只在地平線位於 6 與 117 之間時畫，畫的高度是 `地平線 − 6`——"
            "所以星空隨著地平線下降而變厚。捲動用的是 `SHIPLIB:0x16 stars()`，"
            "與太空航行畫的是同一支。"
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


# ── 降落輪廓的高度表 ────────────────────────────────────────────────────────
# dbldngs 查的那張表。SHIPLIB 開頭以 moveblock 把實體定址的磁碟 block（基底
# block −9 = raw 0x3600）載進 0x300 word 的緩衝，表在緩衝的 word 0x10e 處，
# 磁碟落點因此是 0x3e00 + 0x21c = 0x401c——防拷軌的資料裡。
# 8 筆形狀 × 8 bytes，每筆第 8 byte 恆 0（只畫 7 欄），值 0–6。
SKYLINE_TABLE_RAW = 0x401C
SKYLINE_SHAPES = 8
SKYLINE_COLUMNS = 7


def skyline_height_table(disk: bytes) -> dict[str, object]:
    span = disk[SKYLINE_TABLE_RAW : SKYLINE_TABLE_RAW + SKYLINE_SHAPES * 8]
    rows: list[list[int]] = []
    for shape in range(SKYLINE_SHAPES):
        record = span[shape * 8 : shape * 8 + 8]
        if record[7] != 0:
            raise ValueError(f"skyline shape {shape} 的第 8 byte 是 {record[7]}，預期 0")
        if any(value > 6 for value in record[:SKYLINE_COLUMNS]):
            raise ValueError(f"skyline shape {shape} 有超過 6 的欄高：{list(record)}")
        rows.append(list(record[:SKYLINE_COLUMNS]))
    return {
        "rawOffset": f"0x{SKYLINE_TABLE_RAW:05x}",
        "bytes": span.hex(" "),
        "sha256": hashlib.sha256(span).hexdigest(),
        "shapes": rows,
        "note": (
            "dbldngs 以 [形狀 × 8 + 欄] 取 byte 當欄高；第 8 byte 恆 0，未被讀取。"
            "表藏在防拷資料（實體 block 5–6，raw 0x4000–0x4400）裡，經 SHIPLIB 開頭的 "
            "moveblock 載入。讀取端的簽章早已釘在 ship-rules.json 的 landing.bar-rows "
            "與 landing.skyline，本表是那兩條規則指向的資料本體。"
        ),
    }


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    nested_name, disk_name, disk = load_disk(archive_path, variant)
    pinned = pin_rules(disk)
    subsystems: dict[str, int] = {}
    for rule in pinned:
        key = str(rule["subsystem"])
        subsystems[key] = subsystems.get(key, 0) + 1
    return {
        "schema": 1,
        "tool": "tools/atari_st_city_subsystems.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "nestedArchive": nested_name,
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "evidenceLevel": "proven（位置與唯一性）／語意為自原版 p-code 靜態讀出，未經執行對拍",
        "segmentBases": {name: f"0x{base:05x}" for name, base in SEGMENT_BASES.items()},
        "psystemStartupRaw": f"0x{PSYSTEM_STARTUP_RAW:05x}",
        "ruleCount": len(pinned),
        "rulesBySubsystem": subsystems,
        "rules": pinned,
        "skylineHeightTable": skyline_height_table(disk),
        "openQuestions": [
            {
                "topic": "interior room size",
                "status": "unknown",
                "detail": (
                    "房間的寬高由 `move_in_building` 的呼叫端傳進來，`init_building` 只把它夾成 "
                    "2–17 × 2–7。呼叫端在 `GRNDLIB:0x06` 之後，尚未追。跳表本身已逐支讀完，"
                    "四種建築的其餘數值見 interior.layout-* 四條。"
                ),
            },
            {
                "topic": "thug stats",
                "status": "proven",
                "detail": (
                    "暴徒的血量是 `rand(70, 250)`、速度是 `random() mod 8 + 6`，都與遭遇型別無關；"
                    "人數由 `XDOCOMBA:0x0a initialize` 依型別給（16 → `rand(2,6)`、15 → 有槍且 2 人、"
                    "其餘 → 2 人）。見 docs/atari-st/ground-rules.md 的街頭遭遇。"
                ),
            },
            {
                "topic": "arena entry gate",
                "status": "proven",
                "detail": (
                    "`xdo_combat` 開頭那條 `NOT G17 OR Gdb != 16 OR random() mod 10 == 4` 是"
                    "**要不要先講一段開場話**：預設會講，只有「人在第 3 號星系又碰上型別 16」"
                    "才直接開打，而且還有十分之一機率照樣先講。`Gdb` 在地面戰鬥裡裝的是遭遇型別，"
                    "不是敵艦護盾。見 docs/atari-st/ground-rules.md。"
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
