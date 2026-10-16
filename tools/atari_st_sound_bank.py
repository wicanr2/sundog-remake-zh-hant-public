#!/usr/bin/env python3
"""解 Atari ST SunDog 的音效指令表。

磁碟上放的不是波形，是 XBIOS 32（Dosound）的暫存器指令流：``MAINLIB:0x03 fore_sound``
從一張位移表算出第 n 段的長度，把那段 bytes 複製進緩衝區，交給 ``GEMBIND:0x08 do_sound``
（其原生碼是 ``move.w #$20,-(sp)`` 後接 ``trap #14``）。

位移表在 raw ``0x04400``，36 個 big-endian word；資料緊接在 ``0x04448``。表尾以「不再遞增」
收束，其後為零。另有三段 warp 音效不在表內，是 SHIPLIB 裡連續三段字面量，在 raw ``0x3441a``；
第一段沒有終止指令，靠掃頻上限收尾，遊戲改寫它的第 26 個 byte 來換 warp 速度。

**輸出不含磁碟 bytes**：只記 raw offset、長度、用到哪些暫存器、指令統計與播放長度。
原始音訊素材屬於使用者自己的磁片，任何時候都不進版控。

每段都必須完整 parse 成 Dosound 指令流才會被收錄，parse 不過就 fail-closed：
位址記錯或換了一份磁碟時，得到的是錯誤而不是雜訊。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


SOUND_TABLE_OFFSET = 0x04400
SOUND_TABLE_WORDS = 36
SOUND_DATA_OFFSET = SOUND_TABLE_OFFSET + SOUND_TABLE_WORDS * 2

WARP_OFFSET = 0x3441A
WARP_LENGTHS = (28, 26, 28)
WARP_SPEED_BYTE = 26

FRAME_RATE = 50
MAX_SWEEP_CYCLES = 256

# 名稱取自 secondary oracle laanwj/sundog 的 tools/extract_sounds.py，
# 那份是在遊戲中逐一聽出來的；本工具不宣稱自己驗證過每個名稱的語意。
SOUND_NAMES = [
    "off", "tractor", "nme_laser", "jackpot", "menu_click", "hit_shield",
    "hit_hull", "part_dies", "laser", "cannon", "fuel_here", "stinger",
    "ship_arrives", "pod", "hit_pirate", "disk_dead", "attack_siren",
    "distress", "jettison", "repair_bay", "warp_fail", "teleport_up",
    "teleport_down", "explosion", "phase_done", "hail", "pirate_laser",
    "disk_warn",
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


def parse_list(data: bytes) -> dict[str, object]:
    """走完一段 Dosound 指令流，回報它用到什麼、播多久。

    指令集只有四種形狀，所以位址錯掉的那一段幾乎一定會走出界或撞到不存在的指令。
    """
    cursor = 0
    registers: set[int] = set()
    frames = 0
    writes = sweeps = waits = 0
    terminated = False

    while cursor < len(data):
        command = data[cursor]
        cursor += 1
        if command < 0x10:
            if cursor >= len(data):
                raise ValueError(f"register write at {cursor - 1} has no value")
            registers.add(command)
            cursor += 1
            writes += 1
        elif command == 0x80:
            if cursor >= len(data):
                raise ValueError(f"0x80 at {cursor - 1} has no value")
            cursor += 1
        elif command == 0x81:
            if cursor + 2 >= len(data):
                raise ValueError(f"0x81 at {cursor - 1} is truncated")
            register, step, end = data[cursor], data[cursor + 1], data[cursor + 2]
            if register > 0x0F:
                raise ValueError(f"0x81 at {cursor - 1} names register {register}")
            registers.add(register)
            cursor += 3
            sweeps += 1
            # 掃頻每幀一步，走到 end 為止；走不到就停在上限。
            frames += sweep_frames(step, end)
        elif command >= 0x82:
            if cursor >= len(data):
                raise ValueError(f"wait at {cursor - 1} has no count")
            count = data[cursor]
            cursor += 1
            waits += 1
            if count == 0:
                terminated = True
                break
            frames += count
        else:
            raise ValueError(f"unknown command 0x{command:02x} at {cursor - 1}")

    return {
        "registers": sorted(registers),
        "registerWrites": writes,
        "sweeps": sweeps,
        "waits": waits,
        "terminated": terminated,
        "frames": frames,
        "seconds": round(frames / FRAME_RATE, 3),
    }


def sweep_frames(step: int, end: int) -> int:
    """掃頻要跑幾幀。步進走不到 end 的那些是故意的，靠上限收尾。"""
    value = 0
    for cycle in range(1, MAX_SWEEP_CYCLES + 1):
        value = (value + step) & 0xFF
        if value == end:
            return cycle
    return MAX_SWEEP_CYCLES


def read_table(disk: bytes) -> list[int]:
    offsets = []
    for index in range(SOUND_TABLE_WORDS):
        base = SOUND_TABLE_OFFSET + index * 2
        offsets.append(int.from_bytes(disk[base:base + 2], "big"))
    if offsets[0] != 0:
        raise ValueError(f"sound table starts at {offsets[0]}, want 0")
    last = 0
    while last + 1 < len(offsets) and offsets[last + 1] > offsets[last]:
        last += 1
    if last < 8:
        raise ValueError(f"sound table holds only {last} entries")
    return offsets[:last + 1]


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    nested_name, disk_name, disk = load_disk(archive_path, variant)
    offsets = read_table(disk)

    sounds = []
    for index in range(len(offsets) - 1):
        start, end = offsets[index], offsets[index + 1]
        body = disk[SOUND_DATA_OFFSET + start:SOUND_DATA_OFFSET + end]
        entry = parse_list(body)
        entry.update({
            "index": index,
            "name": SOUND_NAMES[index] if index < len(SOUND_NAMES) else f"sound{index}",
            "tableOffset": start,
            "rawOffset": f"0x{SOUND_DATA_OFFSET + start:05x}",
            "bytes": end - start,
        })
        sounds.append(entry)

    warps = []
    cursor = WARP_OFFSET
    for name, length in zip(("warp1", "warp2", "warp3"), WARP_LENGTHS):
        body = disk[cursor:cursor + length]
        entry = parse_list(body)
        entry.update({
            "name": name,
            "rawOffset": f"0x{cursor:05x}",
            "bytes": length,
        })
        if name != "warp1" and not entry["terminated"]:
            raise ValueError(f"{name} does not end with a terminator")
        warps.append(entry)
        cursor += length

    return {
        "schemaVersion": 1,
        "title": "Atari ST SunDog 音效指令表",
        "evidenceLevel": (
            "位移表位置與 36 word 長度、資料起點、warp 三段位置皆由本工具在磁碟上驗證："
            "表首為 0、遞增至結束、每一段都完整 parse 成 Dosound 指令流。"
            "音效名稱來自 secondary oracle，未經本專案獨立驗證。"
        ),
        "source": {
            "archive": archive_path.name,
            "archiveSha256": sha256(archive_path.read_bytes()),
            "variant": variant,
            "nested": nested_name,
            "disk": disk_name,
            "diskSha256": sha256(disk),
        },
        "layout": {
            "tableOffset": f"0x{SOUND_TABLE_OFFSET:05x}",
            "tableWords": SOUND_TABLE_WORDS,
            "dataOffset": f"0x{SOUND_DATA_OFFSET:05x}",
            "dataBytes": offsets[-1],
            "warpOffset": f"0x{WARP_OFFSET:05x}",
            "warpLengths": list(WARP_LENGTHS),
            "warpSpeedByte": WARP_SPEED_BYTE,
        },
        "consumer": {
            "foreSound": "MAINLIB:0x03 fore_sound：長度取 G53f[n+1] - G53f[n]，來源 G563 + G53f[n]",
            "backSound": "MAINLIB:0x04 back_sound：把參數存進 MAINLIB_G6e5 後轉呼叫 fore_sound",
            "doSound": "GEMBIND:0x08 do_sound：原生 68000，move.w #$20,-(sp) 後 trap #14 即 XBIOS 32",
        },
        "counts": {"sounds": len(sounds), "warpLists": len(warps)},
        "sounds": sounds,
        "warpSounds": warps,
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
