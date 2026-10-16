#!/usr/bin/env python3
"""Docker內沿真實HD保存錄製正常操作；入口 docs/promo/README.md。"""
import argparse, hashlib, json, os, shutil, signal, subprocess, time
from pathlib import Path

def sha(p): return hashlib.sha256(Path(p).read_bytes()).hexdigest()

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ('binary','disk','save','settings','output'):
        parser.add_argument('--'+name,type=Path,required=True)
    a=parser.parse_args(); assert Path('/.dockerenv').is_file()
    assert not a.output.exists(), '錄影目錄禁止覆寫'
    sources={n:dict(path=str(getattr(a,n)),sha256=sha(getattr(a,n))) for n in ('binary','disk','save','settings')}
    a.output.mkdir(parents=True); out=a.output.resolve(); assert out.stat().st_uid==os.getuid()
    for n in ('save','settings'): shutil.copyfile(getattr(a,n),out/(n+'.json'))
    env=dict(os.environ,DISPLAY=':142',HOME='/tmp',LC_ALL='C.UTF-8')
    display=app=recorder=None; events=[]; clips=[]
    def run(*cmd): return subprocess.check_output(list(map(str,cmd)),env=env,text=True,timeout=15).strip()
    def key(name,settle=2):
        events.append(dict(key=name,monotonic=time.monotonic(),settleSeconds=settle))
        (out/'inputs.json').write_text(json.dumps(events,ensure_ascii=False,indent=2)+'\n')
        run('xdotool','keydown','--clearmodifiers',name);time.sleep(.2);run('xdotool','keyup',name)
        time.sleep(settle)
        if name!='q':assert app.poll() is None
    def clip(name,seconds):
        nonlocal recorder
        assert app.poll() is None
        shot=out/(name+'.png');run('import','-window',window,shot);video=out/(name+'.mp4')
        recorder=subprocess.Popen(['ffmpeg','-v','error','-f','x11grab','-framerate','30','-video_size','1280x800','-i',':142+0,0','-t',str(seconds),'-threads','2','-c:v','libx264','-pix_fmt','yuv444p','-crf','12','-preset','ultrafast',str(video)],env=env)
        assert recorder.wait(timeout=seconds+15)==0;recorder=None
        clips.append(dict(name=name,path=str(video),seconds=seconds,sha256=sha(video),screenshotSHA256=sha(shot)))
    try:
        display=subprocess.Popen(['Xvfb',':142','-screen','0','1280x800x24','-nolisten','tcp','-noreset'],env=env,stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT);time.sleep(.7)
        cmd=[str(a.binary.resolve()),'-load','-scale','4','-locale','zh-Hant','-atari-disk',str(a.disk.resolve()),'-save',str(out/'save.json'),'-settings',str(out/'settings.json'),'-library',str(out/'library.json')]
        app=subprocess.Popen(cmd,env=env,stdout=(out/'game.log').open('w'),stderr=subprocess.STDOUT)
        window=None
        for _ in range(120):
            assert app.poll() is None,'遊戲提早退出'
            r=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,text=True,capture_output=True,timeout=10)
            if r.returncode==0 and r.stdout.strip():window=r.stdout.strip().splitlines()[-1];break
            time.sleep(.2)
        assert window
        run('xdotool','windowmove',window,'0','0');run('xdotool','windowfocus','--sync',window);time.sleep(4)
        clip('cabin',5)
        key('n');clip('sector',3)
        key('Down');clip('system',3)
        key('Right');clip('planet',3)
        key('Escape');key('e');clip('engineering',5)
        key('Escape');key('b');clip('stores',5)
        key('Escape');key('g');key('space');clip('ground',6)
        key('F8');clip('apple',3)
        key('F8');clip('atari',3)
        key('F8');clip('hd-restored',3)
        key('q',.2);assert app.wait(timeout=15)==0
        assert json.loads((out/'save.json').read_text())['state']['tileset']=='atari-st-hd-tiles'
        for n,record in sources.items():assert sha(getattr(a,n))==record['sha256'],n
        data=dict(status='recorded-pending-visual-review',sources=sources,inputs=events,clips=clips,launch=cmd,scope='複製既有正常玩家保存與F8偏好後，由正式-load入口錄製，沒有狀態注入。錄影供展示，玩家驗收另記。',audio='沒有現場收音；獨立原版音效展示在合成階段另行標示。')
        (out/'recording.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');print('recorded',len(clips),'clips')
    finally:
        for process in (recorder,app,display):
            if process is not None and process.poll() is None:
                process.send_signal(signal.SIGINT if process is recorder else signal.SIGTERM)
                try:process.wait(timeout=5)
                except subprocess.TimeoutExpired:process.kill();process.wait(timeout=5)

if __name__=='__main__':main()
