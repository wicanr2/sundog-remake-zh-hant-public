#!/usr/bin/env python3
"""UCSD p-system p-code 反組譯器。

這個 repo 先前讀 p-code 靠人工對位元組，位址一錯就整段偏掉，而且錯了不會有任何徵兆。
這支把那件事機械化：給一份記憶體 dump（或磁碟映像）和一個段基底，把指定範圍解成
助憶符與運算元。

**opcode 表逐字取自 laanwj/sundog 的 `src/psys/psys_opcodes.c`**（MIT，Wladimir J. van der
Laan 2017），那份表本身是從 SunDog 的 `SYSTEM.INTERP` dispatch 表產生的。運算元的取碼規則
照抄同一份原始碼的 `fetch_UB` / `fetch_SB` / `fetch_W` / `fetch_V`：

* `UB` 一個位元組；`SB` 一個位元組有號。
* `W` 兩個位元組——**程式碼流裡的 word 一律 little-endian**，與段的 endian 旗標無關。
* `V`（原始碼叫 big）變長：第一個位元組小於 `0x80` 就是它本身，否則去掉最高位再吃一個
  位元組接在低位。
* 帶 `RELFLAG` 的運算元是相對於**指令結束後**的位移，所以目標位址要用指令尾端去加。

段的 endian 旗標只影響**資料**（常數池、跳表），不影響指令流。`--jump-table` 解 `xjp`
的跳表時會用它。

反組譯是機械翻譯，不是理解：它告訴你「這裡寫了什麼」，不告訴你「這是什麼意思」。語意仍然
要人讀，寫進 `docs/atari-st/*.md`。
"""

from __future__ import annotations

import argparse
import pathlib
import struct
import sys

# 運算元型別（psys_opcodes.h 的 psys_opcode_argtype）。
ARG_VAR, ARG_BYTE, ARG_WORD = 0, 1, 2
ARG_MASK, ARG_SIGNED, ARG_REL = 15, 16, 32

# 段標頭欄位（psys_constants.h）。
SEG_NAME, SEG_ENDIAN, SEG_CPOOLOFS = 0x04, 0x0c, 0x0e
ENDIAN_NATIVE = 0x0001

# opcode 表：編號 → (助憶符, 運算元型別)。取自 laanwj/sundog src/psys/psys_opcodes.c（MIT）。
OPCODES: dict[int, tuple[str, tuple[int, ...]]] = {
    0x00: ("sldc0", ()),
    0x01: ("sldc1", ()),
    0x02: ("sldc2", ()),
    0x03: ("sldc3", ()),
    0x04: ("sldc4", ()),
    0x05: ("sldc5", ()),
    0x06: ("sldc6", ()),
    0x07: ("sldc7", ()),
    0x08: ("sldc8", ()),
    0x09: ("sldc9", ()),
    0x0a: ("sldc10", ()),
    0x0b: ("sldc11", ()),
    0x0c: ("sldc12", ()),
    0x0d: ("sldc13", ()),
    0x0e: ("sldc14", ()),
    0x0f: ("sldc15", ()),
    0x10: ("sldc16", ()),
    0x11: ("sldc17", ()),
    0x12: ("sldc18", ()),
    0x13: ("sldc19", ()),
    0x14: ("sldc20", ()),
    0x15: ("sldc21", ()),
    0x16: ("sldc22", ()),
    0x17: ("sldc23", ()),
    0x18: ("sldc24", ()),
    0x19: ("sldc25", ()),
    0x1a: ("sldc26", ()),
    0x1b: ("sldc27", ()),
    0x1c: ("sldc28", ()),
    0x1d: ("sldc29", ()),
    0x1e: ("sldc30", ()),
    0x1f: ("sldc31", ()),
    0x20: ("sldl1", ()),
    0x21: ("sldl2", ()),
    0x22: ("sldl3", ()),
    0x23: ("sldl4", ()),
    0x24: ("sldl5", ()),
    0x25: ("sldl6", ()),
    0x26: ("sldl7", ()),
    0x27: ("sldl8", ()),
    0x28: ("sldl9", ()),
    0x29: ("sldl10", ()),
    0x2a: ("sldl11", ()),
    0x2b: ("sldl12", ()),
    0x2c: ("sldl13", ()),
    0x2d: ("sldl14", ()),
    0x2e: ("sldl15", ()),
    0x2f: ("sldl16", ()),
    0x30: ("sldo1", ()),
    0x31: ("sldo2", ()),
    0x32: ("sldo3", ()),
    0x33: ("sldo4", ()),
    0x34: ("sldo5", ()),
    0x35: ("sldo6", ()),
    0x36: ("sldo7", ()),
    0x37: ("sldo8", ()),
    0x38: ("sldo9", ()),
    0x39: ("sldo10", ()),
    0x3a: ("sldo11", ()),
    0x3b: ("sldo12", ()),
    0x3c: ("sldo13", ()),
    0x3d: ("sldo14", ()),
    0x3e: ("sldo15", ()),
    0x3f: ("sldo16", ()),
    0x40: ("und40", ()),
    0x41: ("und41", ()),
    0x42: ("und42", ()),
    0x43: ("und43", ()),
    0x44: ("und44", ()),
    0x45: ("und45", ()),
    0x46: ("und46", ()),
    0x47: ("und47", ()),
    0x48: ("und48", ()),
    0x49: ("und49", ()),
    0x4a: ("und4a", ()),
    0x4b: ("und4b", ()),
    0x4c: ("und4c", ()),
    0x4d: ("und4d", ()),
    0x4e: ("und4e", ()),
    0x4f: ("und4f", ()),
    0x50: ("und50", ()),
    0x51: ("und51", ()),
    0x52: ("und52", ()),
    0x53: ("und53", ()),
    0x54: ("und54", ()),
    0x55: ("und55", ()),
    0x56: ("und56", ()),
    0x57: ("und57", ()),
    0x58: ("und58", ()),
    0x59: ("und59", ()),
    0x5a: ("und5a", ()),
    0x5b: ("und5b", ()),
    0x5c: ("und5c", ()),
    0x5d: ("und5d", ()),
    0x5e: ("und5e", ()),
    0x5f: ("und5f", ()),
    0x60: ("slla1", ()),
    0x61: ("slla2", ()),
    0x62: ("slla3", ()),
    0x63: ("slla4", ()),
    0x64: ("slla5", ()),
    0x65: ("slla6", ()),
    0x66: ("slla7", ()),
    0x67: ("slla8", ()),
    0x68: ("sstl1", ()),
    0x69: ("sstl2", ()),
    0x6a: ("sstl3", ()),
    0x6b: ("sstl4", ()),
    0x6c: ("sstl5", ()),
    0x6d: ("sstl6", ()),
    0x6e: ("sstl7", ()),
    0x6f: ("sstl8", ()),
    0x70: ("scxg1", (1,)),
    0x71: ("scxg2", (1,)),
    0x72: ("scxg3", (1,)),
    0x73: ("scxg4", (1,)),
    0x74: ("scxg5", (1,)),
    0x75: ("scxg6", (1,)),
    0x76: ("scxg7", (1,)),
    0x77: ("scxg8", (1,)),
    0x78: ("sind0", ()),
    0x79: ("sind1", ()),
    0x7a: ("sind2", ()),
    0x7b: ("sind3", ()),
    0x7c: ("sind4", ()),
    0x7d: ("sind5", ()),
    0x7e: ("sind6", ()),
    0x7f: ("sind7", ()),
    0x80: ("ldcb", (1,)),
    0x81: ("ldci", (2,)),
    0x82: ("lco", (0,)),
    0x83: ("ldc", (1, 0, 1)),
    0x84: ("lla", (0,)),
    0x85: ("ldo", (0,)),
    0x86: ("lao", (0,)),
    0x87: ("ldl", (0,)),
    0x88: ("lda", (1, 0)),
    0x89: ("lod", (1, 0)),
    0x8a: ("ujp", (49,)),
    0x8b: ("ujpl", (50,)),
    0x8c: ("mpi", ()),
    0x8d: ("dvi", ()),
    0x8e: ("stm", (1,)),
    0x8f: ("modi", ()),
    0x90: ("clp", (1,)),
    0x91: ("cgp", (1,)),
    0x92: ("cip", (1, 1)),
    0x93: ("cxl", (1, 1)),
    0x94: ("cxg", (1, 1)),
    0x95: ("cxi", (1, 1, 1)),
    0x96: ("rpu", (0,)),
    0x97: ("cfp", ()),
    0x98: ("ldcn", ()),
    0x99: ("lsl", (1,)),
    0x9a: ("lde", (1, 0)),
    0x9b: ("lae", (1, 0)),
    0x9c: ("nop", ()),
    0x9d: ("lpr", ()),
    0x9e: ("bpt", ()),
    0x9f: ("bnot", ()),
    0xa0: ("lor", ()),
    0xa1: ("land", ()),
    0xa2: ("adi", ()),
    0xa3: ("sbi", ()),
    0xa4: ("stl", (0,)),
    0xa5: ("sro", (0,)),
    0xa6: ("str", (1, 0)),
    0xa7: ("ldb", ()),
    0xa8: ("native", ()),
    0xa9: ("nat-info", (48,)),
    0xaa: ("undaa", ()),
    0xab: ("cap", (0,)),
    0xac: ("csp", (1,)),
    0xad: ("slod1", (0,)),
    0xae: ("slod2", (0,)),
    0xaf: ("undaf", ()),
    0xb0: ("equi", ()),
    0xb1: ("neqi", ()),
    0xb2: ("leqi", ()),
    0xb3: ("geqi", ()),
    0xb4: ("leusw", ()),
    0xb5: ("geusw", ()),
    0xb6: ("eqpwr", ()),
    0xb7: ("lepwr", ()),
    0xb8: ("gepwr", ()),
    0xb9: ("eqbyte", (1, 1, 0)),
    0xba: ("lebyte", (1, 1, 0)),
    0xbb: ("gebyte", (1, 1, 0)),
    0xbc: ("srs", ()),
    0xbd: ("swap", ()),
    0xbe: ("undbe", ()),
    0xbf: ("undbf", ()),
    0xc0: ("undc0", ()),
    0xc1: ("undc1", ()),
    0xc2: ("undc2", ()),
    0xc3: ("undc3", ()),
    0xc4: ("sto", ()),
    0xc5: ("mov", (1, 0)),
    0xc6: ("dup2", ()),
    0xc7: ("adj", (1,)),
    0xc8: ("stb", ()),
    0xc9: ("ldp", ()),
    0xca: ("stp", ()),
    0xcb: ("chk", ()),
    0xcc: ("flt", ()),
    0xcd: ("eqreal", ()),
    0xce: ("lereal", ()),
    0xcf: ("gereal", ()),
    0xd0: ("ldm", (1,)),
    0xd1: ("spr", ()),
    0xd2: ("efj", (49,)),
    0xd3: ("nfj", (49,)),
    0xd4: ("fjp", (49,)),
    0xd5: ("fjpl", (50,)),
    0xd6: ("xjp", (0,)),
    0xd7: ("ixa", (0,)),
    0xd8: ("ixp", (1, 1)),
    0xd9: ("ste", (1, 0)),
    0xda: ("inn", ()),
    0xdb: ("uni", ()),
    0xdc: ("int", ()),
    0xdd: ("dif", ()),
    0xde: ("signal", ()),
    0xdf: ("wait", ()),
    0xe0: ("abi", ()),
    0xe1: ("ngi", ()),
    0xe2: ("dup1", ()),
    0xe3: ("abr", ()),
    0xe4: ("ngr", ()),
    0xe5: ("lnot", ()),
    0xe6: ("ind", (0,)),
    0xe7: ("inc", (0,)),
    0xe8: ("eqstr", (1, 1)),
    0xe9: ("lestr", (1, 1)),
    0xea: ("gestr", (1, 1)),
    0xeb: ("astr", (1, 1)),
    0xec: ("cstr", ()),
    0xed: ("inci", ()),
    0xee: ("deci", ()),
    0xef: ("scip1", (1,)),
    0xf0: ("scip2", (1,)),
    0xf1: ("tjp", (49,)),
    0xf2: ("ldcrl", ()),
    0xf3: ("ldrl", ()),
    0xf4: ("strl", ()),
    0xf5: ("undf5", ()),
    0xf6: ("undf6", ()),
    0xf7: ("undf7", ()),
    0xf8: ("undf8", ()),
    0xf9: ("undf9", ()),
    0xfa: ("undfa", ()),
    0xfb: ("undfb", ()),
    0xfc: ("undfc", ()),
    0xfd: ("undfd", ()),
    0xfe: ("undfe", ()),
    0xff: ("undff", ()),}


class Reader:
    """在一塊位元組上依 p-code 的取碼規則前進。"""

    def __init__(self, data: bytes, pos: int):
        self.data, self.pos, self.start = data, pos, pos

    def ub(self) -> int:
        value = self.data[self.pos]
        self.pos += 1
        return value

    def sb(self) -> int:
        value = self.ub()
        return value - 0x100 if value >= 0x80 else value

    def w(self) -> int:
        # 指令流裡的 word 永遠是 little-endian，不看段的 endian 旗標。
        value = struct.unpack_from("<h", self.data, self.pos)[0]
        self.pos += 2
        return value

    def var(self) -> int:
        value = self.ub()
        if value >= 0x80:
            value = ((value & 0x7F) << 8) | self.ub()
        return value


def decode_one(data: bytes, pos: int) -> dict:
    """解一條指令，回傳助憶符、運算元與長度。"""
    opcode = data[pos]
    name, argtypes = OPCODES.get(opcode, (f"?{opcode:02x}", ()))
    reader = Reader(data, pos + 1)
    args = []
    for argtype in argtypes:
        kind = argtype & ARG_MASK
        if kind == ARG_WORD:
            raw = reader.w() if argtype & ARG_SIGNED else struct.unpack_from(
                "<H", data, reader.pos)[0]
            if not argtype & ARG_SIGNED:
                reader.pos += 2
        elif kind == ARG_BYTE:
            raw = reader.sb() if argtype & ARG_SIGNED else reader.ub()
        else:
            raw = reader.var()
        args.append({"value": raw, "relative": bool(argtype & ARG_REL)})
    length = reader.pos - pos
    for arg in args:
        if arg["relative"]:
            # RELFLAG 的位移是相對於指令結束之後。
            arg["target"] = pos + length + arg["value"]
    return {"offset": pos, "opcode": opcode, "name": name, "args": args, "length": length}


def format_one(instruction: dict, data: bytes, base_label: int) -> str:
    raw = " ".join(f"{b:02x}" for b in
                   data[instruction["offset"]:instruction["offset"] + instruction["length"]])
    parts = []
    for arg in instruction["args"]:
        if "target" in arg:
            parts.append(f"{arg['value']:+d} → 0x{base_label + arg['target'] - instruction['offset']:04x}"
                         if False else f"0x{base_label + (arg['target'] - instruction['offset']):04x}")
        else:
            parts.append(f"0x{arg['value']:x}")
    label = base_label
    return f"{label:04x}:  {raw:<17} {instruction['name']:<8} {', '.join(parts)}"


def segment_info(memory: bytes, base: int) -> dict:
    name = memory[base + SEG_NAME:base + SEG_NAME + 8]
    endian = struct.unpack_from("<H", memory, base + SEG_ENDIAN)[0]
    flip = endian != ENDIAN_NATIVE
    fmt = ">H" if flip else "<H"
    return {
        "name": name.decode("latin-1").rstrip("\x00 "),
        "endianFlip": flip,
        "cpoolWords": struct.unpack_from(fmt, memory, base + SEG_CPOOLOFS)[0],
    }


def jump_table(memory: bytes, base: int, info: dict, operand: int) -> dict:
    """解 xjp 的跳表：seg_cpool_ofs(seg, operand) 之後是 min、max、各分支的位移。"""
    fmt = ">h" if info["endianFlip"] else "<h"
    table = (info["cpoolWords"] + operand) * 2
    low = struct.unpack_from(fmt, memory, base + table)[0]
    high = struct.unpack_from(fmt, memory, base + table + 2)[0]
    deltas = [struct.unpack_from(fmt, memory, base + table + 4 + i * 2)[0]
              for i in range(high - low + 1)]
    return {"tableOffset": table, "min": low, "max": high, "deltas": deltas}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("image", type=pathlib.Path, help="記憶體 dump 或磁碟映像")
    parser.add_argument("--base", type=lambda v: int(v, 0), required=True,
                        help="段基底在檔案裡的 offset")
    parser.add_argument("--start", type=lambda v: int(v, 0), required=True, help="段內起始位址")
    parser.add_argument("--end", type=lambda v: int(v, 0), required=True, help="段內結束位址（不含）")
    parser.add_argument("--jump-table", type=lambda v: int(v, 0),
                        help="另外解一張 xjp 跳表，給 xjp 的運算元")
    args = parser.parse_args(argv)

    memory = args.image.read_bytes()
    info = segment_info(memory, args.base)
    print(f"段 {info['name']}  基底 0x{args.base:05x}  "
          f"常數池 0x{info['cpoolWords']:04x} words  "
          f"endian {'flip' if info['endianFlip'] else 'native'}")
    print()

    pos = args.base + args.start
    stop = args.base + args.end
    while pos < stop:
        instruction = decode_one(memory, pos)
        print(format_one(instruction, memory, pos - args.base))
        pos += instruction["length"]

    if args.jump_table is not None:
        table = jump_table(memory, args.base, info, args.jump_table)
        print(f"\nxjp 0x{args.jump_table:x} 的跳表 @ 段內 0x{table['tableOffset']:04x}："
              f"min={table['min']} max={table['max']}")
        for index, delta in enumerate(table["deltas"]):
            print(f"  碼 {table['min'] + index:2d} → 位移 {delta:+d}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
