#!/usr/bin/env python3
"""固定 Modern Apple A/B 美術 prototype 的展示尺寸與證據雜湊。

這個工具只處理可丟棄的展示圖，不會產生 runtime atlas，也不會修改正式 v1 資產。
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


DISPLAY_SIZE = (960, 600)
COMPARISON_SIZE = (1920, 600)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def normalize_scene(source: Path, destination: Path) -> None:
    with Image.open(source) as image:
        image = image.convert("RGB")
        source_ratio = image.width / image.height
        target_ratio = DISPLAY_SIZE[0] / DISPLAY_SIZE[1]
        if source_ratio > target_ratio:
            width = round(image.height * target_ratio)
            left = (image.width - width) // 2
            image = image.crop((left, 0, left + width, image.height))
        elif source_ratio < target_ratio:
            height = round(image.width / target_ratio)
            top = (image.height - height) // 2
            image = image.crop((0, top, image.width, top + height))
        # 這是展示 mockup 的固定畫框，不是正式 sprite／tileset 的重採樣流程。
        image = image.resize(DISPLAY_SIZE, Image.Resampling.LANCZOS)
        image.save(destination, format="PNG", optimize=True)


def copy_contact_sheet(source: Path, destination: Path) -> None:
    with Image.open(source) as image:
        image.convert("RGB").save(destination, format="PNG", optimize=True)


def make_labeled_contact_sheet(source: Path, destination: Path) -> None:
    with Image.open(source) as source_image:
        image = source_image.convert("RGB")
    draw = ImageDraw.Draw(image)
    try:
        font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", 15)
    except OSError:
        font = ImageFont.load_default()

    # 這些標註位於 sprite 間的深色留白，只描述展示尺寸／佔地，不是 sprite 內容。
    labels = [
        (82, 306, "G1 60px 1x1"),
        (373, 306, "G2 60px 1x1"),
        (661, 306, "G3 60px 1x1"),
        (954, 306, "G4 60px 1x1"),
        (82, 606, "W1 60px 1x1"),
        (373, 606, "W2 60px 1x1"),
        (661, 606, "W3 60px 1x1"),
        (954, 606, "W4 60px 1x1"),
        (82, 884, "R1 60px 1x1"),
        (289, 884, "R2 60px 1x1"),
        (514, 884, "R3 60px 1x1"),
        (751, 884, "R4 60px 1x1"),
        (1003, 884, "SHORE 60px 1x1"),
        (111, 1173, "PLAYER 60px 1x1"),
        (305, 1173, "CARGO 60px 1x1"),
        (556, 1173, "CITY 120px 2x2"),
        (884, 1173, "SHUTTLE 120px 2x2"),
    ]
    for x, y, label in labels:
        bbox = draw.textbbox((x, y), label, font=font)
        pad = 3
        draw.rectangle(
            (bbox[0] - pad, bbox[1] - pad, bbox[2] + pad, bbox[3] + pad),
            fill=(8, 12, 18),
            outline=(202, 145, 49),
            width=1,
        )
        draw.text((x, y), label, fill=(238, 228, 195), font=font)
    image.save(destination, format="PNG", optimize=True)


def make_comparison(a_path: Path, b_path: Path, destination: Path) -> None:
    with Image.open(a_path) as a, Image.open(b_path) as b:
        comparison = Image.new("RGB", COMPARISON_SIZE, (8, 12, 18))
        comparison.paste(a.convert("RGB"), (0, 0))
        comparison.paste(b.convert("RGB"), (DISPLAY_SIZE[0], 0))
        draw = ImageDraw.Draw(comparison)
        draw.line((DISPLAY_SIZE[0] - 1, 0, DISPLAY_SIZE[0] - 1, DISPLAY_SIZE[1]), fill=(202, 145, 49), width=2)
        comparison.save(destination, format="PNG", optimize=True)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--a", type=Path, required=True, help="A prototype source image")
    parser.add_argument("--b", type=Path, required=True, help="B prototype source image")
    parser.add_argument("--contact-sheet", type=Path, required=True, help="B contact sheet source image")
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()

    for path in (args.a, args.b, args.contact_sheet):
        if not path.is_file():
            raise SystemExit(f"missing source image: {path}")

    args.output_dir.mkdir(parents=True, exist_ok=True)
    a_output = args.output_dir / "modern-apple-adjustment-a-v2.png"
    b_output = args.output_dir / "modern-apple-adjustment-b-v2.png"
    contact_output = args.output_dir / "modern-apple-adjustment-b-contact-v2.png"
    labeled_contact_output = args.output_dir / "modern-apple-adjustment-b-contact-labeled-v2.png"
    comparison_output = args.output_dir / "modern-apple-adjustment-ab-v2.png"

    normalize_scene(args.a, a_output)
    normalize_scene(args.b, b_output)
    copy_contact_sheet(args.contact_sheet, contact_output)
    make_labeled_contact_sheet(args.contact_sheet, labeled_contact_output)
    make_comparison(a_output, b_output, comparison_output)

    manifest = {
        "schema": 1,
        "status": "prototype-only",
        "created": "2026-08-12",
        "theme": "modern-apple",
        "version": "2026-08-12-adjustment-v2",
        "sourceImages": {
            "a": {"label": "Luna A conservative adjustment source", "sha256": sha256(args.a)},
            "b": {"label": "Luna B controlled pixel-group source", "sha256": sha256(args.b)},
            "bContactSheet": {"label": "Luna B asset contact sheet source", "sha256": sha256(args.contact_sheet)},
        },
        "outputs": {
            "a": {"path": a_output.name, "size": list(DISPLAY_SIZE), "sha256": sha256(a_output)},
            "b": {"path": b_output.name, "size": list(DISPLAY_SIZE), "sha256": sha256(b_output)},
            "abComparison": {
                "path": comparison_output.name,
                "size": list(COMPARISON_SIZE),
                "sha256": sha256(comparison_output),
            },
            "bContactSheet": {
                "path": contact_output.name,
                "size": list(Image.open(contact_output).size),
                "sha256": sha256(contact_output),
            },
            "bContactSheetLabeled": {
                "path": labeled_contact_output.name,
                "size": list(Image.open(labeled_contact_output).size),
                "sha256": sha256(labeled_contact_output),
            },
        },
        "boundaries": [
            "所有輸出都是可丟棄 prototype，不是正式 runtime 資產。",
            "現有 Modern Apple v1 atlas、manifest、runtime、碰撞與存檔保持不變。",
            "Apple II 只提供語意與資訊階層基準；沒有複製或嵌入 Apple II／Atari ST 原始像素。",
            "等待使用者選擇 A 或 B 後，才進行正式 sprite／tileset 拆分。",
        ],
    }
    manifest_path = args.output_dir / "modern-apple-adjustment-v2-manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
