#!/usr/bin/env python3
"""Static review fixtures, separate from real GUI captures. Run inside Docker."""
import argparse
import hashlib
import json
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[1]
FONT = ROOT / 'assets/fonts/NotoSansCJKtc-Regular.otf'


def main():
    assert Path('/.dockerenv').exists()
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ['model', 'scenes', 'art', 'cabin', 'output']:
        parser.add_argument('--' + name, type=Path, required=True)
    args = parser.parse_args()
    for name in ['model', 'scenes', 'art', 'cabin', 'output']:
        setattr(args, name, getattr(args, name).resolve())
    assert not args.output.exists()
    args.output.mkdir()
    model = json.loads(args.model.read_text())
    native = json.loads(args.scenes.read_text())
    assert native['cases'] == 24
    sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
    outputs = []
    for scene in native['scenes']:
        if scene['variant'] and (scene['locale'] != 'zh-Hant' or scene['scale'] != 5):
            continue
        scale, state = scene['scale'], scene['state']
        image = Image.open(args.cabin / f'cabin-{320*scale}-dark.png').convert('RGBA')
        assert image.size == (320*scale, 200*scale)
        chrome = Image.open(args.art / f'window-{scale}.png').convert('RGBA')
        atlas = Image.open(args.art / f'icons-{scale}.png').convert('RGBA')
        image.alpha_composite(chrome, (80*scale, 0))
        draw = ImageDraw.Draw(image)
        for slot in model['slots']:
            code = 0 if slot['kind'] == 'hand' else state['stores' if slot['kind'] == 'store' else 'parts'][slot['index']] & 31
            sx, sy = code % 8 * 16 * scale, code // 8 * 16 * scale
            image.alpha_composite(atlas.crop((sx, sy, sx+16*scale, sy+16*scale)), (slot['x']*scale, slot['y']*scale))
            if slot['kind'] == 'store' and slot['index'] == state['selectedPartIndex']:
                x, y = slot['x'], slot['y']
                draw.rectangle(((x-1)*scale, (y-1)*scale, (x+17)*scale, (y+17)*scale), outline='#f3f6fa', width=max(1, round(.6*scale)))
        fields = []
        font = ImageFont.truetype(str(FONT), 8*scale)
        for index, field in enumerate(scene['fields']):
            text = field['text']
            x, y, w, h = (v*scale for v in field['safe'])
            ink = draw.textbbox((0, 0), text, font=font)
            iw, ih = ink[2]-ink[0], ink[3]-ink[1]
            assert iw <= w and ih <= h, (scene['locale'], text, ink)
            dx = x+(w-iw)/2-ink[0] if index == 0 else x-ink[0]
            dy = y+(h-ih)/2-ink[1]
            draw.text((dx, dy), text, font=font, fill='#1762ff' if index == 0 else '#f3f6fa')
            fields.append({'text': text, 'safe': field['safe'], 'pillowInk': [iw, ih], 'nativeInk': field['inkPixels']})
        if state['handItem']:
            code = state['handItem'] & 31
            sx, sy = code % 8 * 16 * scale, code // 8 * 16 * scale
            hand = next(slot for slot in model['slots'] if slot['kind'] == 'hand')
            image.alpha_composite(atlas.crop((sx, sy, sx+16*scale, sy+16*scale)), (hand['x']*scale, hand['y']*scale))
        path = args.output / f"stores-{scene['locale']}-{320*scale}-v{scene['variant']}.png"
        image.save(path)
        outputs.append({'file': path.name, 'sha256': sha(path), 'locale': scene['locale'], 'scale': scale, 'variant': scene['variant'], 'fields': fields})
    assert len(outputs) == 10
    report = {'status': 'PASS-static-fixtures; not normal GUI or GPU captures', 'toolSHA256': sha(Path(__file__)), 'modelSHA256': sha(args.model), 'nativeScenesSHA256': sha(args.scenes), 'fontSHA256': sha(FONT), 'inputs': {str(p.relative_to(ROOT)): sha(p) for p in list(args.art.glob('*.png')) + list(args.cabin.glob('cabin-*-dark.png'))}, 'outputs': outputs, 'limits': 'Pillow composition uses formal exported slots and native fixture state. Held item uses fallback hand slot. Text raster differs from native font drawing. Actual GUI evidence separate.'}
    (args.output / 'static-receipt.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print('PASS 10 static review fixtures;4 locales x2 sizes plus selected/held; ink fits')


if __name__ == '__main__':
    main()
