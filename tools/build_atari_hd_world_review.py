#!/usr/bin/env python3
"""Build a disposable cabin/Drahew review with the existing navigation prototype."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output',type=Path,required=True);p.add_argument('--art',type=Path,required=True);args=p.parse_args()
    assert Path('/.dockerenv').exists()
    root=Path(__file__).resolve().parents[1];out=args.output.resolve();assert out.is_relative_to(root/'workplace') and not out.exists()
    original=root/'cmd/sundog-ebiten/main.go';main_text=original.read_text();start=main_text.index('func (a *app) Draw(');end=main_text.index('\nfunc ',start+1);draw=main_text[start:end]
    needle='\tif isMenuScreen(a.state.Screen) {';assert draw.count(needle)==1
    draw=draw.replace(needle,'\tif a.drawHDNavigationReview(screen) { return }\n'+needle,1)
    close=draw.rfind('\n}');draw=draw[:close]+'\n\ta.drawHDWorldReview(screen)\n'+draw[close:]
    main_text=main_text[:start]+draw+main_text[end:];main_text=main_text.replace('ebiten.SetWindowTitle("SunDog Remake")','ebiten.SetWindowTitle("SunDog HD World Review Prototype")',1)
    sector=root/'tools/atari_hd_navigation_game.go.txt';art_text=sector.read_text();guard=' || a.state.NavigationView != game.NavigationSector';assert art_text.count(guard)==1;art_text=art_text.replace(guard,'',1)
    begin=art_text.index('\ttarget, hasTarget := a.reducer.World.Destination(a.state.NavigationTargetID)');finish=art_text.index('\tp.rect(141, 120, 72, 10, 0x020508)',begin)
    art_text=art_text[:begin]+'\tif a.state.NavigationView == game.NavigationSector {\n'+art_text[begin:finish]+'\t} else { p.drawNavigationMaps(a) }\n'+art_text[finish:]
    maps=root/'tools/atari_hd_navigation_maps.go.txt';map_text=maps.read_text();header='func (p *hdReviewPainter) drawNavigationMaps(a *app) {\n\tscene := hdNavigationMapScene(a)\n';assert map_text.count(header)==1
    map_text=map_text.replace(header,'func (p *hdReviewPainter) drawNavigationMaps(a *app) { p.drawHDMapScene(hdNavigationMapScene(a)) }\nfunc (p *hdReviewPainter) drawHDMapScene(scene hdMapScene) {\n a := p.a\n',1)
    out.mkdir();(out/'main.go').write_text(main_text);(out/'navigation.go').write_text(art_text);(out/'maps.go').write_text(map_text)
    asset=root/args.art;assert asset.is_file();(out/'art.go').write_text('package main\nvar hdWorldArtJSON = []byte('+json.dumps(asset.read_text())+')\n')
    world=root/'tools/atari_hd_world_review.go.txt'
    replacement={str(original):str(out/'main.go'),str(root/'cmd/sundog-ebiten/zz_hd_navigation_game.go'):str(out/'navigation.go'),str(root/'cmd/sundog-ebiten/zz_hd_navigation_maps.go'):str(out/'maps.go'),str(root/'cmd/sundog-ebiten/zz_hd_world.go'):str(world),str(root/'cmd/sundog-ebiten/zz_hd_world_art.go'):str(out/'art.go')}
    overlay=out/'overlay.json';overlay.write_text(json.dumps({'Replace':replacement},indent=2)+'\n')
    binary=out/'sundog-hd-review';command=['go','build','-overlay',str(overlay),'-o',str(binary),'./cmd/sundog-ebiten'];subprocess.run(command,cwd=root,check=True,timeout=180)
    sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
    receipt=dict(status='built-isolated-world-prototype',binarySHA256=sha(binary),command=command,sourceHashes={str(p.relative_to(root)):sha(p) for p in [original,sector,maps,world,asset,Path(__file__)]},overlayHashes={p.name:sha(p) for p in out.glob('*.go')},scope='Only shipboard, Drahew ground and three navigation views. Source collision path and formal gameplay unchanged.')
    (out/'build-receipt.json').write_text(json.dumps(receipt,indent=2)+'\n');print(binary)


if __name__=='__main__':main()
