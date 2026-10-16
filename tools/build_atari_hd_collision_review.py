#!/usr/bin/env python3
"""Local source-mask diagnostics over independent HD vectors, never game art."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
from collections import deque

from PIL import Image, ImageDraw, ImageFont
import build_atari_hd_navigation_prototype as base
from build_atari_hd_navigation_maps_prototype import scene


def render_asset(nodes, size, scale, font):
    painter = base.Painter(scale, font)
    painter.image = Image.new('RGBA', (320*scale, 200*scale))
    painter.draw = ImageDraw.Draw(painter.image)
    scene(painter, {'nodes': nodes}, font)
    return painter.image.crop((0, 0, size[0]*scale, size[1]*scale))


def ground_image(data, art, scale, font):
    image = Image.new('RGBA', (320*scale, 200*scale), '#020508')
    grass = render_asset(art['groundBackdrop'], (40, 40), scale, font)
    for cell in data['tiles']:
        x, y = cell['x'], cell['y']
        if cell['background']:
            image.alpha_composite(grass, (x*scale, y*scale))
        code = cell['code']
        if cell['valid'] and code not in (0, 1):
            tile = render_asset(art['tiles'][str(code)][cell['look']], (32, 32), scale, font)
            image.alpha_composite(tile, ((x+4)*scale, (y+4)*scale))
    draw = ImageDraw.Draw(image)
    rect = lambda box, colour: draw.rectangle(tuple(round(v*scale) for v in box), fill=colour)
    line = lambda points, width, colour: draw.line([(round(x*scale), round(y*scale)) for x,y in points], fill=colour, width=max(1,round(width*scale)))
    # Same source anchor, independent glyph, static diagnostic phase only.
    for mark in data['marks']:
        x,y,ch=mark['x'],mark['y'],mark['character']
        if ch==11:
            x-=3;y-=4
            line([(x,y+4),(x+3,y+1)],1.2,'#f3f6fa');line([(x,y+4),(x+3,y+7)],1.2,'#f3f6fa')
            rect([x+2,y+2,x+7,y+6],'#dde7ee');rect([x+5,y+1,x+11,y+2],'#f3f6fa');rect([x+5,y+6,x+11,y+7],'#f3f6fa');rect([x+5,y+3,x+8,y+5],'#020508')
        elif ch==0:rect([x-2,y-3,x+1,y+3],'#f3f6fa')
    x,y=data['walker']
    rect([x+2,y,x+5,y+2],'#f3f6fa');rect([x+2,y+2,x+5,y+4.5],'#f3f6fa')
    line([(x+.7,y+2.3),(x+6,y+3.8)],.8,'#f3f6fa');line([(x+2.7,y+4),(x+1.7,y+6.4)],1,'#f3f6fa');line([(x+4.3,y+4),(x+5.3,y+6.4)],1,'#f3f6fa')
    return image


def candidates(image, data, scale):
    legal=data['legalAnchors'];floor=data['sourceFloor'];reachable=data['reachableAnchors']
    distance=[64000]*64000;queue=deque()
    for i,valid in enumerate(reachable):
        if valid:distance[i]=0;queue.append(i)
    while queue:
        i=queue.popleft();x,y=i%320,i//320
        for nx,ny in [(x-1,y),(x+1,y),(x,y-1),(x,y+1)]:
            if 0<=nx<320 and 0<=ny<200:
                n=ny*320+nx
                if distance[n]>distance[i]+1:distance[n]=distance[i]+1;queue.append(n)
    green=[]
    for y in range(200):
        for x in range(320):
            r,g,b,_=image.getpixel((x*scale+scale//2,y*scale+scale//2))
            if r<50 and g>110 and g>2*r and g>1.4*b:green.append(y*320+x)
    selected=[]
    for i in sorted((i for i in green if not floor[i]),key=lambda i:(-distance[i],i)):
        x,y=i%320,i//320
        if all((x-p['x'])**2+(y-p['y'])**2>=12**2 for p in selected):
            selected.append(dict(x=x,y=y,distanceToReachableAnchor=distance[i],sourcePixelPassable=False,legalAnchor=legal[i],reachableAnchor=reachable[i]))
        if len(selected)==8:break
    return dict(greenSamples=len(green),greenOverSourceBlockedPixel=sum(not floor[i] for i in green),greenWithBlockedWalkerAnchor=sum(not legal[i] for i in green),reviewPoints=selected,interpretation='Heuristic review cues only. HD green is not a collision flag; a source-pixel mask differs from a whole-walker legal-anchor mask. Counts do not establish a visual defect or parity.')


def diagnostic(image, data, title, font, scale, points=()):
    reachable=data.get('reachableAnchors',data['legalAnchors'])
    mask=Image.new('RGBA',(320,200));mask.putdata([(38,218,240,75) if reachable[i] else (185,124,240,25) if valid else (0,0,0,0) for i,valid in enumerate(data['legalAnchors'])])
    image=image.copy();image.alpha_composite(mask.resize(image.size,Image.Resampling.NEAREST))
    draw=ImageDraw.Draw(image);small=ImageFont.truetype(str(font),18)
    for s in data.get('stations',[]):
        x0,y0,x1,y1=s['bounds'];colour='#ffe16a' if s['reachableAnchors'] else '#ed6cab'
        draw.rectangle((x0*scale,y0*scale,(x1+1)*scale,(y1+1)*scale),outline=colour,width=2)
        draw.text((x0*scale,(y0-5)*scale),str(s['window']),font=small,fill=colour)
    for number,point in enumerate(points,1):
        x,y=point['x']*scale,point['y']*scale
        draw.ellipse((x-8,y-8,x+8,y+8),outline='#ff614e',width=2)
        draw.text((x+10,y-9),str(number),font=small,fill='#ff614e')
    result=Image.new('RGB',(image.width,image.height+80),'#101820');result.paste(image,(0,80))
    d=ImageDraw.Draw(result);face=ImageFont.truetype(str(font),23)
    d.text((16,8),title+'：診斷合成圖，非遊玩截圖',font=face,fill='#edf4fa')
    legend=('青色＝起點連通錨點；淡紫＝合法但不連通；黃／粉＝站位；紅圈只供外觀審查。'
            if 'reachableAnchors' in data else '青色＝步行人物合法錨點；此圖未計算連通性。顏色只作診斷，不是美術圖層。')
    d.text((16,43),legend,font=small,fill='#edf4fa')
    return result


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--build',type=Path,required=True)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args();assert Path('/.dockerenv').exists()
    root=Path(__file__).resolve().parents[1];out=args.output.resolve();assert out.is_dir() and not (out/'local-model.json').exists()
    overlay=json.loads((args.build/'overlay.json').read_text())
    for virtual,source in [('zz_hd_collision_test.go','atari_hd_collision_export_test.go.txt'),('zz_hd_maps_review_test.go','atari_hd_navigation_maps_test.go.txt')]:
        overlay['Replace'][str(root/'cmd/sundog-ebiten'/virtual)]=str(root/'tools'/source)
    overlay_path=out/'export-overlay.json';overlay_path.write_text(json.dumps(overlay,indent=2)+'\n')
    env=dict(os.environ,DISPLAY=':88',HD_REVIEW_MODEL=str(root/'docs/art/prototypes/atari-st-hd-navigation-review-model-20261015.json'),HD_WORLD_DISK=str(root/'dist-all/v.1.0.0-20261015/full-local/SunDog-v.1.0.0-20261015-private-full/sundog.st'),HD_WORLD_FIXTURES=str(root/'dist-all/v.1.0.0-20261015/smoke/linux-x86_64/receipt.json'),HD_COLLISION_OUTPUT=str(out/'local-model.json'))
    xvfb=subprocess.Popen(['Xvfb',':88','-screen','0','1600x1000x24','-nolisten','tcp'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    command=['go','test','-overlay',str(overlay_path),'./cmd/sundog-ebiten','-run','^TestHDCollisionReviewExport$','-count=1','-v']
    try:
        time.sleep(.5)
        run=subprocess.run(command,cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120)
        (out/'export.log').write_text(run.stdout);print(run.stdout);assert run.returncode==0
    finally:xvfb.terminate();xvfb.wait(timeout=10)
    model=json.loads((out/'local-model.json').read_text());art_path=root/'docs/art/prototypes/atari-st-hd-world-art-20261015.json';art=json.loads(art_path.read_text())
    font=root/'assets/fonts/NotoSansCJKtc-Regular.otf';scale=5
    cabin=render_asset(art['cabinClosed'],(320,200),scale,font)
    ground=ground_image(model['ground'],art,scale,font)
    cues=candidates(cabin,model['cabin'],scale)
    outputs=[]
    for name,image,data,title,points in [('cabin',cabin,model['cabin'],'船艙',cues['reviewPoints']),('ground',ground,model['ground'],'Drahew地面',[])]:
        image.save(out/(name+'-independent.png'))
        diagnostic(image,data,title,font,scale,points).save(out/(name+'-diagnostic.png'))
        outputs.append(name+'-diagnostic.png')
    sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
    report=dict(schema='sundog-hd-collision-review-1',status='DRAFT-visual-review-not-parity',modelSHA256=sha(out/'local-model.json'),artSHA256=sha(art_path),toolHashes={str(p.relative_to(root)):sha(p) for p in [Path(__file__),root/'tools/atari_hd_collision_export_test.go.txt']},exportCommand=command,cabin=dict(sourcePassablePixels=sum(model['cabin']['sourceFloor']),legalAnchors=sum(model['cabin']['legalAnchors']),reachableAnchors=sum(model['cabin']['reachableAnchors']),start=model['cabin']['startAnchor'],stations=model['cabin']['stations'],visualCues=cues),ground=dict(legalAnchors=sum(model['ground']['legalAnchors']),origin=model['ground']['origin'],walker=model['ground']['walker'],pattern=model['ground']['pattern'],rebuilds=model['ground']['rebuilds']),images={name:sha(out/name) for name in outputs},limitations=[model['limits'],'PNG is a diagnostic composition, not a GUI capture. Masks/raw model remain local only. No HD art changed.'])
    (out/'review.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    # This page contains source-derived masks and is intentionally local only.
    payload=json.dumps({'cabin':model['cabin']['legalAnchors'],'ground':model['ground']['legalAnchors'],'cabinReachable':model['cabin']['reachableAnchors']})
    html='''<!doctype html><meta charset="utf-8"><title>HD通行邊界本機審查</title><style>body{background:#101820;color:#edf4fa;font:18px sans-serif}button{margin:8px;padding:8px}img{max-width:100%;display:block}#result{min-height:2em}</style><p>本機診斷。包含原版來源遮罩，不能進美術資產、公開包或正式遊戲。點圖只查錨點，沒有遊戲動作。</p><button id="cabin">船艙</button><button id="ground">Drahew</button><button id="toggle">顯示／隱藏診斷</button><p id="result"></p><img id="view"><script>const masks=MASKS;let kind='cabin',overlay=true;const img=document.querySelector('#view');function show(){img.src=kind+(overlay?'-diagnostic.png':'-independent.png')}for(const k of ['cabin','ground'])document.getElementById(k).onclick=()=>{kind=k;show()};document.querySelector('#toggle').onclick=()=>{overlay=!overlay;show()};img.onclick=e=>{const b=img.getBoundingClientRect(),nativeY=(e.clientY-b.top)*img.naturalHeight/b.height-(overlay?80:0),x=Math.floor((e.clientX-b.left)*320/b.width),y=Math.floor(nativeY/5);document.querySelector('#result').textContent=y<0||y>=200?'標題區':`人物錨點(${x},${y})：${masks[kind][y*320+x]?'合法':'受阻'}。僅診斷，未移動人物。`};show()</script>'''
    html=html.replace("masks[kind][y*320+x]?'合法':'受阻'", "masks[kind][y*320+x]?(kind==='cabin'&&!masks.cabinReachable[y*320+x]?'合法，但不與起點連通':'合法，可達性依場景圖樣判定'):'受阻'")
    (out/'local-review.html').write_text(html.replace('MASKS',payload))
    print('Local review:',out/'local-review.html');print('Cabin cues:',cues)


if __name__=='__main__':main()
