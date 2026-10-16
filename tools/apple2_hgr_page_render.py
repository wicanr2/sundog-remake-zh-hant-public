#!/usr/bin/env python3
"""將一份 8,192-byte Apple II HGR page dump 轉為邏輯單色 PNG。

這是 oracle 輔助工具，不是 SunDog 原始資產解碼器：page dump 必須由呼叫端在隔離
環境取得，PNG 與 manifest 只寫到明確的輸出目錄。輸入 page dump 不會被加入
repository、公開封包或 remake runtime。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path

from apple2_hgr_probe import HGR_PAGE_SIZE, HGR_HEIGHT, HGR_WIDTH, render_hgr, write_png


def validate_page(page: bytes) -> None:
    if len(page) != HGR_PAGE_SIZE:
        raise ValueError(f"HGR page dump is {len(page)} bytes; want exactly {HGR_PAGE_SIZE}")


def build_manifest(page_path: Path, page: bytes, output_name: str, scale: int) -> dict[str, object]:
    return {
        "schema": 1,
        "tool": "tools/apple2_hgr_page_render.py",
        "source": {
            "pageDump": page_path.name,
            "size": len(page),
            "sha256": hashlib.sha256(page).hexdigest(),
        },
        "output": output_name,
        "logicalImage": {"width": HGR_WIDTH, "height": HGR_HEIGHT, "scale": scale},
        "mode": "Apple II HGR logical monochrome page render",
        "evidence": {
            "proven": "standard HGR row interleave and bit 0..6 logical pixel placement",
            "unknown": [
                "page dump 是否由 SunDog 顯示、備用、scratch 或其他 consumer 使用",
                "composite artifact color、bit 7、palette、原版 active page 與 renderer",
                "原始 DSK 資產、frame、anchor、collision 與 remake mapping",
            ],
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("page_dump", type=Path)
    parser.add_argument("--label", default="hgr-page")
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--scale", type=int, default=2)
    args = parser.parse_args()

    page = args.page_dump.read_bytes()
    try:
        validate_page(page)
    except ValueError as exc:
        parser.error(str(exc))
    if args.scale <= 0:
        parser.error("scale must be positive")

    args.output_dir.mkdir(parents=True, exist_ok=True)
    output_name = f"{args.label}.png"
    write_png(args.output_dir / output_name, render_hgr(page), args.scale)
    manifest = build_manifest(args.page_dump, page, output_name, args.scale)
    (args.output_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
