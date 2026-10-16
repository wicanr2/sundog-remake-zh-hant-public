#!/usr/bin/env python3
"""以固定位址空間輸出 6502 原始 bytes 的非破壞性反組譯視窗。

這是研究證據工具，不會替原始函式改名，也不會把猜測的語意寫回二進位檔。
輸出保留輸入雜湊、檔案偏移、CPU 位址、原始 bytes 與未知 opcode 標記，方便把
Apple II `SYSTEM.APPLE` 的資料 consumer 與原始 bytes 交叉回查。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import struct
from pathlib import Path


# Official NMOS 6502 opcodes.  Undocumented opcodes are emitted as .byte so a
# malformed or data region is never silently promoted to an instruction.
OPCODES: dict[int, tuple[str, str]] = {
    0x00: ("BRK", "imp"), 0x01: ("ORA", "indx"), 0x05: ("ORA", "zp"),
    0x06: ("ASL", "zp"), 0x08: ("PHP", "imp"), 0x09: ("ORA", "imm"),
    0x0A: ("ASL", "acc"), 0x0D: ("ORA", "abs"), 0x0E: ("ASL", "abs"),
    0x10: ("BPL", "rel"), 0x11: ("ORA", "indy"), 0x15: ("ORA", "zpx"),
    0x16: ("ASL", "zpx"), 0x18: ("CLC", "imp"), 0x19: ("ORA", "absy"),
    0x1D: ("ORA", "absx"), 0x1E: ("ASL", "absx"),
    0x20: ("JSR", "abs"), 0x21: ("AND", "indx"), 0x24: ("BIT", "zp"),
    0x25: ("AND", "zp"), 0x26: ("ROL", "zp"), 0x28: ("PLP", "imp"),
    0x29: ("AND", "imm"), 0x2A: ("ROL", "acc"), 0x2C: ("BIT", "abs"),
    0x2D: ("AND", "abs"), 0x2E: ("ROL", "abs"),
    0x30: ("BMI", "rel"), 0x31: ("AND", "indy"), 0x35: ("AND", "zpx"),
    0x36: ("ROL", "zpx"), 0x38: ("SEC", "imp"), 0x39: ("AND", "absy"),
    0x3D: ("AND", "absx"), 0x3E: ("ROL", "absx"),
    0x40: ("RTI", "imp"), 0x41: ("EOR", "indx"), 0x45: ("EOR", "zp"),
    0x46: ("LSR", "zp"), 0x48: ("PHA", "imp"), 0x49: ("EOR", "imm"),
    0x4A: ("LSR", "acc"), 0x4C: ("JMP", "abs"), 0x4D: ("EOR", "abs"),
    0x4E: ("LSR", "abs"),
    0x50: ("BVC", "rel"), 0x51: ("EOR", "indy"), 0x55: ("EOR", "zpx"),
    0x56: ("LSR", "zpx"), 0x58: ("CLI", "imp"), 0x59: ("EOR", "absy"),
    0x5D: ("EOR", "absx"), 0x5E: ("LSR", "absx"),
    0x60: ("RTS", "imp"), 0x61: ("ADC", "indx"), 0x65: ("ADC", "zp"),
    0x66: ("ROR", "zp"), 0x68: ("PLA", "imp"), 0x69: ("ADC", "imm"),
    0x6A: ("ROR", "acc"), 0x6C: ("JMP", "ind"), 0x6D: ("ADC", "abs"),
    0x6E: ("ROR", "abs"),
    0x70: ("BVS", "rel"), 0x71: ("ADC", "indy"), 0x75: ("ADC", "zpx"),
    0x76: ("ROR", "zpx"), 0x78: ("SEI", "imp"), 0x79: ("ADC", "absy"),
    0x7D: ("ADC", "absx"), 0x7E: ("ROR", "absx"),
    0x81: ("STA", "indx"), 0x84: ("STY", "zp"), 0x85: ("STA", "zp"),
    0x86: ("STX", "zp"), 0x88: ("DEY", "imp"), 0x8A: ("TXA", "imp"),
    0x8C: ("STY", "abs"), 0x8D: ("STA", "abs"), 0x8E: ("STX", "abs"),
    0x90: ("BCC", "rel"), 0x91: ("STA", "indy"), 0x94: ("STY", "zpx"),
    0x95: ("STA", "zpx"), 0x96: ("STX", "zpy"), 0x98: ("TYA", "imp"),
    0x99: ("STA", "absy"), 0x9A: ("TXS", "imp"), 0x9D: ("STA", "absx"),
    0xA0: ("LDY", "imm"), 0xA1: ("LDA", "indx"), 0xA2: ("LDX", "imm"),
    0xA4: ("LDY", "zp"), 0xA5: ("LDA", "zp"), 0xA6: ("LDX", "zp"),
    0xA8: ("TAY", "imp"), 0xA9: ("LDA", "imm"), 0xAA: ("TAX", "imp"),
    0xAC: ("LDY", "abs"), 0xAD: ("LDA", "abs"), 0xAE: ("LDX", "abs"),
    0xB0: ("BCS", "rel"), 0xB1: ("LDA", "indy"), 0xB4: ("LDY", "zpx"),
    0xB5: ("LDA", "zpx"), 0xB6: ("LDX", "zpy"), 0xB8: ("CLV", "imp"),
    0xB9: ("LDA", "absy"), 0xBA: ("TSX", "imp"), 0xBC: ("LDY", "absx"),
    0xBD: ("LDA", "absx"), 0xBE: ("LDX", "absy"),
    0xC0: ("CPY", "imm"), 0xC1: ("CMP", "indx"), 0xC4: ("CPY", "zp"),
    0xC5: ("CMP", "zp"), 0xC6: ("DEC", "zp"), 0xC8: ("INY", "imp"),
    0xC9: ("CMP", "imm"), 0xCA: ("DEX", "imp"), 0xCC: ("CPY", "abs"),
    0xCD: ("CMP", "abs"), 0xCE: ("DEC", "abs"),
    0xD0: ("BNE", "rel"), 0xD1: ("CMP", "indy"), 0xD5: ("CMP", "zpx"),
    0xD6: ("DEC", "zpx"), 0xD8: ("CLD", "imp"), 0xD9: ("CMP", "absy"),
    0xDD: ("CMP", "absx"), 0xDE: ("DEC", "absx"),
    0xE0: ("CPX", "imm"), 0xE1: ("SBC", "indx"), 0xE4: ("CPX", "zp"),
    0xE5: ("SBC", "zp"), 0xE6: ("INC", "zp"), 0xE8: ("INX", "imp"),
    0xE9: ("SBC", "imm"), 0xEA: ("NOP", "imp"), 0xEC: ("CPX", "abs"),
    0xED: ("SBC", "abs"), 0xEE: ("INC", "abs"),
    0xF0: ("BEQ", "rel"), 0xF1: ("SBC", "indy"), 0xF5: ("SBC", "zpx"),
    0xF6: ("INC", "zpx"), 0xF8: ("SED", "imp"), 0xF9: ("SBC", "absy"),
    0xFD: ("SBC", "absx"), 0xFE: ("INC", "absx"),
}

MODE_LENGTH = {
    "imp": 1, "acc": 1, "imm": 2, "zp": 2, "zpx": 2, "zpy": 2,
    "indx": 2, "indy": 2, "rel": 2, "abs": 3, "absx": 3, "absy": 3,
    "ind": 3,
}


def parse_int(value: str) -> int:
    return int(value, 0)


def operand(mode: str, raw: bytes, cpu_address: int) -> str:
    if mode in {"imp"}:
        return ""
    if mode == "acc":
        return "A"
    if mode == "imm":
        return f"#$%02X" % raw[1]
    if mode == "zp":
        return f"$%02X" % raw[1]
    if mode == "zpx":
        return f"$%02X,X" % raw[1]
    if mode == "zpy":
        return f"$%02X,Y" % raw[1]
    if mode == "indx":
        return f"($%02X,X)" % raw[1]
    if mode == "indy":
        return f"($%02X),Y" % raw[1]
    if mode == "rel":
        delta = raw[1] if raw[1] < 0x80 else raw[1] - 0x100
        return f"$%04X" % ((cpu_address + 2 + delta) & 0xFFFF)
    target = raw[1] | (raw[2] << 8)
    if mode == "abs":
        return f"$%04X" % target
    if mode == "absx":
        return f"$%04X,X" % target
    if mode == "absy":
        return f"$%04X,Y" % target
    if mode == "ind":
        return f"($%04X)" % target
    raise ValueError(f"unsupported addressing mode: {mode}")


def disassemble(data: bytes, start: int, end: int, base: int) -> list[dict[str, object]]:
    instructions: list[dict[str, object]] = []
    offset = start
    while offset < end:
        opcode = data[offset]
        cpu_address = base + offset
        decoded = OPCODES.get(opcode)
        if decoded is None:
            instructions.append(
                {
                    "rawOffset": offset,
                    "cpuAddress": cpu_address,
                    "bytes": data[offset : offset + 1].hex(),
                    "mnemonic": ".byte",
                    "operand": f"$%02X" % opcode,
                    "status": "unknown-opcode-or-data",
                }
            )
            offset += 1
            continue
        mnemonic, mode = decoded
        length = MODE_LENGTH[mode]
        raw = data[offset : min(offset + length, end)]
        if len(raw) != length:
            instructions.append(
                {
                    "rawOffset": offset,
                    "cpuAddress": cpu_address,
                    "bytes": raw.hex(),
                    "mnemonic": ".byte",
                    "operand": " ".join(f"$%02X" % value for value in raw),
                    "status": "truncated-instruction",
                }
            )
            offset += len(raw)
            continue
        instructions.append(
            {
                "rawOffset": offset,
                "cpuAddress": cpu_address,
                "bytes": raw.hex(),
                "mnemonic": mnemonic,
                "operand": operand(mode, raw, cpu_address),
                "status": "official-6502-opcode",
            }
        )
        offset += length
    return instructions


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--base", type=parse_int, required=True, help="CPU address of raw byte 0")
    parser.add_argument("--start", type=parse_int, default=0)
    parser.add_argument("--end", type=parse_int)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    data = args.binary.read_bytes()
    end = len(data) if args.end is None else args.end
    if not (0 <= args.start < end <= len(data)):
        parser.error("start/end must be within the input binary")
    rows = disassemble(data, args.start, end, args.base)
    document = {
        "schema": 1,
        "tool": "tools/disassemble_6502.py",
        "processor": "6502-NMOS-official-opcodes",
        "input": {
            "name": args.binary.name,
            "sha256": hashlib.sha256(data).hexdigest(),
            "size": len(data),
        },
        "addressSpace": {
            "base": f"0x{args.base:04x}",
            "rawStart": f"0x{args.start:04x}",
            "rawEndExclusive": f"0x{end:04x}",
        },
        "instructions": rows,
        "limit": "Disassembly preserves raw locations but does not prove code/data boundaries or semantic consumers.",
    }
    if args.json:
        print(json.dumps(document, ensure_ascii=False, indent=2))
    else:
        for row in rows:
            suffix = f" {row['operand']}" if row["operand"] else ""
            print(
                f"{row['rawOffset']:04x} {row['cpuAddress']:04x} "
                f"{row['bytes']:<8} {row['mnemonic']:<5}{suffix:<12} "
                f"[{row['status']}]"
            )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
