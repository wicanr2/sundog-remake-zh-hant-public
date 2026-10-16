#!/usr/bin/env python3
"""Independent cabin structure revision; source masks are review inputs only."""
import argparse
import copy
import hashlib
import json
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont
from build_atari_hd_world_art import Art
from build_atari_hd_collision_review import render_asset, candidates, diagnostic


BASE_SHA256 = 'a8b5f660bcfdbc7bb364adf65962e9dda85dbeff4e4bdbafb274ea5154924e09'
ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT/'docs/art/prototypes/atari-st-hd-world-art-20261015.json'


def revised_cabin(nodes):
    nodes=copy.deepcopy(nodes)
    floor=[[59,82],[88,66],[97,51],[112,38],[128,35],[145,36],[151,42],[151,48],[145,52],[131,53],[116,52],[108,62],[108,77],[118,85],[131,86],[148,82],[163,82],[169,90],[169,111],[156,117],[146,118],[133,112],[118,112],[109,123],[107,139],[115,151],[128,155],[145,152],[150,155],[151,162],[143,165],[124,163],[109,154],[99,141],[95,123],[90,114],[59,114]]
    changed=0
    for node in nodes:
        if node['kind']=='polygon' and node['colour']==0x099f35:
            assert node['points'][0]==[49,75];node['points']=floor;changed+=1
        if node['kind']=='polygon' and node['colour']==0x0dc43a:
            assert node['points'][0]==[51,77];node['points']=[[59,82],[75,82],[85,74],[89,76],[88,108],[83,114],[59,114]];changed+=1
        if node['kind']=='line' and node['points'][0]==[52,78]:
            node['points']=[[88,68],[97,52],[113,39],[127,36]];changed+=1
    assert changed==3
    # Replace the entire invented small-console cluster with coherent banks.
    old=Art(320,200)
    for x,y in [(52,76),(55,89),(56,104),(69,73),(77,72)]:
        old.panel(x,y,5,7,'164dcb');old.rect(x+1,y+1,2,2,'29e7ef')
    starts=[i for i in range(len(nodes)-len(old.nodes)+1) if nodes[i:i+len(old.nodes)]==old.nodes]
    assert len(starts)==1
    bank=Art(320,200)
    bank.poly([[56,73],[85,60],[88,62],[88,75],[81,78],[78,82],[72,82],[70,77],[59,81]],'233747')
    bank.poly([[57,74],[85,61],[87,63],[87,74],[80,77],[77,81],[73,81],[71,76],[59,80]],'164dcb')
    bank.line([[58,73],[84,61]],'f1df38',.65)
    bank.poly([[60,78],[69,74],[77,74],[77,79],[74,82],[71,80],[70,76],[60,80]],'11b5bd')
    bank.line([[63,77],[68,75],[74,75]],'76edf1',.5)
    bank.poly([[51,77],[57,77],[59,84],[59,111],[57,117],[51,117]],'11b5bd')
    bank.line([[51,78],[51,115],[56,115]],'dce5ec',.6)
    bank.panel(52,84,6,14,'020508','24454d')
    bank.rect(53.5,87,2,4,'174fc1');bank.rect(54,93,2,2,'e738da')
    bank.panel(53,101,5,13,'164dcb','24454d');bank.rect(54.2,104,1.5,4,'ed37db')
    i=starts[0];nodes[i:i+len(old.nodes)]=bank.nodes
    # The central V-shaped bulkhead and adjacent equipment are source-visible
    # structures, independently reinterpreted in vectors, never mask cutouts.
    bulk=Art(320,200)
    bulk.poly([[120,92],[144,92],[144,97],[139,102],[135,107],[135,120],[129,125],[127,118],[124,110],[120,106]],'1b2631')
    bulk.poly([[121.2,93.2],[131,93.2],[131,100],[127,103],[127.5,113],[129,120],[133.5,117],[133.5,106],[137.5,101],[142.5,96],[142.5,93.2],[134,93.2],[133,100],[129,104],[126,103],[121.2,99]],'8e9ca7')
    bulk.line([[120.5,94],[120.5,105],[125.5,110],[128,120]],'e4edf0',.8)
    bulk.line([[141,95],[137,101],[133.5,105]],'e4edf0',.8)
    bulk.poly([[135,104],[140,100],[144,100],[144,119],[140,122],[135,122]],'020508')
    bulk.poly([[136.2,105],[140.5,101.3],[143,101.3],[143,118.5],[139.5,120.5],[136.2,120.5]],'244bed')
    bulk.line([[137,106],[137,118],[140,118]],'70e8ee',.65)
    bulk.rect(141,104,1,3,'f1df38');bulk.rect(141,111,1,4,'ed37db')
    slot=[i for i,n in enumerate(nodes) if n['kind']=='polygon' and n['points'][0]==[107,55]]
    assert len(slot)==1;nodes[slot[0]:slot[0]]=bulk.nodes
    return nodes


def build(output):
    raw=BASE.read_bytes();assert hashlib.sha256(raw).hexdigest()==BASE_SHA256
    art=json.loads(raw);before=copy.deepcopy(art)
    for key in ['cabinClosed','cabinLit']:art[key]=revised_cabin(art[key])
    art['status']='DRAFT-independent-cabin-structure-revision'
    art['revision']={'baseSHA256':BASE_SHA256,'method':'Independent vector cockpit banks, corridor outline and central bulkhead from visual structural review of image8; no source bitmap/mask input to build.','preserved':'Hull exterior, station rectangles, five lamp phase changes, ground art/tiles and all game rules.'}
    assert art['tiles']==before['tiles'] and art['groundBackdrop']==before['groundBackdrop']
    assert len(art['cabinClosed'])==len(art['cabinLit'])
    phases=0
    for off,on in zip(art['cabinClosed'],art['cabinLit']):
        a=copy.deepcopy(off);a['colour']=on['colour'];assert a==on
        phases+=off['colour']!=on['colour']
    assert phases==5 and not output.exists()
    output.write_text(json.dumps(art,ensure_ascii=False,indent=2)+'\n')
    print('Independent revision written; ground nodes unchanged; exactly five lamp colour changes')


def review(art_path,model_path,output):
    assert output.is_dir() and not (output/'review.json').exists()
    raw=art_path.read_bytes();art=json.loads(raw);base=json.loads(BASE.read_text());model=json.loads(model_path.read_text())
    font=ROOT/'assets/fonts/NotoSansCJKtc-Regular.otf';scale=5
    old=render_asset(base['cabinClosed'],(320,200),scale,font)
    new=render_asset(art['cabinClosed'],(320,200),scale,font)
    old_cues=candidates(old,model['cabin'],scale);new_cues=candidates(new,model['cabin'],scale)
    old_points=old_cues['reviewPoints'];resolutions=[]
    features=['cockpit lower rim','cockpit upper rim','central bulkhead rim','upper corridor boundary','central equipment face','upper console bank','upper console bank','left console face']
    for i,(point,feature) in enumerate(zip(old_points,features),1):
        x,y=point['x'],point['y'];pixel=new.getpixel((x*scale+2,y*scale+2))
        resolutions.append({'id':i,'x':x,'y':y,'feature':feature,'newRGB':list(pixel[:3]),'evidence':'source-visible structure, independent geometric reinterpretation; not a collision rule','visualStatus':'requires single-image inspection; not automatic parity'})
    old.save(output/'before.png');new.save(output/'after.png')
    diagnostic(new,model['cabin'],'船艙結構修訂',font,scale,old_points).save(output/'diagnostic.png')
    board=Image.new('RGB',(3200,1080),'#101820');board.paste(old,(0,80));board.paste(new,(1600,80))
    d=ImageDraw.Draw(board);face=ImageFont.truetype(str(font),28)
    d.text((20,18),'前版獨立原型',font=face,fill='#edf4fa');d.text((1620,18),'結構修訂原型：控制台、走道邊緣與中央艙壁',font=face,fill='#edf4fa')
    board.save(output/'comparison.png')
    assert art_path.read_bytes()==raw
    sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
    report={'status':'DRAFT-independent-structure-revision-not-parity','artSHA256':sha(art_path),'baseSHA256':BASE_SHA256,'modelSHA256':sha(model_path),'builderSHA256':sha(Path(__file__)),'before':old_cues,'after':new_cues,'eightPointReview':resolutions,'images':{p.name:sha(p) for p in output.glob('*.png')},'limits':'Green overlap counts are heuristic review cues. All source collision masks and rules unchanged. Pixels/masks are read only in review mode after independent art creation; no clipping/tracing feeds art.'}
    (output/'review.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'before':old_cues['greenOverSourceBlockedPixel'],'after':new_cues['greenOverSourceBlockedPixel'],'reviewPoints':resolutions},ensure_ascii=False))


def main():
    p=argparse.ArgumentParser(description=__doc__);sub=p.add_subparsers(dest='mode',required=True)
    b=sub.add_parser('build');b.add_argument('--output',type=Path,required=True)
    r=sub.add_parser('review');r.add_argument('--art',type=Path,required=True);r.add_argument('--model',type=Path,required=True);r.add_argument('--output',type=Path,required=True)
    args=p.parse_args();assert Path('/.dockerenv').exists()
    if args.mode=='build':build(args.output)
    else:review(args.art,args.model,args.output)


if __name__=='__main__':main()
