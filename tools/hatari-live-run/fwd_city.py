"""城市地面的正向模擬，對快照 VRAM 解出的那一幀逐格比。

鏈：city-maps.json 的格子 → 地形碼 → 圖塊編號（tile.look-roll，含 ±16 變體）→
三條圖形帶（header tileCodes）→ 8×5 格、間距 40、起點 (4,4)（tile.blit-pitch／city-gap）。
碼 0 鋪 40×40 地表背景（tile.background-blit）、碼 1 塗黑（tile.solid-cell）。
用法（容器內，repo 掛在 /work，快照目錄掛在 /out）：
  python3 /out/fwd_city.py <city index> /out/k-ground2.sav
"""
import io, json, sys, zipfile, gzip
sys.path.insert(0, '/work/tools')
sys.path.insert(0, '/out')
from atari_st_fat_inventory import choose_disk, choose_nested
from decode_atari_st_images import decompress_image
from atari_st_image_directory import decode_directory

RAM_IN_FILE = 0x1b089


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
        rows.append(bytes(row))
    return rows

with zipfile.ZipFile('/work/Sundog-Frozen-Legacy_Atari-ST_EN.zip') as outer:
    _, nb = choose_nested(outer, '[a2]')
    with zipfile.ZipFile(io.BytesIO(nb)) as nested:
        _, disk = choose_disk(nested)
images = {int(e['index']): e for e in decode_directory(disk)}


def load_image(idx):
    e = images[idx]
    off = int(e['rawOffset'], 16)
    w, h = int(e['declaredWidth']), int(e['declaredHeight'])
    px, _ = decompress_image(disk[off + 4: off + int(e['byteLength'])], w, h)
    px = bytes(px)
    return [px[y * w:(y + 1) * w] for y in range(h)]


def tiles(idx):
    a = load_image(idx)
    return [[row[t * 32:(t + 1) * 32] for row in a] for t in range(len(a[0]) // 32)]


def background():
    for i, e in images.items():
        if int(e['rawOffset'], 16) == 0x1b166:
            return [row[:40] for row in load_image(i)[:40]]
    raise SystemExit('沒有 0x1b166')


city_i = int(sys.argv[1])
cm = json.load(open('/work/docs/atari-st/city-maps.json'))['cities'][city_i]
tc = cm['header']['tileCodes']
band_b = tiles(tc['0'])      # 編號 ≥ 0x20
band_lo = tiles(tc['1'])     # 編號 2–17
band_hi = tiles(tc['2'])     # 編號 18–30
bg = background()


def tile_pixels(tid):
    if tid >= 0x20:
        return band_b[tid - 0x20]
    if tid >= 18:
        return band_hi[tid - 18]
    return band_lo[tid - 2]


def candidates(code):
    """地形碼 → 可能畫出的圖塊編號集合（tile.look-roll）。"""
    if code == 0:
        return 'bg'
    if code == 1:
        return 'black'
    if code == 5:
        return [0x21]
    if code == 15:
        return [0x20]
    if code == 11:
        return [5, 11, 21, 27]
    if code >= 0x20:
        return [code]
    return [code, code + 16]


BW, BH = 40, 24
buf = [[0] * BH for _ in range(BW)]
for i in range(20):
    for j in range(12):
        buf[10 + i][6 + j] = cm['grid'][i][j]
for b in cm['buildings']:
    buf[10 + b['row']][6 + b['column']] = b['code']

frame = decode_frame(gzip.open(sys.argv[2], 'rb').read())


def cell(c, r):
    x0, y0 = 4 + 40 * c, 4 + 40 * r
    return ([row[x0:x0 + 32] for row in frame[y0:y0 + 32]],
            [row[x0 - 4:x0 + 36] for row in frame[y0 - 4:y0 + 36]])


def eqfrac(a, b):
    tot = sum(len(r) for r in a)
    same = sum(1 for ra, rb in zip(a, b) for pa, pb in zip(ra, rb) if pa == pb)
    return same / tot


def match_cell(c, r, code):
    inner, full = cell(c, r)
    cand = candidates(code)
    if cand == 'black':
        return ('hit' if all(p == 0 for r in inner for p in r) else 'miss'), 'black'
    if cand == 'bg':
        eq = eqfrac(full, bg)
        return ('hit' if eq > 0.999 else 'miss'), 'bg %.3f' % eq
    best = None
    for tid in cand:
        eq = eqfrac(inner, tile_pixels(tid))
        if best is None or eq > best[0]:
            best = (eq, tid)
    return ('hit' if best[0] > 0.999 else 'miss'), 'tile %d %.3f' % (best[1], best[0])


# 先找捲動位置：整張 8×5 對每個候選原點的命中數
scores = []
for c0 in range(0, BW - 8 + 1):
    for r0 in range(0, BH - 5 + 1):
        hit = 0
        for r in range(5):
            for c in range(8):
                if (c, r) == (4, 2):
                    continue
                st, _ = match_cell(c, r, buf[c0 + c][r0 + r])
                hit += st == 'hit'
        scores.append((hit, c0, r0))
scores.sort(reverse=True)
print('原點候選:', scores[:3])
hit, c0, r0 = scores[0]
print('捲動位置 (px,py) =', (c0, r0), '命中', hit, '/ 39')
for r in range(5):
    row = []
    for c in range(8):
        code = buf[c0 + c][r0 + r]
        st, why = match_cell(c, r, code)
        row.append('%2d:%s(%s)' % (code, st[0], why.split()[0] + ('' if why.startswith('black') else ' ' + why.split()[-1])))
    print('  '.join(row))
