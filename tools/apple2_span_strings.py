#!/usr/bin/env python3
"""從明確指定的 Apple II raw span 輸出可回查的 ASCII 字串候選。

這個工具只證明 bytes、raw offset 與 hash；它不判定檔案系統、句界、執行 consumer 或
玩家可見語意。原始 archive 以記憶體方式讀取，不解壓寫入工作樹。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import platform
import re
import zipfile
from pathlib import Path


def parse_int(value: str) -> int:
    return int(value, 0)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--disk", default="SUNDOG1.DSK")
    parser.add_argument("--start-block", type=parse_int, required=True)
    parser.add_argument("--next-block", type=parse_int, required=True)
    parser.add_argument("--base-offset", type=parse_int, default=0)
    parser.add_argument("--block-size", type=parse_int, default=512)
    parser.add_argument("--min-length", type=int, default=4)
    args = parser.parse_args()

    if args.next_block <= args.start_block:
        parser.error("--next-block must be greater than --start-block")
    if args.base_offset < 0 or args.block_size <= 0 or args.min_length < 1:
        parser.error("base offset must be non-negative; block size and min length must be positive")

    archive_bytes = args.archive.read_bytes()
    with zipfile.ZipFile(args.archive) as archive:
        disk = archive.read(args.disk)

    start = args.base_offset + args.start_block * args.block_size
    end = args.base_offset + args.next_block * args.block_size
    if not (0 <= start < end <= len(disk)):
        parser.error(f"raw span 0x{start:x}..0x{end:x} is outside {args.disk} ({len(disk)} bytes)")

    span = disk[start:end]
    candidates = []
    for match in re.finditer(rb"[ -~]{%d,}" % args.min_length, span):
        raw = match.group()
        candidates.append(
            {
                "rawOffset": f"0x{start + match.start():05x}",
                "endOffsetExclusive": f"0x{start + match.end():05x}",
                "length": len(raw),
                "rawHex": raw.hex(),
                "source": raw.decode("ascii", "replace"),
                "evidenceLevel": "proven-bytes",
                "consumerStatus": "unknown",
            }
        )

    result = {
        "schema": 1,
        "tool": "apple2_span_strings",
        "pythonVersion": platform.python_version(),
        "sourceArchive": {"name": args.archive.name, "sha256": hashlib.sha256(archive_bytes).hexdigest()},
        "disk": {"name": args.disk, "size": len(disk), "sha256": hashlib.sha256(disk).hexdigest()},
        "span": {
            "baseOffset": f"0x{args.base_offset:05x}",
            "blockSize": args.block_size,
            "startBlock": args.start_block,
            "nextBlock": args.next_block,
            "rawStart": f"0x{start:05x}",
            "rawEndExclusive": f"0x{end:05x}",
            "byteLength": len(span),
            "sha256": hashlib.sha256(span).hexdigest(),
        },
        "candidateCount": len(candidates),
        "candidates": candidates,
        "status": "pass",
        "limit": "Printable runs are evidence candidates only; consumer, sentence boundaries, encoding, and translation remain unknown.",
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
