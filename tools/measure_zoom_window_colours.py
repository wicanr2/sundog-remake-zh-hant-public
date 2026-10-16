#!/usr/bin/env python3
"""逐列量一張原版擷取裡 zoom 視窗的顏色（調色盤位置）。

視窗的骨架是固定的：`+0` 黃色外框、`+1` 與 `+13` 是標題列上下那條線、`+2`–`+12` 是
標題列、`+14` 起是內容。所以只要找到黃框就能定位，不必手動給座標。

輸出的是**硬體調色盤位置**。p-code 裡的顏色是 GEM VDI 色號，兩套不同，要對照得查
`{0,15,1,2,4,6,3,5,7,8,9,10,12,14,11,13}`（見 docs/atari-st/remake-lessons.md 十二）。

擷取來自 laanwj port（`tools/port_oracle_capture.sh`），它把三位元的通道展成 v×255/7，
所以白是 (255,255,255) 而不是 Hatari 的 238。

要 Pillow，跑在 `mm2-pil:pillow11`：

    docker run --rm --network none -u "$(id -u):$(id -g)" \
      -v ~/cht/tmp:/w:ro -w /w mm2-pil:pillow11 \
      python3 /w/measure_zoom_window_colours.py <擷取>...
"""
import sys
from collections import Counter
from PIL import Image

PAL = {
    (0,0,0):0,(0,0,182):1,(0,146,0):2,(0,146,182):3,(182,0,0):4,(182,0,182):5,
    (182,146,0):6,(182,182,182):7,(73,73,73):8,(36,73,255):9,(0,255,0):10,
    (0,255,255):11,(255,0,0):12,(255,0,255):13,(255,255,0):14,(255,255,255):15,
}

def idx(c):
    return PAL.get(c, c)

def main(path, rows=20):
    im = Image.open(path).convert("RGB").resize((320, 200), Image.NEAREST)
    px = im.load()
    yel = lambda c: c[0] > 200 and c[1] > 200 and c[2] < 100
    ys = [y for y in range(200) if sum(1 for x in range(320) if yel(px[x, y])) > 100]
    if not ys:
        print(f"{path}: 找不到黃框"); return
    y0, y1 = ys[0], ys[-1]
    xs = [x for x in range(320) if yel(px[x, y0])]
    x0, x1 = min(xs), max(xs)
    print(f"{path}  視窗 ({x0},{y0})-({x1},{y1})")
    for dy in range(rows):
        y = y0 + dy
        c = Counter(idx(px[x, y]) for x in range(x0, x1 + 1))
        print(f"  +{dy:2d}: {c.most_common(3)}")

for p in sys.argv[1:]:
    main(p)
