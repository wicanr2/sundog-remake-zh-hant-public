#!/usr/bin/env python3
"""由已解碼的 Atari ST image stream 建立暫存 normalized atlas 候選。

這個工具只產生使用者本機的研究／視覺審查輸出，不把原始或解出的遊戲資產寫進
repository。每個 semantic tile 的 mapping 都標成 ``hypothesis``；它不能證明
tileset、碰撞、anchor 或正常遊戲 consumer。
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from decode_atari_st_images import PALETTE, load_disk, walk_images, write_png


# These crops are deliberately explicit and easy to replace after a real ST
# consumer is found. The offsets are image-stream evidence, not semantic proof.
CANDIDATE_TILES = (
    ("grass", 0x1B166, 0, 0),
    ("water", 0x1B166, 8, 0),
    ("road", 0x1B166, 16, 0),
    ("wall", 0x1CD18, 0, 0),
    ("ship", 0x1B35A, 0, 0),
    ("cargo", 0x11552, 0, 0),
    ("city", 0x1CD18, 32, 0),
    ("player", 0x11552, 64, 0),
)
TILE_WIDTH = 8
TILE_HEIGHT = 8


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--variant", choices=("[a2]", "[a]"), default="[a2]")
    parser.add_argument("--output-dir", type=Path, required=True)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    source, disk = load_disk(args.archive, args.variant)
    records = {int(record["rawOffsetDecimal"]): (record, pixels) for record, pixels in walk_images(disk, 0x0D800)}

    atlas = bytearray()
    tiles: dict[str, dict[str, object]] = {}
    for index, (semantic, raw_offset, crop_x, crop_y) in enumerate(CANDIDATE_TILES):
        try:
            record, pixels = records[raw_offset]
        except KeyError as exc:
            raise SystemExit(f"candidate source image 0x{raw_offset:x} was not decoded") from exc
        width = int(record["width"])
        height = int(record["height"])
        if crop_x < 0 or crop_y < 0 or crop_x + TILE_WIDTH > width or crop_y + TILE_HEIGHT > height:
            raise SystemExit(
                f"candidate crop {semantic} {crop_x},{crop_y} is outside {width}x{height} image 0x{raw_offset:x}"
            )
        for row in range(crop_y, crop_y + TILE_HEIGHT):
            start = row * width + crop_x
            atlas.extend(pixels[start : start + TILE_WIDTH])
        tiles[semantic] = {
            "x": index * TILE_WIDTH,
            "y": 0,
            "width": TILE_WIDTH,
            "height": TILE_HEIGHT,
            "source": f"raw:0x{raw_offset:x}+{crop_x},{crop_y}",
            "evidenceLevel": "hypothesis",
        }

    args.output_dir.mkdir(parents=True, exist_ok=True)
    write_png(args.output_dir / "normalized-atlas.png", len(CANDIDATE_TILES) * TILE_WIDTH, TILE_HEIGHT, bytes(atlas))
    source_metadata = json.dumps(source, ensure_ascii=False, sort_keys=True)
    manifest = {
        "schema": 1,
        "theme": "atari-st-original",
        "atlas": "normalized-atlas.png",
        "tileSize": 16,
        "pixelScale": 2,
        "palette": ["#%02x%02x%02x%02x" % value for value in PALETTE],
        "ui": {"background": 0, "foreground": 15, "accent": 14, "panel": 8},
        "provenance": "temporary candidate crops from decoded Atari ST image stream",
        "sourceStatus": "hypothesis; semantic mapping and runtime consumer are unknown",
        "evidenceNotes": {
            "source": "decoded image dimensions, offsets and pixels are proven by tools/decode_atari_st_images.py",
            "semanticMapping": "hypothesis; crop identity, anchor, collision and Apple II correspondence are unknown",
            "distribution": "temporary user-owned research output; do not commit the source image or atlas",
            "sourceMetadata": source_metadata,
            "variant": args.variant,
        },
        "tiles": tiles,
    }
    (args.output_dir / "theme.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(manifest, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
