#!/usr/bin/env python3
"""把 Apple II raw bytes 以標準 HGR 交錯做可審查的單色 probe。

這不是 SunDog 圖形格式解碼器：輸入 bytes 只會被放入一個 8,192-byte HGR page，
bit 0..6 以邏輯單色像素輸出，bit 7、composite artifact color、page 選擇與遊戲
consumer 都保持未知。輸出只包含指定目錄的 PNG 與 hash／offset manifest。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import struct
import zipfile
from pathlib import Path


HGR_PAGE_SIZE = 8192
HGR_WIDTH = 280
HGR_HEIGHT = 192


def parse_int(value: str) -> int:
    return int(value, 0)


def hgr_row_offset(row: int) -> int:
    if not 0 <= row < HGR_HEIGHT:
        raise ValueError(f"HGR row outside 0..191: {row}")
    return ((row & 0x07) * 0x400) + (((row >> 3) & 0x07) * 0x80) + ((row >> 6) * 0x28)


def render_hgr(raw: bytes) -> bytes:
    if len(raw) > HGR_PAGE_SIZE:
        raise ValueError(f"HGR probe is {len(raw)} bytes; maximum is {HGR_PAGE_SIZE}")
    page = raw.ljust(HGR_PAGE_SIZE, b"\x00")
    pixels = bytearray(HGR_WIDTH * HGR_HEIGHT)
    for row in range(HGR_HEIGHT):
        source = hgr_row_offset(row)
        target = row * HGR_WIDTH
        for column in range(40):
            value = page[source + column]
            for bit in range(7):
                pixels[target + column * 7 + bit] = 255 if value & (1 << bit) else 0
    return bytes(pixels)


def png_chunk(kind: bytes, payload: bytes) -> bytes:
    import zlib

    return (
        struct.pack(">I", len(payload))
        + kind
        + payload
        + struct.pack(">I", zlib.crc32(kind + payload) & 0xFFFFFFFF)
    )


def write_png(path: Path, pixels: bytes, scale: int) -> None:
    if scale <= 0:
        raise ValueError("scale must be positive")
    rows = bytearray()
    for row in range(HGR_HEIGHT):
        expanded = bytearray()
        for value in pixels[row * HGR_WIDTH : (row + 1) * HGR_WIDTH]:
            expanded.extend(bytes((value, value, value, 255)) * scale)
        for _ in range(scale):
            rows.append(0)
            rows.extend(expanded)
    width, height = HGR_WIDTH * scale, HGR_HEIGHT * scale
    import zlib

    png = b"\x89PNG\r\n\x1a\n"
    png += png_chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0))
    png += png_chunk(b"IDAT", zlib.compress(bytes(rows), level=9))
    png += png_chunk(b"IEND", b"")
    path.write_bytes(png)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--disk", default="SUNDOG1.DSK")
    parser.add_argument("--offset", type=parse_int, required=True, help="raw disk byte offset")
    parser.add_argument("--length", type=parse_int, required=True, help="raw byte count, at most 8192")
    parser.add_argument("--scale", type=int, default=2)
    parser.add_argument("--label", default="hgr-probe")
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()

    archive_bytes = args.archive.read_bytes()
    with zipfile.ZipFile(args.archive) as archive:
        disk = archive.read(args.disk)
    if args.offset < 0 or args.length <= 0 or args.offset + args.length > len(disk):
        parser.error("offset/length must select an in-bounds positive raw disk span")
    raw = disk[args.offset : args.offset + args.length]
    pixels = render_hgr(raw)
    args.output_dir.mkdir(parents=True, exist_ok=True)
    png_name = f"{args.label}-{args.offset:05x}-{args.length:04x}.png"
    write_png(args.output_dir / png_name, pixels, args.scale)
    manifest = {
        "schema": 1,
        "tool": "tools/apple2_hgr_probe.py",
        "source": {
            "archive": args.archive.name,
            "archiveSha256": hashlib.sha256(archive_bytes).hexdigest(),
            "disk": args.disk,
            "diskSha256": hashlib.sha256(disk).hexdigest(),
            "diskSize": len(disk),
        },
        "rawSpan": {
            "offset": f"0x{args.offset:x}",
            "length": args.length,
            "sha256": hashlib.sha256(raw).hexdigest(),
        },
        "output": png_name,
        "logicalImage": {"width": HGR_WIDTH, "height": HGR_HEIGHT, "scale": args.scale},
        "mode": "Apple II HGR logical monochrome probe",
        "evidence": {
            "proven": "standard HGR row interleave and bit 0..6 logical pixel placement",
            "unknown": [
                "SunDog screen／asset format、page selection、bit 7 artifact color、palette",
                "raw span 是否是畫面、tile、sprite、資料或程式碼",
                "實際 loader／繪圖 consumer 與原版同狀態畫面",
            ],
        },
    }
    (args.output_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
