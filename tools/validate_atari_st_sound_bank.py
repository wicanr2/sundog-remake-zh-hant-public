#!/usr/bin/env python3
"""驗證音效指令表 evidence 檔的自洽性，以及 Go 實作是否用同一組位址。

不讀原始磁碟，只看已提交的 JSON 與原始碼：

- 位移表首尾相接：每段的 rawOffset 等於 dataOffset 加上前面所有段的長度總和。
- 每段都用到合法的暫存器（0–15）。多數段落收在掃頻或等待指令上而不是終止指令——
  引擎播完會再接一次 0 號音效（``off``）把聲音關掉——所以只要求 ``off`` 自己收尾。
- warp 三段在磁碟上連續，總長與宣告的長度一致。
- ``internal/source/atarist/originalmedia/sound.go`` 裡的位址常數與 evidence 檔相同。
  位址寫在兩個地方，這裡擋的是「只改了一邊」。
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


def check(document: dict, source: str) -> list[str]:
    problems: list[str] = []
    layout = document["layout"]
    data_offset = int(layout["dataOffset"], 16)

    cursor = 0
    for sound in document["sounds"]:
        expected = data_offset + cursor
        actual = int(sound["rawOffset"], 16)
        if actual != expected:
            problems.append(
                f"音效 {sound['index']} 落在 0x{actual:05x}，接續前一段應為 0x{expected:05x}"
            )
        if sound["tableOffset"] != cursor:
            problems.append(
                f"音效 {sound['index']} 的表內位移是 {sound['tableOffset']}，應為 {cursor}"
            )
        for register in sound["registers"]:
            if not 0 <= register <= 15:
                problems.append(f"音效 {sound['index']} 動到不存在的暫存器 {register}")
        cursor += sound["bytes"]

    stopper = document["sounds"][0]
    if stopper["name"] != "off" or not stopper["terminated"]:
        problems.append("0 號音效應為 off 且自己以終止指令收尾")

    if cursor != layout["dataBytes"]:
        problems.append(f"音效總長 {cursor}，宣告為 {layout['dataBytes']}")
    if document["counts"]["sounds"] != len(document["sounds"]):
        problems.append("音效筆數與 counts 不一致")

    warp_cursor = int(layout["warpOffset"], 16)
    for warp, length in zip(document["warpSounds"], layout["warpLengths"]):
        actual = int(warp["rawOffset"], 16)
        if actual != warp_cursor:
            problems.append(f"{warp['name']} 落在 0x{actual:05x}，應為 0x{warp_cursor:05x}")
        if warp["bytes"] != length:
            problems.append(f"{warp['name']} 長 {warp['bytes']} bytes，宣告為 {length}")
        if warp["name"] != "warp1" and not warp["terminated"]:
            problems.append(f"{warp['name']} 沒有終止指令")
        warp_cursor += length

    constants = {
        "soundTableOffset": int(layout["tableOffset"], 16),
        "soundTableWords": layout["tableWords"],
        "warpSoundOffset": int(layout["warpOffset"], 16),
        "warpSpeedByte": layout["warpSpeedByte"],
    }
    for name, want in constants.items():
        match = re.search(rf"^\s*{name}\s*=\s*(0x[0-9a-fA-F]+|\d+)", source, re.MULTILINE)
        if match is None:
            problems.append(f"sound.go 裡找不到常數 {name}")
            continue
        got = int(match.group(1), 0)
        if got != want:
            problems.append(f"sound.go 的 {name} 是 {got}，evidence 檔是 {want}")

    lengths = re.findall(r"warp(?:One|Two|Three)Length\s*=\s*(\d+)", source)
    if [int(value) for value in lengths] != list(layout["warpLengths"]):
        problems.append(f"sound.go 的 warp 長度是 {lengths}，evidence 檔是 {layout['warpLengths']}")

    return problems


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--evidence", type=Path, default=Path("docs/atari-st/sound-bank.json")
    )
    parser.add_argument(
        "--source",
        type=Path,
        default=Path("internal/source/atarist/originalmedia/sound.go"),
    )
    args = parser.parse_args()

    document = json.loads(args.evidence.read_text(encoding="utf-8"))
    problems = check(document, args.source.read_text(encoding="utf-8"))
    if problems:
        for problem in problems:
            print(f"FAIL: {problem}")
        sys.exit(1)
    print(
        f"OK: {document['counts']['sounds']} 段音效與 "
        f"{document['counts']['warpLists']} 段 warp 指令流首尾相接，Go 常數一致。"
    )


if __name__ == "__main__":
    main()
