#!/usr/bin/env python3
"""由原版 XSTARTUP 常數及消費指令解出 Banville 九階段需求。"""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path

DISK_SHA256 = "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6"
BASE = 0x39C00
CALLS = (0x17E3, 0x17EA, 0x17F1, 0x17F8, 0x17FF, 0x1806, 0x180D, 0x1814, 0x181B)

def decode(disk: bytes) -> dict:
    if hashlib.sha256(disk).hexdigest() != DISK_SHA256:
        raise ValueError("原始磁碟 SHA-256 不符，拒絕套用固定定位")
    if disk[BASE+4:BASE+12] != b"XSTARTUP":
        raise ValueError("XSTARTUP 段名不符")
    pool = int.from_bytes(disk[BASE+14:BASE+16], "big") * 2
    if disk[BASE+0x16A1:BASE+0x16A8] != bytes.fromhex("61 20 a7 80 40 a3 ca"):
        raise ValueError("字元轉貨物碼消費指令不符")
    phases = []
    for stage, call in enumerate(CALLS):
        at = BASE + call
        code = disk[at:at+7]
        if code[0] != 0x82 or code[1] < 0x80 or code[3:] != bytes.fromhex("08 9d 90 2b"):
            raise ValueError("階段常數呼叫序列不符")
        word_offset = ((code[1] & 0x7f) << 8) | code[2]
        raw = BASE + pool + word_offset * 2
        literal = disk[raw:raw+9]
        if len(literal) != 9 or literal[0] != 8 or any(c < 64 or c > 95 for c in literal[1:]):
            raise ValueError("需求字串長度或貨物碼越界")
        goods = [c-64 for c in literal[1:]]
        counts = [goods.count(i) for i in range(32)]
        counts[0] = 0  # XDOTRADI:0x06 0x0178..0x017b 跳過空欄。
        phases.append({"stage":stage,"callOffset":hex(call),"rawOffset":hex(raw),
                       "sourceHex":literal.hex(),"goods":goods,"requiredCounts":counts})
    return {"schema":1,"sourceSHA256":DISK_SHA256,"segment":"XSTARTUP",
            "segmentRawBase":hex(BASE),"constantPoolOffset":hex(pool),
            "evidenceLevel":"proven","phases":phases}

def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("disk", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    result = decode(args.disk.read_bytes())
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2)+"\n")

if __name__ == "__main__":
    main()
