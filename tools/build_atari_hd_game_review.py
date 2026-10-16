#!/usr/bin/env python3
"""Build a disposable HD navigation game overlay, never a release package."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args()
    assert Path('/.dockerenv').exists(),'Docker only'
    root=Path(__file__).resolve().parents[1]
    out=args.output.resolve()
    assert out.is_relative_to(root/'workplace'),'review builds stay in workplace/'
    assert not out.exists(),'new output directory required'
    original=root/'cmd/sundog-ebiten/main.go'
    text=original.read_text()
    needle='\tif isMenuScreen(a.state.Screen) {'
    start=text.index('func (a *app) Draw(')
    end=text.index('\nfunc ',start+1)
    draw=text[start:end]
    assert draw.count(needle)==1
    draw=draw.replace(needle,'\tif a.drawHDNavigationReview(screen) { return }\n'+needle,1)
    text=text[:start]+draw+text[end:]
    text=text.replace('ebiten.SetWindowTitle("SunDog Remake")','ebiten.SetWindowTitle("SunDog HD Navigation Review — prototype")',1)
    out.mkdir()
    modified=out/'main.go';modified.write_text(text)
    art=root/'tools/atari_hd_navigation_game.go.txt'
    overlay=out/'overlay.json'
    overlay.write_text(json.dumps({'Replace':{str(original):str(modified),str(root/'cmd/sundog-ebiten/zz_hd_navigation_game.go'):str(art)}},indent=2)+'\n')
    binary=out/'sundog-hd-review'
    cmd=['go','build','-overlay',str(overlay),'-o',str(binary),'./cmd/sundog-ebiten']
    subprocess.run(cmd,cwd=root,check=True,timeout=180)
    sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
    report=dict(status='built-prototype-only',sourceMainSHA256=sha(original),overlayMainSHA256=sha(modified),artSourceSHA256=sha(art),builderSHA256=sha(Path(__file__)),binarySHA256=sha(binary),command=cmd,scope='Only Atari ST navigation sector is independently rendered at output resolution. Other screens use existing art. No F8 identity, gameplay rule, schema, or release package changes.')
    (out/'build-receipt.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(binary)


if __name__=='__main__':main()
