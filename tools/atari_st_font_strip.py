#!/usr/bin/env python3
"""記錄原版字元條在磁碟上的位置與雜湊，並把每個字元當成 8 列位元輸出。

字元條是 1 位元、每列 118 bytes、8 列的圖，raw `0x6600` 起；`GEMBIND:0x13 wchar` 以
每字 6 px 切主字元集，`GEMBIND:0x22 draw_x_char` 從 x=0x300 起以每字 7 px 切小圖。
找到它的方法：把原版實跑畫面上的城市記號（字元 20）當位元圖樣，對整張磁碟以各種列跨距
做位元搜尋，只有 raw 0x6600、跨距 118 命中。
"""
import argparse, hashlib, json, sys, zipfile
from pathlib import Path

OFFSET, STRIDE, ROWS, GLYPH_W, SMALL_X, SMALL_W = 0x6600, 118, 8, 6, 0x300, 7

def read_disk(archive: Path, variant: str) -> tuple[str, bytes]:
    with zipfile.ZipFile(archive) as outer:
        nested = [n for n in outer.namelist() if variant in n and n.endswith(".zip")][0]
        with zipfile.ZipFile(outer.open(nested)) as inner:
            st = [n for n in inner.namelist() if n.endswith(".st")][0]
            return st, inner.read(st)

def glyph(bits: bytes, x0: int, width: int) -> list[str]:
    rows = []
    for y in range(ROWS):
        row = ""
        for x in range(x0, x0 + width):
            row += "#" if bits[y * STRIDE + x // 8] >> (7 - x % 8) & 1 else "."
        rows.append(row)
    return rows

def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--variant", default="[a2]")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    name, disk = read_disk(args.archive, args.variant)
    strip = disk[OFFSET:OFFSET + STRIDE * ROWS]
    glyphs = {str(ch): glyph(strip, ch * GLYPH_W, GLYPH_W) for ch in range(SMALL_X // GLYPH_W)}
    small = {str(i): glyph(strip, SMALL_X + i * SMALL_W, SMALL_W) for i in range((STRIDE * 8 - SMALL_X) // SMALL_W)}
    out = {
        "schema": 1, "tool": Path(sys.argv[0]).name,
        "inputs": {"disk": {"name": name, "size": len(disk), "sha256": hashlib.sha256(disk).hexdigest()}},
        "strip": {"rawOffset": f"0x{OFFSET:x}", "strideBytes": STRIDE, "rows": ROWS, "widthPixels": STRIDE * 8,
                  "sha256": hashlib.sha256(strip).hexdigest(),
                  "consumers": ["GEMBIND:0x13 wchar (ch×6, 6×8)", "GEMBIND:0x22 draw_x_char (0x300 + i×7, 7×8)",
                                "GEMBIND:0x21 draw_char(ch,mode,x,y) = wchar(ch, x−3, y+3)"]},
        "signature": {"character": "A", "rows": glyphs["65"]},
        "evidence": "字元 20 的位元圖樣取自原版實跑畫面（XREADMAP 行星圖的城市記號），整張磁碟只在此處以跨距 118 命中；"
                    "字元 9 與畫面上的船記號、字元 0x40 與星圖遠星的菱形逐像素相同。",
        "glyphs": glyphs, "smallGlyphs": small,
    }
    text = json.dumps(out, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        sys.stdout.write(text)

if __name__ == "__main__":
    main()
