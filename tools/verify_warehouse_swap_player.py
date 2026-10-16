#!/usr/bin/env python3
"""Docker內從正常取貨保存續行Store、購買、Swap及選槽中保存重開；不注入狀態。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('binary', 'disk', 'settings-input', 'library-input', 'output'):
        parser.add_argument('--'+name, type=Path, required=True)
    parser.add_argument('--review-only', action='store_true', help='只從正常倉庫保存重拍五語頁面，不購買／交換貨物')
    args = parser.parse_args()
    assert Path('/.dockerenv').is_file()
    root = Path(__file__).resolve().parents[1]
    sha = lambda p: hashlib.sha256(Path(p).read_bytes()).hexdigest()
    source_hashes = {name: sha(getattr(args,name)) for name in ('binary','disk','settings_input','library_input')}
    initial = json.loads(args.library_input.read_text())['slots']['slot-1']
    assert initial['schema']==44 and initial['screen']=='trade' and initial['groundMode']=='pod'
    assert initial['tradePage']==4 and initial['tradeSaleSlot']==0
    if not args.review_only:assert any(h.get('goods')==31 for h in initial['holds'])
    assert sha(args.disk)=='f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6'
    out=args.output.resolve(); assert not out.exists() and out.parent.stat().st_uid==os.getuid()
    out.mkdir()
    for name in ('assets','docs'): (out/name).symlink_to(root/name,target_is_directory=True)
    shutil.copyfile(args.library_input,out/'library.json')
    shutil.copyfile(args.settings_input,out/'settings.json')
    shutil.copyfile(Path(__file__),out/'tool-source.py')
    env=dict(os.environ,DISPLAY=':93',HOME='/tmp',LIBGL_ALWAYS_SOFTWARE='1',EBITENGINE_SCREENSHOT_KEY='F12')
    process=window=None; checks=[]; inputs=[]; images=[]; reentries=[]
    display=subprocess.Popen(['Xvfb',':93','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
    stable=['schema','screen','tradePage','tradeSaleSlot','tradeSalePrice','tradeSession','holds','warehouses','cash','bank','bankBalances','tradeClassShift','tradeRolls','combatRolls','colony','cityId','systemId','groundMode','fuel']
    financial=['tradeSession','cash','bank','bankBalances','tradeRolls','combatRolls','colony','quotes','quoteIndex']
    def run(*command):
        return subprocess.check_output(list(map(str,command)),env=env,text=True,stderr=subprocess.STDOUT,timeout=20).strip()
    def key(value):
        inputs.append({'key':value,'holdSeconds':.25})
        run('xdotool','keydown','--clearmodifiers',value);time.sleep(.25)
        run('xdotool','keyup',value);time.sleep(.35)
        assert process.poll() is None
    def click(x,y,button=1):
        inputs.append({'click':[x,y,button],'scale':4})
        run('xdotool','mousemove','--window',window,x*4,y*4)
        run('xdotool','click',button);time.sleep(.4)
    def start(load=False):
        nonlocal process,window
        command=[str(args.binary.resolve()),'-scale','4','-atari-disk',str(args.disk.resolve()),'-save',str(out/'save.json'),'-library',str(out/'library.json'),'-settings',str(out/'settings.json'),'-locale','zh-Hant']
        if load: command.append('-load')
        process=subprocess.Popen(command,cwd=out,env=env,stdout=(out/f'game-{len(reentries)}.log').open('a'),stderr=subprocess.STDOUT)
        end=time.monotonic()+25
        while time.monotonic()<end:
            assert process.poll() is None
            result=subprocess.run(['xdotool','search','--onlyvisible','--pid',str(process.pid)],env=env,text=True,capture_output=True)
            if result.returncode==0:
                window=result.stdout.splitlines()[-1];run('xdotool','windowfocus','--sync',window);time.sleep(1);return
            time.sleep(.1)
        raise RuntimeError('No normal game window')
    def capture(name):
        before=(out/'library.json').stat().st_mtime_ns
        for value in ('F5','1','s'): key(value)
        end=time.monotonic()+15
        while time.monotonic()<end and (out/'library.json').stat().st_mtime_ns==before: time.sleep(.1)
        assert (out/'library.json').stat().st_mtime_ns!=before,'F5 did not write library'
        raw=(out/'library.json').read_bytes();(out/f'library-{name}.json').write_bytes(raw)
        state=json.loads(raw)['slots']['slot-1']
        checks.append({'name':name,'state':state,'librarySHA256':hashlib.sha256(raw).hexdigest()})
        key('Escape')
        print(name,state.get('tradePage',0),state['tradeSaleSlot'],flush=True)
        return state
    def shot(name):
        before=set(out.glob('screenshot*.png'));key('F12')
        end=time.monotonic()+15
        while time.monotonic()<end and not set(out.glob('screenshot*.png'))-before:time.sleep(.1)
        created=set(out.glob('screenshot*.png'))-before;assert len(created)==1
        target=out/(name+'.png');created.pop().rename(target)
        images.append({'path':target.name,'sha256':sha(target)})
    def reopen(name):
        # Q performs the real single-file autosave; -load opens that envelope.
        inputs.append({'key':'q','purpose':'normal autosave and exit'})
        run('xdotool','keydown','--clearmodifiers','q');time.sleep(.25)
        run('xdotool','keyup','q');assert process.wait(timeout=20)==0
        before=json.loads((out/'save.json').read_text())['state']
        start(True);after=capture(name)
        assert all(before.get(k)==after.get(k) for k in stable),'saved swap changed on reopen'
        reentries.append({'name':name,'fields':stable,'before':{k:before.get(k) for k in stable},'after':{k:after.get(k) for k in stable}})
        return after
    def unchanged(before,after):
        assert before['holds']==after['holds'] and before['warehouses']==after['warehouses']
        assert all(before.get(k)==after.get(k) for k in financial),'swap changed market or funds'
    def locale_next(expected):
        key('F2');run('xdotool','keydown','--clearmodifiers','Right')
        try:
            end=time.monotonic()+15
            while time.monotonic()<end:
                if json.loads((out/'settings.json').read_text()).get('locale')==expected:break
                time.sleep(.1)
            else:raise RuntimeError('locale preference did not change: '+expected)
        finally:run('xdotool','keyup','Right')
        key('Escape')
    def review_screens(state):
        for attempt in range(3):
            if state.get('tileset')=='atari-st-hd-tiles':break
            key('F8');state=capture(f'12-presentation-{attempt}')
        assert state['tileset']=='atari-st-hd-tiles'
        for index,locale in enumerate(('zh-Hant','zh-Hans','ja','ko','en')):
            if index:locale_next(locale)
            shot('hd-warehouse-'+locale)
            key('x');shot('hd-swap-pod-'+locale)
            key('1');shot('hd-swap-warehouse-'+locale)
            key('Escape')
    def complete(scope):
        for name,value in source_hashes.items():assert sha(getattr(args,name))==value
        receipt={'status':'PASS','scope':scope,'sourceHashes':source_hashes,'checks':checks,'inputs':inputs,'images':images,'reentries':reentries,'saveSchema':44,'originalGUI':'unverified','toolSHA256':sha(out/'tool-source.py')}
        (out/'receipt.json').write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+'\n')
    try:
        time.sleep(.6);start();key('space')
        for value in ('F5','1','l'):key(value)
        state=capture('01-normal-source')
        if args.review_only:
            baseline=state
            assert state['holds'][0].get('kind')=='goods'
            assert any(l['goods']>0 for l in state['warehouses'][state['cityId']])
            review_screens(state)
            final=capture('02-normal-visual-review');unchanged(baseline,final)
            complete('normal saved-game library load and five-language HD warehouse / pod / warehouse pickers, cancel each selection; no purchase or Swap execution in this visual review')
            print('PASS: five-language normal Swap UI visual captures',flush=True)
            return
        pod=next(i for i,h in enumerate(state['holds']) if h.get('goods')==31)
        warehouse=next(i for i,l in enumerate(state['warehouses'][state['cityId']]) if l['goods']==0)
        cryo={k:state['holds'][pod].get(k,0) for k in ('goods','quality','price','sourceCode')}
        key('s');key(str(pod+1));state=capture('02-normal-stored-cryo')
        assert state['warehouses'][state['cityId']][warehouse]==cryo
        assert not state['holds'][pod].get('kind')
        # Buy two real quoted lots using the normal destination picker.
        key('Escape');key('e')
        for count in range(2):
            key('b');quoted=capture(f'03-quote-{count}')
            quote=quoted['quotes'][quoted.get('quoteIndex',0)]
            assert quote['price']*100<=quoted['tradeSession']['funds'] and quote['goods'] not in (0,31)
            key('b');paid=capture(f'04-paid-{count}');assert paid['tradePage']==10
            key('2');state=capture(f'05-normal-purchased-{count}')
            assert state.get('tradePage',0)==0 and sum(h.get('kind')=='goods' for h in state['holds'])==count+1
        key('Escape');key('w');baseline=capture('06-swap-baseline')
        assert baseline['tradePage']==4
        shot('06-warehouse-three-actions')
        key('x');click(220,115,3);canceled=capture('07-cancel-first-picker');unchanged(baseline,canceled)
        key('x');key('1');pending=capture('08-pending-before-reopen');assert pending['tradePage']==12 and pending['tradeSaleSlot']==5
        empty=next(i for i,l in enumerate(pending['warehouses'][pending['cityId']]) if l['goods']==0)
        key(str(empty+1));rejected=capture('09-empty-slot-rejected');unchanged(pending,rejected)
        assert rejected['tradePage']==12 and rejected['tradeSaleSlot']==5
        state=reopen('10-pending-reopened');shot('10-pending-reopened')
        click(220,115,3);canceled=capture('11-cancel-second-picker');unchanged(baseline,canceled)
        assert canceled['tradePage']==4 and canceled['tradeSaleSlot']==0
        # Native five-language screens share the official HD renderer.
        review_screens(state)
        key('x');key('1');key(str(warehouse+1));swapped=capture('13-keyboard-swapped')
        assert swapped['holds'][0].get('goods')==31
        assert swapped['warehouses'][swapped['cityId']][warehouse]=={k:baseline['holds'][0].get(k,0) for k in ('goods','quality','price','sourceCode')}
        assert swapped['holds'][1]==baseline['holds'][1] and all(swapped.get(k)==baseline.get(k) for k in financial)
        state=reopen('14-swapped-reopened');shot('14-swapped-reopened')
        # Same three actual original menu boxes, now operated by mouse.
        click(282,100);click(231,136);click(231,133+13*warehouse)
        restored=capture('15-mouse-swapped-back');unchanged(baseline,restored)
        assert restored['tradePage']==4 and restored['tradeSaleSlot']==0
        state=reopen('16-mouse-result-reopened');unchanged(baseline,state)
        complete('normal saved-game library load, Store31, two real purchases, both-picker cancellation, empty rejection, pending save/reopen, keyboard Swap, mouse reverse Swap, five-language HD screens; no state injection or seed reset')
        print('PASS: normal Store / Buy / Swap / pending and final reopens',flush=True)
    except Exception as error:
        (out/'receipt.json').write_text(json.dumps({'status':'FAIL','error':str(error),'checks':checks,'inputs':inputs},ensure_ascii=False,indent=2)+'\n')
        raise
    finally:
        if process and process.poll() is None:
            process.terminate()
            try:process.wait(timeout=10)
            except subprocess.TimeoutExpired:process.kill();process.wait(timeout=5)
        display.terminate()
        try:display.wait(timeout=5)
        except subprocess.TimeoutExpired:display.kill();display.wait(timeout=5)


if __name__=='__main__':main()
