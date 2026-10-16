#!/usr/bin/env python3
"""建立 Modern Apple 的正式高解析度、原創 raster sprite／tileset atlas。

輸入是本專案授權的 imagegen 原創來源圖，不是 Apple II、Atari ST 或任何原版遊戲媒體。
Apple II 只約束語意、輪廓和資訊階層；這個工具把那些語意名稱映射到新繪製的 128×128
資產格，並寫出可審查的雜湊 manifest。

sprite 來源使用色鍵背景，必須經 imagegen 的 remove_chroma_key.py helper 去背；地形來源則
保持不透明。輸出 atlas 可被 Go 的內嵌 Modern Apple asset group 載入，不能當作原版素材或
原版 parity 的證據。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

from PIL import Image, ImageDraw, ImageOps


SCHEMA = 1
ASSET_VERSION = "2026-08-12-v1"
CELL_SIZE = 128
ATLAS_COLUMNS = 4
ATLAS_ROWS = 2
LOGICAL_TILE_SIZE = 20
CHROMA_KEY = "#ff00ff"
TILE_ORDER = ("grass", "water", "road", "wall", "ship", "cargo", "city", "player")

# These names identify the authorised image-generation source artifacts by basename only. Their
# SHA-256 values are recorded in the produced manifest; raw generation sources remain outside the
# repository and are never a runtime dependency.
SOURCE_FILES = {
    "ship": "exec-0f08a938-36ae-42eb-8e89-08191b22c710.png",
    "grass": "exec-4a4d65e5-3a6b-4347-a1be-298a0872c8f4.png",
    "water": "exec-b4b18129-4962-4380-a703-e3b0d3768947.png",
    "road": "exec-2d6d6061-90b9-404d-9734-cfd233916817.png",
    "wall": "exec-0e478db7-48dd-4791-b2fc-25bc53229427.png",
    "city": "exec-230f80f0-8534-44da-96d5-a5540e9e1f09.png",
    "cargo": "exec-e1d62e9c-5c41-4fc6-9562-8c1ca9764428.png",
    "player": "exec-50ee4811-e265-41d2-b8f1-32dfe9efec7d.png",
}
TERRAIN_TILES = {"grass", "water", "road", "wall"}
SPRITE_FILL = {"ship": 0.91, "cargo": 0.70, "city": 0.84, "player": 0.72}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-dir", type=Path, required=True, help="imagegen 原創來源圖所在的唯讀目錄")
    parser.add_argument("--output-dir", type=Path, required=True, help="atlas、contact sheet 與 manifest 的輸出目錄")
    parser.add_argument(
        "--chroma-key-helper",
        type=Path,
        required=True,
        help="imagegen remove_chroma_key.py 的可讀檔案路徑",
    )
    parser.add_argument("--force", action="store_true", help="只覆寫本工具產生的同名輸出")
    return parser.parse_args()


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def require_file(path: Path, label: str) -> None:
    if not path.is_file():
        raise SystemExit(f"{label} 不存在：{path}")


def alpha_stats(image: Image.Image) -> dict[str, int]:
    alpha = image.getchannel("A")
    values = list(alpha.getdata())
    return {
        "transparentPixels": sum(value == 0 for value in values),
        "partialAlphaPixels": sum(0 < value < 255 for value in values),
        "opaquePixels": sum(value == 255 for value in values),
    }


def magenta_residuals(image: Image.Image) -> int:
    residuals = 0
    for red, green, blue, alpha in image.getdata():
        if alpha > 8 and red >= 220 and green <= 45 and blue >= 220:
            residuals += 1
    return residuals


def prepare_terrain(source: Path) -> tuple[Image.Image, dict[str, Any]]:
    image = Image.open(source).convert("RGBA")
    result = ImageOps.fit(image, (CELL_SIZE, CELL_SIZE), method=Image.Resampling.LANCZOS, centering=(0.5, 0.5))
    result.putalpha(Image.new("L", result.size, 255))
    return result, {
        "transform": "ImageOps.fit 1254px source to a full-bleed 128x128 terrain tile using Lanczos",
        "alpha": alpha_stats(result),
    }


def remove_chroma(source: Path, helper: Path, temp_dir: Path) -> Path:
    output = temp_dir / f"{source.stem}-alpha.png"
    command = [
        sys.executable,
        str(helper),
        "--input",
        str(source),
        "--out",
        str(output),
        "--auto-key",
        "border",
        "--soft-matte",
        "--transparent-threshold",
        "12",
        "--opaque-threshold",
        "220",
        "--despill",
    ]
    subprocess.run(command, check=True, capture_output=True, text=True)
    require_file(output, "色鍵去背輸出")
    return output


def prepare_sprite(source: Path, helper: Path, temp_dir: Path, fill: float) -> tuple[Image.Image, dict[str, Any]]:
    alpha_path = remove_chroma(source, helper, temp_dir)
    image = Image.open(alpha_path).convert("RGBA")
    alpha = image.getchannel("A")
    bbox = alpha.getbbox()
    if bbox is None:
        raise SystemExit(f"sprite {source.name} 去背後沒有不透明像素")
    if any(alpha.getpixel(point) != 0 for point in ((0, 0), (0, image.height - 1), (image.width - 1, 0), (image.width - 1, image.height - 1))):
        raise SystemExit(f"sprite {source.name} 的四個角落未完全透明")
    residuals = magenta_residuals(image)
    if residuals:
        raise SystemExit(f"sprite {source.name} 有 {residuals} 個殘留色鍵像素")

    # Preserve the anti-aliased contour just outside the solid bounding box, then normalize each
    # sprite independently into its declared cell occupancy. The source image itself is never
    # copied into the repository.
    padding = max(2, int(max(bbox[2] - bbox[0], bbox[3] - bbox[1]) * 0.018))
    crop = image.crop(
        (
            max(0, bbox[0] - padding),
            max(0, bbox[1] - padding),
            min(image.width, bbox[2] + padding),
            min(image.height, bbox[3] + padding),
        )
    )
    scale = min((CELL_SIZE * fill) / crop.width, (CELL_SIZE * fill) / crop.height)
    width = max(1, round(crop.width * scale))
    height = max(1, round(crop.height * scale))
    crop = crop.resize((width, height), Image.Resampling.LANCZOS)
    result = Image.new("RGBA", (CELL_SIZE, CELL_SIZE), (0, 0, 0, 0))
    result.alpha_composite(crop, ((CELL_SIZE - width) // 2, (CELL_SIZE - height) // 2))
    return result, {
        "transform": f"chroma-key alpha removal then centered Lanczos normalization at {fill:.2f} cell occupancy",
        "chromaKey": CHROMA_KEY,
        "chromaKeyResidualPixels": residuals,
        "alpha": alpha_stats(result),
        "visibleBounds": list(result.getchannel("A").getbbox() or (0, 0, 0, 0)),
    }


def make_contact_sheet(atlas: Image.Image, output: Path) -> None:
    width = ATLAS_COLUMNS * CELL_SIZE
    height = ATLAS_ROWS * (CELL_SIZE + 22)
    sheet = Image.new("RGBA", (width, height), (14, 18, 28, 255))
    draw = ImageDraw.Draw(sheet)
    for index, tile in enumerate(TILE_ORDER):
        x = (index % ATLAS_COLUMNS) * CELL_SIZE
        y = (index // ATLAS_COLUMNS) * CELL_SIZE
        cell = atlas.crop((x, y, x + CELL_SIZE, y + CELL_SIZE))
        sheet.alpha_composite(cell, (x, (index // ATLAS_COLUMNS) * (CELL_SIZE + 22) + 22))
        draw.text((x + 6, (index // ATLAS_COLUMNS) * (CELL_SIZE + 22) + 5), tile, fill=(244, 231, 188, 255))
    sheet.save(output, format="PNG", optimize=True)


def validate_output(atlas_path: Path, manifest: dict[str, Any]) -> None:
    image = Image.open(atlas_path).convert("RGBA")
    if image.size != (ATLAS_COLUMNS * CELL_SIZE, ATLAS_ROWS * CELL_SIZE):
        raise SystemExit(f"atlas 尺寸 {image.size} 不符預期")
    if manifest["atlasSha256"] != sha256(atlas_path):
        raise SystemExit("atlas SHA-256 與 manifest 不一致")
    if set(manifest["tiles"]) != set(TILE_ORDER):
        raise SystemExit("manifest 的 semantic tile 集合不完整")
    for tile in TILE_ORDER:
        record = manifest["tiles"][tile]
        if record["width"] != CELL_SIZE or record["height"] != CELL_SIZE:
            raise SystemExit(f"{tile} cell 尺寸不符")
        crop = image.crop((record["x"], record["y"], record["x"] + CELL_SIZE, record["y"] + CELL_SIZE))
        if tile in TERRAIN_TILES and crop.getchannel("A").getextrema() != (255, 255):
            raise SystemExit(f"terrain {tile} 不應含透明像素")
        if tile not in TERRAIN_TILES:
            alpha = crop.getchannel("A")
            if alpha.getbbox() is None or alpha.getpixel((0, 0)) != 0:
                raise SystemExit(f"sprite {tile} 的 alpha 邊界不正確")


def build(args: argparse.Namespace) -> dict[str, Any]:
    require_file(args.chroma_key_helper, "色鍵去背 helper")
    output_dir = args.output_dir
    output_dir.mkdir(parents=True, exist_ok=True)
    atlas_path = output_dir / "modern-apple-atlas.png"
    contact_path = output_dir / "modern-apple-atlas-contact.png"
    manifest_path = output_dir / "modern-apple-atlas-manifest.json"
    existing = [path for path in (atlas_path, contact_path, manifest_path) if path.exists()]
    if existing and not args.force:
        raise SystemExit("輸出已存在；如要重建請加 --force：" + ", ".join(str(path) for path in existing))

    atlas = Image.new("RGBA", (ATLAS_COLUMNS * CELL_SIZE, ATLAS_ROWS * CELL_SIZE), (0, 0, 0, 0))
    source_artifacts: dict[str, Any] = {}
    tiles: dict[str, Any] = {}
    with tempfile.TemporaryDirectory(prefix="sundog-modern-apple-chroma-") as temp_name:
        temp_dir = Path(temp_name)
        for index, tile in enumerate(TILE_ORDER):
            source = args.source_dir / SOURCE_FILES[tile]
            require_file(source, f"{tile} 原創來源圖")
            if tile in TERRAIN_TILES:
                prepared, transform = prepare_terrain(source)
                kind = "terrain"
            else:
                prepared, transform = prepare_sprite(source, args.chroma_key_helper, temp_dir, SPRITE_FILL[tile])
                kind = "sprite"
            x = (index % ATLAS_COLUMNS) * CELL_SIZE
            y = (index // ATLAS_COLUMNS) * CELL_SIZE
            atlas.alpha_composite(prepared, (x, y))
            source_artifacts[tile] = {
                "basename": source.name,
                "sha256": sha256(source),
                "kind": kind,
                **transform,
            }
            tiles[tile] = {
                "x": x,
                "y": y,
                "width": CELL_SIZE,
                "height": CELL_SIZE,
                "source": "remake-owned original high-resolution raster design",
                # The art bytes themselves are proven by the manifest hash. The correspondence to
                # original Apple II visual semantics remains an explicit hypothesis until more
                # original graphics consumers are decoded.
                "evidenceLevel": "hypothesis",
                "layer": "terrain" if tile in TERRAIN_TILES else "sprite",
            }

    atlas.save(atlas_path, format="PNG", optimize=True)
    make_contact_sheet(atlas, contact_path)
    manifest: dict[str, Any] = {
        "schema": SCHEMA,
        "assetVersion": ASSET_VERSION,
        "theme": "modern-apple",
        "atlas": atlas_path.name,
        "atlasSha256": sha256(atlas_path),
        "atlasWidth": atlas.width,
        "atlasHeight": atlas.height,
        "cellSize": CELL_SIZE,
        "logicalTileSize": LOGICAL_TILE_SIZE,
        "filter": "linear",
        "provenance": "original high-resolution 2D raster redesign approved by the user on 2026-08-12",
        "sourceStatus": "official Modern Apple production asset; original Apple II graphic consumer and exact source-frame mapping remain unknown",
        "boundaries": {
            "appleII": "Apple II is the sole original visual/game-semantic oracle; its RAM/HGR is never used as remake runtime, save, or asset format.",
            "atariST": "Atari ST remains an asset-research source only and does not replace Apple II semantic evidence.",
            "originalMedia": "No Apple II, Atari ST, FTL, SunDog, or other original game image was copied into this atlas.",
            "gameplay": "This atlas must not change map semantics, collision, secrets, state, or save data.",
        },
        "build": {
            "tool": "tools/build_modern_apple_atlas.py",
            "chromaKey": CHROMA_KEY,
            "chromaKeyHelper": "remove_chroma_key.py with border auto-key, soft matte, thresholds 12/220, despill",
            "sourceArtifactAvailability": "imagegen source images are external inputs; only their basenames and SHA-256 values are recorded here",
        },
        "sourceArtifacts": source_artifacts,
        "tiles": tiles,
    }
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    validate_output(atlas_path, manifest)
    return {
        "status": "pass",
        "atlas": str(atlas_path),
        "contactSheet": str(contact_path),
        "manifest": str(manifest_path),
        "atlasSha256": manifest["atlasSha256"],
        "tiles": len(tiles),
    }


def main() -> int:
    args = parse_args()
    print(json.dumps(build(args), ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
