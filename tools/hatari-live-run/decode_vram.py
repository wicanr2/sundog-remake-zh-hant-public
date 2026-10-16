"""從 Hatari 記憶體快照解出當下那一幀（ST low-res 4 平面 320×200），
畫面與 RAM 變數因此是同一瞬間，沒有截圖與存檔之間的時間差。

用法：decode_vram.py enc-a.sav [out.png]
"""
import gzip, sys
import numpy as np
from PIL import Image

RAM_IN_FILE = 0x1b089
PAL = [(0, 0, 0), (238, 238, 238), (238, 0, 0), (0, 238, 0), (34, 68, 238), (238, 238, 0),
       (0, 238, 238), (238, 0, 238), (136, 136, 136), (204, 204, 204), (136, 0, 0), (0, 136, 0),
       (0, 0, 136), (136, 136, 0), (0, 136, 136), (136, 0, 136)]


def decode(buf, base):
    idx = np.zeros((200, 320), dtype=np.uint8)
    o = RAM_IN_FILE + base
    for y in range(200):
        for cx in range(20):
            p = o + y * 160 + cx * 8
            w = [buf[p + k] << 8 | buf[p + k + 1] for k in (0, 2, 4, 6)]
            for b in range(16):
                v = 0
                for k in range(4):
                    v |= ((w[k] >> (15 - b)) & 1) << k
                idx[y, cx * 16 + b] = v
    return idx


def find_base(buf):
    """找 32000 byte 的畫面：候選在 512K／1M RAM 的頂端；挑「像素索引分佈最像畫面」的。"""
    best = None
    for base in (0x78000, 0xf8000, 0x7d000, 0xfd000):
        if RAM_IN_FILE + base + 32000 > len(buf):
            continue
        idx = decode(buf, base)
        score = int((idx == 0).sum())
        if 10000 < score < 60000 and (best is None or score > best[0]):
            best = (score, base, idx)
    return best


if __name__ == '__main__':
    buf = gzip.open(sys.argv[1], 'rb').read()
    best = find_base(buf)
    if len(sys.argv) > 3:
        b = int(sys.argv[3], 16); best = (0, b, decode(buf, b))
    print('RAM bytes', hex(len(buf) - RAM_IN_FILE), 'base', hex(best[1]) if best else None)
    idx = best[2]
    rgb = np.array(PAL, dtype=np.uint8)[idx]
    im = Image.fromarray(rgb)
    out = sys.argv[2] if len(sys.argv) > 2 else sys.argv[1].replace('.sav', '-vram.png')
    im.resize((960, 600), Image.NEAREST).save(out)
    np.save(out.replace('.png', '.npy'), idx)
    print('寫出', out)
