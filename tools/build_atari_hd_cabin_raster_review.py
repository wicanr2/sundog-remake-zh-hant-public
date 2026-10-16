#!/usr/bin/env python3
"""Cache independently rendered cabin PNGs in a disposable Go overlay."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import subprocess

from PIL import Image


RASTER_GO = '''package main

import (
    "bytes"
    "encoding/base64"
    "fmt"
    "image/png"
    "github.com/hajimehoshi/ebiten/v2"
)

var hdCabinRasterCache = map[string]*ebiten.Image{}

func (a *app) hdCabinRaster(key string, w, h int) *ebiten.Image {
    if (key != "cabin-closed" && key != "cabin-lit") || w != 320 || h != 200 {
        return nil
    }
    cacheKey := fmt.Sprintf("%s/%d", key, a.viewScale())
    if cached := hdCabinRasterCache[cacheKey]; cached != nil { return cached }
    encoded, ok := hdCabinRasterData[cacheKey]
    if !ok { return nil } // Other review scales retain the existing vector path.
    raw, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil { panic(err) }
    decoded, err := png.Decode(bytes.NewReader(raw))
    if err != nil { panic(err) }
    if decoded.Bounds().Dx() != w*a.viewScale() || decoded.Bounds().Dy() != h*a.viewScale() {
        panic("HD cabin raster size mismatch")
    }
    sprite := ebiten.NewImageFromImage(decoded)
    hdCabinRasterCache[cacheKey] = sprite
    return sprite
}
'''


def main():
    assert Path('/.dockerenv').exists()
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--vector-build',type=Path,required=True)
    p.add_argument('--renders',type=Path,required=True)
    p.add_argument('--output',type=Path,required=True)
    args=p.parse_args();root=Path(__file__).resolve().parents[1]
    out=args.output.resolve();assert out.is_relative_to(root/'workplace') and not out.exists()
    prior=args.vector_build.resolve();renders=args.renders.resolve()
    sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
    build=json.loads((prior/'build-receipt.json').read_text())
    for name,h in build['sourceHashes'].items():assert sha(root/name)==h,name
    for name,h in build['overlayHashes'].items():assert sha(prior/name)==h,name
    static=json.loads((renders/'review.json').read_text());encoded={};hashes={}
    for scale,size in [(4,1280),(5,1600)]:
        for label,key in [('dark','cabin-closed'),('lit','cabin-lit')]:
            path=renders/f'cabin-{size}-{label}.png'
            assert sha(path)==static['outputs'][path.name]
            with Image.open(path) as im:assert im.size==(320*scale,200*scale)
            encoded[f'{key}/{scale}']=base64.b64encode(path.read_bytes()).decode('ascii')
            hashes[str(path.relative_to(root))]=sha(path)
    overlay=json.loads((prior/'overlay.json').read_text())
    world=root/'tools/atari_hd_world_review.go.txt';text=world.read_text()
    needle='func (a *app) hdWorldSprite(key string, w, h int, nodes []hdMapNode) *ebiten.Image {\n'
    assert text.count(needle)==1
    text=text.replace(needle,needle+'\tif raster := a.hdCabinRaster(key, w, h); raster != nil { return raster }\n',1)
    out.mkdir();(out/'world.go').write_text(text)
    declarations='\nvar hdCabinRasterData = map[string]string{\n'+''.join(json.dumps(k)+': '+json.dumps(v)+',\n' for k,v in encoded.items())+'}\n'
    (out/'raster.go').write_text(RASTER_GO+declarations)
    overlay['Replace'][str(root/'cmd/sundog-ebiten/zz_hd_world.go')]=str(out/'world.go')
    overlay['Replace'][str(root/'cmd/sundog-ebiten/zz_hd_cabin_raster.go')]=str(out/'raster.go')
    (out/'overlay.json').write_text(json.dumps(overlay,indent=2)+'\n')
    binary=out/'sundog-hd-review'
    command=['go','build','-overlay',str(out/'overlay.json'),'-o',str(binary),'./cmd/sundog-ebiten']
    subprocess.run(command,cwd=root,check=True,timeout=180)
    result={'status':'built-isolated-native-size-cabin-raster-cache','binarySHA256':sha(binary),'command':command,'vectorBuildReceiptSHA256':sha(prior/'build-receipt.json'),'vectorBuild':build,'rasterInputHashes':hashes,'toolSourceHashes':{'tools/build_atari_hd_cabin_raster_review.py':sha(Path(__file__))},'overlayHashes':{path.name:sha(path) for path in out.glob('*.go')},'scope':'Only cabin sprite preparation at scales4/5. Same approved-detail pixels, original lamp phase selection, ground/vector paths and all gameplay unchanged. Other review scales retain vectors.'}
    (out/'build-receipt.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    print('Native-size cabin raster cache:',sha(binary))


if __name__=='__main__':main()
