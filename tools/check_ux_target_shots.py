#!/usr/bin/env python3
"""檢查 UX-02 的實際 AppImage 截圖；在具 Pillow 的 Docker 映像內執行。"""
import argparse
import json
from pathlib import Path
from PIL import Image

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('shots', type=Path)
parser.add_argument('--baseline', type=Path)
args = parser.parse_args()
results = {}
# originalmedia.PaletteRGBA 的 ST 0x700／0x070 展開為 238，而非 255。
for name, expected in [('target-zero', (238, 0, 0)), ('target-one', (238, 0, 0)), ('target-none', (0, 238, 0))]:
    image = Image.open(args.shots / name / 'display.png').convert('RGB')
    assert image.size == (960, 600), (name, image.size)
    # 原版 CONDITION 內部的固定點（邏輯座標 20,10），避開框線與文字。
    actual = image.getpixel((60, 30))
    assert actual == expected, (name, actual, expected)
    # 目標輪廓／損傷內容區；無目標時不可留下前一艘的資訊。
    target = image.crop((435, 435, 795, 579))
    has_target_ink = any(pixel != (0, 0, 0) for pixel in target.getdata())
    assert has_target_ink == (name != 'target-none'), name
    results[name] = {'conditionRGB': actual, 'hasTargetInk': has_target_ink}
if args.baseline:
    old = Image.open(args.baseline).convert('RGB')
    assert old.size == (960, 600)
    assert old.getpixel((60, 30)) != (238, 0, 0), '舊版收據應能重現零號敵艦漏報'
    results['baselineRejected'] = True
print(json.dumps(results, ensure_ascii=False, indent=2))
