#!/usr/bin/env python3
"""把 SunDog 對話 VM 的條件、態度與前後置碼釘在磁碟上，連同它夾帶的防拷檢查。

`dialog-frames.json` 解出了 frame 的版面、`dialog-rendered.json` 解出了句子，這一支管的是
**中間那一層**：frame 怎麼決定要說哪一句、態度怎麼變、以及 `new_frame` 每次順手做的那件
與對話無關的事。

三件事值得先講清楚：

- **`XDOINTER:0x06 passed(x)` 是條件求值器**，不是布林旗標。參數編碼成「選哪一項檢定 +
  參數」，共 21 種。底層兩支：`made_roll(t)` ＝ `random() mod 100 < t`，
  `made_attr(n, m)` ＝ `made_roll(MAINLIB_G35[n] + m)`；**`n == 4` 時用完會把該屬性折半**。
- **`XDOINTER_01_L1b` 是 NPC 對你的態度**，0–4，由 `handle_precode` 的 2-bit 欄位維護。
  對話裡那個 `[laser brain|laser brain|mac|mac|sir]` 就是它。
- **`XDOINTER:0x22 check_sects` 與對話無關**，名字有誤導性：它重讀磁碟 block 1 與 6、
  算校驗和，再交給 `MAINLIB:0x34 copy_check` 對帳。累計三次不符就把 `GEMBIND_G238` 設成
  `0x4d2`，而 `SUNDOG:0x04 do_ground` 一看到那個值就 `quit`——**延後發作的防拷**。
  這解釋了 `do_ground` 裡那個先前看不出用途的離開判斷。

語意是人讀反組譯讀出來的；工具做的是把位置釘住：每條規則帶一段位元組簽章，要求它在整張
磁碟映像裡恰好出現一次，且同一 segment 的規則要落在同一個基底上。
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
        "id": "attributes.five-stats",
        "procedure": "XDOUSERM:0x0d show_stats",
        "segmentOffset": "0x0373",
        "signature": (
            "6182 802a eb01 0f00 6820 04b2 d45b 8280 3008 9d73 1b61 9884 0a01 2002 8ca2 0276"
            "0484 0a98 731b 8280 3108 9d73 1b20 04b3 f112 9b04 3520 a703 0073 1d82 8032 089d"
            "731b 8a07 8280 3408 9d73 1b9b 0435 2005 a2a7 0300 731d 2004 b3"
        ),
        "summary": (
            "五項屬性的標籤是常數字串 STIQDXCHLK 兩字一組：ST、IQ、DX、CH、LK。"
            "MAINLIB_G35[i] 是目前值、G35[i+5] 是上限；LK 的目前值印成 ??，玩家看不到。"
            "所以 made_attr(3, m) 擲的是魅力，made_attr(4, m) 擲的是運氣。"
        ),
    },
    {
        "id": "attributes.ground-combat",
        "procedure": "XDOCOMBA:0x08 init_people",
        "segmentOffset": "0x01ec",
        "signature": "24e7 029b 043d 02a7 9b04 3500 a78c 805a 8dc4 24e7 019b 0435 02a7 058d 03a2 c4",
        "summary": (
            "地面戰鬥裡玩家的血量 = MAINLIB_G3d[2]（健康）× ST / 90，速度 = DX / 5 + 3。"
            "對手那一側完全不看這些，只由型別決定人數與有沒有槍。"
        ),
    },
    {
        "id": "dialogue.made-roll",
        "procedure": "XDOINTER:0x07 passed:made_roll",
        "segmentOffset": "0x0206",
        "signature": "0073 0b80 648f 20b3 9f69",
        "summary": "made_roll(t) = random() mod 100 < t，一個百分比檢定。",
    },
    {
        "id": "dialogue.made-attr",
        "procedure": "XDOINTER:0x08 passed:made_attr",
        "segmentOffset": "0x0216",
        "signature": "9b04 3522 a768 0020 21a2 ef07 6b22 04d2 089b 0435 2220 028d c8",
        "summary": (
            "made_attr(n, m) = made_roll(MAINLIB_G35[n] + m)。n == 4（運氣）時，"
            "檢定完會把 G35[4] 折半——運氣是可消耗的。"
        ),
    },
    {
        "id": "dialogue.precode-bitfield",
        "procedure": "XDOINTER:0x19 set_frame:handle_precode",
        "segmentOffset": "0x08ee",
        "signature": "60f0 0e20 d404 01a6 020e 2002 8dd4 0401 a602 0920 048d 6820 048f",
        "summary": (
            "前置碼是位元欄位不是操作碼：bit 0 設 Le、bit 1 設 L9（pick_one 用來重用上一次的"
            "選擇）、bits 2–3 是態度運算（1 加、2 減、3 設成 4，隨後夾在 0–4）、bit 4 清掉目前"
            "談的物品、bit 5 呼叫 set_up_item、bits 6–7 是金錢運算。"
        ),
    },
    {
        "id": "dialogue.postcode-offer",
        "procedure": "XDOINTER:0x1b set_frame:call_postcode",
        "segmentOffset": "0x0a01",
        "signature": "8802 4a00 ae1a 0a04 ae1b a38c 0073 0b80 408f a2ed 8c73 0d",
        "summary": (
            "後置碼操作碼 0 開出一個金額：價格倍率 × (10 × (4 − 態度) + random() mod 64 + 1)。"
            "態度越好開價越低。"
        ),
    },
    {
        "id": "dialogue.postcode-charges",
        "procedure": "XDOINTER:0x1b set_frame:call_postcode",
        "segmentOffset": "0x0a45",
        "signature": "ae83 640f d210 ae21 0001 0773 0c80 208c a2a6 0221 8a08 ae21 80e0 a2a6 0221",
        "summary": (
            "後置碼操作碼 1 給要談的物品充能次數：物品 11（shield）與 13（scattergun）拿 "
            "rand(1,7) × 32，其餘一律 + 0xe0（7 次）。這是「口袋位元組高 3 位是剩餘次數」的"
            "第二個獨立證據。"
        ),
    },
    {
        "id": "protection.check-sects",
        "procedure": "XDOINTER:0x22 new_frame:check_sects",
        "segmentOffset": "0x0e92",
        "signature": (
            "0069 21ed 6904 8802 6400 0023 0270 1200 701e 00b0 6a04 8802 6400 0004 0270 1222"
            "0070 1e00 b0a1 6a22 2103 b0a0 d4d4 22f1 0882 8078 089d 0173 44"
        ),
        "summary": (
            "名字有誤導性：這一支與對話的 section 無關。它用 unitread 重讀指定的磁碟 block "
            "與 block 4，最多三次；三次都失敗就喊 Fatal disk error 1，成功就對整個 block 算校驗和。"
        ),
    },
    {
        "id": "protection.new-frame-hook",
        "procedure": "XDOINTER:0x1f new_frame",
        "segmentOffset": "0x0f55",
        "signature": "9b04 83b1 0190 229b 0483 9206 9022 8137 ccd9 0382 38",
        "summary": (
            "每載入一個對話 frame 就重算一次：block 1 的校驗和進 MAINLIB_G3b1、block 6 的"
            "進 G392，然後把 GEMBIND_G238 設成 0xcc37 當「剛做過這個檢查」的記號。"
        ),
    },
    {
        "id": "protection.copy-check",
        "procedure": "MAINLIB:0x34 copy_check",
        "segmentOffset": "0x0ac4",
        "signature": (
            "9a03 8238 819d 12d2 3485 8a73 81b7 0ab1 8583 b181 3a01 b1a0 8583 b185 8392 b0a0"
            "d409 8583 aced a583 ac8a 0400 a583 ac81 3a01 a583 b181 2001 a583 928a 3b9a 0382"
            "3881 37cc d232 8583 b181 3a01 b185 8392 8120 01b1 a085 83b1 8583 92b0 a0d4 0985"
            "83ab eda5 83ab 8a04 00a5 83ab 813a 01a5 83b1 8120 01a5 8392 8583 ad85 83ac a285"
            "83ab a203 b3d4 0981 d204 d903 8238 8a07 811a f7d9 0382 38"
        ),
        "summary": (
            "對帳：block 1 的校驗和必須是 0x13a、block 6 的必須是 0x120，兩者不得相等，"
            "另有 MAINLIB_Ga73 == 0xab7。不符就把對應的計數器加一，符合就歸零。"
            "三個計數器（G3ab／G3ac／G3ad）加起來大於等於 3 時把 GEMBIND_G238 設成 0x4d2。"
        ),
    },
    {
        "id": "protection.deferred-quit",
        "procedure": "SUNDOG:0x04 do_ground",
        "segmentOffset": "0x02a3",
        "signature": "9a03 8238 81d2 04d2 0273 059b 0432 0408 c902 b29b 0432 0408 c90b b3",
        "summary": (
            "主迴圈每一輪檢查 GEMBIND_G238，等於 0x4d2 就 MAINLIB:0x05 quit。"
            "發作的地點離檢查的地點很遠，這是延後懲罰式的防拷。"
            "同樣的守衛在 SUNDOG:0x01d6 還有一份。"
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

    return {
        "schema": 1,
        "tool": "tools/atari_st_dialog_vm.py",
        "segmentBases": {segment: f"0x{base:05x}" for segment, base in sorted(bases.items())},
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "whatThisProves": (
            "每條規則對應的位元組確實存在於這張磁碟上，且在整張映像裡唯一。"
            "語意是人讀反組譯得到的，寫在 docs/atari-st/dialog-rendered.md；本檔只把它們釘住。"
        ),
        "evidence": "proven（位置與唯一性）",
        "semanticsEvidence": "proven（自原版 p-code 靜態讀出，未經執行對拍）",
        "rules": rules,
        "openQuestions": [
            {
                "topic": "attributes in dialogue",
                "status": "proven",
                "detail": (
                    "passed 的 21 種檢定逐條讀完，只用到 G35[3]（CH）與 G35[4]（LK）——"
                    "ST／IQ／DX 在對話裡沒有檢定。這不是缺口而是**有界否定觀察**：那三項用在別處，"
                    "地面戰鬥的玩家血量是 健康 × ST / 90、速度是 DX / 5 + 3。"
                ),
            },
            {
                "topic": "MAINLIB_G3ad",
                "status": "unknown",
                "detail": "copy_check 把它算進三個計數器的總和，但自己不寫它；寫入端未追。",
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
