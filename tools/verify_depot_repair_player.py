#!/usr/bin/env python3
"""正常受損船救援、航行、depot付費修船及重開；入口ftl-boost-spec.md。"""
import argparse
from collections import deque
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import time

from verify_banville_growth_player import verify_awake_resume


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    p=argparse.ArgumentParser(description=__doc__)
    for n in ['binary','disk','save-input','output']:p.add_argument('--'+n,type=Path,required=True)
    a=p.parse_args();assert Path('/.dockerenv').exists()
    root=Path(__file__).resolve().parents[1]
    for n in ['binary','disk','save_input']:setattr(a,n,getattr(a,n).resolve(strict=True))
    initial=json.loads(a.save_input.read_text())['state']
    assert initial['schema']==44 and initial['screen']=='shipboard' and initial['hullCondition']<16 and not initial['combatActive']
    out=a.output.resolve();assert not out.exists() and out.parent.stat().st_uid==os.getuid();out.mkdir()
    shutil.copyfile(a.save_input,out/'save.json');shutil.copyfile(Path(__file__),out/'tool-at-start.py')
    (out/'settings.json').write_text(json.dumps(dict(schema=1,theme='atari-st-original',tileset='atari-st-tiles',layout='atari-st')))
    env=dict(os.environ,DISPLAY=':118',HOME='/tmp')
    checks=[];inputs=[];app=None;window=None;reentries=[]
    def write(n,d):(out/n).write_text(json.dumps(d,ensure_ascii=False,indent=2)+'\n')
    def run(*argv):return subprocess.check_output(list(map(str,argv)),env=env,text=True,timeout=20).strip()
    def key(k,pause=.15):
        inputs.append(dict(key=k,pause=pause));run('xdotool','keydown','--clearmodifiers',k);time.sleep(.1);run('xdotool','keyup',k);time.sleep(pause)
    def start():
        nonlocal app,window
        cmd=[str(a.binary),'-scale','3','-load','-atari-disk',str(a.disk),'-save',str(out/'save.json'),'-library',str(out/'library.json'),'-settings',str(out/'settings.json'),'-locale','zh-Hant']
        inputs.append(dict(command=cmd,saveSHA256=sha(out/'save.json')))
        app=subprocess.Popen(cmd,cwd=out,env=env,stdout=(out/f'game-{len(checks)}.log').open('w'),stderr=subprocess.STDOUT)
        for _ in range(150):
            assert app.poll() is None
            result=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,capture_output=True,text=True)
            if result.returncode==0 and result.stdout.strip():window=result.stdout.strip().splitlines()[-1];break
            time.sleep(.1)
        else:raise RuntimeError('找不到視窗')
        run('xdotool','windowfocus',window);time.sleep(.5)
    def capture(name):
        before=(out/'library.json').stat().st_mtime_ns if (out/'library.json').exists() else 0
        key('F5')
        if not (out/'library.json').exists():key('m')
        for k in ['1','s','Escape']:key(k)
        assert (out/'library.json').stat().st_mtime_ns!=before
        shutil.copyfile(out/'library.json',out/f'library-{name}.json')
        s=json.loads((out/'library.json').read_text())['slots']['slot-1'];checks.append(dict(name=name,state=s,sha256=sha(out/'library.json')))
        write('inputs.json',inputs);write('checks.json',checks)
        print(name,s['screen'],s['cityId'],s['position'],'hull',s['hullCondition'],'fuel',s['fuel'],'cash',s['cash'],flush=True)
        return s
    def shot(name):
        time.sleep(2.2);run('import','-window',window,out/(name+'.png'))
    def reopen(name,fields):
        key('q');app.wait(timeout=20);assert app.returncode==0
        before=json.loads((out/'save.json').read_text())['state'];shutil.copyfile(out/'save.json',out/(name+'-before.json'))
        start();after=capture(name+'-reopened')
        assert all(before.get(k)==after.get(k) for k in fields),'重開核心欄位不同'
        clock=verify_awake_resume(root,out,before,after,name)
        reentries.append(dict(name=name,fields=fields,clock=clock))
        return after
    world=json.loads((root/'internal/source/atarist/assets/world-data.json').read_text())
    cities=world['tables']['cities']['records'];maps=json.loads((root/'internal/source/atarist/assets/city-maps.json').read_text())['cities']
    slug=lambda s:re.sub(r'[^a-z0-9]+','-',s.lower()).strip('-')
    def walk_grid(s,goal,name):
        city=next(c for c in cities if slug(c['name'])==s['cityId']);m=maps[city['index']]
        buildings={(c['row'],c['column']) for c in m['buildings']};start=(s['position']['x'],s['position']['y']);ship=(s['shipPosition']['x'],s['shipPosition']['y'])
        q=deque([start]);prev={start:None}
        while q and goal not in prev:
            at=q.popleft()
            for k,dx,dy in [('Up',0,-1),('Right',1,0),('Down',0,1),('Left',-1,0)]:
                n=(at[0]+dx,at[1]+dy);x,y=n
                if n in prev or n==ship and n!=goal or not (-1<=x<32 and -1<=y<20):continue
                if 0<=x<20 and 0<=y<12 and n not in buildings and m['grid'][x][y] in [6,7,12,13,14]:continue
                prev[n]=(at,k);q.append(n)
        assert goal in prev,'無正常路徑'
        route=[];end=goal
        while end!=start:end,k=prev[end];route.append(k)
        for i,k in enumerate(reversed(route)):
            key(k);s=capture(name+f'-{i}')
            if s['screen']=='dialog':key('Escape');s=capture(name+f'-{i}-dialog')
            if s['screen']=='arena':key('x');s=capture(name+f'-{i}-fled')
            assert s['screen'] in ['ground','shipboard']
        assert s['position']==dict(x=goal[0],y=goal[1])
        return s
    display=subprocess.Popen(['Xvfb',':118','-screen','0','1280x800x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
    try:
        time.sleep(.5);start();s=capture('01-loaded')
        if s['fuel']==0:
            key('d');s=capture('02-rescue-requested');assert s['fuelRescuePending']
            due=s['fuelRescueDue']
            for i in range(60):
                if not s['fuelRescuePending']:break
                time.sleep(2);s=capture(f'03-rescue-wait-{i}');assert s['fuelRescueDue']==due
            assert not s['fuelRescuePending'] and s['fuel']>0
        key('n');s=capture('04-navigation')
        for i in range(12):
            if s['navigationTarget'].split('／')[0].lower()=='lafser':break
            key('bracketright');s=capture(f'04-system-{i}')
        for i in range(4):
            if s['navigationTargetId'] in ['haiapolis','einkolt']:break
            key('Down');s=capture(f'04-planet-{i}')
        for i in range(6):
            if s['navigationTargetId']=='haiapolis':break
            key('Right');s=capture(f'04-city-{i}')
        assert s['navigationTargetId']=='haiapolis'
        key('Return');key('space');s=capture('05-departed')
        if s['screen']=='flight':key('e');key('space');s=capture('06-arrived')
        assert s['screen']=='shipboard' and s['cityId']=='haiapolis'
        key('g');key('space');s=capture('07-ground');assert s['screen']=='ground'
        key('F8');s=capture('08-grid');assert s['layout']=='remake'
        city=next(c for c in cities if slug(c['name'])=='haiapolis');depot=next(b for b in maps[city['index']]['buildings'] if b['slot']==1)
        s=walk_grid(s,(depot['row'],depot['column']),'09-depot-walk')
        key('F8');s=capture('10-atari-ground');key('f');s=capture('11-depot');assert s['screen']=='depot'
        key('2');quote=capture('12-repair-quote');assert quote['fuelPurchase']['kind']=='hull' and quote['hullCondition']<16
        shot('12-repair-quote');key('2');one=capture('13-one-pending')
        price=quote['fuelPurchase']['unitPrice'];assert price>0
        assert one['fuelPurchase']['quantity']==quote['fuelPurchase']['quantity']+1
        assert one['fuelPurchase']['funds']==quote['fuelPurchase']['funds']-price
        fields=['schema','screen','cityId','hullCondition','fuel','fuelPurchase','groundReturn','cash','bank','bankBalances','holds','shipCityId']
        one=reopen('14-pending-repair',fields);key('3');paid=capture('15-one-paid')
        assert paid['screen']=='depot' and paid['hullCondition']==quote['hullCondition']+1
        assert paid['cash']+max(0,paid['bank'])==quote['cash']+max(0,quote['bank'])-price
        key('2');fullquote=capture('16-full-quote');cost=(16-fullquote['hullCondition'])*fullquote['fuelPurchase']['unitPrice']
        key('1');key('3');full=capture('17-full-paid')
        assert full['screen']=='depot' and full['hullCondition']==16 and full['cash']+max(0,full['bank'])==fullquote['cash']+max(0,fullquote['bank'])-cost
        assert all(full[k]==quote[k] for k in ['fuel','holds','warehouses','parts','stores','colony'])
        key('2');key('1');key('2');key('3');unchanged=capture('18-full-no-charge')
        assert all(unchanged[k]==full[k] for k in ['cash','bank','hullCondition','fuel','holds'])
        shot('18-repaired');key('1');fuelquote=capture('19-fuel-quote');key('1');key('3');fueled=capture('20-refueled')
        assert fueled['screen']=='depot' and fueled['fuel']==255
        key('Escape');s=capture('21-ground');key('F8');s=capture('22-grid-return')
        ship=s['shipPosition'];s=walk_grid(s,(ship['x']+1,ship['y']),'23-return-walk');key('Left');s=capture('24-boarded')
        assert s['screen']=='shipboard'
        key('F8');s=capture('25-atari-shipboard');assert s['layout']=='atari-st'
        final_fields=['schema','screen','hullCondition','cash','bank','bankBalances','holds','warehouses','colony','shipCityId','shipPosition','cityId','parts','stores','shipSystems']
        after=reopen('26-final',final_fields);shot('26-final-reopened')
        key('q');app.wait(timeout=20);assert app.returncode==0
        write('receipt.json',dict(status='PASS',scope='正式完整版由真實戰後受損／無油保存正常救援、飛至Haiapolis、depot修一單位／待結算保存重開／買滿／已滿不扣款、補油及回船重開。地面格移動使用Apple II呈現，服務與航行為Atari ST；非原版完整GUI。',inputs=inputs,checks=checks,reentries=reentries,quote=quote,paid=paid,fullQuote=fullquote,fullPaid=full,fueled=fueled,final=after,inputSHA256=dict(binary=sha(a.binary),disk=sha(a.disk),save=sha(a.save_input)),toolSHA256=sha(out/'tool-at-start.py'),outputSaveSHA256=sha(out/'save.json'),originalGUI='unverified'))
        print('PASS 正常付費修船／兩次重開',flush=True)
    except Exception as e:
        write('failure.json',dict(status='FAIL',error=str(e),inputs=inputs,checks=checks,toolSHA256=sha(out/'tool-at-start.py')));raise
    finally:
        if app is not None and app.poll() is None:
            app.terminate()
            try:app.wait(timeout=5)
            except subprocess.TimeoutExpired:app.kill();app.wait(timeout=5)
        display.terminate();display.wait(timeout=5)


if __name__=='__main__':main()
