#!/usr/bin/env python3
"""解碼 Atari ST SunDog 的星系／行星／城市資料表。

輸入是唯讀的外層 Atari ST archive；工具只讀 nested raw ``.st``，不改寫原始媒體，
也不把磁碟 bytes 寫進輸出。輸出是含 raw offset、來源雜湊、原始 tail hex 與證據
等級的 JSON manifest。

區段位置取自公開 secondary oracle ``laanwj/sundog`` 的 ``doc/disklayout.md``：
raw ``0x05800`` 起 3 個 logical block 標記為 "Systems,planets,cities"。**位置是
該文件提供的線索，record 邊界與欄位是本工具自行從 bytes 解出並驗證的**，兩者的
證據等級必須分開記。

record 邊界由三項獨立條件同時成立而確立：

1. 每筆都是「長度前綴的 9-char 固定欄位名稱」（1 + 9 bytes），長度合法、超出長度
   的 bytes 全為零、字元可列印。
2. 三張表首尾相接、總長恰好填滿 1,152 bytes，其後到 block 邊界為全零。
3. 城市在表中的順序與本專案既有的 ``docs/apple2/world-map-crosscheck.json``
   （來自原版紙本星圖的 58 筆 system/planet/city 三元組）逐筆吻合，且同一顆行星的
   城市在表中構成連續區間。

name 之後的 tail bytes 一律保留原始 hex。欄位語意則是逐支讀 p-code 的讀寫端讀出來的：
行星的三個 word 與城市的一個 word 現在都有名字，星系表還剩兩處未解（見 openQuestions）。
每個命名都由 ``PCODE_RULES`` 裡一段位元組簽章釘住，該簽章必須在整張磁碟映像裡恰好出現
一次，對不上就 fail-closed——做法與 ``tools/atari_st_ground_rules.py`` 相同。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


# laanwj/sundog doc/disklayout.md：`0x05800 0x08 3 Systems,planets,cities`。
WORLD_REGION_START = 0x05800
WORLD_REGION_BLOCKS = 3
BLOCK_SIZE = 512
WORLD_REGION_END = WORLD_REGION_START + WORLD_REGION_BLOCKS * BLOCK_SIZE

NAME_FIELD_SIZE = 10
NAME_MAX_LENGTH = NAME_FIELD_SIZE - 1

SYSTEM_RECORD_SIZE = 20
PLANET_RECORD_SIZE = 16
CITY_RECORD_SIZE = 12

# 星系 record 的第 8 個 word（byte 16–17）裝著三個 4-bit 座標。這個版面來自遊戲自己的
# p-code：SHIPLIB:0x0a dist 以 `ixa 0xa`（20 bytes 步距）取得 record，`inc 0x8` 移到第 8 個
# word，再以 `ixp 0x4,0x4` 取第 0–2 個 4-bit 欄位。p-machine 的 IXP 把欄位 i 放在
# 「第 i*4 個 bit 起算」，因此**欄位 0 是最低的 nibble**。第 3 個 nibble 不參與距離計算，
# 用途未解，原值保留。
SYSTEM_COORDINATE_WORD = 8
COORDINATE_FIELD_BITS = 4
COORDINATE_FIELDS = 3

# 行星 record 的第 6 個 word（byte 12–13）是該行星在星系裡的極座標。這個版面同樣來自
# p-code：``XSTARTUP:0x2f init_location`` 以 `ixa 0x8`（16 bytes 步距）取得 record，
# `inc 0x6` 移到第 6 個 word，再以 `ldp` 取兩個 byte 欄位分別填進 ``MAINLIB_G29`` 的高、
# 低 byte；而 ``SHIPLIB:0x0c polar_to_rect`` 讀的正是「低 byte 是角度、高 byte 是半徑」。
# 68000 的 word 是 big-endian，所以高 byte 是 record 裡先出現的那個。
PLANET_ORBIT_WORD = 6
PLANET_MOTION_WORD = 5
PLANET_LAYOUT_WORD = 7

# 降落格座標的基準點：column = 10 + (byte mod 20)、row = 6 + (byte div 20)。
# 20 這個除數、10 與 6 這兩個基準，都直接寫在 XSTARTUP:init_location 的位元組裡。
PLANET_LANDING_MODULUS = 20
PLANET_LANDING_COLUMN_BASE = 10
PLANET_LANDING_ROW_BASE = 6


# 星系 record 的第 9 個 word 又是四個 4-bit 欄位。第 0 個是**物價等級**：
# ``XDOINTER:0x24 initialize`` 以 `ixa 0xa`（20 bytes 步距）取 record、`inc 0x8` 移到第 8 個
# word，再以 `ixp 0x4,0x4` 取第 4 個欄位——四個欄位一個 word，所以索引 4 就是第 9 個 word 的
# 第 0 個欄位。那個值接著在 ``XDOINTER:0x02 set_up_price`` 裡當價格倍率的分子。
# 第 1 個欄位另有用途未解，第 2、3 個欄位十二個星系都是 0。
SYSTEM_ECONOMY_WORD = 9
ECONOMY_FIELD_BITS = 4

# 城市 record 的第 5 個 word（byte 10–11）裝著它在行星圖上的位置。版面來自
# `XREADMAP:0x06 disp_map:disp_planet`：它以 `ldp` 取寬 6／位移 0 與寬 5／位移 6 兩個
# 欄位，交給 `change_xy` 換成畫面座標。最高的 5 個 bit 沒有被讀，實測 60 筆全為 0。
CITY_POSITION_WORD = 5
CITY_POSITION_COLUMN_BITS = 6
CITY_POSITION_ROW_BITS = 5

SYSTEM_COUNT = 12
PLANET_COUNT = 18
CITY_COUNT = 60

# 表中兩筆保留名稱。它們不是遊戲地點，不進 remake 目的地清單。
FILLER_CITY_NAME = "dummy"

# 紙本星圖證據檔與磁碟表的拼寫差異。列成明表而不用模糊比對，讓新的差異會直接失敗。
MAP_NAME_ALIASES = {
    "Woromed": "Woremed",
    "Worrad": "W'orrad",
    "NewShoot": "New Shoot",
    "GloryI": "Glory I",
    "GloryII": "Glory II",
    "GloryIII": "Glory III",
    "Llorac": "Llorrac",
}


EXPECTED_MATCHES = 1
MINIMUM_SIGNATURE_BYTES = 10

# SYSTEM.STARTUP 在磁碟映像上的位置；段基底加上這個值才是 raw offset。
PSYSTEM_STARTUP_RAW = 0x2E000

# 欄位語意的來源：每條規則指出「哪一支程序、段內哪個位址」讀或寫這個欄位，並附一段
# 位元組簽章。段基底取自 SYSTEM.STARTUP 在磁碟上的位置（0x2e000）加上 segment 起點。
PCODE_RULES: list[dict[str, object]] = [
    {
        "id": "planet.orbit-rate",
        "field": "planets word +5 高位元組",
        "procedure": "XMOVEONS:0x07 check_planets",
        "segmentBase": 0x9E00,
        "segmentOffset": "0x0298",
        "signature": (
            "9b04 8197 21ee d708 6b23 e706 0800 c923 e705 0808 c922 8ca2 6820 00b3 "
            "f108 2081 0001 a268 8af3 23e7 0608 0020 ca21 ed8a c9"
        ),
        "summary": (
            "行星會繞恆星公轉：angle := angle + rate × elapsed，取 256 為一圈、以 8 bit 截斷。"
            "rate 就是 word +5 的高位元組。18 顆行星的 rate 是 0..8，其中六顆為 0（不動）。"
            "負值會被 while 迴圈補回正區間，資料裡沒有出現負值。"
        ),
    },
    {
        "id": "planet.orbit-angle-is-live-state",
        "field": "planets word +6 低位元組",
        "procedure": "XSTARTUP:0x29 utility:set_new_game:init_planets",
        "segmentBase": 0xBC00,
        "segmentOffset": "0x164e",
        "signature": (
            "9b04 8197 21ee d708 6a22 e707 010f 00ca 22e7 0608 0000 0080 ff73 0cca "
            "22e7 0608 00c9 20a3 e00f b2f1 e7"
        ),
        "summary": (
            "開新遊戲時 18 顆行星的角度全部重骰：repeat angle := rand(0,255) until "
            "abs(angle − 前一顆的 angle) > 15。因此磁碟上的角度只是出廠值，開局就被蓋掉；"
            "半徑（高位元組）不動。這一段同時把 word +7 的 bit 15 清成 0。"
        ),
    },
    {
        "id": "planet.landing-square",
        "field": "planets word +5 低位元組",
        "procedure": "XSTARTUP:0x2f utility:set_new_game:init_location",
        "segmentBase": 0xBC00,
        "segmentOffset": "0x19d4",
        "signature": (
            "9b04 2b06 000a 9b04 8197 9b04 2a05 04c9 eed7 08e7 0508 00c9 148f a2ca "
            "9b04 2c05 0006 9b04 8197 9b04 2a05 04c9 eed7 08e7 0508 00c9 148d a2ca"
        ),
        "summary": (
            "降落在城市地面的哪一格：column = 10 + (byte mod 20)、row = 6 + (byte div 20)，"
            "分別寫進 MAINLIB_G2b 的 bits 0–5 與 MAINLIB_G2c 的 bits 0–4。"
        ),
    },
    {
        "id": "planet.default-city",
        "field": "planets word +7 bits 3–8",
        "procedure": "XSTARTUP:0x2f utility:set_new_game:init_location",
        "segmentBase": 0xBC00,
        "segmentOffset": "0x19bc",
        "signature": "9b04 2a06 099b 0481 979b 042a 0504 c9ee d708 e707 0603 c9ca",
        "summary": (
            "開局所在的城市：MAINLIB_G2a 的 bits 9–14（1-based 城市編號）取自這個欄位。"
            "與下一條的 bits 9–14 在 18 筆資料裡數值全部相同，但讀取端不同、用途也不同——"
            "這一個是「預設落在哪座城市」，那一個是「這顆行星的城市從哪裡開始」。"
        ),
    },
    {
        "id": "planet.city-run",
        "field": "planets word +7 bits 0–2 與 bits 9–14",
        "procedure": "XPILOTAG:0x0a build_menu",
        "segmentBase": 0x7200,
        "segmentOffset": "0x03a2",
        "signature": "9b04 8197 9b04 4a05 04c9 eed7 08e7 0706 09c9 b1a0 a1",
        "summary": (
            "同一顆行星的城市在城市表裡是連續區間：bits 9–14 是首座城市的 1-based 編號、"
            "bits 0–2 是城市數。與紙本星圖交叉比對出來的 planetCityRuns 逐筆吻合（18/18）。"
            "XDOTRADI:0x15 current_price:find_location（段內 0x08a3）用同一組欄位換算城市在行星內的序號。"
        ),
    },
    {
        "id": "system.bank-balance",
        "field": "systems 第 6、7 個 word",
        "procedure": "XDOUNITE:0x0d do_teller:do_transfer",
        "segmentBase": 0x14E00,
        "segmentOffset": "0x0628",
        "signature": "2a63 ef0c 9b04 811f 2001 0ccb eed7 0aa4 0a61 29e7 06",
        "summary": (
            "這兩個 word 是玩家在該星系的銀行存款，不是磁碟資料：櫃台的轉帳把身上的現金"
            "加進 systems[目前星系] + 6 這一對 word。磁碟上 12 筆全為零，因為它是執行時狀態，"
            "開新遊戲才由 XSTARTUP:init_systems 填初值。"
        ),
    },
    {
        "id": "system.bank-balance-gate",
        "field": "systems 第 6、7 個 word",
        "procedure": "XSHOWMOV:0x0d do_on_world",
        "segmentBase": 0x9200,
        "segmentOffset": "0x06a9",
        "signature": "2287 1700 b39f 8718 00b3 9fa0 a1d4 0806 ef04",
        "summary": (
            "行星內起飛前檢查同一對 word：任一個為負就擋下，印出第 6 號錯誤訊息"
            "「Lift clearance denied due to unpaid debts」。這是這兩個 word 的讀取端，"
            "與 do_transfer 的寫入端互相印證它是欠款／存款。"
        ),
    },
    {
        "id": "money.base-1000",
        "field": "所有兩個 word 的金額",
        "procedure": "MAINLIB:0x0d set_money",
        "segmentBase": 0x0800,
        "segmentOffset": "0x0686",
        "signature": (
            "22e7 0100 2091 0820 e081 e803 8f8c c422 2100 2091 0820 e081 e803 8d8c a2c4"
        ),
        "summary": (
            "金額一律以 1000 為底拆成兩個 word：低位 word 存 |v| mod 1000、高位 word 存 "
            "既有高位加上 |v| div 1000，兩者同號。因此讀金額要算 hi × 1000 + lo，"
            "不能當成 16-bit 或 32-bit 整數直接讀。"
        ),
    },
    {
        "id": "system.new-game-bank",
        "field": "systems 第 6、7 個 word",
        "procedure": "XSTARTUP:0x26 utility:set_new_game:init_systems",
        "segmentBase": 0xBC00,
        "segmentOffset": "0x154b",
        "signature": (
            "24e7 066d 2503 0073 0b03 8fa2 c422 01d2 0725 2578 8020 a2c4 25e7 0100 730b "
            "81e8 038f c4"
        ),
        "summary": (
            "開新遊戲時每個星系的存款設成 3 + random() mod 3（千位），第 2 到第 12 個星系"
            "再加 32 千；個位那個 word 是 random() mod 1000。換算成金額就是第 1 個星系 "
            "3,000–5,999 Cr，其餘 35,000–37,999 Cr。"
        ),
    },
    {
        "id": "system.coordinate-jitter",
        "field": "systems 第 8、9 個 word 的座標 nibble",
        "procedure": "XSTARTUP:0x26 utility:set_new_game:init_systems",
        "segmentBase": 0xBC00,
        "segmentOffset": "0x156e",
        "signature": (
            "220a b122 03b1 a1d4 3e00 6921 02b2 d431 24e7 0821 d804 04c9 6821 8a15 6001 "
            "0c90 278a 1160"
        ),
        "summary": (
            "開新遊戲時三個座標 nibble 各自呼叫 set_random 微調，範圍 x 1–12、y 1–9、z 0–10，"
            "然後呼叫 0x28 做間距檢查：與任何已放好的星系（索引比自己小的，加上第 10 個）"
            "|ΔX| < 2 且 |ΔY| < 2 就判太近、整組重擲——所以單一局的位移可以疊到好幾格。"
            "第 10 個星系寫死成 (6, 5, 6) 不動；第 3 個星系（Enlie）在主迴圈跳過，之後單獨處理："
            "X、Y 照樣微調並對全部星系做間距檢查，Z 則直接寫成 12 + random() mod 4，"
            "所以它在星圖上永遠是退化成一個字元點的最遠記號；擺好後再算它到其他星系的最小"
            "跳躍距離（SHIPLIB:0x0a dist，含 Enlie 的 +1），不等於 9 就重擺，25 次不成就把整張圖"
            "再漂一輪——新船的最大跳躍是 8，所以 Enlie 開局永遠差一格跳不到。原版實跑（hatari-live-run.md 第十輪）"
            "從快照 RAM 讀出的執行時座標，12 個記號的位置與半徑全部與畫面吻合。"
        ),
    },
    {
        "id": "system.jitter-step",
        "field": "systems 第 8、9 個 word 的座標 nibble",
        "procedure": "XSTARTUP:0x27 set_random",
        "segmentBase": 0xBC00,
        "segmentOffset": "0x1482",
        "signature": "2222 7800 730b 038f a2ee c421 2220 730a",
        "summary": (
            "set_random(&v, lo, hi) 做的是 v := v + random() mod 3 − 1，然後 condition(lo, &v, hi) "
            "夾回範圍。單次是 ±1 格，但 init_systems 會在間距檢查不過時重擲，"
            "一局下來可以離出廠值好幾格（實跑量到 Hepa X 12→8、Sosai (5,1,7)→(2,2,10)）。"
        ),
    },
]


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, str, bytes]:
    """從外層 archive 取出 nested raw .st，不解壓到檔案系統。"""
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        nested_name, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return nested_name, disk_name, disk_bytes


def decode_name(field: bytes, raw_offset: int) -> str:
    """解出長度前綴的固定欄位名稱，並要求填充 bytes 全為零。"""
    if len(field) != NAME_FIELD_SIZE:
        raise ValueError(f"name field at 0x{raw_offset:05x} is {len(field)} bytes")
    length = field[0]
    if length > NAME_MAX_LENGTH:
        raise ValueError(
            f"name at 0x{raw_offset:05x} declares length {length}, max {NAME_MAX_LENGTH}"
        )
    body = field[1 : 1 + length]
    padding = field[1 + length :]
    if any(byte for byte in padding):
        raise ValueError(f"name at 0x{raw_offset:05x} has non-zero padding")
    for byte in body:
        if byte < 0x20 or byte > 0x7E:
            raise ValueError(f"name at 0x{raw_offset:05x} has non-printable byte 0x{byte:02x}")
    return body.decode("ascii")


def decode_coordinates(record: bytes) -> dict[str, object]:
    """讀出星系 record 裡的三個 4-bit 座標。"""
    base = SYSTEM_COORDINATE_WORD * 2
    word = (record[base] << 8) | record[base + 1]
    mask = (1 << COORDINATE_FIELD_BITS) - 1
    values = [(word >> (field * COORDINATE_FIELD_BITS)) & mask for field in range(COORDINATE_FIELDS)]
    unused = (word >> (COORDINATE_FIELDS * COORDINATE_FIELD_BITS)) & mask
    economy_base = SYSTEM_ECONOMY_WORD * 2
    economy_word = (record[economy_base] << 8) | record[economy_base + 1]
    economy_mask = (1 << ECONOMY_FIELD_BITS) - 1
    price_level = economy_word & economy_mask
    if price_level == 0:
        raise ValueError("a star system has price level 0, which would make everything free")
    street_deal_rating = (economy_word >> ECONOMY_FIELD_BITS) & economy_mask
    return {
        "coordinateWord": f"0x{word:04x}",
        "economyWord": f"0x{economy_word:04x}",
        "priceLevel": price_level,
        "priceLevelNote": (
            "第 9 個 word 的第 0 個 4-bit 欄位是該星系的物價等級："
            "XDOINTER:0x02 set_up_price 以 `30 × 物價等級 / 城市係數` 當價格倍率，"
            "所以值越大東西越貴。見 docs/atari-st/ship-rules.md。"
        ),
        "coordinates": values,
        "coordinatesNote": (
            "磁碟上的座標是出廠值。開新遊戲時 XSTARTUP:init_systems 讓三個 nibble 各自漂移 "
            "±1 並夾在 x 1–12、y 1–9、z 0–10，間距檢查不過就重擲，所以一局下來可以差好幾格；"
            "第 10 個星系寫死成 (6, 5, 6)，第 3 個（Enlie）的 Z 固定寫成 12–15。"
            "要拿座標對畫面，得讀執行時的記錄（MAINLIB G11f），不能用出廠值。"
        ),
        "bankBalanceNote": (
            "第 6、7 個 word（tail bytes 2–5）12 筆全為零，因為它是執行時狀態而非磁碟資料："
            "那是玩家在該星系的銀行存款，以 1000 為底存成兩個 word。銀行櫃台的轉帳寫它，"
            "行星內起飛前讀它，為負就擋下並印「Lift clearance denied due to unpaid debts」。"
        ),
        "streetDealRating": street_deal_rating,
        "streetDealRatingNote": (
            "第 9 個 word 的第 1 個 4-bit 欄位（八個欄位裡的 nibble 5）。已解出的唯一消費端是 "
            "XMOVEONG:0x36 confirm 對街頭遭遇型別 22 的抑制門檻——`random() mod 20 < 它` 就取消，"
            "所以值越大，街上主動來談生意的人越少。XDOINTER:0x24 initialize 也讀它，"
            "那一側做什麼還沒解出。見 docs/atari-st/ground-rules.md。"
        ),
        "encounterRating": unused,
        "encounterRatingNote": (
            "第 4 個 4-bit 欄位不參與 dist 的計算，它是該星系的遭遇設定："
            "SHIPLIB:0x1f update_movement 以「值 <= random() mod 18 就不生成敵艦」判定，"
            "所以值越大越安全；生成時的敵艦類別是 1 - (值 - 1) / 4。"
            "見 docs/atari-st/ship-rules.md。"
        ),
    }


def decode_city_position(record: bytes, index: int) -> dict[str, object]:
    """讀出城市在行星圖上的位置。

    `change_xy` 把兩個欄位換成畫面座標：`x = 7 × ((column + 1) / 2) + 4`，column 為**奇數**
    時再加 3（`fjp` 只看最低位元：`BOOL(x) = x & 1`，「if column」是「if column 是奇數」；
    原版實跑：欄 12 的 Drahew 畫在 x=46、欄 40 的船在 x=144）；`y = 4 × row + 2`。畫面座標留給消費端算，這裡只交出欄位與原版的公式結果，
    兩者都不猜語意。
    """
    base = CITY_POSITION_WORD * 2
    word = (record[base] << 8) | record[base + 1]
    column = word & ((1 << CITY_POSITION_COLUMN_BITS) - 1)
    row = (word >> CITY_POSITION_COLUMN_BITS) & ((1 << CITY_POSITION_ROW_BITS) - 1)
    unused = word >> (CITY_POSITION_COLUMN_BITS + CITY_POSITION_ROW_BITS)
    if unused:
        raise ValueError(f"city record {index} has unread bits set: 0x{word:04x}")
    x = 7 * ((column + 1) // 2) + 4
    if column & 1:
        x += 3
    return {
        "positionWord": f"0x{word:04x}",
        "position": {"column": column, "row": row},
        "planetMapPixel": {"x": x, "y": 4 * row + 2},
        "positionNote": (
            "第 5 個 word 的低 6 bit 是 column、接著 5 bit 是 row；"
            "XREADMAP:0x07 change_xy 換成畫面座標 x = 7 × ((column + 1) / 2) + 4"
            "（column 為奇數時再加 3：p-machine 的 fjp 只看最低位元）、y = 4 × row + 2。"
            "原版在該位置畫字元 20（白色方塊），玩家所在的城市另外疊一個船艦字元。"
        ),
    }


def decode_orbit(record: bytes, index: int) -> dict[str, object]:
    """讀出行星在星系裡的極座標。半徑為 0 會讓行星與恆星重疊，因此不接受。"""
    base = PLANET_ORBIT_WORD * 2
    radius = record[base]
    angle = record[base + 1]
    if radius == 0:
        raise ValueError(f"planet record {index} has orbit radius 0, which would sit on the star")
    return {
        "orbitWord": f"0x{(radius << 8) | angle:04x}",
        "orbit": {"radius": radius, "angle": angle},
        "orbitNote": (
            "角度是 256 為一圈的整圈刻度，半徑是 polar_to_rect 直接乘上去的長度；"
            "投影用的四分之一正弦表見 docs/atari-st/sine-table.json。"
            "**角度是執行時狀態，不是常數**：XSTARTUP:init_planets 在開新遊戲時把 18 顆的角度"
            "全部重骰，XMOVEONS:check_planets 之後每次再依 orbitRate 推進。此處記的是出廠值，"
            "玩家實際看到的角度與它無關。半徑不會被改寫。"
        ),
    }


def decode_planet_fields(record: bytes) -> dict[str, object]:
    """讀出行星 record 的另外兩個 word：公轉速率與降落格、以及城市區間與預設城市。"""
    motion = record[PLANET_MOTION_WORD * 2 : PLANET_MOTION_WORD * 2 + 2]
    rate, landing = motion[0], motion[1]
    layout_base = PLANET_LAYOUT_WORD * 2
    layout = (record[layout_base] << 8) | record[layout_base + 1]

    city_count = layout & 0x7
    default_city = ((layout >> 3) & 0x3F) - 1
    first_city = ((layout >> 9) & 0x3F) - 1
    spare_bit = (layout >> 15) & 1

    return {
        "motionWord": f"0x{(rate << 8) | landing:04x}",
        "orbitRate": rate,
        "orbitRateNote": (
            "每一次 XMOVEONS:check_planets 把角度推進 orbitRate × 經過時間，對 256 取模。"
            "值為 0 的行星不公轉。"
        ),
        "landingSquare": {
            "column": PLANET_LANDING_COLUMN_BASE + landing % PLANET_LANDING_MODULUS,
            "row": PLANET_LANDING_ROW_BASE + landing // PLANET_LANDING_MODULUS,
        },
        "landingNote": (
            "開新遊戲時降落在城市地面的哪一格；"
            f"column = {PLANET_LANDING_COLUMN_BASE} + (byte mod {PLANET_LANDING_MODULUS})、"
            f"row = {PLANET_LANDING_ROW_BASE} + (byte div {PLANET_LANDING_MODULUS})。"
        ),
        "layoutWord": f"0x{layout:04x}",
        "cityCount": city_count,
        "firstCityIndex": first_city,
        "defaultCityIndex": default_city,
        "layoutSpareBit": spare_bit,
        "layoutNote": (
            "bits 0–2 城市數、bits 3–8 開局所在城市（1-based）、bits 9–14 城市區間起點（1-based）、"
            "bit 15 沒有讀取端。中間兩個欄位在 18 筆資料裡數值相同，但讀取端與用途不同。"
            "此處輸出的 index 已減 1，與 tables.cities 的 index 同基準。"
        ),
    }


def pin_pcode_rules(disk: bytes) -> list[dict[str, object]]:
    """把每條欄位語意規則釘在磁碟上。簽章不唯一、或與宣告的位址對不上就 fail-closed。"""
    pinned: list[dict[str, object]] = []
    for rule in PCODE_RULES:
        signature = bytes.fromhex(str(rule["signature"]).replace(" ", ""))
        if len(signature) < MINIMUM_SIGNATURE_BYTES:
            raise ValueError(f"{rule['id']}: 簽章只有 {len(signature)} bytes，太短不足以釘住位置")
        matches = []
        start = disk.find(signature)
        while start != -1:
            matches.append(start)
            start = disk.find(signature, start + 1)
        if len(matches) != EXPECTED_MATCHES:
            raise ValueError(f"{rule['id']}: 簽章在磁碟上出現 {len(matches)} 次，預期 {EXPECTED_MATCHES} 次")
        raw = matches[0]
        expected = PSYSTEM_STARTUP_RAW + int(rule["segmentBase"]) + int(str(rule["segmentOffset"]), 16)
        if raw != expected:
            raise ValueError(
                f"{rule['id']}: 簽章落在 0x{raw:05x}，但宣告的段基底與段內位址算出 0x{expected:05x}"
            )
        entry = {key: value for key, value in rule.items() if key != "segmentBase"}
        entry["rawOffset"] = f"0x{raw:05x}"
        entry["signatureBytes"] = len(signature)
        pinned.append(entry)
    return pinned


def decode_table(
    disk: bytes, start: int, record_size: int, count: int, kind: str
) -> list[dict[str, object]]:
    records: list[dict[str, object]] = []
    for index in range(count):
        offset = start + index * record_size
        record = disk[offset : offset + record_size]
        if len(record) != record_size:
            raise ValueError(f"{kind} record {index} truncated at 0x{offset:05x}")
        name = decode_name(record[:NAME_FIELD_SIZE], offset)
        tail = record[NAME_FIELD_SIZE:]
        entry: dict[str, object] = {
            "index": index,
            "rawOffset": f"0x{offset:05x}",
            "name": name,
            "tailHex": tail.hex(),
            "recordSha256": sha256(record),
        }
        if kind == "system":
            entry.update(decode_coordinates(record))
        if kind == "planet":
            entry.update(decode_orbit(record, index))
            entry.update(decode_planet_fields(record))
        if kind == "city":
            entry.update(decode_city_position(record, index))
        records.append(entry)
    return records


def normalize_map_name(name: str) -> str:
    return MAP_NAME_ALIASES.get(name, name)


def load_map_triples(path: Path) -> tuple[list[dict[str, str]], list[dict[str, str]]]:
    document = json.loads(path.read_text(encoding="utf-8"))
    auxiliary = document["auxiliaryMap"]
    pairs = [
        {
            "system": normalize_map_name(pair["system"]),
            "planet": normalize_map_name(pair["planet"]),
        }
        for pair in auxiliary["planetPairs"]
    ]
    triples = [
        {
            "system": normalize_map_name(triple["system"]),
            "planet": normalize_map_name(triple["planet"]),
            "city": normalize_map_name(triple["city"]),
        }
        for triple in auxiliary["cityTriples"]
    ]
    return pairs, triples


def crosscheck(
    systems: list[dict[str, object]],
    planets: list[dict[str, object]],
    cities: list[dict[str, object]],
    pairs: list[dict[str, str]],
    triples: list[dict[str, str]],
) -> dict[str, object]:
    """以紙本星圖三元組驗證磁碟順序，並產出 planet→city／system→planet 分組。"""
    city_to_triple = {triple["city"]: triple for triple in triples}
    if len(city_to_triple) != len(triples):
        raise ValueError("map evidence contains duplicate city names")
    planet_to_system = {pair["planet"]: pair["system"] for pair in pairs}

    system_names = [str(record["name"]) for record in systems]
    planet_names = [str(record["name"]) for record in planets]

    # 表尾的保留名稱切開兩個區段：其前是逐行星的連續區間，其後是不屬於任何區間的
    # 尾端城市。兩者的驗證條件不同，不能混在一起檢查。
    filler_indices = [
        int(record["index"]) for record in cities if str(record["name"]) == FILLER_CITY_NAME
    ]
    if not filler_indices:
        raise ValueError("city table has no reserved filler record")
    boundary = min(filler_indices)

    def assign(record: dict[str, object], section: str) -> dict[str, object]:
        name = str(record["name"])
        if name == FILLER_CITY_NAME:
            return {"index": record["index"], "city": name, "planet": None, "kind": "filler"}
        triple = city_to_triple.get(name)
        if triple is None:
            return {"index": record["index"], "city": name, "planet": None, "kind": "unmatched"}
        return {
            "index": record["index"],
            "city": name,
            "planet": triple["planet"],
            "system": triple["system"],
            "kind": "assigned",
            "section": section,
        }

    leading_assignments = [assign(record, "contiguous") for record in cities[:boundary]]
    trailing_assignments = [assign(record, "trailing") for record in cities[boundary:]]
    unmatched = [
        str(assignment["city"])
        for assignment in leading_assignments + trailing_assignments
        if assignment["kind"] == "unmatched"
    ]
    if unmatched:
        raise ValueError(f"cities absent from map evidence: {unmatched}")
    if any(assignment["kind"] != "assigned" for assignment in leading_assignments):
        raise ValueError("reserved filler record appears inside the contiguous city section")

    # 連續性：同一顆行星的城市必須是一段不中斷的區間。
    contiguous_runs: list[dict[str, object]] = []
    for assignment in leading_assignments:
        if contiguous_runs and contiguous_runs[-1]["planet"] == assignment["planet"]:
            contiguous_runs[-1]["count"] = int(contiguous_runs[-1]["count"]) + 1
            continue
        contiguous_runs.append(
            {
                "planet": assignment["planet"],
                "system": assignment["system"],
                "start": assignment["index"],
                "count": 1,
            }
        )

    seen_planets = [str(run["planet"]) for run in contiguous_runs]
    if len(seen_planets) != len(set(seen_planets)):
        raise ValueError("a planet has non-contiguous cities in the disk table")

    # 磁碟行星順序必須與城市區間的出現順序完全一致。
    if seen_planets != planet_names:
        raise ValueError(
            f"planet order mismatch: disk {planet_names} versus city runs {seen_planets}"
        )

    hidden = [
        assignment for assignment in trailing_assignments if assignment["kind"] == "assigned"
    ]

    # 系統→行星分組同樣要求連續。
    system_runs: list[dict[str, object]] = []
    for planet in planet_names:
        system = planet_to_system.get(planet)
        if system is None:
            raise ValueError(f"planet {planet!r} absent from map planet pairs")
        if system_runs and system_runs[-1]["system"] == system:
            system_runs[-1]["count"] = int(system_runs[-1]["count"]) + 1
            continue
        system_runs.append({"system": system, "start": planet_names.index(planet), "count": 1})
    ordered_systems = [str(run["system"]) for run in system_runs]
    if len(ordered_systems) != len(set(ordered_systems)):
        raise ValueError("systems have non-contiguous planets in the disk table")
    if sorted(ordered_systems) != sorted(system_names):
        raise ValueError(
            f"system set mismatch: disk {sorted(system_names)} versus map {sorted(ordered_systems)}"
        )

    return {
        "mapEvidence": "docs/apple2/world-map-crosscheck.json",
        "planetPairCount": len(pairs),
        "cityTripleCount": len(triples),
        "contiguousCityCount": len(leading_assignments),
        "fillerCityCount": len(filler_indices),
        "trailingCityCount": len(hidden),
        "planetCityRuns": contiguous_runs,
        "trailingCities": hidden,
        "systemPlanetRuns": system_runs,
        "nameAliases": MAP_NAME_ALIASES,
        "evidence": "proven",
        "note": (
            "磁碟城市順序與紙本星圖三元組逐筆吻合，且每顆行星的城市構成連續區間；"
            "表尾保留名稱之後的城市不屬於任何連續區間，另列為 trailing。"
        ),
    }


def crosscheck_planet_layout(
    planets: list[dict[str, object]], runs: list[dict[str, object]]
) -> None:
    """行星 word +7 解出來的城市區間，必須與紙本星圖推出來的 planetCityRuns 逐筆吻合。

    這是欄位命名的第二個獨立證據：位元切法來自 p-code 的 ``ldp`` 寬度與位移，區間內容
    來自原版紙本星圖。兩邊各自為政，18 筆全中才算數；對不上就 fail-closed。
    """
    if len(planets) != len(runs):
        raise ValueError(f"planet count {len(planets)} != city run count {len(runs)}")
    for planet, run in zip(planets, runs):
        if planet["name"] != run["planet"]:
            raise ValueError(f"planet {planet['name']} does not line up with run {run['planet']}")
        if planet["cityCount"] != run["count"]:
            raise ValueError(
                f"{planet['name']}: word +7 說有 {planet['cityCount']} 座城市，星圖說 {run['count']} 座"
            )
        if planet["firstCityIndex"] != run["start"]:
            raise ValueError(
                f"{planet['name']}: word +7 說城市從 {planet['firstCityIndex']} 起，星圖說 {run['start']}"
            )


def build_document(
    archive_path: Path, variant: str, map_path: Path
) -> dict[str, object]:
    nested_name, disk_name, disk = load_disk(archive_path, variant)
    region = disk[WORLD_REGION_START:WORLD_REGION_END]
    if len(region) != WORLD_REGION_BLOCKS * BLOCK_SIZE:
        raise ValueError("world region is truncated")

    systems = decode_table(disk, WORLD_REGION_START, SYSTEM_RECORD_SIZE, SYSTEM_COUNT, "system")
    planet_start = WORLD_REGION_START + SYSTEM_COUNT * SYSTEM_RECORD_SIZE
    planets = decode_table(disk, planet_start, PLANET_RECORD_SIZE, PLANET_COUNT, "planet")
    city_start = planet_start + PLANET_COUNT * PLANET_RECORD_SIZE
    cities = decode_table(disk, city_start, CITY_RECORD_SIZE, CITY_COUNT, "city")

    used = city_start + CITY_COUNT * CITY_RECORD_SIZE
    trailer = disk[used:WORLD_REGION_END]
    if any(byte for byte in trailer):
        raise ValueError(f"world region trailer at 0x{used:05x} is not zero-filled")

    pairs, triples = load_map_triples(map_path)
    grouping = crosscheck(systems, planets, cities, pairs, triples)
    pcode_rules = pin_pcode_rules(disk)
    crosscheck_planet_layout(planets, grouping["planetCityRuns"])

    return {
        "schema": 1,
        "tool": "tools/atari_st_world_data.py",
        "inputs": {
            "archive": {
                "name": archive_path.name,
                "sha256": sha256(archive_path.read_bytes()),
            },
            "nestedArchive": nested_name,
            "disk": {
                "name": disk_name,
                "size": len(disk),
                "sha256": sha256(disk),
            },
            "variant": variant,
        },
        "region": {
            "rawStart": f"0x{WORLD_REGION_START:05x}",
            "rawEnd": f"0x{WORLD_REGION_END:05x}",
            "blocks": WORLD_REGION_BLOCKS,
            "blockSize": BLOCK_SIZE,
            "sha256": sha256(region),
            "usedBytes": used - WORLD_REGION_START,
            "trailerBytes": len(trailer),
            "trailerIsZero": True,
            "locationSource": "laanwj/sundog doc/disklayout.md (secondary oracle, commit e56d8e2532134f0741a7f82b80f9357776779f27)",
            "locationEvidence": "proven（次級文件所載）",
            "recordBoundaryEvidence": "proven",
        },
        "tables": {
            "systems": {
                "rawOffset": f"0x{WORLD_REGION_START:05x}",
                "recordSize": SYSTEM_RECORD_SIZE,
                "recordCount": SYSTEM_COUNT,
                "nameFieldSize": NAME_FIELD_SIZE,
                "records": systems,
            },
            "planets": {
                "rawOffset": f"0x{planet_start:05x}",
                "recordSize": PLANET_RECORD_SIZE,
                "recordCount": PLANET_COUNT,
                "nameFieldSize": NAME_FIELD_SIZE,
                "records": planets,
            },
            "cities": {
                "rawOffset": f"0x{city_start:05x}",
                "recordSize": CITY_RECORD_SIZE,
                "recordCount": CITY_COUNT,
                "nameFieldSize": NAME_FIELD_SIZE,
                "records": cities,
            },
        },
        "grouping": grouping,
        "fieldSemantics": {
            "note": (
                "欄位語意是逐支讀 p-code 讀寫端得到的；每一條由一段位元組簽章釘在磁碟上，"
                "該簽章在整張映像裡必須恰好出現一次，且落點要與宣告的段基底＋段內位址相符。"
                "行星的城市區間另有第二個獨立證據：與紙本星圖推出的 planetCityRuns 逐筆吻合。"
            ),
            "evidence": "proven",
            "rules": pcode_rules,
        },
        "openQuestions": [
            {
                "topic": "system word +9 nibble 2",
                "status": "unknown",
                "detail": (
                    "星系 record 的 bytes 6–9（第 8、9 個 word）是八個 4-bit 欄位。座標、遭遇設定與"
                    "物價等級見本檔上方欄位，另有兩個欄位的地面消費端見 docs/atari-st/ground-rules.md；"
                    "只有第 9 個 word 的 nibble 2 至今找不到任何讀寫端。"
                ),
            },
            {
                "topic": "planet word +7 bit 15",
                "status": "unknown",
                "detail": (
                    "18 筆磁碟資料裡這個 bit 全為 0，XSTARTUP:init_planets 開新遊戲時又把它清成 0，"
                    "而全段 p-code 裡沒有第二處以 width 1／rightbit 15 存取這個 word。"
                    "沒有寫 1 的地方，也沒有讀取端。行星 record 的其餘欄位都已命名，見 fieldSemantics。"
                ),
            },
        ],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path, help="外層 Atari ST archive（唯讀）")
    parser.add_argument("--variant", default="[a2]", help="nested archive 變體標記")
    parser.add_argument(
        "--map-evidence",
        type=Path,
        default=Path("docs/apple2/world-map-crosscheck.json"),
        help="紙本星圖三元組證據檔",
    )
    parser.add_argument("--output", type=Path, help="輸出 JSON 路徑；未指定則印到 stdout")
    args = parser.parse_args()

    document = build_document(args.archive, args.variant, args.map_evidence)
    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
