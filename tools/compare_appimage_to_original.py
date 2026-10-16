#!/usr/bin/env python3
"""封裝好的 AppImage 跑出來的畫面，與原版磁碟一手資料的逐項對拍。

輸入是 tools/capture_appimage_screens.sh 拍下的 960×600 截圖目錄與原版 raw ``.st``。
五個對拍軸，全部以原版磁碟解出的資料當基準：

1. ``cockpit``／``tactical``：介面框的分區座標 vs ``docs/ui/original-layout.json``。
   量法與 ``atari_st_ui_frame.py`` 相同——邊框色的連通區塊取外接矩形。量出的矩形含
   1px 框線，所以寬高會比版面表大 1；x、y 必須全等。
2. ``status``：狀態視窗攜帶列的物品圖示 vs 圖示條（raw ``0x11552``）逐格像素。
3. ``ground``：Atari ST UX 的滿版城市是 8×5 個 40px pitch；每格先鋪 image 30 的
   40×40 背景，再於 (4,4) 畫原版 32×32 圖塊。逐格分成原版背景、黑色實心格、原版圖塊
   或有玩家／停放載具／未知 overlay 的 mixed，不能沿用 remake 版面的 10×7／16px 視埠。
4. ``engineering``：依 `show_system` 的原始座標公式檢查 4×4 格位；每格必須是同一張磁碟
   圖示條解出的完整 16×16 圖示。這證明 source pixels 與格位，不冒充原版同狀態畫面。
5. 標題畫面**不在對拍軸裡**：重製版的標題是刻意自有的中文手冊封面（B 區設計決定），
   與原版 image 0 不同是設計不是缺陷。

色彩展開使用 Hatari 實測與目前 runtime 相同的 ST 三位元 DAC 階梯（``v*0x22``，最大 238）。
舊版比較器曾用 ``v*255//7``，會把相同的原版色票誤報成差異。
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from PIL import Image

sys.path.insert(0, str(Path(__file__).parent))
from decode_atari_st_images import decode_record  # noqa: E402

# XSTARTUP:0x02 set_palette 的十六色（Atari ST 0RGB，各 3 bits），
# 展開式與 internal/source/atarist/originalmedia 的 PaletteRGBA 相同。
ST_PALETTE = (
    0x000, 0x005, 0x040, 0x055, 0x500, 0x505, 0x430, 0x555,
    0x222, 0x127, 0x070, 0x077, 0x700, 0x707, 0x770, 0x777,
)
RUNTIME_RGB = tuple(
    ((v >> 8 & 7) * 0x22, (v >> 4 & 7) * 0x22, (v & 7) * 0x22)
    for v in ST_PALETTE
)

# 駕駛台畫面上由即時狀態或子繪圖器擁有的矩形。其餘每一格都來自 raw image 10，
# 必須維持「由位元組推導」的像素證據。tools/check_title_pixel_parity.py 也 import
# 這一份——兩邊各留一份會漂移。
COCKPIT_DYNAMIC_REGIONS = (
    (5, 21, 85, 125),     # left menu content
    (16, 6, 73, 17),      # condition fill
    (96, 6, 258, 118),    # viewport content
    (141, 120, 213, 131), # clock and arrow band through y=130
    (273, 26, 310, 86),   # four gauges
    (278, 6, 304, 17),    # warp readout
    (5, 132, 272, 198),   # message/scanner/status content through bottom line
    (278, 132, 316, 199), # hull and six system bands
)

ICON_STRIP_OFFSET = 0x11552
FRAME_OFFSET = 0x14670
ST_GROUND_PITCH = 40
ST_GROUND_INSET = 4
ST_GROUND_TILE = 32
ST_GROUND_COLUMNS = 8
ST_GROUND_ROWS = 5
ST_GROUND_BACKDROP_OFFSET = 0x1B166
ST_REPAIR_WINDOW_X = 72
ST_REPAIR_WINDOW_Y = 64
ST_REPAIR_GRID_X0 = 51
ST_REPAIR_GRID_Y0 = 24
ST_REPAIR_GRID_DX = 22
ST_REPAIR_GRID_DY = 17


def downscale3(image: Image.Image) -> Image.Image:
    """960×600 → 320×200：取每個 3×3 區塊的中心像素（整數倍 nearest 放大的逆）。"""

    if image.size != (960, 600):
        raise ValueError(f"screenshot is {image.size}, expected 960x600")
    small = Image.new("RGB", (320, 200))
    source = image.convert("RGB").load()
    out = small.load()
    for y in range(200):
        for x in range(320):
            out[x, y] = source[x * 3 + 1, y * 3 + 1]
    return small


def image_rows(image: Image.Image) -> list[bytes]:
    raw = image.convert("RGB").tobytes()
    stride = image.size[0] * 3
    return [raw[y * stride : (y + 1) * stride] for y in range(image.size[1])]


def find_template(rows: list[bytes], cell, cell_w: int, cell_h: int) -> list[tuple[int, int]]:
    """整格全等的模板搜尋：第一列用 bytes.find 起步，其餘列逐一驗證。"""

    cell_rows = [
        b"".join(bytes(cell[cy * cell_w + cx]) for cx in range(cell_w))
        for cy in range(cell_h)
    ]
    hits: list[tuple[int, int]] = []
    for y in range(len(rows) - cell_h + 1):
        start = 0
        while True:
            found = rows[y].find(cell_rows[0], start)
            if found < 0:
                break
            start = found + 3
            if found % 3:
                continue
            x = found // 3
            if all(
                rows[y + cy][x * 3 : x * 3 + cell_w * 3] == cell_rows[cy]
                for cy in range(1, cell_h)
            ):
                hits.append((x, y))
                if len(hits) > 8:
                    return hits
    return hits


def connected_boxes(image: Image.Image, colour) -> list[dict[str, int]]:
    pixels = image.load()
    width, height = image.size
    seen = [[False] * width for _ in range(height)]
    found = []
    for sy in range(height):
        for sx in range(width):
            if seen[sy][sx] or pixels[sx, sy] != colour:
                continue
            stack = [(sx, sy)]
            seen[sy][sx] = True
            xs, ys = [sx], [sy]
            while stack:
                x, y = stack.pop()
                for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                    nx, ny = x + dx, y + dy
                    if 0 <= nx < width and 0 <= ny < height and not seen[ny][nx]:
                        if pixels[nx, ny] == colour:
                            seen[ny][nx] = True
                            stack.append((nx, ny))
                            xs.append(nx)
                            ys.append(ny)
            if len(xs) < 40:
                continue
            found.append({
                "x": min(xs),
                "y": min(ys),
                "width": max(xs) - min(xs) + 1,
                "height": max(ys) - min(ys) + 1,
            })
    return found


def decoded_picture(disk: bytes, offset: int) -> Image.Image:
    record, pixels = decode_record(disk, offset)
    width, height = int(record["width"]), int(record["height"])
    image = Image.new("RGB", (width, height))
    image.putdata([RUNTIME_RGB[index] for index in pixels])
    return image


def compare_frame(shot_path: Path, layout_path: Path, disk: bytes) -> dict[str, object]:
    layout = json.loads(layout_path.read_text())
    small = downscale3(Image.open(shot_path))
    viewport = next(r for r in layout["regions"] if r["id"] == "viewport")
    accent = small.load()[viewport["x"] + viewport["width"] // 2, viewport["y"]]
    boxes = connected_boxes(small, accent)
    results = []
    for region in layout["regions"]:
        expected = (region["x"], region["y"], region["width"], region["height"])
        best = None
        for box in boxes:
            got = (box["x"], box["y"], box["width"], box["height"])
            # 量出的矩形含 1px 框線：x、y 要全等，寬高允許剛好大 1。
            delta = max(
                abs(expected[0] - got[0]),
                abs(expected[1] - got[1]),
                abs(expected[2] + 1 - got[2]),
                abs(expected[3] + 1 - got[3]),
            )
            if best is None or delta < best[0]:
                best = (delta, got)
        results.append({
            "region": region["id"],
            "expected": list(expected),
            # A distant connected component is not a measurement of this region.
            # Fail closed instead of printing a plausible-looking wrong box.
            "measured": list(best[1]) if best and best[0] <= 2 else None,
            "exact": bool(best and best[0] == 0),
            "status": (
                "exact" if best and best[0] == 0 else
                "within-two-pixels" if best and best[0] <= 2 else
                "unverified"
            ),
        })
    original = decoded_picture(disk, FRAME_OFFSET)
    if original.size != (320, 200):
        raise ValueError(f"frame image is {original.size}, expected 320x200")
    dynamic = COCKPIT_DYNAMIC_REGIONS
    shot_pixels, original_pixels = small.load(), original.load()
    differences = []
    for y in range(200):
        for x in range(320):
            if any(x0 <= x < x1 and y0 <= y < y1 for x0, y0, x1, y1 in dynamic):
                continue
            if shot_pixels[x, y] != original_pixels[x, y]:
                differences.append([x, y])
    return {
        "sourceFrame": {
            "rawOffset": hex(FRAME_OFFSET),
            "staticPixelsCompared": 320 * 200 - sum((x1-x0)*(y1-y0) for x0,y0,x1,y1 in dynamic),
            "differentPixels": len(differences),
            "status": "exact" if not differences else "mismatch",
            "firstDifferences": differences[:20],
        },
        "regions": results,
    }


def decoded_icons(disk: bytes) -> list[list[tuple[int, int, int]]]:
    record, pixels = decode_record(disk, ICON_STRIP_OFFSET)
    width = int(record["width"])
    icons = []
    for index in range(31):
        icons.append([
            RUNTIME_RGB[pixels[y * width + index * 16 + x]]
            for y in range(16)
            for x in range(16)
        ])
    return icons


def decoded_icon_indices(disk: bytes) -> list[list[int]]:
    """Return the raw cells; palette index zero is transparent at runtime."""

    record, pixels = decode_record(disk, ICON_STRIP_OFFSET)
    width = int(record["width"])
    return [
        [pixels[y * width + index * 16 + x] for y in range(16) for x in range(16)]
        for index in range(31)
    ]


def find_transparent_icon(image: Image.Image, cell: list[int]) -> tuple[int, int] | None:
    """Find a 3x icon while ignoring source index zero, as Ebiten alpha does."""

    pixels = image.convert("RGB").load()
    width, height = image.size
    opaque = [
        (x * 3, y * 3, RUNTIME_RGB[cell[y * 16 + x]])
        for y in range(16) for x in range(16) if cell[y * 16 + x] != 0
    ]
    if not opaque:
        return None
    anchor_x, anchor_y, anchor_colour = opaque[0]
    for y in range(height - 48 + 1):
        for x in range(width - 48 + 1):
            if pixels[x + anchor_x, y + anchor_y] != anchor_colour:
                continue
            if all(
                pixels[x + dx + sx, y + dy + sy] == colour
                for dx, dy, colour in opaque
                for sy in range(3) for sx in range(3)
            ):
                return x, y
    return None


def compare_icons(shot_path: Path, disk: bytes, expected: list[int] | None = None) -> dict[str, object]:
    found = {}
    image = Image.open(shot_path)
    cells = decoded_icon_indices(disk)
    indices = range(len(cells)) if expected is None else expected
    for index in indices:
        if index < 0 or index >= len(cells):
            continue
        cell = cells[index]
        hit = find_transparent_icon(image, cell)
        if hit is not None:
            found[index] = list(hit)
    missing = [] if expected is None else [index for index in expected if index not in found]
    return {
        "status": "exact-source-icons" if expected is not None and not missing else "unverified",
        "expectedIcons": expected,
        "iconsFound": {str(k): v for k, v in sorted(found.items())},
        "missingIcons": missing,
        "distinctIcons": len(found),
    }


def compare_engineering_grid(shot_path: Path, disk: bytes) -> dict[str, object]:
    """Check show_system's 4×4 centres against exact disk-derived icon pixels."""

    small = downscale3(Image.open(shot_path))
    pixels = small.load()
    icons = decoded_icons(disk)
    slots = []
    matched = 0
    for row in range(4):
        for column in range(4):
            # add_item's coordinate is the icon centre, relative to the zoom's
            # inside origin (outer edge + 1).
            x = ST_REPAIR_WINDOW_X + 1 + ST_REPAIR_GRID_X0 + ST_REPAIR_GRID_DX * column - 8
            y = ST_REPAIR_WINDOW_Y + 1 + ST_REPAIR_GRID_Y0 + ST_REPAIR_GRID_DY * row - 8
            # show_function's 7×7 row-status window is drawn after the icons.
            # Its source coordinates overlap the second icon by 2×7 pixels;
            # mask precisely that documented overlay, not the whole icon.
            status_x0 = ST_REPAIR_WINDOW_X + 1 + 60
            status_y0 = ST_REPAIR_WINDOW_Y + 1 + 20 + ST_REPAIR_GRID_DY * row
            masked = []
            icon = None
            for index, candidate in enumerate(icons):
                equal = True
                for dy in range(16):
                    for dx in range(16):
                        sx, sy = x + dx, y + dy
                        if status_x0 <= sx < status_x0 + 7 and status_y0 <= sy < status_y0 + 7:
                            masked.append([dx, dy])
                            continue
                        if pixels[sx, sy] != candidate[dy * 16 + dx]:
                            equal = False
                            break
                    if not equal:
                        break
                if equal:
                    icon = index
                    break
            if icon is not None:
                matched += 1
            slots.append({
                "row": row,
                "column": column,
                "origin": [x, y],
                "iconIndex": icon,
                "knownStatusOverlayPixels": len({tuple(point) for point in masked}),
            })
    return {
        "status": "exact-source-icons-with-known-status-overlays" if matched == 16 else "unverified",
        "rawIconStripOffset": hex(ICON_STRIP_OFFSET),
        "matchedSlots": matched,
        "expectedSlots": 16,
        "slots": slots,
        "scope": "source pixels and show_system geometry only; not an original same-state screenshot",
    }


def decoded_tiles(disk: bytes, directory: dict) -> list[tuple]:
    tiles = []
    for strip in directory["tileStrips"]:
        record, pixels = decode_record(disk, int(str(strip["rawOffset"]), 16))
        width = int(record["width"])
        size = int(strip["tileSize"])
        for tile in range(int(strip["tileCount"])):
            tiles.append(tuple(
                RUNTIME_RGB[pixels[y * width + tile * size + x]]
                for y in range(size)
                for x in range(size)
            ))
    return tiles


def compare_ground(shot_path: Path, disk: bytes, directory_path: Path) -> dict[str, object]:
    directory = json.loads(directory_path.read_text())
    return classify_ground_cells(downscale3(Image.open(shot_path)), disk, directory)


def classify_ground_cells(small: Image.Image, disk: bytes, directory: dict) -> dict[str, object]:
    """把 320×200 的城市地面切成 8×5 格 40px pitch，逐格判定它是什麼。

    每一格只有四種身分：整格就是背景圖、整格全黑、內縮 4px 的 32×32 是磁碟上的某個圖塊，
    或以上皆非（`mixed`）。`mixed` 不等於錯——玩家與訊息窗蓋住的那幾格本來就會落在這裡，
    所以另外列 `unknownMixedCells`：那才是需要解釋的。

    `tools/check_ground_cells.py` 也呼叫這一支，差別只在它餵進來的是 port oracle 的畫面。
    """
    pixels = small.load()
    tileset = set(decoded_tiles(disk, directory))
    backdrop_record, backdrop_pixels = decode_record(disk, ST_GROUND_BACKDROP_OFFSET)
    backdrop_width = int(backdrop_record["width"])
    background = tuple(
        RUNTIME_RGB[backdrop_pixels[y * backdrop_width + x]]
        for y in range(ST_GROUND_PITCH)
        for x in range(ST_GROUND_PITCH)
    )
    black = (RUNTIME_RGB[0],) * (ST_GROUND_PITCH * ST_GROUND_PITCH)
    counts = {"background": 0, "solidBlack": 0, "originalTile": 0, "mixed": 0}
    mixed = []
    for row in range(ST_GROUND_ROWS):
        for column in range(ST_GROUND_COLUMNS):
            origin_x = column * ST_GROUND_PITCH
            origin_y = row * ST_GROUND_PITCH
            cell = tuple(
                pixels[origin_x + x, origin_y + y]
                for y in range(ST_GROUND_PITCH)
                for x in range(ST_GROUND_PITCH)
            )
            tile = tuple(
                pixels[origin_x + ST_GROUND_INSET + x, origin_y + ST_GROUND_INSET + y]
                for y in range(ST_GROUND_TILE)
                for x in range(ST_GROUND_TILE)
            )
            if cell == background:
                counts["background"] += 1
            elif cell == black:
                counts["solidBlack"] += 1
            elif tile in tileset:
                counts["originalTile"] += 1
            else:
                counts["mixed"] += 1
                mixed.append([column, row])
    known_overlays = {
        (4, 2),  # walker fixed at the original's screen cell
        (0, 4), (1, 4), (2, 4), (3, 4), (4, 4),  # message window x=4..203, y=172..195
    }
    unknown_mixed = [cell for cell in mixed if tuple(cell) not in known_overlays]
    status = "exact-source-cells-with-known-overlays" if not unknown_mixed else "unverified-mixed-cells"
    return {
        "status": status,
        "screenCells": ST_GROUND_COLUMNS * ST_GROUND_ROWS,
        "counts": counts,
        "mixedCells": mixed,
        "unknownMixedCells": unknown_mixed,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("shots", type=Path, help="capture_appimage_screens.sh 的輸出目錄")
    parser.add_argument("disk", type=Path, help="原版 raw .st")
    parser.add_argument("--repo", type=Path, default=Path(__file__).parent.parent)
    arguments = parser.parse_args()

    disk = arguments.disk.read_bytes()
    layout = arguments.repo / "docs/ui/original-layout.json"
    directory = arguments.repo / "docs/atari-st/image-directory.json"
    status_shot = arguments.shots / "status-items.png"
    status_fixture = arguments.shots / "status-items.fixture"
    expected_status_icons = None
    if status_fixture.exists():
        fixture_path = arguments.shots / status_fixture.read_text(encoding="utf-8").strip()
        fixture = json.loads(fixture_path.read_text(encoding="utf-8"))["state"]
        item_codes = ([fixture["handItem"]] if fixture.get("handItem") else []) + (fixture.get("parts") or [])
        expected_status_icons = sorted(set(code - 1 for code in item_codes))
    report = {
        "tool": "tools/compare_appimage_to_original.py",
        "cockpitFrame": compare_frame(arguments.shots / "cockpit.png", layout, disk),
        "tacticalFrame": compare_frame(arguments.shots / "tactical.png", layout, disk),
        "statusIcons": compare_icons(status_shot, disk, expected_status_icons) if status_shot.exists() and status_fixture.exists() else {
            "status": "unverified",
            "reason": "capture did not include a declared items-save fixture",
        },
        "engineeringGrid": compare_engineering_grid(arguments.shots / "engineering.png", disk),
        "ground": compare_ground(arguments.shots / "ground-atari.png", disk, directory),
    }
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
