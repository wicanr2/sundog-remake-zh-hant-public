#!/usr/bin/env python3
"""Render exported navigation scenes at output resolution; no source image input."""
import argparse
import base64
import copy
import hashlib
import json
from pathlib import Path

from PIL import Image, ImageDraw
import build_atari_hd_navigation_prototype as sector


def scene(painter, model, font):
    scale = painter.scale
    painter.rect([96, 6, 160, 112], '#020508')
    layer = None
    clip = None

    def flush():
        if layer is None:
            return
        x, y, w, h = clip
        bounds = tuple(round(v * scale) for v in [x, y, x + w, y + h])
        crop = layer.image.crop(bounds)
        painter.image.paste(crop, bounds, crop)
        painter.text_checks.extend(layer.text_checks)

    for node in model['nodes']:
        if node['clip'] != clip:
            flush()
            clip = node['clip']
            layer = sector.Painter(scale, font)
            layer.image = Image.new('RGBA', painter.image.size)
            layer.draw = ImageDraw.Draw(layer.image)
        colour = f"#{node['colour']:06x}"
        kind = node['kind']
        if kind == 'line':
            layer.line(node['points'], colour, node['width'])
        elif kind == 'polygon':
            layer.draw.polygon([(x * scale, y * scale) for x, y in node['points']], fill=colour)
        elif kind == 'ellipse':
            x, y, w, h = node['box']
            box = (x * scale, y * scale, (x + w) * scale, (y + h) * scale)
            if node.get('width', 0):
                layer.draw.ellipse(box, outline=colour, width=max(1, round(node['width'] * scale)))
            else:
                layer.draw.ellipse(box, fill=colour)
        elif kind == 'text':
            layer.text(node['text'], node['box'], colour, 5, 4, True)
        else:
            raise ValueError(kind)
    flush()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--model', type=Path, required=True)
    parser.add_argument('--prefix', type=Path, required=True)
    args = parser.parse_args()
    assert Path('/.dockerenv').exists(), 'Docker only'
    root = Path(__file__).resolve().parents[1]
    font = root / 'assets/fonts/NotoSansCJKtc-Regular.otf'
    model = json.loads(args.model.read_text())
    original = json.dumps(model, sort_keys=True)
    sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
    report = dict(status='DRAFT-navigation-maps-review', modelSHA256=sha(args.model), builderSHA256=sha(Path(__file__)), sectorBuilderSHA256=sha(root/'tools/build_atari_hd_navigation_prototype.py'), frames=[], sourceStateUnchanged=True)
    images = {view: {} for view in ['sector', 'system', 'planet']}
    for frame in model['frames']:
        view, locale = frame['view'], frame['locale']
        base = copy.deepcopy(model['base'])
        state = next(s for s in model['states'] if s['navigationView'] == view)
        base['state'] = state
        base['warpGauge'] = frame['warpGauge']
        base['warpSuccess'] = frame['warpSuccess']
        minutes = state['timeMinutes']
        base['clock'] = f'{minutes//1440:03d}/{minutes//60%24:02d}:{minutes%60:02d}'
        for scale in [4, 5]:
            painter = sector.render(base, frame, scale, font)
            if view != 'sector':
                scene(painter, model['scenes'][view], font)
            path = args.prefix.with_name(args.prefix.name+f'-{view}-{locale}-{scale}x.png')
            assert not path.exists(), 'new prefix required'
            painter.image.save(path)
            report['frames'].append(dict(path=path.name, sha256=sha(path), view=view, locale=locale, size=list(painter.image.size), text=painter.text_checks))
            if scale == 5:
                images[view][locale] = 'data:image/png;base64,'+base64.b64encode(path.read_bytes()).decode()
    assert json.dumps(model, sort_keys=True) == original
    data = json.dumps(dict(images=images, hotspots=model['base']['hotspots'], fixtures=model['base']['hitFixtures']), ensure_ascii=False).replace('</','<\\/')
    html = '''<!doctype html><html lang="zh-Hant"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Atari ST HD 三視圖導航審查</title>
<style>body{margin:24px;background:#101820;color:#edf4fa;font:16px sans-serif}main{max-width:1600px;margin:auto}button{margin:0 8px 10px 0;padding:8px 14px;background:#29394b;color:white;border:1px solid #82a7cf;cursor:pointer}figure{margin:0;position:relative}img{width:100%;display:block}svg{position:absolute;inset:0;width:100%;height:100%;pointer-events:none}#readout{min-height:2em;color:#ffe16a}h1{font-size:24px}</style>
<main><h1>Atari ST HD A：三視圖導航</h1><p>隔離審查原型，尚未加入正式v1。切換圖片或檢查操作區不會執行遊戲導航。</p><nav id="views"></nav><nav id="languages"></nav><button id="toggle">顯示／隱藏操作範圍</button><figure><img id="preview" alt="HD導航審查"><svg id="overlay" viewBox="0 0 320 200" style="display:none"></svg></figure><p id="readout">沿正式邏輯座標檢查原有操作區。</p></main><script>
const data=MODEL;const preview=document.querySelector('#preview'),overlay=document.querySelector('#overlay'),readout=document.querySelector('#readout');let currentView='sector',currentLocale='zh-Hant';
function show(view,locale){currentView=view;currentLocale=locale;preview.src=data.images[view][locale];preview.alt=view+' '+locale+' 導航審查';preview.dataset.view=view;preview.dataset.locale=locale;}
for(const [view,label] of Object.entries({sector:'星區圖',system:'星系圖',planet:'行星圖'})){const b=document.createElement('button');b.textContent=label;b.onclick=()=>show(view,currentLocale);document.querySelector('#views').append(b);}
for(const locale of Object.keys(data.images.sector)){const b=document.createElement('button');b.textContent=locale;b.onclick=()=>show(currentView,locale);document.querySelector('#languages').append(b);}
for(const s of data.hotspots){const [x,y,x1,y1]=s.rect;overlay.innerHTML+=`<rect x="${x}" y="${y}" width="${x1-x}" height="${y1-y}" fill="#ffcc0018" stroke="#ffcc00" stroke-width=".5"/>`;}
document.querySelector('#toggle').onclick=()=>{overlay.style.display=overlay.style.display==='none'?'block':'none'};
function actionAt(x,y){for(const s of data.hotspots){const [x0,y0,x1,y1]=s.rect;if(x>=x0&&x<x1&&y>=y0&&y<y1)return s.action;}return '';}
preview.onclick=e=>{const b=preview.getBoundingClientRect();const x=Math.floor((e.clientX-b.left)*320/b.width),y=Math.floor((e.clientY-b.top)*200/b.height);const action=actionAt(x,y);readout.dataset.action=action;readout.textContent=`邏輯座標 (${x}, ${y})：${action||'沒有動作'}。只檢查位置，未改變遊戲狀態。`;};show(currentView,currentLocale);
</script></html>'''
    page = args.prefix.with_suffix('.html')
    assert not page.exists()
    page.write_text(html.replace('MODEL', data))
    report['interactive'] = dict(path=page.name, sha256=sha(page), gameplay=False)
    args.prefix.with_suffix('.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print(f"PASS {len(report['frames'])} independent images, four locales, three views, two sizes")


if __name__ == '__main__':
    main()
