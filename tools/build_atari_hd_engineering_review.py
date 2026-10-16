#!/usr/bin/env python3
"""Isolated engineering composition using approved independent HD UI/icons."""
import argparse,base64,hashlib,json,subprocess
from pathlib import Path
from build_atari_hd_stores_art import window_art,FONT
from build_atari_hd_collision_review import render_asset

def main():
    assert Path('/.dockerenv').exists()
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['base-build','model','output']:p.add_argument('--'+name,type=Path,required=True)
    args=p.parse_args();root=Path(__file__).resolve().parents[1];base=args.base_build.resolve();out=args.output.resolve();assert out.is_relative_to(root/'workplace') and not out.exists()
    model=json.loads(args.model.read_text());assert model['window']==[72,64,160,100]
    nodes=window_art(model)
    # Formal engineering uses a red system box. Same approved UI geometry.
    boxx=model['partsBox'][0]-model['window'][0];boxy=model['partsBox'][1]-model['window'][1]
    found=0
    for node in nodes:
        if node['kind']=='line' and node['points'][0]==[boxx,boxy]:node['colour']=0xee3441;found+=1
    assert found==1
    out.mkdir();sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest();data={};rasters={}
    for scale in [4,5]:
        path=out/f'window-{scale}.png';render_asset(nodes,(160,100),scale,FONT).save(path);rasters[path.name]=sha(path);data[scale]=base64.b64encode(path.read_bytes()).decode()
    (out/'art.json').write_text(json.dumps({'status':'approved-UI-vocabulary-engineering-variant','windowNodes':nodes,'sourceModelSHA256':sha(args.model),'limits':'Independent vectors;only system box colour follows formal repair variant;no original bitmap/mask'},indent=2)+'\n')
    overlay=json.loads((base/'overlay.json').read_text());original=root/'cmd/sundog-ebiten/main.go';parent=Path(overlay['Replace'][str(original)]);text=parent.read_text();start=text.index('func (a *app) Draw(');end=text.index('\nfunc ',start+1);draw=text[start:end];needle='\tif isMenuScreen(a.state.Screen) {';assert draw.count(needle)==1;draw=draw.replace(needle,'\tif a.drawHDEngineeringReview(screen) { return }\n'+needle,1);(out/'main.go').write_text(text[:start]+draw+text[end:])
    (out/'data.go').write_text('package main\nvar hdEngineeringData=map[int]string{\n'+''.join(str(k)+':'+json.dumps(v)+',\n' for k,v in data.items())+'}\n')
    overlay['Replace'][str(original)]=str(out/'main.go')
    overlay['Replace'][str(root/'cmd/sundog-ebiten/zz_hd_engineering_review.go')]=str(root/'tools/atari_hd_engineering_review.go.txt')
    overlay['Replace'][str(root/'cmd/sundog-ebiten/zz_hd_engineering_data.go')]=str(out/'data.go');(out/'overlay.json').write_text(json.dumps(overlay,indent=2)+'\n')
    command=['go','build','-overlay',str(out/'overlay.json'),'-o',str(out/'sundog-hd-review'),'./cmd/sundog-ebiten'];subprocess.run(command,cwd=root,check=True,timeout=180)
    report={'status':'built-isolated-engineering-review','binarySHA256':sha(out/'sundog-hd-review'),'command':command,'baseReceiptSHA256':sha(base/'build-receipt.json'),'modelSHA256':sha(args.model),'rasterHashes':rasters,'artSHA256':sha(out/'art.json'),'toolSourceHashes':{str(p.relative_to(root)):sha(p) for p in [Path(__file__),root/'tools/atari_hd_engineering_review.go.txt']},'overlayHashes':{p.name:sha(p) for p in out.glob('*.go')},'limits':'Same rules/input/saves,approved independent icon cache inherited. Only Engineering scene rendered differently.'}
    (out/'build-receipt.json').write_text(json.dumps(report,indent=2)+'\n');print('Built',report['binarySHA256'])

if __name__=='__main__':main()
