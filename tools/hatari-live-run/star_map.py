"""3D 星圖：從快照 VRAM 解幀，把畫面上的星系記號與 world-data 的 (X,Y,Z) 配對。

map.sector-projection：x = X×12 + 原點 + 8、y = Y×10 + 原點 + 8、r = 6 − Z/2；
map.sector-star-glyphs：r ≤ 0 退化成 (x−1, y+1) 的一個字元點。座標每局有 ±1 nibble 漂移
（world-data coordinatesNote），所以配對允許 X、Y、Z 各差 1。
用法：python3 star_map.py k-star.sav
"""
import gzip, json, sys, os

RAM_IN_FILE = 0x1b089
REPO = '/work' if os.path.isdir('/work/docs') else '/home/anr2/cht/sundog'


def decode_frame(buf, base=0xf8000):
    rows = []
    o = RAM_IN_FILE + base
    for y in range(200):
        row = bytearray(320)
        for cx in range(20):
            p = o + y * 160 + cx * 8
            w = [buf[p + k] << 8 | buf[p + k + 1] for k in (0, 2, 4, 6)]
            for b in range(16):
                v = 0
                for k in range(4):
                    v |= ((w[k] >> (15 - b)) & 1) << k
                row[cx * 16 + b] = v
        rows.append(row)
    return rows


def blobs(frame, x0, y0, x1, y1):
    pts = {(x, y) for y in range(y0, y1) for x in range(x0, x1) if frame[y][x]}
    seen, out = set(), []
    for p in pts:
        if p in seen:
            continue
        st, g = [p], []
        seen.add(p)
        while st:
            q = st.pop(); g.append(q)
            for dx in (-1, 0, 1):
                for dy in (-1, 0, 1):
                    n = (q[0] + dx, q[1] + dy)
                    if n in pts and n not in seen:
                        seen.add(n); st.append(n)
        xs = [t[0] for t in g]; ys = [t[1] for t in g]
        cols = {frame[y][x] for x, y in g}
        out.append({'n': len(g), 'x0': min(xs), 'y0': min(ys), 'x1': max(xs), 'y1': max(ys), 'colors': sorted(cols)})
    out.sort(key=lambda b: (b['y0'], b['x0']))
    return out


frame = decode_frame(gzip.open(sys.argv[1], 'rb').read())
wd = json.load(open(REPO + '/docs/atari-st/world-data.json'))
systems = wd['tables']['systems']['records']
bl = blobs(frame, 95, 5, 258, 119)
print('視埠團塊 %d 個：' % len(bl))
for b in bl:
    w, h = b['x1'] - b['x0'] + 1, b['y1'] - b['y0'] + 1
    b['cx'], b['cy'], b['r'] = (b['x0'] + b['x1']) / 2, (b['y0'] + b['y1']) / 2, (max(w, h) - 1) / 2
    print('  bbox (%d,%d)-(%d,%d) %dx%d n=%d 色=%s' % (b['x0'], b['y0'], b['x1'], b['y1'], w, h, b['n'], b['colors']))

# 原點：Woremed 是規則寫死的不動點 (6,5,6) → 中心 (6×12+8+ox, 5×10+8+oy)；第一輪量到 (176,64)
cands = [b for b in bl if b['n'] < 200]
wm = min(cands, key=lambda b: abs(b['cx'] - 176) + abs(b['cy'] - 64))
ox, oy = wm['cx'] - 80, wm['cy'] - 58
print('原點 (%.0f,%.0f)（Woremed 量到中心 (%.0f,%.0f)）' % (ox, oy, wm['cx'], wm['cy']))
for b in cands:
    b['X'] = (b['cx'] - 8 - ox) / 12; b['Y'] = (b['cy'] - 8 - oy) / 10
    print('  中心(%5.1f,%5.1f) r=%.1f → 格 (%.2f, %.2f)' % (b['cx'], b['cy'], b['r'], b['X'], b['Y']))
used = set()
for s2 in sorted(systems, key=lambda s: s['index']):
    X, Y, Z = s2['coordinates']
    best = None
    for b in cands:
        if id(b) in used: continue
        dX, dY = b['X'] - X, b['Y'] - Y
        if abs(dX) <= 1.01 and abs(dY) <= 1.01 and abs(dX - round(dX)) < 0.1 and abs(dY - round(dY)) < 0.1:
            cost = abs(dX) + abs(dY) + abs(b['r'] - (6 - Z // 2)) * 0.1
            if best is None or cost < best[0]: best = (cost, b, round(dX), round(dY))
    if best:
        _, b, dX, dY = best; used.add(id(b))
        rz = 6 - Z // 2
        zmeas = 'Z→%d' % int(2 * (6 - b['r'])) if b['r'] != rz else ''
        print('  %-10s (%2d,%2d,%2d) 漂移(%+d,%+d,%s) 預期 r=%d 量到 r=%.0f %s' % (s2['name'], X, Y, Z, dX, dY, ('%+d' % (int(2*(6-b['r'])) - Z)) if b['r'] != rz else '+0', rz, b['r'], zmeas))
    else:
        print('  %-10s (%2d,%2d,%2d) 沒配到' % (s2['name'], X, Y, Z))
print('沒配到的團塊：', [(b['cx'], b['cy'], b['r']) for b in cands if id(b) not in used])
