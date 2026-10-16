#!/usr/bin/env python3
"""導航 DRAFT 審查圖。入口與界線見 docs/art/atari-st-hd-plan.md。"""
import argparse
import base64
import hashlib
import html
import json
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class Painter:
    def __init__(self, scale, font):
        self.scale = scale
        self.font_path = font
        self.image = Image.new('RGB', (320*scale, 200*scale), '#a7afb8')
        self.draw = ImageDraw.Draw(self.image)
        self.text_checks = []

    def rect(self, box, fill, stroke=None, width=.5):
        x,y,w,h = box
        self.draw.rectangle((round(x*self.scale), round(y*self.scale),
                             round((x+w)*self.scale)-1, round((y+h)*self.scale)-1),
                            fill=fill, outline=stroke, width=max(1,round(width*self.scale)))

    def line(self, points, fill, width=.5):
        self.draw.line([(round(x*self.scale),round(y*self.scale)) for x,y in points],
                       fill=fill, width=max(1,round(width*self.scale)), joint='curve')

    def panel(self, box, colour='#f1df00'):
        x,y,w,h = box
        self.rect(box, '#1c2630')
        self.line([(x,y+h-1),(x,y),(x+w-1,y)], '#edf1f4', .6)
        self.line([(x+w-1,y),(x+w-1,y+h-1),(x,y+h-1)], '#59616a', .6)
        self.rect([x+.7,y+.7,w-1.4,h-1.4], '#020508',colour,.55)

    def font(self, size):
        return ImageFont.truetype(str(self.font_path), round(size*self.scale))

    def fit(self, text, box, preferred=8, minimum=6):
        for i in range(round((preferred-minimum)*4)+1):
            size = preferred-i/4
            font=self.font(size)
            b=font.getbbox(text,anchor='lt')
            if b[2]-b[0] <= box[2]*self.scale and b[3]-b[1] <= box[3]*self.scale:
                return size,font,b
        raise ValueError(f'text does not fit at {minimum}px: {text!r}, box={box}')

    def text(self, text, box, colour='#f3f6fa', preferred=8, minimum=6, center=False):
        size,font,b=self.fit(text,box,preferred,minimum)
        x=box[0]*self.scale-b[0]
        y=box[1]*self.scale-b[1]
        if center:
            x+=(box[2]*self.scale-(b[2]-b[0]))/2
            y+=(box[3]*self.scale-(b[3]-b[1]))/2
        self.draw.text((round(x),round(y)),text,font=font,fill=colour,anchor='lt')
        ink=[round(x)+b[0],round(y)+b[1],round(x)+b[2],round(y)+b[3]]
        safe=[round(box[0]*self.scale),round(box[1]*self.scale),round((box[0]+box[2])*self.scale),round((box[1]+box[3])*self.scale)]
        assert ink[0]>=safe[0] and ink[1]>=safe[1] and ink[2]<=safe[2] and ink[3]<=safe[3]
        self.text_checks.append(dict(text=text,size=size,safe=safe,ink=ink))

    def wrapped(self, text, box, colour, max_lines=2):
        font=self.font(8)
        lines=[]; current=''
        # Prefer word boundaries for Latin; CJK breaks between glyphs.
        import re
        tokens=re.findall(r'\S+|\s+',text)
        for token in tokens:
            if font.getlength(current+token)>box[2]*self.scale and current:
                lines.append(current.rstrip());current=token.lstrip()
            else:current+=token
        if current:lines.append(current.rstrip())
        if len(lines)>max_lines:
            raise ValueError(f'needs {len(lines)} lines, maximum {max_lines}: {text}')
        for i,line in enumerate(lines):self.text(line,[box[0],box[1]+9*i,box[2],8.5],colour)
        return len(lines)


def render(model, language, scale, font):
    p=Painter(scale,font);state=model['state']
    for box in model['regions'].values():p.panel(box)
    p.panel(model['regions']['viewport'],'#1258ff')
    p.rect([96,6,161,112],'#020508')
    for key,box in [('CONDITION',[16,0,56,5]),('WARP',[278,0,25,5])]:
        p.text(key,box,preferred=5,minimum=4.5,center=True)
    p.panel([15,5,59,13],'#1258ff');p.rect([16,6,57,11],'#f4242d' if state['combatActive'] else '#20ef14')
    p.panel([277,5,28,13],'#1258ff');p.text(f"{model['warpSuccess']:03d}",[278,6,25,11],center=True)
    for rect,label in zip(model['selectorRects'],language['selectors']):
        p.rect(rect,'#1762ff');x,y,w,h=rect;p.rect([x+2,y+2,w-4,h-4],'#030609')
        p.text(label,[x+2.5,y+2,w-5,h-4],center=True)
    target=next((s for s in model['stars'] if s['id']==state['systemId']),None)
    assert target is not None
    for star in model['stars']:
        x,y,r=star['x'],star['y'],star['r']
        if r>0:
            p.draw.ellipse(((x-r)*scale,(y-r)*scale,(x+r)*scale,(y+r)*scale),outline='#f4f7fa',width=max(1,round(.6*scale)))
        else:
            p.line([(x,y-2),(x+2,y),(x,y+2),(x-2,y),(x,y-2)],'#f4f7fa',.6)
        if star['current']:
            rr=max(2,r+1);p.rect([x-rr,y-rr,2*rr+1,2*rr+1],None,'#1762ff',.6)
    p.rect([141,120,72,10],'#020508')
    p.text(model['clock'],[144,121,66,8],center=True)
    for x,direction in [(144,-1),(210,1)]:
        points=[(x,122),(x+3*direction,125),(x,128)]
        p.draw.polygon([(round(a*scale),round(b*scale)) for a,b in points],fill='#2ddad9')
    gauges=[(state['fuel'],255,'#1762ff','FUEL'),(state['weaponCharge'],255,'#f4242d','GUNS'),(state['shieldLevel'],255,'#20ef14','SHLD'),(model['warpGauge'],10,'#ed22ee','WARP')]
    for i,(value,maximum,colour,label) in enumerate(gauges):
        x=[273,283,293,303][i];p.rect([x,26,7,60],'#071018')
        h=max(0,min(60,value*60//maximum))
        if h:p.rect([x,86-h,7,h],colour)
        for j,c in enumerate(label):p.text(c,[x,88+j*8,7,7.8],colour,7,6,True)
    hull=min(32,max(0,state['hullCondition']*2));p.rect([279,134,4,32],'#071018')
    if hull:p.rect([279,166-hull,4,hull],'#20ef14' if hull>30 else '#f1df00' if hull>10 else '#f4242d')
    for i,c in enumerate('HULL'):p.text(c,[274,142+8*i,4.5,7],preferred=6,minimum=5,center=True)
    for i,(label,statuses) in enumerate(zip(['WARP','SUB-C','GUNS','TAC','SHLDS','PILOT'],model['rowStatuses'])):
        p.text(label,[288,131+i*11,28,6],preferred=5.5,minimum=5,center=True)
        for j,status in enumerate(statuses):
            colour='#020508' if status==0 else '#f4242d' if status<3 else '#f1df00' if status==3 else '#20ef14'
            p.rect([289+j*7,137+i*11,4,4],colour)
    for i,text in enumerate([language['message'],language['target'],language['cost']]):
        p.text(text,[9,135+i*9,259,8.5])
    p.wrapped(language['keys'],[9,162,259,17.5],'#ff714e')
    p.wrapped(language['help'],[9,180,259,17.5],'#f3f6fa')
    return p


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--model',type=Path,required=True)
    parser.add_argument('--prefix',type=Path,required=True)
    args=parser.parse_args()
    assert Path('/.dockerenv').exists(),'Docker only'
    root=Path(__file__).resolve().parents[1]
    font=root/'assets/fonts/NotoSansCJKtc-Regular.otf'
    model=json.loads(args.model.read_text());assert model['sourceStateUnchanged']
    assert model['canvas']==[320,200] and model['state']['screen']=='navigation'
    before=json.dumps(model,sort_keys=True)
    receipt=dict(status='DRAFT-technical-review',scope='星區圖原型；四語文字與幾何審查，非可玩HD或原版全機對拍。',modelSHA256=sha(args.model),fontSHA256=sha(font),builderSHA256=sha(Path(__file__)),stateUnchanged=True,frames=[],selectorStress=[],coordinateChecks=[])
    images={}
    for language in model['languages']:
        locale=language['locale']
        for scale in [4,5]:
            painter=render(model,language,scale,font)
            path=args.prefix.with_name(args.prefix.name+f'-{locale}-{scale}x.png')
            assert not path.exists(),'use a new prefix'
            painter.image.save(path)
            receipt['frames'].append(dict(path=path.name,sha256=sha(path),size=list(painter.image.size),locale=locale,text=painter.text_checks))
            if scale==5:images[locale]='data:image/png;base64,'+base64.b64encode(path.read_bytes()).decode()
            for rect in model['selectorRects']+[model['regions']['viewport']]:
                x,y,w,h=rect
                for px,py in [(x,y),(x+w-1,y+h-1),(x+w//2,y+h//2)]:
                    assert (px*scale)//scale==px and (py*scale)//scale==py
                    # Last physical pixel belongs to the same logical cell.
                    assert (px*scale+scale-1)//scale==px
                    receipt['coordinateChecks'].append([scale,px,py])
        probe=Painter(5,font)
        for labels in language['selectorWidthFixtures']:
            for rect,label in zip(model['selectorRects'],labels):
                x,y,w,h=rect
                try:
                    size,_,_=probe.fit(label,[x+2.5,y+2,w-5,h-4])
                    receipt['selectorStress'].append(dict(locale=locale,text=label,size=size,status='fits'))
                except ValueError:
                    receipt['selectorStress'].append(dict(locale=locale,text=label,status='needs-layout-review'))
    assert json.dumps(model,sort_keys=True)==before
    receipt['sourceHashes']={n:sha(root/n) for n in ['tools/atari_hd_navigation_export_test.go.txt','cmd/sundog-ebiten/main.go','cmd/sundog-ebiten/stlayout.go','internal/game/state.go','internal/world/registry.go'] if (root/n).exists()}
    review=args.prefix.with_suffix('.html')
    data=json.dumps(dict(images=images,hotspots=model['hotspots'],fixtures=model['hitFixtures']),ensure_ascii=False).replace('</','<\\/')
    page='''<!doctype html><html lang="zh-Hant"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Atari ST HD 導航審查</title>
<style>body{margin:24px;background:#101820;color:#edf4fa;font:16px sans-serif}main{max-width:1600px;margin:auto}button{margin:0 8px 12px 0;padding:8px 14px;background:#29394b;color:white;border:1px solid #82a7cf;cursor:pointer}figure{margin:0;position:relative}img{width:100%;display:block}svg{position:absolute;inset:0;width:100%;height:100%;pointer-events:none}#readout{min-height:2em;margin-top:12px;color:#ffe16a}h1{font-size:24px}</style>
<main><h1>Atari ST HD A 清晰精繪：導航星區圖</h1><p>審查原型，尚未加入正式遊戲。切換語系、點畫面可核對原有操作區；點擊不會執行導航。</p><nav id="languages"></nav><button id="toggle">顯示／隱藏操作範圍</button><figure><img id="preview" alt="導航高解析度審查原型"><svg id="overlay" viewBox="0 0 320 200" style="display:none"></svg></figure><p id="readout">選擇列與地圖沿用320×200邏輯座標。</p></main><script>
const data=MODEL;const preview=document.querySelector('#preview');const overlay=document.querySelector('#overlay');const readout=document.querySelector('#readout');
function show(lang){preview.src=data.images[lang];preview.alt=lang+' 導航審查原型';}
for(const lang of Object.keys(data.images)){const b=document.createElement('button');b.textContent=lang;b.onclick=()=>show(lang);document.querySelector('#languages').append(b);}
for(const spot of data.hotspots){const [x,y,x1,y1]=spot.rect;overlay.innerHTML+=`<rect x="${x}" y="${y}" width="${x1-x}" height="${y1-y}" fill="#ffcc0018" stroke="#ffcc00" stroke-width=".5"/>`;}
document.querySelector('#toggle').onclick=()=>{overlay.style.display=overlay.style.display==='none'?'block':'none'};
function actionAt(x,y){for(const spot of data.hotspots){const [x0,y0,x1,y1]=spot.rect;if(x>=x0&&x<x1&&y>=y0&&y<y1)return spot.action;}return '';}
preview.onclick=e=>{const b=preview.getBoundingClientRect();const x=Math.floor((e.clientX-b.left)*320/b.width),y=Math.floor((e.clientY-b.top)*200/b.height);const action=actionAt(x,y);readout.dataset.action=action;readout.textContent=`邏輯座標 (${x}, ${y})：${action||'沒有動作'}。僅檢查位置，沒有改變遊戲狀態。`;};show('zh-Hant');
</script></html>'''
    review.write_text(page.replace('MODEL',data))
    receipt['interactiveReview']=dict(path=review.name,sha256=sha(review),gameplay=False)
    receipt['selectorOverflowCount']=sum(q['status']!='fits' for q in receipt['selectorStress'])
    args.prefix.with_suffix('.json').write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+'\n')
    print(f"{len(receipt['frames'])} review frames; {len(receipt['selectorStress'])} selector fixtures; {receipt['selectorOverflowCount']} need layout review")


if __name__=='__main__':
    main()
