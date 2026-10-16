#!/usr/bin/env python3
"""Docker內接真實戰鬥保存驗證勝敗；入口docs/atari-st/ship-rules.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

from verify_banville_growth_player import verify_awake_resume


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    p=argparse.ArgumentParser(description=__doc__)
    for n in ('binary','disk','output'):
        p.add_argument('--'+n,type=Path,required=True)
    source=p.add_mutually_exclusive_group(required=True)
    source.add_argument('--save-input',type=Path)
    source.add_argument('--library-input',type=Path)
    p.add_argument('--shield-policy',choices=['full','off'],default='full')
    p.add_argument('--defeat-batch',type=int,default=1,choices=range(1,21),help='每次觀察前的正常R按鍵數；不注入遊戲時間')
    p.add_argument('--outcome',choices=['victory','defeat'],required=True)
    a=p.parse_args();assert Path('/.dockerenv').is_file()
    root=Path(__file__).resolve().parents[1]
    source_key='library_input' if a.library_input is not None else 'save_input'
    for n in ('binary','disk',source_key):
        setattr(a,n,getattr(a,n).resolve(strict=True))
    source_path=getattr(a,source_key)
    envelope=json.loads(source_path.read_text())
    initial=envelope['slots']['slot-1'] if a.library_input else envelope['state']
    assert initial['schema']==44 and initial['screen'] in ['shipboard','tactical']
    assert initial['combatActive'] and not initial['combatVictory'] and initial['hullCondition']>0
    assert sha(a.disk)=='f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6'
    out=a.output.resolve();assert not out.exists() and out.parent.stat().st_uid==os.getuid();out.mkdir()
    shutil.copyfile(Path(__file__),out/'tool-at-start.py')
    shutil.copyfile(source_path,out/('library.json' if a.library_input else 'save.json'))
    (out/'settings.json').write_text(json.dumps(dict(schema=1,theme='atari-st-original',tileset='atari-st-tiles',layout='atari-st')))
    env=dict(os.environ,DISPLAY=':116',HOME='/tmp')
    inputs=[];checks=[];app=None;window=None
    def write(name,d):
        (out/name).write_text(json.dumps(d,ensure_ascii=False,indent=2)+'\n')
    def run(*args):
        return subprocess.check_output(list(map(str,args)),env=env,text=True,timeout=20).strip()
    def key(value,pause=.07):
        inputs.append(dict(key=value,pause=pause))
        run('xdotool','keydown','--clearmodifiers',value);time.sleep(.07)
        run('xdotool','keyup',value);time.sleep(pause)
    def start(first=False):
        nonlocal app,window
        loading_library=first and a.library_input is not None
        command=[str(a.binary),'-scale','3','-atari-disk',str(a.disk),'-save',str(out/'save.json'),'-library',str(out/'library.json'),'-settings',str(out/'settings.json'),'-locale','zh-Hant']
        command+=['-start','title'] if loading_library else ['-load']
        inputs.append(dict(event='start',command=command,inputSHA256=sha(source_path) if first else sha(out/'save.json')))
        app=subprocess.Popen(command,cwd=out,env=env,stdout=(out/f'game-{len(checks)}.log').open('w'),stderr=subprocess.STDOUT)
        for _ in range(150):
            assert app.poll() is None,'正式程序提早退出'
            r=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,capture_output=True,text=True)
            if r.returncode==0 and r.stdout.strip():window=r.stdout.strip().splitlines()[-1];break
            time.sleep(.1)
        else:raise RuntimeError('找不到遊戲視窗')
        run('xdotool','windowfocus',window);time.sleep(.5)
        if loading_library:
            for value in ['space','c','1','l']:key(value)
    def observe(name):
        key('F5')
        if not (out/'library.json').exists():key('m')
        key('1');key('s')
        d=json.loads((out/'library.json').read_text())['slots']['slot-1']
        assert d['screen'] in ['tactical','shipboard'],'保存庫入口不符，停止後續操作'
        shutil.copyfile(out/'library.json',out/f'library-{name}.json')
        checks.append(dict(name=name,state=d,librarySHA256=sha(out/'library.json')))
        key('Escape')
        write('inputs.json',inputs);write('checks.json',checks)
        print(name,'enemy',d['enemyHull'],'hull',d['hullCondition'],'fuel',d['fuel'],'angle',d['enemyBearing'],d['enemyPitch'],flush=True)
        return d
    def single(name):
        run('import','-window',window,out/(name+'.png'))
        key('q');app.wait(timeout=20);assert app.returncode==0
        shutil.copyfile(out/'save.json',out/(name+'.json'))
        return json.loads((out/'save.json').read_text())['state']
    display=subprocess.Popen(['Xvfb',':116','-screen','0','1280x800x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
    try:
        time.sleep(.5);start(first=True)
        if initial['screen']=='shipboard':key('x')
        s=observe('start');run('import','-window',window,out/'start.png')
        raw=a.disk.read_bytes()[0xd422:0xd462]
        table=[int.from_bytes(raw[i:i+2],'big',signed=True) for i in range(0,64,2)]
        if a.outcome=='victory' and a.shield_policy=='full':key('u')
        else:key('o')
        s=observe('shield-selection')
        shots=[];deadline=time.monotonic()+600
        for turn in range(1,301):
            if not s['combatActive']:break
            assert time.monotonic()<deadline,'有界正常操作未到勝敗結果，不重抽種子'
            if a.outcome=='defeat':
                for _ in range(a.defeat_batch):key('r',.015)
            else:
                distance=max(10,s['combatRange']);base=(15+s['attributes'][2])//10
                cannon=max(0,4*base-distance//12);laser=max(0,base-distance//32)
                error=abs(s['enemyBearing']-512)+abs(s['enemyPitch']-512)
                if s['weaponCharge']>=80 and error<=cannon:
                    weapon='g' if error<=laser else 'f';key(weapon)
                    shots.append(dict(turn=turn,key=weapon,beforeHull=s['enemyHull'],aimError=error,charge=s['weaponCharge']))
                elif s['weaponCharge']<80:key('r')
                else:
                    # M的keydown、S探測及開F5前仍在轉向，均算入估計。
                    duration=.25 if error>60 else .015;n=max(1,round((duration+.07+.14+.03)*15))
                    weight=3-distance//32;db=s.get('enemyDriftBearing',0);dp=s.get('enemyDriftPitch',0)
                    tb=(s['enemyBearing']-db*4-512)/n;tp=(512-s['enemyPitch']-dp*4)/n
                    xi=min(range(32),key=lambda i:abs(2*table[31-i]+weight*db-tb))
                    yi=min(range(32),key=lambda i:abs(2*table[i]+weight*dp-tp))
                    x,y=xi*10+5,max(0,yi*6-2)
                    run('xdotool','mousemove','--window',window,x*3,y*3)
                    inputs.append(dict(mouseLogical=[x,y],purpose='唯讀保存輔助瞄準'))
                    key('m',duration)
            # Outcome不允許F5。先用正常S／Q單檔讀實際狀態，不合成保存庫。
            key('s')
            if (out/'save.json').exists():
                probe=json.loads((out/'save.json').read_text())['state']
                if probe['screen']=='outcome':
                    s=probe;checks.append(dict(name=f'turn-{turn}-outcome',state=s));break
            s=observe(f'turn-{turn:03d}')
            if shots and shots[-1]['turn']==turn:shots[-1]['afterHull']=s['enemyHull']
        assert not s['combatActive'],'有界操作仍未結束戰鬥'
        if a.outcome=='victory':
            assert s['combatVictory'] and s['enemyHull']==0 and s['hullCondition']>0
            time.sleep(2.2);run('import','-window',window,out/'victory.png')
            key('Escape');s=observe('return-shipboard');assert s['screen']=='shipboard'
        else:
            assert s['screen']=='outcome' and s['outcomeKey']=='ui.outcome.failure' and s['hullCondition']<=0
            key('Escape');key('x')
        before=single('result');start();time.sleep(2.2)
        after=single('reopened')
        stable=['schema','screen','combatActive','combatVictory','enemyHull','enemyType','enemyMaxHull','hullCondition','holds','warehouses','cash','bank','colony','outcomeKey','deaths']
        assert all(before.get(k)==after.get(k) for k in stable),'勝敗核心欄位重開不符'
        clock=verify_awake_resume(root,out,before,after,'combat-outcome')
        write('receipt.json',dict(status='PASS',outcome=a.outcome,shieldPolicy='off' if a.outcome=='defeat' else a.shield_policy,inputKind=source_key,scope='實際正式封包從既有真實戰鬥保存正常按鍵／滑鼠續行至勝敗、Q保存及新程序重開。瞄準讀正常保存輔助，非純視覺或原版完整GUI對拍。',inputs=inputs,checks=checks,shots=shots,stableFields=stable,clockReentry=clock,inputSHA256=dict(binary=sha(a.binary),disk=sha(a.disk),source=sha(source_path)),outputSaveSHA256=sha(out/'save.json'),toolSHA256=sha(out/'tool-at-start.py'),originalGUI='unverified'))
        print('PASS',a.outcome,flush=True)
    except Exception as e:
        write('failure.json',dict(status='FAIL',error=str(e),inputs=inputs,checks=checks,toolSHA256=sha(Path(__file__))))
        raise
    finally:
        if app is not None and app.poll() is None:
            app.terminate()
            try:app.wait(timeout=5)
            except subprocess.TimeoutExpired:app.kill();app.wait(timeout=5)
        display.terminate();display.wait(timeout=5)


if __name__=='__main__':main()
