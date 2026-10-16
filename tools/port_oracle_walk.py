#!/usr/bin/env python3
"""在城市地面上把走路人帶進建築——閉迴路版。

開迴路（算一個座標、按住幾秒、放開）走不進去，因為**地圖會捲動**：按住的那幾秒視野一直在移，
放開時走路人腳下那一格已經不是按下時算好的那一格（四輪實測都停在地面，見
`docs/atari-st/ground-rules.md`）。

所以這支每一輪都重新看畫面：截圖 → 逐格判定找出建築在哪一格 → 算出那一格的門座標 →
只按住一小段就放開 → 再看一次。放開本身是安全的，`check_exit` 只在腳下地面碼 > 9 時才分派，
走道上放開什麼都不會發生。

判成功的方式不預設進了哪一棟：城市地面左右兩側是連續的背景圖塊，只要畫面上**一格背景都
沒有**就當作離開了地面，把最後一張截圖留給呼叫者判讀。用「多數格不是背景」當判準會被對話框
騙過去——街頭遭遇的視窗蓋掉中間大半個畫面，但那時人還站在地面上。

**現況（2026-09-05）：還沒走進建築。** 已經work 的部分是定位與移動——差分定位穩定回報走路人
在哪一格，瞄準之後它確實會朝那一格走（實測從格 (6,2) 走到 (6,3)）。六個門位置也窮舉過。
沒 work 的原因是**從畫面看不出地面碼**：`t`（畫面上是磁碟圖塊）不等於「可進入的建築」，
而 `check_building` 的跳表只認 10–20 這個範圍。要知道哪一格可進去，得讀 `G3a1` 那個地面圖
緩衝——下一步是用 `-Dpsys_debugger=true` 重建 port，從除錯介面把它 dump 出來。

街頭遭遇會隨機打斷（每個迴圈迭代擲 `random() mod 1000 == 21`），而且不只一種：問路的是
No／Yes 兩個選項，搶匪是 Discuss it／Refuse／Agree 三個，NPC 走掉時還會留一句沒有選項的訊息。
處理方式是**先判斷有沒有視窗擋著，有才點掉**——每輪盲目送右鍵會一直觸發新的「拒絕交談」
訊息，迴圈就卡在那裡（實測踩過）。判斷用格子分類：地面那 40 格裡「既不是背景也不是圖塊」的
格子一多，就是有視窗蓋著。點的位置取**最後一個選項**（螢幕 y=619）：兩個選項時是 Yes、三個選項時是 Agree。
選它是為了安全不是划算——拒絕搶匪會打起來，而這支腳本用的「440 點完全不分配」角色打不過，
實測拒絕一次就死了。
"""

from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys
import time

from PIL import Image

from check_ground_cells import to_runtime_palette
from compare_appimage_to_original import (
    RUNTIME_RGB,
    ST_GROUND_BACKDROP_OFFSET,
    ST_GROUND_COLUMNS,
    ST_GROUND_INSET,
    ST_GROUND_PITCH,
    ST_GROUND_ROWS,
    ST_GROUND_TILE,
    decode_record,
    decoded_tiles,
)

# port 的視窗：320×200 的 4 倍，位於螢幕 (0,80)。
SCALE = 4
WINDOW_Y = 80
# check_building 解出來的全部門位置（docs/atari-st/ground-rules.md）。腳下是哪個地面碼
# 從畫面看不出來，所以每輪換一個試——候選只有六個，窮舉比猜便宜。
DOORS = [(13, 21), (14, 26), (15, 16), (19, 21), (24, 21), (23, 29)]
# 街頭遭遇的選項在固定的 y：倒數第二個是 No／Refuse，最後一個是 Yes／Agree。
# 點**最後一個**。理由是安全而不是划算：拒絕搶匪會打起來，而這支腳本用的角色是
# 「440 點完全不分配」的（為了起始狀態可重現），打不過——實測拒絕一次就死了
# （`Sorry, Ab, you died of lacerations, burns, and massive blood loss.`）。
# 交錢只損失現金，走路人還能繼續走。純訊息框點這裡也關得掉。
ACCEPT = (900, 619)
# 沒有選項的純訊息框（NPC 走掉時留的那種）點選項位置關不掉，要點訊息區本身。
MESSAGE = (900, 400)


def sh(container: str, command: str) -> None:
    subprocess.run(
        ["docker", "exec", "-e", "DISPLAY=:97", container, "bash", "-c", command],
        check=True,
        stdout=subprocess.DEVNULL,
    )


def grab(container: str, work: pathlib.Path, name: str) -> Image.Image:
    sh(container, f'win=$(xdotool search --onlyvisible --name -- "." | head -1); '
                  f'import -window "$win" /w/shots/{name}')
    for _ in range(20):
        path = work / "shots" / name
        if path.exists() and path.stat().st_size > 0:
            time.sleep(0.2)
            return Image.open(path)
        time.sleep(0.3)
    raise RuntimeError(f"截不到 {name}")


def classify(image: Image.Image, disk: bytes, directory: dict) -> list[list[str]]:
    """把畫面切成 8×5 格，每格回一個字：背景 b、全黑 k、磁碟圖塊 t、其他 m。"""
    small = to_runtime_palette(image)
    px = small.load()
    record, pixels = decode_record(disk, ST_GROUND_BACKDROP_OFFSET)
    width = int(record["width"])
    background = tuple(
        RUNTIME_RGB[pixels[y * width + x]]
        for y in range(ST_GROUND_PITCH)
        for x in range(ST_GROUND_PITCH)
    )
    black = (RUNTIME_RGB[0],) * (ST_GROUND_PITCH * ST_GROUND_PITCH)
    tileset = set(decoded_tiles(disk, directory))
    grid = []
    for row in range(ST_GROUND_ROWS):
        line = []
        for column in range(ST_GROUND_COLUMNS):
            ox, oy = column * ST_GROUND_PITCH, row * ST_GROUND_PITCH
            cell = tuple(
                px[ox + x, oy + y]
                for y in range(ST_GROUND_PITCH)
                for x in range(ST_GROUND_PITCH)
            )
            tile = tuple(
                px[ox + ST_GROUND_INSET + x, oy + ST_GROUND_INSET + y]
                for y in range(ST_GROUND_TILE)
                for x in range(ST_GROUND_TILE)
            )
            line.append("b" if cell == background else
                        "k" if cell == black else
                        "t" if tile in tileset else "m")
        grid.append(line)
    return grid


def window_is_open(grid: list[list[str]]) -> bool:
    """畫面上有沒有浮動視窗擋著。

    地面正常時只有走路人與船那幾格會是 `m`；街頭遭遇的視窗蓋掉中間大半，`m` 會暴增。
    """
    return sum(kind == "m" for line in grid for kind in line) >= 15


def looks_like_ground(grid: list[list[str]]) -> bool:
    """城市地面左右兩側是連續的背景圖塊；一格都沒有才算離開了地面。

    不要用「多數格不是背景」當判準：街頭遭遇的對話框會蓋掉中間大半個畫面，但那時人還在
    地面上，那樣判會提早收工（實測踩過）。
    """
    return any(kind == "b" for line in grid for kind in line)


def diff_blobs(before: Image.Image, after: Image.Image, limit: int = 400):
    """兩張 320×200 之間變動的像素，粗略分成幾群；變動太多代表地圖捲動了。"""
    a = to_runtime_palette(before).load()
    b = to_runtime_palette(after).load()
    points = [
        (x, y)
        for y in range(ST_GROUND_ROWS * ST_GROUND_PITCH)
        for x in range(ST_GROUND_COLUMNS * ST_GROUND_PITCH)
        if a[x, y] != b[x, y]
    ]
    if not points or len(points) > limit:
        return None, len(points)
    groups: list[dict] = []
    for x, y in points:
        for group in groups:
            if abs(group["cx"] - x) <= 10 and abs(group["cy"] - y) <= 10:
                group["pts"].append((x, y))
                xs = [p[0] for p in group["pts"]]
                ys = [p[1] for p in group["pts"]]
                group["cx"], group["cy"] = sum(xs) / len(xs), sum(ys) / len(ys)
                break
        else:
            groups.append({"pts": [(x, y)], "cx": x, "cy": y})
    groups.sort(key=lambda g: -len(g["pts"]))
    return groups, len(points)


def step_and_locate(container: str, work: pathlib.Path, tag: str,
                    target: tuple[int, int], hold: float):
    """往 `target` 推一小步，順便用推前推後的差分定出走路人在哪。

    移動與定位合併成同一個動作是刻意的：先前把探測固定成「往畫面中央推」，結果那一步
    跟接下來瞄準建築的那一步方向相反，走路人在原地振盪（實測十輪都在同兩個座標之間跳）。

    不用白色連通區塊掃描：建築圖塊本身就有十幾個 32×32 的白色區塊，走路人只有幾個像素，
    混在二十幾個小區塊裡分不出來。差分只會留下真的動過的東西。

    地圖一捲動整張畫面都變，那一次就沒有結論（回 None），交給呼叫端下一輪再定位。
    """
    before = grab(container, work, f"probe-{tag}-a.png")
    sh(container, f"xdotool mousemove {target[0]} {target[1]}; sleep 0.3; "
                  f"xdotool mousedown 1; sleep {hold}; xdotool mouseup 1; sleep 1.2")
    after = grab(container, work, f"probe-{tag}-b.png")
    groups, total = diff_blobs(before, after)
    if groups is None:
        return None, total
    # 走路人朝目標走，所以「靠近目標的那一群」是它的新位置
    target_st = (target[0] / SCALE, (target[1] - WINDOW_Y) / SCALE)
    best = min(groups[:3],
               key=lambda g: (g["cx"] - target_st[0]) ** 2 + (g["cy"] - target_st[1]) ** 2)
    return (best["cx"], best["cy"]), total


def door_screen_position(row: int, column: int, door: tuple[int, int]) -> tuple[int, int]:
    """該顯示格的門在螢幕上的座標：格左上 + 內縮 + 門的格內偏移。"""
    st_x = column * ST_GROUND_PITCH + ST_GROUND_INSET + door[0]
    st_y = row * ST_GROUND_PITCH + ST_GROUND_INSET + door[1]
    return st_x * SCALE, st_y * SCALE + WINDOW_Y


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("work", type=pathlib.Path, help="port_oracle_capture.sh 的工作目錄")
    parser.add_argument("disk", type=pathlib.Path, help="原版 raw .st")
    parser.add_argument("--container", default="sundog-port-ground")
    parser.add_argument("--rounds", type=int, default=12)
    parser.add_argument("--hold", type=float, default=0.8,
                        help="每輪按住幾秒。短一點才能一步一步逼近，也比較不會走出視野")
    parser.add_argument(
        "--directory",
        type=pathlib.Path,
        default=pathlib.Path(__file__).parent.parent / "docs/atari-st/image-directory.json",
    )
    args = parser.parse_args(argv)

    disk = args.disk.read_bytes()
    directory = json.loads(args.directory.read_text())
    walker = None
    stuck = 0

    for attempt in range(1, args.rounds + 1):
        shot = grab(args.container, args.work, f"walk-{attempt:02d}.png")
        grid = classify(shot, disk, directory)
        print(f"第 {attempt} 輪：" + " ".join("".join(line) for line in grid))

        if window_is_open(grid) and looks_like_ground(grid):
            # 有視窗擋著：左鍵點掉。不用右鍵——右鍵是「拒絕交談」，會再招來一句訊息。
            # 兩種視窗要點不同地方，所以交替：有選項的點最後一個選項，純訊息的點訊息區。
            stuck += 1
            spot = ACCEPT if stuck % 2 else MESSAGE
            print(f"  有視窗擋著（連續 {stuck} 輪），點 {spot} 再看")
            sh(args.container, f"xdotool mousemove {spot[0]} {spot[1]}; sleep 0.4; "
                               f"xdotool click 1; sleep 2")
            continue
        stuck = 0

        if not looks_like_ground(grid):
            print(f"PASS：畫面不再是地面版面，停在 walk-{attempt:02d}.png")
            return 0

        door = DOORS[(attempt - 1) % len(DOORS)]
        targets = [(r, c) for r, line in enumerate(grid)
                   for c, kind in enumerate(line) if kind == "t"]
        if not targets:
            print("  這一幀看不到磁碟圖塊的格子，往右下推一段再看")
            target = (1100, 760)
        elif walker is None:
            # 還不知道走路人在哪：先往最靠上那排推一步，順便把它定出來
            row, column = min(targets)
            target = door_screen_position(row, column, door)
            print(f"  尚未定位，先往格 ({column},{row}) 的門 {door} 推一步")
        else:
            # 挑離走路人最近的那一格。建築排成好幾列、中間夾走道，瞄一個要穿過別排的
            # 目標會讓走路人卡在半路（實測 12 輪原地不動）。
            wc, wr = walker[0] / ST_GROUND_PITCH, walker[1] / ST_GROUND_PITCH
            row, column = min(targets, key=lambda t: (t[0] - wr) ** 2 + (t[1] - wc) ** 2)
            target = door_screen_position(row, column, door)
            print(f"  瞄準格 ({column},{row}) 的門 {door} → 螢幕 {target}")

        walker, moved = step_and_locate(
            args.container, args.work, f"{attempt:02d}", target, args.hold)
        if walker is None:
            print(f"  這一步有 {moved} 個像素在動（地圖捲了），位置下一輪再定")
        else:
            wc, wr = int(walker[0]) // ST_GROUND_PITCH, int(walker[1]) // ST_GROUND_PITCH
            print(f"  走路人約在 ST ({walker[0]:.0f},{walker[1]:.0f}) → 格 ({wc},{wr})")

    print(f"FAIL：{args.rounds} 輪之後仍在地面")
    return 1


if __name__ == "__main__":
    sys.exit(main())
