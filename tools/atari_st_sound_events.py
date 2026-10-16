#!/usr/bin/env python3
"""把 Atari ST SunDog 的 28 段音效對回「哪個事件播它」。

`docs/atari-st/sound-bank.json` 解出了音效表本身（位移、長度、暫存器寫入），
但「哪個事件播哪一段」一直掛著未解，remake 的事件對應因此是自有的。這支工具補上：
掃過 `MAINLIB:0x03 fore_sound` 與 `MAINLIB:0x04 back_sound` 的全部呼叫點，把段號、
呼叫它的程序、以及那一小段位元組釘在磁碟上。

兩支入口都在 `MAINLIB`：`fore_sound(n)` 直接播，`back_sound(n)` 先把 n 存進
`MAINLIB_G6e5` 再轉呼叫 `fore_sound`——所以背景音是「會被記住的前景音」，
`back_sound(0)` 就是停掉背景音。`XDOUSERM:check_quit` 進離開對話前先讀 `G6e5` 存起來、
`back_sound(0)` 靜音，離開對話再播回去，是這個模型最直接的旁證。

**語意是人讀反組譯讀出來的**，工具做的是把位置釘住：每條規則帶一段位元組簽章，
要求它在整張磁碟映像裡恰好出現一次，且同一 segment 的規則要落在同一個基底上。

全部 50 個呼叫點裡，37 個帶常數段號（下表），另外 8 個是 `back_sound(0)`（停背景音），
剩下 5 個是同一支程序裡位元組完全相同的重複呼叫，併進同一條規則。
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

RULES: list[dict[str, object]] = [
    {
        "id": "windowli.find-menu-i-x-y.4",
        "procedure": "WINDOWLI:find_menu(i,x,y)",
        "segmentOffset": "0x0635",
        "signature": "a1a1 d410 2508 00c9 a40b 20a5 0604 7303",
        "sound": 4,
        "soundName": "menu_click",
        "call": "fore_sound",
        "summary": "選單命中一個項目",
    },
    {
        "id": "shiplib.update-movement.12",
        "procedure": "SHIPLIB:update_movement",
        "segmentOffset": "0x0a4c",
        "signature": "e69b 0443 c500 060c 7303",
        "sound": 12,
        "soundName": "ship_arrives",
        "call": "fore_sound",
        "summary": "星系內出現另一艘船",
    },
    {
        "id": "shiplib.update-attack.26",
        "procedure": "SHIPLIB:update_attack",
        "segmentOffset": "0x0d85",
        "signature": "1e34 d5a6 0135 6f1a 7303",
        "sound": 26,
        "soundName": "pirate_laser",
        "call": "fore_sound",
        "summary": "敵艦開火",
    },
    {
        "id": "shiplib.update-attack.5",
        "procedure": "SHIPLIB:update_attack",
        "segmentOffset": "0x0e11",
        "signature": "9a04 80e3 b2d4 1005 7303",
        "sound": 5,
        "soundName": "hit_shield",
        "call": "fore_sound",
        "summary": "打中護盾（護盾吸收得住）",
    },
    {
        "id": "shiplib.update-attack.6",
        "procedure": "SHIPLIB:update_attack",
        "segmentOffset": "0x0e21",
        "signature": "d904 80e3 8bfd 0006 7303",
        "sound": 6,
        "soundName": "hit_hull",
        "call": "fore_sound",
        "summary": "打穿護盾、打到船體",
    },
    {
        "id": "shiplib.update-attack.16",
        "procedure": "SHIPLIB:update_attack",
        "segmentOffset": "0x0f3c",
        "signature": "0419 00b0 a1d4 1d10 7303",
        "sound": 16,
        "soundName": "attack_siren",
        "call": "fore_sound",
        "summary": "進入被攻擊狀態",
    },
    {
        "id": "shiplib.update-levels.10",
        "procedure": "SHIPLIB:update_levels",
        "segmentOffset": "0x11de",
        "signature": "15d4 262e 80ff c40a 7303",
        "sound": 10,
        "soundName": "fuel_here",
        "call": "fore_sound",
        "summary": "燃料相關的水位變化",
    },
    {
        "id": "xpilotag.xpilotage.17",
        "procedure": "XPILOTAG:xpilotage",
        "segmentOffset": "0x051f",
        "signature": "6101 0171 048a 3c11 7303",
        "sound": 17,
        "soundName": "distress",
        "call": "fore_sound",
        "summary": "駕駛畫面",
    },
    {
        "id": "xdorepai.init-repairs.19",
        "procedure": "XDOREPAI:init_repairs",
        "segmentOffset": "0x0118",
        "signature": "2208 a3ee a601 0613 7304",
        "sound": 19,
        "soundName": "repair_bay",
        "call": "back_sound",
        "summary": "進入維修艙（背景音）",
    },
    {
        "id": "xdorepai.update-status-blast-component.7",
        "procedure": "XDOREPAI:update_status:blast_component",
        "segmentOffset": "0x03e9",
        "signature": "6807 0621 2072 2007 7303",
        "sound": 7,
        "soundName": "part_dies",
        "call": "fore_sound",
        "summary": "零件被炸毀",
    },
    {
        "id": "xshowmov.do-jump.20",
        "procedure": "XSHOWMOV:do_jump",
        "segmentOffset": "0x0319",
        "signature": "1006 d701 7871 1a14 7303",
        "sound": 20,
        "soundName": "warp_fail",
        "call": "fore_sound",
        "summary": "曲速跳躍",
    },
    {
        "id": "xdofight.fire-it.8",
        "procedure": "XDOFIGHT:fire_it",
        "segmentOffset": "0x01eb",
        "signature": "b2f1 10ad 06d4 0508 7303",
        "sound": 8,
        "soundName": "laser",
        "call": "fore_sound",
        "summary": "我方主砲開火",
    },
    {
        "id": "xdofight.fire-it.9",
        "procedure": "XDOFIGHT:fire_it",
        "segmentOffset": "0x01f0",
        "signature": "d405 0873 038a 0309 7303",
        "sound": 9,
        "soundName": "cannon",
        "call": "fore_sound",
        "summary": "我方另一種武器開火",
    },
    {
        "id": "xdofight.fire-it.14",
        "procedure": "XDOFIGHT:fire_it",
        "segmentOffset": "0x0238",
        "signature": "23a2 22b2 d55c 010e 7303",
        "sound": 14,
        "soundName": "hit_pirate",
        "call": "fore_sound",
        "summary": "打中敵艦",
    },
    {
        "id": "xdofight.fire-it.23",
        "procedure": "XDOFIGHT:fire_it",
        "segmentOffset": "0x0387",
        "signature": "25ed 8aac 00ef 0517 7303",
        "sound": 23,
        "soundName": "explosion",
        "call": "fore_sound",
        "summary": "敵艦爆炸",
    },
    {
        "id": "xdofight.do-tractor.1",
        "procedure": "XDOFIGHT:do_tractor",
        "segmentOffset": "0x0cc4",
        "signature": "1b02 e1d9 0480 d101 7304",
        "sound": 1,
        "soundName": "tractor",
        "call": "back_sound",
        "summary": "牽引光束（背景音）",
    },
    {
        "id": "xdofight.do-jettison.18",
        "procedure": "XDOFIGHT:do_jettison",
        "segmentOffset": "0x0da3",
        "signature": "80ec 21ee d702 6b12 7303",
        "sound": 18,
        "soundName": "jettison",
        "call": "fore_sound",
        "summary": "拋棄貨物",
    },
    {
        "id": "xdofight.do-comm.25",
        "procedure": "XDOFIGHT:do_comm",
        "segmentOffset": "0x0e9f",
        "signature": "9605 000f 7800 0019 7303",
        "sound": 25,
        "soundName": "hail",
        "call": "fore_sound",
        "summary": "通訊呼叫",
    },
    {
        "id": "xstartup.request-sundog.27",
        "procedure": "XSTARTUP:request_sundog",
        "segmentOffset": "0x0408",
        "signature": "8280 1a08 9d74 1400 6921 d403 1b73 03",
        "sound": 27,
        "soundName": "disk_warn",
        "call": "fore_sound",
        "summary": "要求換片",
    },
    {
        "id": "xstartup.utility-verified.27",
        "procedure": "XSTARTUP:utility:verified",
        "segmentOffset": "0x0c03",
        "signature": "8281 6508 9d73 1c1b 7303",
        "sound": 27,
        "soundName": "disk_warn",
        "call": "fore_sound",
        "summary": "換片／驗證提示",
    },
    {
        "id": "xstartup.utility-request-sun-dog.27",
        "procedure": "XSTARTUP:utility:request_sun_dog",
        "segmentOffset": "0x0d50",
        "signature": "8282 0508 9d74 1400 6921 d403 1b73 03",
        "sound": 27,
        "soundName": "disk_warn",
        "call": "fore_sound",
        "summary": "要求換片",
    },
    {
        "id": "xstartup.utility-library-warning.27",
        "procedure": "XSTARTUP:utility:library:warning",
        "segmentOffset": "0x0e67",
        "signature": "8282 1508 9d73 1c1b 7303",
        "sound": 27,
        "soundName": "disk_warn",
        "call": "fore_sound",
        "summary": "保存庫警告",
    },
    {
        "id": "xstartup.utility-library-load-lib-game-custom-read-menu.4",
        "procedure": "XSTARTUP:utility:library:load_lib_game:custom_read_menu",
        "segmentOffset": "0x128e",
        "signature": "a121 20b2 a1d4 3704 7303",
        "sound": 4,
        "soundName": "menu_click",
        "call": "fore_sound",
        "summary": "選單命中",
    },
    {
        "id": "xstartup.utility-select-name-write-messages.4",
        "procedure": "XSTARTUP:utility:select_name:write_messages",
        "segmentOffset": "0x24b5",
        "signature": "a214 a268 2073 3904 7303",
        "sound": 4,
        "soundName": "menu_click",
        "call": "fore_sound",
        "summary": "選單命中",
    },
    {
        "id": "xstartup.end-game-completion.4",
        "procedure": "XSTARTUP:end_game:completion",
        "segmentOffset": "0x29ee",
        "signature": "9b04 1f20 a773 1a04 7303",
        "sound": 4,
        "soundName": "menu_click",
        "call": "fore_sound",
        "summary": "結局畫面的提示音",
    },
    {
        "id": "xdointer.read-frame.4",
        "procedure": "XDOINTER:read_frame",
        "segmentOffset": "0x0fc8",
        "signature": "b3a1 d423 2101 c404 7303",
        "sound": 4,
        "soundName": "menu_click",
        "call": "fore_sound",
        "summary": "對話推進一格",
    },
    {
        "id": "xdocomba.its-ahit.11",
        "procedure": "XDOCOMBA:its_ahit",
        "segmentOffset": "0x0581",
        "signature": "a409 8a03 0da4 090b 7303",
        "sound": 11,
        "soundName": "stinger",
        "call": "fore_sound",
        "summary": "被 stinger 打中",
    },
    {
        "id": "xdocomba.its-ahit.2",
        "procedure": "XDOCOMBA:its_ahit",
        "segmentOffset": "0x0589",
        "signature": "7303 8a06 0da4 0902 7303",
        "sound": 2,
        "soundName": "nme_laser",
        "call": "fore_sound",
        "summary": "被對手的槍打中",
    },
    {
        "id": "xdocomba.fire-gun.5",
        "procedure": "XDOCOMBA:fire_gun",
        "segmentOffset": "0x06d2",
        "signature": "1cad 0aee a601 0a05 7303",
        "sound": 5,
        "soundName": "hit_shield",
        "call": "fore_sound",
        "summary": "地面戰鬥開槍",
    },
    {
        "id": "xdocomba.fire-gun.7",
        "procedure": "XDOCOMBA:fire_gun",
        "segmentOffset": "0x06da",
        "signature": "7303 ad0a 00d2 0c07 7303",
        "sound": 7,
        "soundName": "part_dies",
        "call": "fore_sound",
        "summary": "彈藥用盡",
    },
    {
        "id": "xdocomba.handle-player.7",
        "procedure": "XDOCOMBA:handle_player",
        "segmentOffset": "0x08ec",
        "signature": "039b 043f 0007 c807 7303",
        "sound": 7,
        "soundName": "part_dies",
        "call": "fore_sound",
        "summary": "手上的武器壞掉（同時把 MAINLIB_G3f[0] 設成 7）",
    },
    {
        "id": "xmoveong.handle-teleport-do-jump.21",
        "procedure": "XMOVEONG:handle_teleport:do_jump",
        "segmentOffset": "0x2306",
        "signature": "cb8f c415 0080 23cb 7303",
        "sound": 21,
        "soundName": "teleport_up",
        "call": "fore_sound",
        "summary": "傳送離開",
    },
    {
        "id": "xmoveong.handle-teleport-do-jump.22",
        "procedure": "XMOVEONG:handle_teleport:do_jump",
        "segmentOffset": "0x233b",
        "signature": "ed72 2616 0080 23cb 7303",
        "sound": 22,
        "soundName": "teleport_down",
        "call": "fore_sound",
        "summary": "傳送抵達",
    },
    {
        "id": "xmoveong.xmove-on-ground.13",
        "procedure": "XMOVEONG:xmove_on_ground",
        "segmentOffset": "0x2837",
        "signature": "0072 2a0d 0080 23cb 7304",
        "sound": 13,
        "soundName": "pod",
        "call": "back_sound",
        "summary": "在地面走動時的載具／環境背景音",
    },
    {
        "id": "xdotradi.set-demand.24",
        "procedure": "XDOTRADI:set_demand",
        "segmentOffset": "0x00d0",
        "signature": "d58b 0018 0080 23cb 7303",
        "sound": 24,
        "soundName": "phase_done",
        "call": "fore_sound",
        "summary": "交易階段算完",
    },
    {
        "id": "xmoveinb.init-building.13",
        "procedure": "XMOVEINB:init_building",
        "segmentOffset": "0x0b8a",
        "signature": "13d2 570d 0080 23cb 7304",
        "sound": 13,
        "soundName": "pod",
        "call": "back_sound",
        "summary": "進入型別 19 的建築（船艙）時的背景音",
    },
    {
        "id": "xslots.get-button.4",
        "procedure": "XSLOTS:get_button",
        "segmentOffset": "0x03c6",
        "signature": "6374 1104 0080 23cb 7303",
        "sound": 4,
        "soundName": "menu_click",
        "call": "fore_sound",
        "summary": "選單命中",
    },
    {
        "id": "xslots.roll-slots.3",
        "procedure": "XSLOTS:roll_slots",
        "segmentOffset": "0x05b0",
        "signature": "0073 1d03 0080 23cb 7303",
        "sound": 3,
        "soundName": "jackpot",
        "call": "fore_sound",
        "summary": "吃角子老虎中獎",
    },
]


# 這幾段沒有常數呼叫點：0 是「停背景音」，15 在 p-code 裡完全沒有觸發點。
SOUNDS_WITHOUT_A_CONSTANT_CALLER = {
    0: "back_sound(0)：停掉背景音",
    15: "整份 p-code 的呼叫點逐一交代過，沒有一個播它；碟外也沒有，見 openQuestions",
}


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


# ── 呼叫點普查 ──────────────────────────────────────────────────────────────
# 上面那張規則表是逐條人讀反組譯釘出來的。這一段做的是另一件事：把兩個入口的**呼叫
# 位元組**在整份 p-code 裡數一遍，再看哪些落在既有簽章之外。用途是回答「規則表漏掉了
# 哪些呼叫點」，而不是回答「那些呼叫點播的是第幾段」。
#
# 呼叫的位元組形式是從既有簽章 clock.task-table 反推的：那條規則已知是七個週期任務的
# case 表，位元組是 `73 35`（MAINLIB:0x35 the_time）、`75 12`、`73 32`…，也就是 **opcode
# 帶著段別**——0x70 呼叫 KERNEL、0x73 呼叫 MAINLIB、0x75 呼叫另一段。七個任務與文件記載
# 的七項逐一對上，所以 `73 03` 是 fore_sound、`73 04` 是 back_sound。
#
# **這一段不判斷引數。** p-code 是變長的，從呼叫位元組往回讀不成立：實測有一組慣用法
# `<sldc 段號> 00 80 23 cb` 夾在引數與呼叫之間，七個已釘住的規則都是這個形狀，段號是
# 常數但不緊貼呼叫。要可靠地認出引數得從程序進入點往前解碼，那需要一支真正的 p-code
# 反組譯器，本工具沒有。
SYSTEM_STARTUP_RANGE = (0x2E000, 0x2E000 + 106496)
FORE_SOUND_CALL = bytes((0x73, 0x03))
BACK_SOUND_CALL = bytes((0x73, 0x04))
# 夾在引數與呼叫之間的那組慣用法。單獨統計它，是因為它解釋了「為什麼不能往回讀」。
INTERPOSED_IDIOM = bytes.fromhex("008023cb")
CONTEXT_BYTES = 12

# 全域參照的兩個 opcode，以及它們的運算元編碼。
#
# `9a`／`9b` 後面接一個層級位元組，再接一個**變長**的全域編號：第一個位元組的最高位若
# 為 1，就再吃一個位元組，值是 ((first & 0x7f) << 8) | second。這條規則不是猜的——照它
# 解出來的編號會落在文件裡既有的名字上：`80 db` → `Gdb`（敵艦型號）、`80 e5` → `Ge5`、
# `81 0b` → `G10b`（對話參數塊）、`8a 74` → `Ga74`（凍結時間的鎖）、`86 e5` → `G6e5`
# （存起來的背景音）。全部簽章跑一遍共解出 76 種 (opcode, 層級, 編號)。
#
# 這條規則有一個直接後果：**一個看起來像呼叫的位元組，可能只是全域編號的第二個位元組。**
GLOBAL_REFERENCE_OPCODES = (0x9A, 0x9B)
GLOBAL_REFERENCE_HEADER = 3  # opcode、層級、變長編號的第一個位元組


def is_a_global_number_byte(disk: bytes, position: int) -> bool:
    """這個位元組是不是某個全域編號的第二個位元組（而不是一個 opcode）。"""
    if position < GLOBAL_REFERENCE_HEADER:
        return False
    return (
        disk[position - GLOBAL_REFERENCE_HEADER] in GLOBAL_REFERENCE_OPCODES
        and bool(disk[position - 1] & 0x80)
    )


def find_all(disk: bytes, needle: bytes, span: tuple[int, int]) -> list[int]:
    low, high = span
    found: list[int] = []
    position = low - 1
    while True:
        position = disk.find(needle, position + 1)
        if position < 0 or position >= high:
            return found
        found.append(position)


def census_call_sites(
    disk: bytes, rules: list[dict], span: tuple[int, int] = SYSTEM_STARTUP_RANGE
) -> dict[str, object]:
    """數出兩個入口的呼叫位元組，並標出落在既有簽章之外的那些。"""
    covered = []
    for rule in rules:
        offset = int(str(rule["rawOffset"]), 16)
        length = len(signature_bytes(str(rule["signature"])))
        covered.append((offset, offset + length))

    def inside_a_signature(position: int) -> bool:
        return any(low <= position < high for low, high in covered)

    result: dict[str, object] = {}
    for name, call in (("foreSound", FORE_SOUND_CALL), ("backSound", BACK_SOUND_CALL)):
        sites = find_all(disk, call, span)
        outside = [site for site in sites if not inside_a_signature(site)]
        result[name] = {
            "callBytes": call.hex(" "),
            "occurrences": len(sites),
            "insideAPinnedSignature": len(sites) - len(outside),
            "outside": [
                {
                    "rawOffset": f"0x{site:05x}",
                    "precedingBytes": disk[site - CONTEXT_BYTES : site].hex(" "),
                    "isAGlobalNumberByte": is_a_global_number_byte(disk, site),
                }
                for site in outside
            ],
        }
    idiom = find_all(disk, INTERPOSED_IDIOM, span)
    result["interposedIdiom"] = {
        "bytes": INTERPOSED_IDIOM.hex(" "),
        "sites": len(idiom),
        "followedByACall": sum(
            1 for site in idiom if disk[site + len(INTERPOSED_IDIOM) : site + len(INTERPOSED_IDIOM) + 1] == b"\x73"
        ),
        "note": (
            "這組位元組每一次出現都緊接著一個 MAINLIB 呼叫。七個已釘住的規則是同一個形狀，"
            "而它們的段號是**押在這組位元組之前**的常數。這就是「不能從呼叫位元組往回讀引數」"
            "的直接證據——引數與呼叫之間隔著別的東西，隔多少要解碼才知道。"
        ),
    }
    result["notACallAtAll"] = sum(
        1
        for name in ("foreSound", "backSound")
        for entry in result[name]["outside"]
        if entry["isAGlobalNumberByte"]
    )
    result["whatThisDoesNotShow"] = (
        "本普查只數位元組樣式，不解碼。落在簽章之外的位置有兩種可能：規則表沒收的合法"
        "呼叫點，或者根本不是呼叫（位元組撞上了別的指令的運算元）。要分辨得靠從程序進入點"
        "往前解碼。另外「簽章之內」只比對**本檔**的規則——例如 back_sound 有一處落在 "
        "ship-rules.json 的 tractor.wait-loop 簽章裡，本檔會把它算成簽章之外。"
    )
    return result


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)

    rules = []
    for rule in RULES:
        matches = locate(disk, str(rule["signature"]))
        if len(matches) != EXPECTED_MATCHES:
            raise ValueError(
                f"rule {rule['id']} signature matched {len(matches)} times, want {EXPECTED_MATCHES}"
            )
        entry = dict(rule)
        entry["rawOffset"] = f"0x{matches[0]:05x}"
        rules.append(entry)

    bases: dict[str, int] = {}
    for rule in rules:
        segment = str(rule["procedure"]).split(":")[0]
        base = int(str(rule["rawOffset"]), 16) - int(str(rule["segmentOffset"]), 16)
        if segment in bases and bases[segment] != base:
            raise ValueError(
                f"segment {segment} resolves to base 0x{bases[segment]:05x} and 0x{base:05x}"
            )
        bases[segment] = base

    used = sorted({int(str(rule["sound"])) for rule in rules})
    return {
        "schema": 1,
        "tool": "tools/atari_st_sound_events.py",
        "segmentBases": {segment: f"0x{base:05x}" for segment, base in sorted(bases.items())},
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "entryPoints": {
            "fore": "MAINLIB:0x03 fore_sound(n)",
            "back": "MAINLIB:0x04 back_sound(n)：存進 MAINLIB_G6e5 後轉呼叫 fore_sound",
        },
        "whatThisProves": (
            "每條規則對應的位元組確實存在於這張磁碟上，且在整張映像裡唯一。"
            "事件語意是人讀反組譯得到的，寫在 docs/atari-st/sound-events.md；本檔只把它們釘住。"
        ),
        "evidence": "proven（位置與唯一性）",
        "semanticsEvidence": "proven（自原版 p-code 靜態讀出，未經執行對拍）",
        "soundsTriggered": used,
        "callSiteCensus": census_call_sites(disk, rules),
        "soundsWithoutAConstantCaller": {
            str(index): note for index, note in SOUNDS_WITHOUT_A_CONSTANT_CALLER.items()
        },
        "rules": rules,
        "openQuestions": [
            {
                "topic": "sound 15",
                "status": "proven（有界）：p-code 播不到它",
                "detail": (
                    "第 15 段（sound-bank.json 記為 disk_dead）在整份 p-code 裡沒有任何一條路徑會播到。"
                    "2026-08-23 把兩個入口的呼叫位元組數過一遍：54 個位置，38 個落在既有簽章之內"
                    "（那 38 條規則涵蓋第 1–14、16–27 段，就是沒有 15）。其餘 16 個逐一交代："
                    "**3 個根本不是呼叫**——`9b 04 87 73` 的 `73` 是全域編號 0x773 的第二個位元組，"
                    "不是 opcode；6 個是 `back_sound(0)`（停背景音）；3 個是 XSTARTUP 的第 27 段；"
                    "1 個是 WINDOWLI 的第 4 段；1 個落在 ship-rules.json 的 tractor.wait-loop 簽章裡；"
                    "剩下 2 個是**轉手**而不是新的來源——MAINLIB 的 back_sound 把自己的參數轉呼叫 "
                    "fore_sound，XDOUSERM 的 check_quit 把存在 G6e5 的背景音放回去。轉手只會重播"
                    "別處已經傳進來的編號，不會生出新的。"
                ),
            },
            {
                "topic": "sound 15 的碟外可能性",
                "status": "proven（有界）",
                "detail": (
                    "整張磁碟只有兩個 Dosound（XBIOS 32，3f3c 0020 4e4e）呼叫點："
                    "一個在 SYSTEM.STARTUP 裡（已知的 do_sound），另一個在開機低區 0x027e6，"
                    "它 pea $0ea4 餵的是自己內嵌在低位記憶體的一段資料，不是這張音效 bank。"
                    "第 15 段那 50 個位元組在整張碟上只出現一次（0x045ce），沒有第二份複本。"
                    "SYSTEM.PASCAL 與 SYSTEM.INTERP 也在這次 Dosound 普查範圍內，零命中。"
                ),
            },
            {
                "topic": "sound names",
                "status": "strong inference",
                "detail": (
                    "sound-bank.json 的段名先前是推測的。本檔的呼叫點與其中大多數吻合，"
                    "但第 20 段名為 warp_fail、實際由 XSHOWMOV:0x09 do_jump 在跳躍時播，"
                    "名稱可能偏窄。段號同時服務多個事件是常態（28 段而已），不算矛盾。"
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
