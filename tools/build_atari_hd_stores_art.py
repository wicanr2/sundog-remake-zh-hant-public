#!/usr/bin/env python3
"""Independent vectors for a review-only Atari ST stores window and item set."""
import argparse
import hashlib
import json
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont
from build_atari_hd_world_art import Art
from build_atari_hd_collision_review import render_asset

ROOT=Path(__file__).resolve().parents[1]
FONT=ROOT/'assets/fonts/NotoSansCJKtc-Regular.otf'


def item(code):
    a=Art(16,16)
    if code==0:
        a.rect(7,5,2,5,'f3f6fa');a.rect(5,7,6,1,'f3f6fa')
    elif code==1:
        a.poly([[2,1],[12,1],[14,3],[14,15],[2,15]],'c6d0d8')
        a.rect(5,1,6,5,'f3f6fa');a.rect(9,2,1,3,'8899a6')
        a.rect(4,8,8,6,'fa3444');a.rect(5,11,6,3,'f3f6fa')
    elif code==2:
        a.poly([[3,4],[13,4],[12,15],[4,15]],'b5c6d1');a.rect(2,3,12,2,'ecf4f8')
        a.rect(6,1,4,2,'b5c6d1')
        for x in [5,8,11]:a.line([[x,5],[x,13]],'18b7c5',.65)
    elif code==3:a.poly([[1,6],[11,6],[11,3],[15,8],[11,13],[11,10],[1,10]],'edf5f8')
    elif code==4:
        a.rect(6,0,4,16,'9c8518');a.ellipse(8,8,6,6,'f1df20');a.ellipse(8,8,4.7,4.7,'061018')
        a.line([[8,4],[8,8],[5,8]],'eef6f9',1)
    elif code==5:
        a.poly([[5,1],[11,1],[10,4],[14,9],[13,13],[10,15],[5,15],[2,12],[2,9],[6,4]],'dbe6eb')
        a.line([[5,4],[11,4]],'dbbc22',.8);a.line([[8,6],[8,13]],'314354',.9)
        a.line([[11,7],[6,7],[5,9],[10,10],[10,12],[5,12]],'314354',.8)
    elif code==6:
        a.line([[3,14],[3,4],[12,13],[12,4]],'eef6f9',1.3)
        a.line([[8,1],[8,6]],'18e23d',1);a.poly([[8,0],[6,3],[10,3]],'18e23d')
    elif code==7:
        a.poly([[8,0],[10,4],[15,2],[12,7],[16,9],[11,10],[13,15],[8,12],[4,16],[5,11],[0,10],[4,7],[1,3],[6,4]],'f5cd21')
        a.poly([[8,3],[10,6],[13,8],[10,10],[8,13],[6,10],[3,8],[6,6]],'ff4134');a.ellipse(8,8,2,3,'f9f6ca')
    elif code==8:
        a.poly([[2,6],[3,3],[6,2],[11,2],[14,4],[14,6]],'d9a823')
        for x,y in [(5,4),(8,3.6),(11,4.3)]:a.ellipse(x,y,.45,.2,'fff2b1')
        a.poly([[1,7],[5,6.5],[8,7.5],[12,6.5],[15,7],[14,9],[2,9]],'1bdb43')
        a.rect(2,9,12,2,'c94329');a.poly([[2,11],[14,11],[13,14],[3,14]],'a17612')
        a.line([[3,12],[13,12]],'e4b523',.6)
    elif code==9:
        a.panel(3,3,9,12,'d2a418','1fe0df');a.rect(4.5,4.5,6,2,'fff6d0')
        a.line([[6,7],[6,13]],'f3da55',.5);a.line([[8,7],[8,13]],'f3da55',.5)
        a.line([[12,5],[15,5],[15,11],[12,11]],'17aebb',1)
        a.poly([[2,3],[3,1],[8,1],[11,2],[11,4]],'f1f7f4')
    elif code==10:
        a.rect(1,7,4,2,'ee28d9');a.rect(5,7,3,2,'bce6ee')
        a.poly([[8,6],[10,6],[10,4],[11,4],[11,11],[10,11],[10,9],[8,9]],'2457e4')
        a.line([[11,7.5],[15,7.5]],'19d85b',1);a.line([[14,6],[14,9]],'19d85b',.6)
    elif code==11:
        a.poly([[1,6],[4,4],[12,4],[15,6],[15,11],[12,13],[4,13],[1,11]],'234cdb')
        a.ellipse(8,8,4.5,6,'e9f2f6');a.poly([[8,3],[11,5],[11,10],[8,13],[5,10],[5,5]],'f73532')
    elif code in [12,13]:
        if code==12:
            a.poly([[2,5],[12,5],[14,7],[11,9],[8,9],[6,14],[3,14],[4,9],[2,9]],'c4d0d7')
            a.rect(3,4,9,1.5,'25d9de');a.rect(9,6,2,1,'dbcf20');a.line([[7,9],[9,9],[9,11],[7,11]],'758388',.6)
        else:
            a.poly([[1,4],[6,4],[8,6],[12,3],[14,5],[11,8],[14,12],[12,14],[8,10],[5,15],[2,15],[5,8],[1,7]],'2457db')
            a.poly([[5,8],[8,9],[5,15],[2,15]],'1bd3cb');a.line([[11,4],[8,7],[11,11]],'f52e30',1.8);a.rect(12,5,3,2,'f2db25')
    elif code==14:
        a.line([[4,2],[4,5],[10,5],[13,7],[13,10],[11,12]],'f1df20',1)
        a.line([[4,2],[2,2],[2,0],[5,0],[5,2]],'ea2ce0',.8)
        a.panel(4,10,7,5,'21d6d5','77edf1');a.rect(6,11,1,3,'f53c34');a.rect(8,12,1,2,'f53c34')
    elif code==15:
        a.ellipse(8,8,6.5,6.5,'eef4f7');a.line([[6,12],[6,4],[10,4],[11,5],[11,7],[9,8],[6,8]],'ce2930',1.4)
    elif code==16:
        a.poly([[8,3],[11,7],[10,11],[7,13],[4,10],[5,6]],'234ed4')
        a.poly([[8,7],[10,10],[8,11],[5,10]],'f532d8')
        for x,y in [(3,2),(8,1),(13,3),(14,8),(12,13),(7,15),(2,12),(1,7)]:a.rect(x,y,.8,.8,'f1df24')
    elif code==17:
        a.poly([[3,6],[6,3],[12,5],[14,9],[12,14],[7,15],[3,13]],'ec322b')
        a.poly([[2,5],[5,1],[10,1],[13,4],[10,8],[5,9]],'e8df18')
        for x,y in [(4,4),(7,2),(10,4),(6,6)]:a.line([[x,y],[x,y+2]],'22d1df',1)
    elif code==18:
        a.poly([[2,2],[12,2],[14,4],[14,14],[3,15],[2,13]],'c2d0d8');a.rect(3,3,9,1,'e73638')
        a.poly([[5,5],[11,5],[11,8],[8,8],[8,12],[5,12]],'e6edf0');a.rect(6,6,6,2,'1fbd43')
        a.line([[3,5],[3,12],[5,14]],'6b8394',.6)
    elif code in [19,20,21,22,23,24,25,26,27,28,29,30,31]:
        if code not in [27,31]:a.line([[1,8],[15,8]],'ec29d8',1)
        if code==19:
            a.poly([[5,4],[7,1],[10,1],[12,4],[10,7],[12,10],[10,14],[7,14],[5,11],[7,8]],'2049d4')
            a.line([[9,3],[10,6],[8,9],[9,12]],'dbeaf0',.6);a.rect(8,2,1,2,'f23326')
        elif code==20:
            for x,y in [(6,4),(10,11)]:a.ellipse(x,y,2.5,3,'e5f0f3');a.ellipse(x,y,1.4,1.8,'020508')
        elif code==21:
            a.poly([[5,2],[8,4],[11,2],[11,13],[5,13]],'8a7010');a.rect(7,6,2,5,'020508');a.line([[6,2],[6,5]],'b6a326',.5)
        elif code==22:
            a.line([[4,2],[12,2],[10,5],[9,8],[10,11],[12,14],[4,14],[6,11],[7,8],[6,5],[4,2]],'20d9de',1.1)
            a.rect(5,1,6,1,'e8d91e');a.rect(5,14,6,1,'e8d91e');a.line([[8,3],[8,12]],'e8d91e',.8)
            for y in [5,10]:a.line([[3,y],[13,15-y]],'f02a2d',.65)
        elif code==23:
            a.rect(4,6,8,4,'f2342e');a.rect(6,6,1,4,'f6a19a');a.rect(9,6,1,4,'db322e')
        elif code==24:
            a.poly([[4,5],[12,5],[14,8],[12,11],[4,11],[2,8]],'f3dd1e');a.rect(5,6.8,6,2.4,'2658e9');a.line([[5,6],[11,6]],'fff4a4',.4)
        elif code==25:
            for x in [5,8,11]:a.line([[x,4],[x,12],[x-3,8],[x,4]],'e6eef1',.7)
        elif code==26:
            a.poly([[8,2],[13,8],[8,14],[3,8]],'2454e7');a.poly([[8,4],[11,8],[8,12],[5,8]],'020508');a.line([[8,2],[8,14]],'20cfdf',.7)
        elif code==27:
            for x in [4,7,10,13]:a.rect(x,2,1,2,'1be942');a.rect(x,12,1,2,'1be942')
            a.line([[6,4],[10,4],[10,12],[6,12],[6,4]],'efdb1b',1);a.line([[8,5],[8,11]],'222cd0',.7)
        elif code==28:
            a.rect(4,3,8,10,'e5eff3');a.rect(7,2,4,4,'21d4d3');a.rect(7,11,4,4,'21d4d3');a.rect(8,6,4,4,'020508')
        elif code==29:
            a.line([[4,4],[4,2],[12,2],[12,13],[4,13],[4,4]],'1de83d',1.1);a.rect(7,3,3,10,'24d2db')
            a.line([[8.5,4],[8.5,11]],'f32bdd',1.2)
        elif code==30:
            a.panel(4,1,9,14,'020508','c8d5dd');a.line([[6,2],[8,4],[6,7],[8,10],[6,14]],'edf4f8',.7)
            a.rect(10,4,2,7,'ed2d31');a.rect(8,5,2,1,'20d8d9');a.rect(8,10,2,1,'20d8d9')
        else:
            a.poly([[5,2],[8,2],[8,6],[11,6],[11,2],[13,2],[13,14],[10,14],[10,10],[7,10],[7,14],[5,14],[5,10],[2,10],[2,7],[5,7]],'f22a2f')
            a.line([[9,2],[9,14]],'2145e8',.8)
    else:raise ValueError(code)
    return a.nodes


def window_art(model):
    a=Art(160,100)
    a.rect(0,0,160,100,'f1df00');a.rect(.7,.7,158.6,98.6,'172735')
    a.rect(1.5,1.5,157,97,'020508')
    for key in ['partsBox','strip']:
        x,y,w,h=model[key];x-=model['window'][0];y-=model['window'][1]
        top=max(y+.6,14) if key=='partsBox' else y+.6
        a.rect(x+.6,top,w-1.2,y+h-.6-top,'071018')
    a.rect(1,1,158,13,'ee3441');a.rect(2,2,156,11,'eef3f7')
    a.line([[2,2],[157.5,2]],'ffffff',.35)
    for key in ['partsBox','strip']:
        x,y,w,h=model[key];x-=model['window'][0];y-=model['window'][1]
        a.line([[x,y],[x+w,y],[x+w,y+h],[x,y+h],[x,y]],'1762ff',.6)
        a.line([[x+1,y+h-1],[x+w-1,y+h-1]],'154061',.35)
    points=[[x+1,y+1] for x,y in model['manOutline']]
    a.poly(points,'102d46');a.line(points+[points[0]],'1762ff',.7)
    return a.nodes


def main():
    assert Path('/.dockerenv').exists()
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--model',type=Path,required=True);p.add_argument('--output',type=Path,required=True);args=p.parse_args()
    assert not args.output.exists();args.output.mkdir()
    model=json.loads(args.model.read_text())
    assert model['window']==[80,0,160,100] and len(model['slots'])==25 and len(model['icons'])==31
    art={'schema':'sundog-hd-stores-art-1','status':'DRAFT-independent-UI-and-icons-for-review','sourceMapping':model['icons'],'unknown':'code14 retains source ring/cable visual identity; purpose unknown; no new behavior','windowNodes':window_art(model),'icons':{str(c):item(c) for c in range(32)},'limits':'Independent vectors. No original images/masks read. No extra functional indicators or rules.'}
    (args.output/'art.json').write_text(json.dumps(art,ensure_ascii=False,indent=2)+'\n')
    paths=[]
    for scale in [4,5]:
        chrome=render_asset(art['windowNodes'],(160,100),scale,FONT);path=args.output/f'window-{scale}.png';chrome.save(path);paths.append(path)
        atlas=Image.new('RGBA',(128*scale,64*scale))
        for c in range(32):
            icon=render_asset(art['icons'][str(c)],(16,16),scale,FONT)
            if c:
                opaque=Image.new('RGBA',icon.size,'#020508');opaque.alpha_composite(icon);icon=opaque
                assert icon.getchannel('A').getextrema()==(255,255)
            else:
                assert icon.getchannel('A').getextrema()==(0,255)
            atlas.alpha_composite(icon,((c%8)*16*scale,(c//8)*16*scale))
        path=args.output/f'icons-{scale}.png';atlas.save(path);paths.append(path)
    sheet=Image.new('RGB',(960,520),'#101820');draw=ImageDraw.Draw(sheet);font=ImageFont.truetype(str(FONT),11)
    for c in range(32):
        x=(c%8)*120;y=(c//8)*130;icon=render_asset(art['icons'][str(c)],(16,16),6,FONT)
        sheet.paste(icon,(x+12,y+4),icon);name='empty' if c==0 else model['icons'][c-1]['name'];draw.text((x+3,y+102),str(c)+' '+name,font=font,fill='#e7eef5')
    path=args.output/'icon-contact.png';sheet.save(path);paths.append(path)
    sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
    report={'status':'DRAFT-independent-art-generated','toolSHA256':sha(Path(__file__)),'modelSHA256':sha(args.model),'artSHA256':sha(args.output/'art.json'),'outputs':{path.name:sha(path) for path in paths},'iconCount':32,'sourceIcons':31,'alphaContract':'Codes1..31 opaque independent #020508 cell; empty code0 transparent outside cross. Both sizes asserted.','sizes':['64x64 icons/640x400 chrome','80x80 icons/800x500 chrome'],'visualReview':'pending-single-image-inspection'}
    (args.output/'art-receipt.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print('Independent32 icons and native-size chrome written; source bitmap never read')


if __name__=='__main__':main()
