#!/usr/bin/env python3
"""Check stores review geometry/state/font ink; GUI completion is separate."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time


def main():
    assert Path('/.dockerenv').exists()
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--build',type=Path,required=True);p.add_argument('--model',type=Path,required=True);args=p.parse_args()
    root=Path(__file__).resolve().parents[1];build=args.build.resolve();model=args.model.resolve()
    prior=json.loads((build/'build-receipt.json').read_text())
    sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
    for name,h in prior['toolSourceHashes'].items():assert sha(root/name)==h,name
    overlay=json.loads((build/'overlay.json').read_text())
    for name in ['atari_hd_navigation_maps_test.go.txt','atari_hd_stores_review_test.go.txt']:
        virtual='zz_'+name.removesuffix('.go.txt')+'.go'
        overlay['Replace'][str(root/'cmd/sundog-ebiten'/virtual)]=str(root/'tools'/name)
    (build/'test-overlay.json').write_text(json.dumps(overlay,indent=2)+'\n')
    env=dict(os.environ,DISPLAY=':92',HD_REVIEW_MODEL=str(root/'docs/art/prototypes/atari-st-hd-navigation-review-model-20261015.json'),HD_STORES_MODEL=str(model),HD_WORLD_DISK=str(root/'dist-all/v.1.0.0-20261015/full-local/SunDog-v.1.0.0-20261015-private-full/sundog.st'),HD_STORES_SCENE_OUTPUT=str(build/'native-scenes.json'))
    xvfb=subprocess.Popen(['Xvfb',':92','-screen','0','1600x1000x24','-nolisten','tcp'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    try:
        deadline=time.monotonic()+5
        while not Path('/tmp/.X11-unix/X92').exists() and time.monotonic()<deadline:
            assert xvfb.poll() is None;time.sleep(.02)
        command=['go','test','-overlay',str(build/'test-overlay.json'),'./cmd/sundog-ebiten','-run','^TestHDStoresReview$','-count=1','-v']
        result=subprocess.run(command,cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=150)
        (build/'native-check.log').write_text(result.stdout);print(result.stdout)
        assert result.returncode==0
    finally:xvfb.terminate();xvfb.wait(timeout=5)


if __name__=='__main__':main()
