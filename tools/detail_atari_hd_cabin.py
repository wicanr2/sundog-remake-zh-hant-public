#!/usr/bin/env python3
"""Detail the approved cabin vectors without reading source pixels or masks."""
import argparse
import copy
import hashlib
import json
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont
from build_atari_hd_world_art import Art
from build_atari_hd_collision_review import render_asset


ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT/'docs/art/prototypes/atari-st-hd-cabin-refined-art-20261015.json'
BASE_SHA = '746c7c737821c7dd923c72cbe45705af5fffee7d4bb44a62afb809f559641a6b'
FONT = ROOT/'assets/fonts/NotoSansCJKtc-Regular.otf'


def screw(a, x, y, radius=.45):
    a.ellipse(x, y, radius, radius, '5f707c')
    a.ellipse(x-.08, y-.08, radius*.65, radius*.65, 'e0e9ee')
    a.line([[x-radius*.4, y],[x+radius*.4, y]], '516773', .16)


def details():
    a = Art(320, 200)
    # These are surface insets, not a new hull, a new corridor or new controls.
    a.rect(17.1,82.2,4.7,31.2,'aab8c2')
    a.line([[17.5,112.8],[17.5,82.8],[21,82.8]],'f4f8fa',.35)
    a.line([[21,84],[21,111.5]],'718694',.35)
    for y in [84,111.5]:screw(a,19,y,.35)
    for y in [25,148]:
        # Five exterior modules retain the existing divisions and apertures.
        for x,w in [(165,16),(183,20),(205,20),(227,20),(249,20),(271,20)]:
            a.rect(x+1,y+1.5,w-2,1.2,'eef4f7')
            a.rect(x+1,y+18.8,w-2,1.1,'b1c0ca')
            a.line([[x+1,y+3.5],[x+1,y+17.8]],'c4d0d8',.3)
            for yy in [y+3.1,y+18]:screw(a,x+w-2.3,yy,.32)
        # The dark triangular apertures remain unobstructed.
        for x in [185,207,251,273]:
            for i in range(4):a.line([[x+i*.9,y+13],[x+i*.9,y+16.4]],'a8b8c3',.24)
    for y in [54,81,108]:
        a.poly([[175.2,y+3.2],[276.5,y+3.2],[276.5,y+19.8],[217,y+19.8],[217,y+5],[188,y+5],[188,y+19.8],[175.2,y+19.8]],'c3ced6')
        a.line([[176,y+4.2],[275.5,y+4.2]],'eef4f8',.35)
        a.line([[176,y+20.5],[275.5,y+20.5]],'879ca9',.5)
        a.line([[219,y+7],[253,y+7]],'d9e3e9',.35)
        a.line([[219,y+17],[253,y+17]],'a4b5c0',.35)
        for xx in [178.2,274.2]:
            for yy in [y+5.5,y+18]:screw(a,xx,yy)
        # Existing hatch rectangle and black inset: inset bevels only.
        a.line([[191.2,y+15.8],[191.2,y+8.2],[213.8,y+8.2]],'e0e9ee',.35)
        a.line([[213.8,y+8.8],[213.8,y+15.8],[191.8,y+15.8]],'475f70',.45)
        for xx in [191.8,213]:screw(a,xx,y+9,.28)
        for xx in [260,262,264,266,268]:
            a.rect(xx,y+7,1.05,8,'8d9fae')
            a.line([[xx,y+7],[xx,y+14.7]],'e0e9ee',.22)
        # Static latch grooves cannot be mistaken for numeric cargo readings.
        a.line([[229,y+11],[245,y+11]],'7d929f',.6)
        a.line([[230,y+10.5],[244,y+10.5]],'edf4f8',.25)
    # The approved upper cockpit console bank gets inset surfacing.
    a.poly([[59,74],[83.8,62.8],[85.2,64],[85.2,72.8],[79.7,75],[77.2,78.8],[74.1,79.5],[71.3,74.4],[59.4,78.3]],'235ee3')
    a.line([[60,74.3],[83.5,63.7]],'61b4f0',.3)
    a.line([[82.2,65.2],[82.2,73.3]],'123ba6',.35)
    a.poly([[61,74.5],[67.9,71.4],[69.8,74.5],[62.3,77.5]],'152f69')
    a.poly([[62,74.6],[67.4,72.2],[68.6,74.2],[62.8,76.5]],'35dce8')
    a.line([[63,74.9],[66.8,73.3]],'c4f7f6',.25)
    for x,y in [(72.8,69.8),(75.5,68.6),(78.2,67.4)]:
        a.poly([[x,y],[x+1.5,y-.6],[x+2,y+.8],[x+.5,y+1.4]],'9bd8ef')
        a.line([[x+.5,y+2.2],[x+1.7,y+1.7]],'132c64',.35)
    screw(a,58.8,75.1,.3);screw(a,85.1,64.4,.3)
    # Left black console: keep its black face and blue/magenta accents.
    a.line([[52.9,85.1],[56.5,85.1]],'6a8694',.3)
    a.line([[52.8,96.5],[56.5,96.5]],'132733',.35)
    for y in [86.1,96.1]:screw(a,56.7,y,.25)
    a.line([[53.5,102.3],[56.3,102.3]],'66a9ec',.3)
    a.line([[53.3,112.3],[56.4,112.3]],'132e77',.4)
    # Central V bulkhead: faceted inset on the already approved gray face.
    a.poly([[122.4,94.1],[129.8,94.1],[129.8,99.4],[126,102.5],[126.6,112.8],[128.3,117.7],[125.8,109.3],[121.9,104.8]],'afbec8')
    a.line([[122.8,94.8],[128.7,94.8]],'e4edf2',.35)
    a.line([[128.1,105.2],[129.2,116]],'617a8a',.35)
    for x,y in [(123.1,96.3),(125.4,105.1),(129.4,118.1),(140,94.8)]:screw(a,x,y,.3)
    a.line([[136.8,106],[136.8,117.7]],'a1e9f5',.3)
    a.line([[138.3,103.4],[140.3,102.1]],'73a0ee',.35)
    a.line([[141.5,115.7],[141.5,117.5]],'152b80',.4)
    # Internal dark equipment faces retain their silhouette and black value.
    for x,y in [(115,63),(117,132)]:
        a.poly([[x,y],[x+10,y],[x+12,y+8],[x+9,y+12],[x,y+9]],'0c151e')
        for i in range(5):a.line([[x+2,y+2+i*1.3],[x+8,y+2+i*1.3]],'1b2d3b',.35)
    # Existing airlock rim gets a static recessed latch, without a new label.
    a.line([[69.4,114.5],[78.5,114.5]],'586f7f',.4)
    for x in [69.3,78.7]:screw(a,x,112.9,.35)
    # Draw the existing station faces above bank ornamentation. Their bounds
    # and original blue/teal screen fills must stay visually identifiable.
    for x,y,w,h in [(68,68,12,8),(129,29,16,8),(146,34,8,12),
                    (129,46,16,5),(129,145,16,8),(146,150,8,12),
                    (129,161,16,6),(103,101,8,11)]:
        if (x,y)==(68,68):
            a.panel(x,y,w,h,'1b4be4');a.rect(70,70,6,3,'32dcec')
        else:
            a.panel(x,y,w,h,'11b5bd')
            a.rect(x+1.8,y+1.8,max(1,w-4),max(1,h-4),'174fc1')
        a.line([[x+1.2,y+h-1.3],[x+1.2,y+1.2],[x+w-1.2,y+1.2]],'b7eaf1',.25)
        for xx in [x+.4,x+w-.4]:screw(a,xx,y+h/2,.22)
    return a.nodes


def build(output):
    assert not output.exists()
    raw=BASE.read_bytes();assert hashlib.sha256(raw).hexdigest()==BASE_SHA
    art=json.loads(raw);before=copy.deepcopy(art);extra=details()
    for key in ['cabinClosed','cabinLit']:
        original=art[key]
        assert len(original[-5:])==5 and all(n['kind']=='polygon' for n in original[-5:])
        art[key]=original[:-5]+copy.deepcopy(extra)+original[-5:]
        assert art[key][:len(original)-5]+art[key][-5:]==original
    assert art['groundBackdrop']==before['groundBackdrop'] and art['tiles']==before['tiles']
    changes=0
    for off,on in zip(art['cabinClosed'],art['cabinLit']):
        item=copy.deepcopy(off);item['colour']=on['colour'];assert item==on
        changes+=off['colour']!=on['colour']
    assert changes==5
    art['status']='APPROVED-structure-A-detail-batch; isolated-cabin-only'
    art['detailRevision']={'approvedInputSHA256':BASE_SHA,'userDecision':'同意這一版 開工','method':'Independent vector surface insets and clean bevels; no bitmap, mask, noise, filter or original pixels read by build.','originalNodeCount':len(before['cabinClosed']),'addedNodeCount':len(extra),'preserved':'Every approved structural node in order, all station boxes, exactly five lamp colour changes, ground and tile JSON.'}
    output.write_text(json.dumps(art,ensure_ascii=False,indent=2)+'\n')
    print('Detailed cabin written:',len(extra),'new nodes; all approved nodes preserved; five lamp changes')


def review(art_path, output):
    assert output.is_dir() and not (output/'review.json').exists()
    art=json.loads(art_path.read_text());base=json.loads(BASE.read_text())
    paths=[]
    for size,scale in [('1600',5),('1280',4)]:
        for label,key in [('dark','cabinClosed'),('lit','cabinLit')]:
            p=output/f'cabin-{size}-{label}.png'
            render_asset(art[key],(320,200),scale,FONT).save(p);paths.append(p)
    before=render_asset(base['cabinClosed'],(320,200),5,FONT)
    after=render_asset(art['cabinClosed'],(320,200),5,FONT)
    board=Image.new('RGB',(3200,1080),'#101820');board.paste(before,(0,80));board.paste(after,(1600,80))
    painter=ImageDraw.Draw(board);font=ImageFont.truetype(str(FONT),28)
    painter.text((20,18),'已核定結構',font=font,fill='#edf4fa')
    painter.text((1620,18),'A 清晰精繪：保留結構，補面板與表面細節',font=font,fill='#edf4fa')
    board.save(output/'comparison.png');paths.append(output/'comparison.png')
    sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
    d={'schema':'sundog-hd-cabin-detail-static-1','status':'PASS-reproducible-independent-vector-render; visual-review-separate','approvedInputSHA256':BASE_SHA,'artSHA256':sha(art_path),'toolSHA256':sha(Path(__file__)),'outputs':{p.name:sha(p) for p in paths},'size':'1600x1000 and1280x800, native vector rasterization','limits':'Static art only, no normal game state or original parity. Two variants differ solely at five existing lamp colours.'}
    (output/'review.json').write_text(json.dumps(d,ensure_ascii=False,indent=2)+'\n')
    print('Wrote four native-size panels and comparison; no source image/mask input')


def main():
    assert Path('/.dockerenv').exists()
    p=argparse.ArgumentParser(description=__doc__);sub=p.add_subparsers(dest='mode',required=True)
    b=sub.add_parser('build');b.add_argument('--output',type=Path,required=True)
    r=sub.add_parser('review');r.add_argument('--art',type=Path,required=True);r.add_argument('--output',type=Path,required=True)
    args=p.parse_args()
    if args.mode=='build':build(args.output)
    else:review(args.art,args.output)


if __name__=='__main__':main()
