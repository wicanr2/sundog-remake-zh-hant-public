#!/usr/bin/env python3
"""在實跑畫面上找出所有逐像素等於磁碟圖示條的 16×16 區塊。

工程 4×4 那一格的圖示比對（`compare_appimage_to_original` 的 `compare_status_icons`）是
**先知道座標再比**——版面幾何早就解出來了。Ship's Locker 沒有那份幾何，所以這支反過來做：
不預設位置，掃過整張畫面，把每個 16×16 區塊拿去對 raw `0x11552` 圖示條的 31 格，
完全相等才算命中。找到的位置本身就是版面的證據。

反過來做還有一個好處：它不會因為「我以為圖示在這裡」而漏掉真的畫在別處的圖示，
也不會把一個對不上的區塊硬說成某個圖示。代價是慢——所以先比左上角那一個像素篩掉絕大多數，
再做完整的 256 格比對。

`--expect` 用來把「畫面上該有什麼」寫成閘門，例如新遊戲的 Ship's Locker 是
`--expect shunt=5 --expect '(trash)=1'`（品名取自 `docs/atari-st/item-icons.json`）。

**這支只對「圖示沒有被疊東西」的畫面有效。** 原版的 Ship's Locker 是那樣，所以六個圖示
逐像素命中；重製版的角色卡不是——它會在圖示上疊介面元素，逐像素全等因此掃不到任何東西
（實測 0 命中）。那種畫面要用 `compare_appimage_to_original.compare_status_icons` 那條路：
先知道座標、明文遮掉有文件記載的覆蓋區再比。

放寬成「差 N 個像素以內」不能解決這件事，反而會製造假陽性：`shunt` 只有 32 個 ink 像素，
容差 40 會讓它在任何一塊純色區域上都「接近命中」（實測 34,061 個候選）。

所以重製版那一側走 `--remake`，並要求指定 `--remake-icons`，因為兩邊畫面的內容本來就不同——
原版的 Ship's Locker 有垃圾桶與整櫃物品，重製版的角色卡只有身上那一件。要比的是「同一個狀態下，
兩邊都畫出磁碟同一格圖示」。

比法有兩種，`--remake-mode` 選：

* `icons`（預設）＝透明比對，全畫面找 ink 像素相符的位置。角色卡那種畫面用這個。
* `engineering` ＝ `show_system` 的 4×4 網格，先知道座標再比，並遮掉那個 7×7 狀態視窗。
  維修畫面**必須**用這個：狀態視窗蓋住第二個圖示 2×7 像素，透明比對因此漏掉 `flux modulator`
  （實測 16 格裡只找得到 3 種）。
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys

from PIL import Image

from check_ground_cells import to_runtime_palette
from compare_appimage_to_original import (
    ICON_STRIP_OFFSET,
    compare_engineering_grid,
    compare_icons,
    decoded_icons,
)

ICON_SIZE = 16


def scan(shot: Image.Image, disk: bytes) -> list[dict]:
    """回傳畫面上每一個逐像素命中的圖示：位置與圖示條索引。"""
    icons = decoded_icons(disk)
    small = to_runtime_palette(shot)
    px = small.load()
    width, height = small.size
    hits: list[dict] = []
    for y in range(height - ICON_SIZE):
        for x in range(width - ICON_SIZE):
            corner = px[x, y]
            for index, icon in enumerate(icons):
                if corner != icon[0]:
                    continue
                if all(
                    px[x + dx, y + dy] == icon[dy * ICON_SIZE + dx]
                    for dy in range(ICON_SIZE)
                    for dx in range(ICON_SIZE)
                ):
                    hits.append({"x": x, "y": y, "iconIndex": index})
    return hits


def name_hits(hits: list[dict], icon_table: list[dict]) -> list[dict]:
    by_index = {entry["index"]: entry for entry in icon_table}
    named = []
    for hit in hits:
        entry = by_index.get(hit["iconIndex"], {})
        named.append({**hit, "itemName": entry.get("itemName"), "itemCode": entry.get("itemCode")})
    return named


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("shot", type=pathlib.Path, help="實跑截圖")
    parser.add_argument("disk", type=pathlib.Path, help="原版 raw .st")
    parser.add_argument(
        "--icons",
        type=pathlib.Path,
        default=pathlib.Path(__file__).parent.parent / "docs/atari-st/item-icons.json",
    )
    parser.add_argument(
        "--remake",
        type=pathlib.Path,
        help="重製版在同一狀態下的截圖；用透明比對找 --remake-icons 指定的圖示",
    )
    parser.add_argument(
        "--remake-icons",
        help="要在重製版畫面上找到的圖示索引，逗號分隔（例如 shunt 是 22）",
    )
    parser.add_argument(
        "--remake-mode",
        choices=("icons", "engineering"),
        default="icons",
        help=(
            "重製版那一側怎麼比。icons＝透明比對全畫面找；"
            "engineering＝show_system 的 4×4 網格，先知道座標再比並遮掉狀態視窗"
        ),
    )
    parser.add_argument(
        "--expect",
        action="append",
        default=[],
        metavar="名稱=數量",
        help="要求畫面上某個品名出現幾次；可重複",
    )
    args = parser.parse_args(argv)

    icon_table = json.loads(args.icons.read_text())["icons"]
    hits = name_hits(scan(Image.open(args.shot), args.disk.read_bytes()), icon_table)

    counts: dict[str, int] = {}
    for hit in hits:
        counts[hit["itemName"]] = counts.get(hit["itemName"], 0) + 1

    print(json.dumps(
        {
            "rawIconStripOffset": hex(ICON_STRIP_OFFSET),
            "hits": hits,
            "counts": counts,
        },
        ensure_ascii=False, indent=2, sort_keys=True,
    ))

    failures = []
    for rule in args.expect:
        name, _, want = rule.rpartition("=")
        if not name:
            parser.error(f"--expect 要寫成 名稱=數量：{rule}")
        got = counts.get(name, 0)
        if got != int(want):
            failures.append(f"{name}：期望 {want}，實得 {got}")
    if failures:
        print("FAIL：" + "；".join(failures))
        return 1
    if args.expect:
        print("PASS：原版畫面上的圖示逐像素來自磁碟，且數量符合期望")

    if not args.remake:
        return 0
    if not args.remake_icons:
        parser.error("--remake 要配 --remake-icons")
    wanted = [int(v) for v in args.remake_icons.split(",")]
    missing_here = [i for i in wanted if i not in {h["iconIndex"] for h in hits}]
    if missing_here:
        print(f"FAIL：原版畫面上就沒有圖示 {missing_here}，同狀態的前提不成立")
        return 1
    if args.remake_mode == "engineering":
        grid = compare_engineering_grid(args.remake, args.disk.read_bytes())
        found = sorted({s["iconIndex"] for s in grid["slots"] if s["iconIndex"] is not None})
        print("重製版同狀態（4×4 網格）：" + json.dumps(
            {"status": grid["status"], "matchedSlots": grid["matchedSlots"], "iconIndices": found},
            ensure_ascii=False, sort_keys=True,
        ))
        if grid["matchedSlots"] != grid["expectedSlots"]:
            print(f"FAIL：16 格只命中 {grid['matchedSlots']}")
            return 1
        missing_there = [i for i in wanted if i not in found]
        if missing_there:
            print(f"FAIL：重製版網格裡找不到圖示 {missing_there}")
            return 1
    else:
        remake = compare_icons(args.remake, args.disk.read_bytes(), wanted)
        print("重製版同狀態：" + json.dumps(remake, ensure_ascii=False, sort_keys=True))
        if remake["missingIcons"]:
            print(f"FAIL：重製版畫面上找不到圖示 {remake['missingIcons']}")
            return 1
    print("PASS：原版與重製版在同一狀態下都畫出磁碟同一格圖示")
    return 0


if __name__ == "__main__":
    sys.exit(main())
