#!/usr/bin/env python3
"""Run formal engineering export or review checks in bounded Docker/Xvfb."""
import argparse,json,os,subprocess,time
from pathlib import Path

def main():
    assert Path('/.dockerenv').exists()
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--build',type=Path,required=True);p.add_argument('--output',type=Path,required=True);p.add_argument('--export',action='store_true');a=p.parse_args();root=Path(__file__).resolve().parents[1];out=a.output.resolve();assert out.is_dir()
    overlay=json.loads((a.build/'overlay.json').read_text());name='export' if a.export else 'review'
    for file in ['atari_hd_navigation_maps_test.go.txt',f'atari_hd_engineering_{name}_test.go.txt']:
        overlay['Replace'][str(root/'cmd/sundog-ebiten'/('zz_'+file.removesuffix('.go.txt')+'.go'))]=str(root/'tools'/file)
    path=out/f'{name}-overlay.json';path.write_text(json.dumps(overlay,indent=2)+'\n')
    env=dict(os.environ,DISPLAY=':94',HD_ENGINEERING_SOURCE=str(root/'workplace/hd-ship-ground-review-20261015/followup1600/r1/save.json'),HD_ENGINEERING_MODEL=str(out/'model.json'),HD_ENGINEERING_SCENES=str(out/'native-scenes.json'),HD_REVIEW_MODEL=str(root/'docs/art/prototypes/atari-st-hd-navigation-review-model-20261015.json'),HD_WORLD_DISK=str(root/'dist-all/v.1.0.0-20261015/full-local/SunDog-v.1.0.0-20261015-private-full/sundog.st'))
    server=subprocess.Popen(['Xvfb',':94','-screen','0','1600x1000x24','-nolisten','tcp'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    try:
        deadline=time.monotonic()+5
        while not Path('/tmp/.X11-unix/X94').exists() and time.monotonic()<deadline:assert server.poll() is None;time.sleep(.02)
        target='Export' if a.export else 'Review';cmd=['go','test','-overlay',str(path),'./cmd/sundog-ebiten','-run',f'^TestHDEngineering{target}$','-count=1','-v'];result=subprocess.run(cmd,cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=150);(out/f'{name}-check.log').write_text(result.stdout);print(result.stdout);assert result.returncode==0
    finally:server.terminate();server.wait(timeout=5)

if __name__=='__main__':main()
