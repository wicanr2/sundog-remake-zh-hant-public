#!/usr/bin/env python3
"""Run review-only collision/draw checks using an existing isolated build overlay."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import time


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--build', type=Path, required=True)
    args = p.parse_args()
    assert Path('/.dockerenv').exists()
    root = Path(__file__).resolve().parents[1]
    build = args.build.resolve()
    overlay = json.loads((build/'overlay.json').read_text())
    for name, source in [('zz_hd_world_review_test.go','atari_hd_world_review_test.go.txt'), ('zz_hd_maps_review_test.go','atari_hd_navigation_maps_test.go.txt')]:
        overlay['Replace'][str(root/'cmd/sundog-ebiten'/name)] = str(root/'tools'/source)
    check_overlay = build/'test-overlay.json'
    check_overlay.write_text(json.dumps(overlay,indent=2)+'\n')
    env = dict(os.environ, DISPLAY=':89', HD_REVIEW_MODEL=str(root/'docs/art/prototypes/atari-st-hd-navigation-review-model-20261015.json'), HD_WORLD_DISK=str(root/'dist-all/v.1.0.0-20261015/full-local/SunDog-v.1.0.0-20261015-private-full/sundog.st'), HD_WORLD_FIXTURES=str(root/'dist-all/v.1.0.0-20261015/smoke/linux-x86_64/receipt.json'))
    xvfb = subprocess.Popen(['Xvfb', ':89', '-screen', '0', '1600x1000x24','-nolisten','tcp'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    try:
        time.sleep(.5)
        command = ['go','test','-overlay',str(check_overlay),'./cmd/sundog-ebiten','-run','^TestHDWorldReview$','-count=1','-v']
        result = subprocess.run(command,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=150)
        (build/'native-check.log').write_text(result.stdout)
        print(result.stdout)
        assert result.returncode == 0 and 'HD world review:' not in result.stdout
    finally:
        xvfb.terminate()
        xvfb.wait(timeout=10)


if __name__ == '__main__':
    main()
