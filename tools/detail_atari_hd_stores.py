#!/usr/bin/env python3
"""Disposable surface-detail pass over independent HD vectors, never original pixels."""
import argparse
import copy
import hashlib
import json
from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageFont
from build_atari_hd_world_art import Art
from build_atari_hd_collision_review import render_asset

ROOT = Path(__file__).resolve().parents[1]
FONT = ROOT / 'assets/fonts/NotoSansCJKtc-Regular.otf'
BACKGROUND = '#020508'


def details(code):
    a = Art(16, 16)
    light, dark = 'f2fbff', '526b81'
    if code == 1:
        a.line([[2.7,2],[2.7,14.3],[12.9,14.3]], dark,.35)
        a.line([[3,1.5],[11.8,1.5],[13.5,3.2]],light,.3)
        a.line([[5.6,11.7],[10.4,11.7]],'a8b8c5',.3)
        for x in [4,12]: a.ellipse(x,7,.35,.35,dark)
    elif code == 2:
        a.line([[3.8,5],[4.6,14.2],[11.5,14.2]],dark,.4)
        a.line([[3,3.5],[12.7,3.5]],light,.35)
        for x in [6.2,9.2]:a.line([[x,5.3],[x,12.4]],'e8fbfc',.35)
    elif code == 3:
        a.line([[1.8,6.7],[11.6,6.7],[11.6,4.5]],light,.4)
        a.line([[2,9.4],[11.4,9.4],[11.4,11.5],[13.8,8.6]],'a8becd',.4)
    elif code == 4:
        a.line([[6.5,.5],[6.5,1.6]],'d2b13c',.4)
        a.line([[6.5,14],[6.5,15.5]],'d2b13c',.4)
        for x,y in [(8,3.7),(12.1,8),(8,12.3),(3.9,8)]:a.ellipse(x,y,.22,.22,'ebd746')
        a.line([[4,4],[5,3.2],[6.4,2.6]],'fff6bc',.35)
    elif code == 5:
        a.line([[4.2,7],[3.3,10],[3.7,12]],light,.5)
        a.line([[5,14],[9.7,14],[12.1,12.6]],'a7b8c7',.4)
        a.line([[5.5,2],[10,2]],'f9edb8',.35)
    elif code == 6:
        a.line([[3.2,5.1],[3.2,13.3]],light,.3)
        a.line([[4,5.9],[11.6,13]],'a0b9cb',.3)
        a.line([[8,1.3],[8,4.8]],'b7ffc4',.25)
    elif code == 7:
        a.line([[7.4,5.8],[6.7,7.5],[7,9.5]],'fffef1',.4)
        a.line([[10.5,9],[9.7,10.4]],'ffaf52',.45)
    elif code == 8:
        a.line([[3.4,4.3],[4.5,3.1],[6.5,2.6],[10,2.6]],'f9cb55',.5)
        a.line([[3,9.5],[12.8,9.5]],'f37743',.35)
        a.line([[3.7,13.3],[12,13.3]],'ddb23c',.4)
        for x,y in [(6,5),(10,5.3),(12,4.1)]:a.ellipse(x,y,.32,.14,'fff0b5')
    elif code == 9:
        a.line([[3.7,7],[3.7,13.8],[10.8,13.8]],'ab7510',.4)
        a.line([[4.1,7.3],[4.1,12.9]],'fff3a2',.4)
        a.line([[12.5,5.6],[14.4,5.6],[14.4,10.4]],'91f5ef',.35)
        a.line([[3.4,2],[7.4,2],[9,2.7]],light,.35)
    elif code == 10:
        a.line([[1.5,7.4],[4.5,7.4]],'ff9feb',.3)
        a.line([[5.4,7.3],[7.5,7.3]],light,.3)
        for x in [5.8,6.6,7.4]:a.line([[x,8.3],[x,8.8]],'6b9faa',.2)
        a.line([[10.5,4.7],[10.5,10.3]],'97b8ff',.25)
    elif code == 11:
        a.poly([[8,3.7],[10.4,5.3],[8,7.5]],'ff7460')
        a.poly([[8,7.5],[10.4,5.3],[10.4,9.8],[8,12.1]],'d31f3b')
        a.line([[5.8,5.5],[5.8,9.8],[7.6,11.7]],'ffad91',.3)
        a.line([[2,6.6],[3.6,5.5]],'7a9cff',.4)
    elif code == 12:
        a.line([[3.4,5],[10.8,5]],'b1ffff',.35)
        a.line([[3,6],[8,6]],light,.35)
        a.line([[4.3,9.8],[3.7,13.3],[5.4,13.3]],dark,.45)
        a.line([[10,8.3],[11.8,7.6]],'8999a7',.4)
    elif code == 13:
        a.line([[1.7,4.6],[5.7,4.6],[7,5.9]],'7a9eff',.4)
        a.line([[3.3,13.8],[5.9,9.2]],'a1fbec',.4)
        a.line([[9,7.2],[12.4,11.8]],'ff9984',.35)
    elif code == 14:
        a.line([[4.5,10.6],[10.4,10.6]],'c0ffff',.3)
        a.line([[4.7,14.5],[10.4,14.5]],'158f9f',.35)
        a.line([[6.3,11.4],[6.3,13.5]],'ffbfa1',.25)
    elif code == 15:
        a.line([[3.5,4],[2.6,6],[2.6,9]],light,.5)
        a.line([[4.1,12.4],[6.1,13.3],[9.5,13.2]],'c3d1dd',.5)
        a.line([[6,4.5],[9.7,4.5]],'f87978',.3)
    elif code == 16:
        a.poly([[8,3.7],[9.6,6.5],[7.4,6.2],[5.5,7.4]],'7697ff')
        a.line([[5.5,9.9],[7.7,10.7],[9,10.1]],'ff99ed',.4)
    elif code == 17:
        a.line([[3,4.6],[5.4,2],[9.6,1.6],[11.5,3.3]],'fff27a',.45)
        a.line([[4.3,12.4],[7.4,14],[11.4,13.6]],'ff8b64',.4)
    elif code == 18:
        a.line([[2.7,3.8],[2.7,12.8],[4,14.1]],light,.35)
        a.line([[13.3,5.2],[13.3,13.2],[4.5,14.2]],'8b9cab',.4)
        a.line([[7,6.4],[10.8,6.4]],'a1ffc1',.3)
    elif code == 19:
        a.line([[7.1,2],[9.5,1.7],[11,3.7]],'6e91ff',.4)
        a.line([[6.3,10.8],[7.5,13],[9.5,13.2]],'86a7ff',.35)
    elif code == 20:
        for x,y in [(6,4),(10,11)]:a.line([[x-1.8,y-1],[x-1.8,y+.8]],light,.4)
    elif code == 21:
        a.line([[5.6,3.2],[5.6,12.3],[10.3,12.3]],'c7a841',.35)
        a.line([[10.5,3.2],[10.5,11.8]],'5f4a13',.35)
    elif code == 22:
        a.line([[5.7,1.3],[10.3,1.3]],'fff8a0',.3)
        a.line([[5.7,14.2],[10.3,14.2]],'fff8a0',.3)
        a.line([[7.8,3.6],[7.8,11.5]],'fff1a5',.3)
    elif code == 23:
        a.line([[4.5,6.5],[11.4,6.5]],'ffa49b',.4)
        a.line([[4.5,9.5],[11.4,9.5]],'b2213c',.4)
        for x in [7.4,10.5]:a.line([[x,7.3],[x,8.8]],'d62a3d',.3)
    elif code == 24:
        a.line([[4.3,5.5],[11.5,5.5],[13,7.5]],'fff3a0',.35)
        a.line([[5.5,7.4],[10.5,7.4]],'799eff',.35)
        a.line([[4.5,10.4],[11.5,10.4]],'b99c19',.35)
    elif code == 25:
        for x in [5,8,11]:a.line([[x-.1,5.4],[x-.1,10.7]],light,.25)
    elif code == 26:
        a.line([[7.3,3.8],[4.3,7.6]],'82a5ff',.3)
        a.line([[9,12.5],[11.9,8.4]],'537ddd',.3)
    elif code == 27:
        a.line([[6.3,4.4],[9.7,4.4]],'fff8ac',.35)
        a.line([[6.3,11.7],[9.7,11.7]],'be9b19',.35)
        for x in [4,7,10,13]:a.line([[x+.25,2.3],[x+.25,3.7]],'a6ffbd',.25)
    elif code == 28:
        a.line([[4.6,3.7],[4.6,12.3],[6.5,12.3]],light,.35)
        a.line([[7.5,2.5],[10.5,2.5]],'a8ffff',.35)
        a.line([[7.5,14.4],[10.5,14.4]],'148a9d',.35)
    elif code == 29:
        a.line([[4.2,3],[4.2,11.7]],'a3ffae',.3)
        a.line([[7.3,3.6],[7.3,12.4]],'beffff',.3)
        a.line([[8.3,4.8],[8.3,10.7]],'ffaeed',.25)
    elif code == 30:
        a.line([[4.2,1.2],[12.7,1.2],[12.7,14.7]],light,.3)
        a.line([[10.3,4.5],[10.3,10.5]],'ff9c8e',.3)
    elif code == 31:
        a.line([[5.5,2.6],[7.5,2.6],[7.5,6.4]],'ff9889',.35)
        a.line([[11.5,2.7],[11.5,13.3]],'ff807a',.35)
        a.line([[2.6,9.5],[5.5,9.5],[5.5,13.3]],'ae1934',.35)
    elif code != 0:
        raise ValueError(code)
    return a.nodes


def icon(art, code, scale):
    base = render_asset(art['icons'][str(code)], (16,16), scale, FONT)
    extra = render_asset(art['detailNodes'][str(code)], (16,16), scale, FONT)
    # This is the independent HD outline, never an original-game mask.
    alpha = base.getchannel('A')
    extra.putalpha(ImageChops.multiply(extra.getchannel('A'), alpha))
    result = Image.alpha_composite(base, extra)
    result.putalpha(alpha)
    return result


def main():
    assert Path('/.dockerenv').is_file()
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--input', type=Path, default=ROOT/'docs/art/prototypes/atari-st-hd-stores-art-20261015.json')
    p.add_argument('--output', type=Path, required=True)
    args=p.parse_args();assert not args.output.exists();args.output.mkdir()
    original=json.loads(args.input.read_text());art=copy.deepcopy(original)
    art['status']='DRAFT-independent-stores-detail-review'
    art['detailNodes']={str(c):details(c) for c in range(32)}
    art['detailLimits']='Additive surface nodes clipped only to own independent HD alpha. Original vectors/window/slots/empty cross retained. No original bitmap or collision mask input; no new functionality.'
    assert art['icons']==original['icons'] and art['windowNodes']==original['windowNodes']
    sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
    (args.output/'art.json').write_text(json.dumps(art,ensure_ascii=False,indent=2)+'\n')
    paths=[];checks=[]
    for scale in [4,5]:
        chrome=render_asset(art['windowNodes'],(160,100),scale,FONT);path=args.output/f'window-{scale}.png';chrome.save(path);paths.append(path)
        atlas=Image.new('RGBA',(128*scale,64*scale))
        for c in range(32):
            old=render_asset(original['icons'][str(c)],(16,16),scale,FONT);new=icon(art,c,scale)
            assert old.getchannel('A').tobytes()==new.getchannel('A').tobytes()
            changed=ImageChops.difference(old.convert('RGB'),new.convert('RGB')).getbbox() is not None
            assert changed==(c!=0),(c,scale)
            if c:
                opaque=Image.new('RGBA',new.size,BACKGROUND);opaque.alpha_composite(new);new=opaque
                assert new.getchannel('A').getextrema()==(255,255)
            atlas.alpha_composite(new,((c%8)*16*scale,(c//8)*16*scale))
            checks.append({'code':c,'scale':scale,'alphaUnchanged':True,'surfaceChanged':changed})
        path=args.output/f'icons-{scale}.png';atlas.save(path);paths.append(path)
    sheets=[]
    for detailed in [False,True]:
        sheet=Image.new('RGB',(960,520),'#101820');draw=ImageDraw.Draw(sheet);font=ImageFont.truetype(str(FONT),11)
        for c in range(32):
            x=c%8*120;y=c//8*130;sprite=icon(art,c,6) if detailed else render_asset(original['icons'][str(c)],(16,16),6,FONT)
            sheet.paste(sprite,(x+12,y+4),sprite);name='empty' if c==0 else art['sourceMapping'][c-1]['name'];draw.text((x+3,y+102),str(c)+' '+name,font=font,fill='#e7eef5')
        sheets.append(sheet)
    path=args.output/'icon-contact.png';sheets[1].save(path);paths.append(path)
    comparison=Image.new('RGB',(1920,554),'#101820');comparison.paste(sheets[0],(0,34));comparison.paste(sheets[1],(960,34));draw=ImageDraw.Draw(comparison);font=ImageFont.truetype(str(FONT),22)
    draw.text((12,3),'前版獨立原型',font=font,fill='#e7eef5');draw.text((972,3),'新增表面細節，仍待審查',font=font,fill='#e7eef5')
    path=args.output/'comparison.png';comparison.save(path);paths.append(path)
    report={'status':'PASS-independent-detail-invariants;DRAFT-art-review','toolSHA256':sha(Path(__file__)),'inputSHA256':sha(args.input),'artSHA256':sha(args.output/'art.json'),'outputs':{p.name:sha(p) for p in paths},'addedNodes':sum(len(n) for n in art['detailNodes'].values()),'checks':checks,'windowUnchanged':True,'baseVectorNodesUnchanged':True,'emptyCrossUnchanged':True,'limits':art['detailLimits']}
    (args.output/'art-receipt.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print('PASS64 independent alpha/surface checks;window/empty/base vectors retained;',report['addedNodes'],'new detail nodes')


if __name__=='__main__':main()
