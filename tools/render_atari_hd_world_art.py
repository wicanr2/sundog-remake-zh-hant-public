#!/usr/bin/env python3
"""Rasterize independent review nodes; never opens an original screenshot."""
import argparse
import json
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont
import build_atari_hd_navigation_prototype as base
from build_atari_hd_navigation_maps_prototype import scene


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--art', type=Path, required=True)
    parser.add_argument('--prefix', type=Path, required=True)
    args = parser.parse_args()
    assert Path('/.dockerenv').exists()
    root = Path(__file__).resolve().parents[1]
    font = root/'assets/fonts/NotoSansCJKtc-Regular.otf'
    art = json.loads(args.art.read_text())

    def render(nodes, size, scale):
        painter = base.Painter(scale, font)
        scene(painter, {'nodes': nodes}, font)
        return painter.image.crop((0,0,size[0]*scale,size[1]*scale))

    def save(image, suffix):
        path = args.prefix.with_name(args.prefix.name+suffix+'.png')
        assert not path.exists()
        image.save(path)

    for key in ['cabinClosed','cabinLit']:
        save(render(art[key],(320,200),5),'-'+key)
    codes = sorted(art['tiles'],key=int)
    atlas = Image.new('RGB',(4*160+72,len(codes)*150+50),'#101820')
    draw = ImageDraw.Draw(atlas)
    face = ImageFont.truetype(str(font),18)
    for look in range(4):draw.text((80+look*160,10),f'look {look}',font=face,fill='#edf4fa')
    for row, code in enumerate(codes):
        draw.text((8,58+row*150),code,font=face,fill='#edf4fa')
        for look, nodes in enumerate(art['tiles'][code]):
            image=render(nodes,(32,32),4)
            atlas.paste(image,(72+look*160,50+row*150))
    save(atlas,'-tiles')
    print('Independent cabin phases and 19-code/4-look contact sheet rendered')


if __name__ == '__main__':main()
