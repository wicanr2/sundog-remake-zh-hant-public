#!/usr/bin/env python3
"""Build a disposable HD stores composition over the verified cabin review."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import subprocess


def main():
    assert Path('/.dockerenv').exists()
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-build',type=Path,required=True);p.add_argument('--art',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
    args=p.parse_args();root=Path(__file__).resolve().parents[1]
    base=args.base_build.resolve();art=args.art.resolve();out=args.output.resolve()
    assert out.is_relative_to(root/'workplace') and not out.exists()
    sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
    model=json.loads((art/'art-receipt.json').read_text());data={};inputs={}
    for scale in [4,5]:
        for kind in ['window','icons']:
            path=art/f'{kind}-{scale}.png';assert sha(path)==model['outputs'][path.name]
            data[f'{kind}/{scale}']=base64.b64encode(path.read_bytes()).decode('ascii');inputs[str(path.relative_to(root))]=sha(path)
    overlay=json.loads((base/'overlay.json').read_text());original=root/'cmd/sundog-ebiten/main.go'
    parent=Path(overlay['Replace'][str(original)]);text=parent.read_text()
    start=text.index('func (a *app) Draw(');end=text.index('\nfunc ',start+1);draw=text[start:end]
    needle='\tif isMenuScreen(a.state.Screen) {';assert draw.count(needle)==1
    draw=draw.replace(needle,'\tif a.drawHDStoresReview(screen) { return }\n'+needle,1)
    text=text[:start]+draw+text[end:]
    out.mkdir();(out/'main.go').write_text(text)
    declaration='package main\nvar hdStoresRasterData = map[string]string{\n'+''.join(json.dumps(k)+': '+json.dumps(v)+',\n' for k,v in data.items())+'}\n'
    (out/'data.go').write_text(declaration)
    renderer=root/'tools/atari_hd_stores_review.go.txt'
    overlay['Replace'][str(original)]=str(out/'main.go')
    overlay['Replace'][str(root/'cmd/sundog-ebiten/zz_hd_stores_review.go')]=str(renderer)
    overlay['Replace'][str(root/'cmd/sundog-ebiten/zz_hd_stores_data.go')]=str(out/'data.go')
    (out/'overlay.json').write_text(json.dumps(overlay,indent=2)+'\n')
    binary=out/'sundog-hd-review';command=['go','build','-overlay',str(out/'overlay.json'),'-o',str(binary),'./cmd/sundog-ebiten']
    subprocess.run(command,cwd=root,check=True,timeout=180)
    result={'status':'built-isolated-stores-prototype','binarySHA256':sha(binary),'command':command,'baseBuildReceiptSHA256':sha(base/'build-receipt.json'),'rasterInputHashes':inputs,'toolSourceHashes':{str(path.relative_to(root)):sha(path) for path in [Path(__file__),renderer]},'parentMainSHA256':sha(parent),'overlayHashes':{path.name:sha(path) for path in out.glob('*.go')},'scope':'DRAFT stores scene only; same geometry, input/reducer/save and prior cabin/navigation/ground review. No production changes.'}
    (out/'build-receipt.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    print('Isolated stores binary:',sha(binary))


if __name__=='__main__':main()
