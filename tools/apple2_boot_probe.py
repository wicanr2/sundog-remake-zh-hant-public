#!/usr/bin/env python3
"""抽出 Apple II 16-sector boot code 的 raw／CPU 位址證據。

來源 image 不會被重排或寫回。sector 0,14,13,12 是本作目前的 boot extraction
candidate；工具會保留每段的來源 raw offset，並把線性 6502 decode 與真正的
emulator boot 狀態分開。
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

try:
    from tools.apple2_dsk_inventory import SECTOR_SIZE, load_input
    from tools.disassemble_6502 import disassemble
except ModuleNotFoundError:  # direct ``python tools/...`` invocation
    from apple2_dsk_inventory import SECTOR_SIZE, load_input
    from disassemble_6502 import disassemble


BOOT_PHYSICAL_SECTORS = (0, 14, 13, 12)
BOOT_LOAD_ADDRESS = 0x0800
DISK_SIZE = 35 * 16 * SECTOR_SIZE
MARKERS = (b"MUST BOOT FROM SLOT 6", b"SYSTEM.PASCAL", b"RETURN")


def extract_boot(data: bytes) -> bytes:
    if len(data) != DISK_SIZE:
        raise ValueError(f"expected 143360-byte 16-sector image, got {len(data)}")
    return b"".join(
        data[physical * SECTOR_SIZE : (physical + 1) * SECTOR_SIZE]
        for physical in BOOT_PHYSICAL_SECTORS
    )


def source_raw_offset(boot_offset: int) -> int:
    segment, within_sector = divmod(boot_offset, SECTOR_SIZE)
    if segment >= len(BOOT_PHYSICAL_SECTORS):
        raise ValueError(f"boot offset outside extraction: 0x{boot_offset:x}")
    return BOOT_PHYSICAL_SECTORS[segment] * SECTOR_SIZE + within_sector


def marker_evidence(boot: bytes) -> list[dict[str, object]]:
    matches: list[dict[str, object]] = []
    for marker in MARKERS:
        start = 0
        while True:
            offset = boot.find(marker, start)
            if offset < 0:
                break
            matches.append(
                {
                    "marker": marker.decode("ascii"),
                    "bootBufferOffset": f"0x{offset:04x}",
                    "sourceRawOffset": f"0x{source_raw_offset(offset):04x}",
                    "length": len(marker),
                    "rawHex": marker.hex(" "),
                    "evidenceLevel": "proven-bytes",
                    "consumerStatus": "unknown",
                }
            )
            start = offset + 1
    return matches


def io_instruction_evidence(boot: bytes) -> list[dict[str, object]]:
    rows = disassemble(boot, 0, len(boot), BOOT_LOAD_ADDRESS)
    result: list[dict[str, object]] = []
    for row in rows:
        operand = str(row.get("operand", ""))
        if "$C0" not in operand:
            continue
        offset = int(row["rawOffset"])
        result.append(
            {
                "bootBufferOffset": f"0x{offset:04x}",
                "sourceRawOffset": f"0x{source_raw_offset(offset):04x}",
                "cpuAddress": f"0x{int(row['cpuAddress']):04x}",
                "bytes": row["bytes"],
                "mnemonic": row["mnemonic"],
                "operand": operand,
                "decodeStatus": row["status"],
                "semanticStatus": "disk-io-address-candidate",
                "evidenceLevel": "strong-inference",
            }
        )
    return result


def probe_disk(name: str, data: bytes) -> dict[str, object]:
    boot = extract_boot(data)
    return {
        "name": name,
        "diskSha256": hashlib.sha256(data).hexdigest(),
        "bootSha256": hashlib.sha256(boot).hexdigest(),
        "bootSize": len(boot),
        "loadAddress": f"0x{BOOT_LOAD_ADDRESS:04x}",
        "physicalSectors": list(BOOT_PHYSICAL_SECTORS),
        "sourceRawSegments": [
            {
                "physicalSector": physical,
                "sourceRawOffset": f"0x{physical * SECTOR_SIZE:04x}",
                "bootBufferOffset": f"0x{index * SECTOR_SIZE:04x}",
            }
            for index, physical in enumerate(BOOT_PHYSICAL_SECTORS)
        ],
        "markers": marker_evidence(boot),
        "ioInstructionEvidence": io_instruction_evidence(boot),
        "interpretation": {
            "proven": [
                "指定 physical sectors 的原始 bytes、hash、marker 與 offset",
                "以 NMOS 6502 official opcode table 作線性 decode，保留 raw／CPU 位址",
            ],
            "strongInference": [
                "四段 bytes 在 $0800 形成連續 boot-code candidate",
                "$C080–$C08F 參照形狀符合 Disk II soft-switch address candidate",
            ],
            "unknown": [
                "Apple II ROM 是否實際以此順序載入並跳入 $0800",
                "boot code 後續如何尋找／載入 SYSTEM.PASCAL、live RWTS 與 emulator 畫面",
            ],
        },
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input_path", type=Path)
    parser.add_argument("--disk", help="只輸出名稱或 basename 相符的 disk")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    input_kind, input_sha, source_files, disks = load_input(args.input_path)
    if args.disk:
        disks = [
            item
            for item in disks
            if item[0] == args.disk or Path(item[0]).name == args.disk
        ]
        if not disks:
            raise SystemExit(f"disk not found: {args.disk}")
    print(
        json.dumps(
            {
                "schema": 1,
                "tool": "tools/apple2_boot_probe.py",
                "toolVersion": "Python 3.11.2; repository module at 2026-08-11",
                "input": str(args.input_path),
                "inputKind": input_kind,
                "inputSha256": input_sha,
                "sourceFiles": source_files,
                "disks": [probe_disk(name, data) for name, data in disks],
            },
            ensure_ascii=False,
            indent=2,
            sort_keys=True,
        )
    )


if __name__ == "__main__":
    main()
