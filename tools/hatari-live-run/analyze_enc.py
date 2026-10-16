"""把每份遭遇快照的畫面（自 VRAM 解出）對 RAM 變數逐條比 screen-geometry 的五條戰術斷言。"""
import glob, gzip, json, math, sys
import numpy as np
import os
import decode_vram
REPO = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))

RAM_IN_FILE = 0x1b089
BASE = 0x21368
SINE = json.load(open(REPO + '/docs/atari-st/sine-table.json'))
ENEMY = json.load(open(REPO + '/docs/atari-st/enemy-ships.json'))
OUTLINE_X = [0x67, 0x79, 0x9d, 0x65, 0x71, 0x93, 0x63]
VPC = (176, 62)          # 視埠中心（十字準星）


def sine_table():
    for k in ('quarterSine', 'table', 'sine', 'values'):
        if k in SINE:
            v = SINE[k]
            return v if isinstance(v, list) else v.get('values') or v.get('bytes')
    for k, v in SINE.items():
        if isinstance(v, list) and len(v) == 33:
            return v
    raise SystemExit('sine table key? %s' % list(SINE))


TAB = sine_table()


def polar(angle, r):
    """SHIPLIB:0x0c polar_to_rect（ship-rules.md「投影」）。"""
    def comp(a):
        i = (a // 2) % 0x40
        if i > 0x20:
            i = 0x40 - i
        return r * TAB[i] // 0x80
    a = angle & 0xff
    x = comp((a + 0x40) & 0xff)
    y = comp(a)
    if 0x40 <= a <= 0xc4:
        x = -x
    if a > 0x80:          # SHIPLIB:0x0c 0x0116：角度 ≤ 0x80 跳過取負
        y = -y
    return x, y


def norm(v):
    """plot_dot：角度/4，負的加 0x100、正的 mod 0xff。"""
    v = int(v)
    if v >= 0x8000:
        v -= 0x10000
    a = v // 4 if v >= 0 else -((-v) // 4)
    return (a + 0x100) if a < 0 else a % 0xff


def word(buf, g):
    o = RAM_IN_FILE + BASE + 2 * g
    return buf[o] << 8 | buf[o + 1]


def seg_dist(p, a, b):
    (px, py), (ax, ay), (bx, by) = p, a, b
    dx, dy = bx - ax, by - ay
    t = 0 if dx == dy == 0 else max(0, min(1, ((px - ax) * dx + (py - ay) * dy) / (dx * dx + dy * dy)))
    return math.hypot(px - ax - t * dx, py - ay - t * dy)


def dot_in_circle(idx, cx, cy, segs):
    pts = []
    for y in range(cy - 28, cy + 29):
        for x in range(cx - 28, cx + 29):
            if (x - cx) ** 2 + (y - cy) ** 2 >= 28 ** 2 or idx[y, x] == 0:
                continue
            if any(seg_dist((x, y), a, b) <= 1.2 for a, b in segs):
                continue
            pts.append((x, y))
    if not pts:
        return None
    xs = [p[0] for p in pts]; ys = [p[1] for p in pts]
    return (min(xs) + max(xs)) / 2, (min(ys) + max(ys)) / 2, len(pts), idx[ys[0], xs[0]]


def analyze(path):
    buf = gzip.open(path, 'rb').read()
    idx = decode_vram.decode(buf, 0xf8000)
    cf, d0, db, d8 = word(buf, 0xcf), word(buf, 0xd0), word(buf, 0xdb), word(buf, 0xd8)
    out = {'file': path.split('/')[-1], 'Gcf': cf, 'Gd0': d0, 'Gdb': db, 'Gd8': d8}
    r = db // 4
    # 方位儀：Gd0 + 0x100；俯仰儀：−Gcf
    for name, (cx, cy), ang, segs in (
            ('az', (37, 164), norm(d0 + 0x100), [((37, 164), (16, 143)), ((37, 164), (58, 143))]),
            ('el', (102, 164), norm(-cf), [((102, 164), (81, 143)), ((102, 164), (81, 185))])):
        exp = None if r >= 28 else (cx + polar(ang, r)[0], cy + polar(ang, r)[1])
        got = dot_in_circle(idx, cx, cy, segs)
        out[name] = {'angle': ang, 'r': r, 'expected': exp,
                     'measured': None if got is None else (got[0], got[1], got[2], int(got[3]))}
    # 視埠：敵艦團塊（排除十字準星與單像素星點）
    dx, dy = d0 // 2 - 0x100, cf // 2 - 0x100
    vis = abs(dx) < 75 and abs(dy) < 51 and db < 108
    sub = idx[4:120, 94:259]
    nz = np.argwhere(sub > 0)
    big = []
    seen = set()
    pts = set((int(x), int(y)) for y, x in nz)
    for p in pts:
        if p in seen:
            continue
        st, g = [p], []
        seen.add(p)
        while st:
            q = st.pop(); g.append(q)
            for ex in (-1, 0, 1):
                for ey in (-1, 0, 1):
                    n = (q[0] + ex, q[1] + ey)
                    if n in pts and n not in seen:
                        seen.add(n); st.append(n)
        if len(g) >= 6:
            xs = [t[0] + 94 for t in g]; ys = [t[1] + 4 for t in g]
            big.append((len(g), min(xs), min(ys), max(xs), max(ys)))
    big.sort(key=lambda t: -t[0])
    out['viewport'] = {'dx': dx, 'dy': dy, 'visible': vis,
                       'expectedCenter': (VPC[0] + dx, VPC[1] + dy) if vis else None,
                       'blobs': big[:4]}
    # 狀態視窗輪廓：折線平移後每一段是否都落在畫出的像素上
    pts_o = ENEMY['outlines'][d8]['points']
    drawn = set((int(x), int(y)) for y, x in np.argwhere(idx[130:200, 140:270] > 0))
    drawn = set((x + 140, y + 130) for x, y in drawn)
    best = None
    for ox in range(100, 200):
        for oy in range(120, 170):
            miss = 0
            for (x0, y0), (x1, y1) in zip(pts_o, pts_o[1:]):
                n = max(abs(x1 - x0), abs(y1 - y0), 1)
                for k in range(n + 1):
                    px = round(x0 + (x1 - x0) * k / n) + ox; py = round(y0 + (y1 - y0) * k / n) + oy
                    if (px, py) not in drawn:
                        miss += 1
                        if best and miss > best[0]:
                            break
                if best and miss > best[0]:
                    break
            if best is None or miss < best[0]:
                best = (miss, ox, oy)
    out['outline'] = {'type': d8, 'name': ENEMY['ships'][d8]['name'], 'xConst': OUTLINE_X[d8],
                      'bestOrigin': best[1:], 'missingPixels': best[0],
                      'polyPoints': len(pts_o)}
    return out


if __name__ == '__main__':
    files = sys.argv[1:] or sorted(glob.glob('enc-*.sav'))
    for f in files:
        o = analyze(f)
        print(json.dumps(o, ensure_ascii=False))
