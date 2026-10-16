#!/usr/bin/env python3
"""Docker/Xvfb 正常耗油保存續行；入口見 ship-rules.md 救援契約。

不改寫來源保存，不注入時間／燃料；等待真實 GUI VBL，使用正式鍵鼠操作。
本工具只驗 remake 玩家鏈。原版數值另由 verify_talos_fuel_rescue.py 重生。
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time

from verify_banville_growth_player import verify_awake_resume


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ('binary','disk','settings-input','library-input','output'):
        p.add_argument('--'+name,type=Path,required=True)
    modes=p.add_mutually_exclusive_group()
    modes.add_argument('--resume-arrived',action='store_true',help='由本鏈實際抵達保存接兩種重開，不重新求救')
    modes.add_argument('--resume-pending',action='store_true',help='獨立驗證本鏈實際Pending保存，不重抽期限')
    a=p.parse_args()
    if not Path('/.dockerenv').is_file():p.error('只能在Docker執行')
    root=Path(__file__).resolve().parents[1]
    for name in ('binary','disk','settings_input','library_input'):
        value=getattr(a,name).resolve(strict=True)
        if not value.is_file():p.error('輸入須為檔案：'+name)
        setattr(a,name,value)
    initial=json.loads(a.library_input.read_text())['slots']['slot-1']
    schema=int(re.search(r'const CurrentSchema = (\d+)',(root/'internal/game/state.go').read_text())[1])
    valid_flight=(initial['screen']=='flight' and initial['fuel']==0
            and not initial['shipControls']['subc'] and bool(initial['flightTargetId'])
            and initial['fuelRescuePending']==a.resume_pending)
    valid_arrived=(initial['screen']=='shipboard' and initial['cityId']=='shoothan'
            and initial['fuel']>0 and not initial['fuelRescuePending'] and not initial['flightTargetId'])
    if (initial['schema']!=schema or initial['sleep']['active']
            or not (valid_arrived if a.resume_arrived else valid_flight)):
        p.error('須目前schema的正常耗油航段保存，不遷移或改寫')
    if json.loads(a.settings_input.read_text()).get('layout')!='atari-st':p.error('須既有Atari ST設定')
    out=a.output.resolve()
    if out.exists() or out.parent.stat().st_uid!=os.getuid():p.error('須目前使用者所屬的全新workplace輸出')
    out.mkdir()
    (out/'library.json').write_bytes(a.library_input.read_bytes())
    (out/'settings.json').write_bytes(a.settings_input.read_bytes())
    env=dict(os.environ,DISPLAY=':99',EBITENGINE_SCREENSHOT_KEY='F12')
    checks=[];inputs=[];reentries=[];wait_replays=[];process=None;window=None
    hashes={name:sha(getattr(a,name)) for name in ('binary','disk','settings_input','library_input')}
    stable=('cash','bankBalances','holds','warehouses','colony','colonySite','parts','stores','handItem','cargoAttached')
    display=subprocess.Popen(['Xvfb',':99','-screen','0','1280x800x24','-nolisten','tcp'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)

    def run(command):
        return subprocess.run(command,env=env,check=True,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT).stdout

    def key(value):
        inputs.append({'key':value})
        run(['xdotool','keydown',value]);time.sleep(.13)
        run(['xdotool','keyup',value]);time.sleep(.22)

    def click(x,y):
        inputs.append({'click':[x,y],'scale':3})
        run(['xdotool','mousemove','--window',window,str(x*3),str(y*3)])
        run(['xdotool','click','1']);time.sleep(.3)

    def start(single=False):
        nonlocal process,window
        command=[str(a.binary),'-scale','3','-start','title','-atari-disk',str(a.disk),
                 '-settings',str(out/'settings.json'),'-save',str(out/'save.json'),
                 '-library',str(out/'library.json'),'-locale','zh-Hant']
        if single:command.append('-load')
        process=subprocess.Popen(command,cwd=root,env=env,stdout=(out/f'game-{len(reentries)}.log').open('w'),stderr=subprocess.STDOUT)
        for _ in range(100):
            if process.poll() is not None:raise RuntimeError('遊戲提早退出')
            found=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL)
            if found.returncode==0 and found.stdout.strip():window=found.stdout.strip().splitlines()[-1];break
            time.sleep(.1)
        else:raise RuntimeError('找不到正常遊戲視窗')
        run(['xdotool','windowfocus',window]);time.sleep(1)
        if not single:
            for value in ('space','F5','1','l'):key(value)

    def capture(name):
        previous=(out/'library.json').stat().st_mtime_ns
        for value in ('F5','1','s','Escape'):key(value)
        raw=(out/'library.json').read_bytes()
        assert (out/'library.json').stat().st_mtime_ns!=previous,'正常保存未更新'
        (out/f'library-{name}.json').write_bytes(raw)
        state=json.loads(raw)['slots']['slot-1']
        checks.append({'name':name,'state':state,'sha256':hashlib.sha256(raw).hexdigest()})
        print(name,state['screen'],'Time',state['timeMinutes'],'Fuel',state['fuel'],'Pending',state['fuelRescuePending'],'Due',state['fuelRescueDue'],flush=True)
        return state

    def shot(name):
        before=set(root.glob('screenshot*.png'));key('F12')
        for _ in range(50):
            created=set(root.glob('screenshot*.png'))-before
            if created:break
            time.sleep(.1)
        assert len(created)==1,'正常截圖未完成'
        created.pop().rename(out/(name+'.png'))

    def preserve(state):
        for field in stable:assert state.get(field)==initial.get(field),'正常救援改變既有進度：'+field
        # Bank is the current-system cursor, not a second balance account.
        expected=initial['bank'] if state['systemId']==initial['systemId'] else initial['bankBalances'][state['systemId']]
        assert state['bank']==expected,'到站銀行游標與餘額表不同'

    def reopen(name,single=False):
        if single:
            key('q');process.wait(timeout=20)
            before=json.loads((out/'save.json').read_text())['state']
        else:
            before=json.loads((out/'library.json').read_text())['slots']['slot-1']
            process.terminate();process.wait(timeout=10)
        start(single)
        after=capture(name)
        replay=verify_awake_resume(root,out,before,after,name)
        reentries.append({'name':name,'single':single,'before':before,'after':after,'replay':replay})
        preserve(after)
        return after

    report={'status':'RUNNING','scope':'同一耗油保存的正常鍵鼠求救、真實VBL等待、重新導航與抵達；不含原版完整GUI','inputsSHA256':hashes}
    try:
        for _ in range(50):
            if subprocess.run(['xwininfo','-root'],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0:break
            time.sleep(.1)
        else:raise RuntimeError('Xvfb未就緒')
        start();state=capture('01-fuel-stop-loaded')
        reentries.append({'name':'initial-library','replay':verify_awake_resume(root,out,initial,state,'initial')})
        preserve(state)
        if a.resume_arrived:
            state=reopen('11-arrived-single-reopened',single=True)
            state=reopen('12-arrived-library-reopened')
            shot('12-arrived-reopened')
            report.update(status='PASS-normal-arrival-reentry',final=state,continuation='只接同鏈實際抵達保存，不重新求救或改寫進度')
            return
        assert state['fuel']==0 and not state['shipControls']['subc']
        shot('01-fuel-stop-loaded')
        key('d');pending=capture('02-requested')
        preserve(pending)
        assert pending['fuelRescuePending'] and pending['fuel']==0 and not pending['shipControls']['subc']
        if a.resume_pending:assert pending['fuelRescueDue']==initial['fuelRescueDue'],'Pending期限被重抽'
        else:assert 60<=pending['fuelRescueDue']-state['timeMinutes']<=112,'太空ETA超出原版邊界'
        due=pending['fuelRescueDue'];draws=pending['combatRolls']
        assert draws==state['combatRolls']+(0 if a.resume_pending else 1),'發起／Pending重按亂數消費不同'
        shot('02-pending')
        # DISTRESS is row2 in the left menu; R Resume is row3. These are
        # the existing 13-pixel WINDOWLI row geometry, not injected actions.
        click(42,21+2*13+6);state=capture('03-repeat-mouse')
        assert state['fuelRescuePending'] and state['fuelRescueDue']==due and state['combatRolls']==draws
        key('r');state=capture('04-empty-resume-rejected')
        assert state['fuel']==0 and not state['shipControls']['subc'] and state['flightTick']==initial['flightTick']
        state=reopen('05-pending-single-reopened',single=True)
        assert state['fuelRescuePending'] and state['fuelRescueDue']==due
        state=reopen('06-pending-library-reopened')
        assert state['fuelRescuePending'] and state['fuelRescueDue']==due
        deadline=time.monotonic()+180
        while state['fuelRescuePending'] and time.monotonic()<deadline:
            before=state
            time.sleep(3)
            state=capture(f'07-wait-{len(wait_replays):02d}')
            wait_replays.append(verify_awake_resume(root,out,before,state,f'wait-{len(wait_replays):02d}'))
            preserve(state)
            assert not state['shipControls']['subc'] and state['flightTick']==initial['flightTick'],'待援偷偷恢復航行'
        assert not state['fuelRescuePending'] and 0<state['fuel']<=255 and state['timeMinutes']>=due,'正常等待未得到補油'
        shot('08-refueled-engine-off')
        report['refueled']=state
        click(42,21+3*13+6);resumed=capture('09-resumed-mouse')
        preserve(resumed)
        assert resumed['shipControls']['subc'] or resumed['screen']=='shipboard','滑鼠未恢復導航'
        if resumed['screen']=='flight':key('e')
        arrived=capture('10-arrived')
        preserve(arrived)
        assert arrived['screen']=='shipboard' and arrived['cityId']==initial['flightTargetId'] and not arrived['flightTargetId'],'正常救援後未抵達'
        assert arrived['shipCityId']==arrived['cityId'] and arrived['cargoCityId']==arrived['cityId']
        shot('10-arrived')
        state=reopen('11-arrived-single-reopened',single=True)
        state=reopen('12-arrived-library-reopened')
        assert state['screen']=='shipboard' and state['cityId']=='shoothan'
        report.update(status='PASS-normal-rescue-player',requested=pending,resumed=resumed,arrived=arrived,final=state)
    except Exception as err:
        report.update(status='FAIL',error=repr(err))
        raise
    finally:
        if process is not None and process.poll() is None:
            process.terminate()
            try:process.wait(timeout=10)
            except subprocess.TimeoutExpired:process.kill();process.wait(timeout=5)
        display.terminate()
        try:display.wait(timeout=5)
        except subprocess.TimeoutExpired:display.kill();display.wait(timeout=5)
        report.update(checks=checks,inputs=inputs,reentries=reentries,waitReplays=wait_replays,toolSHA256=sha(Path(__file__).resolve()))
        assert hashes=={name:sha(getattr(a,name)) for name in hashes},'實跑時原始輸入被修改'
        (out/'receipt.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
        print(report['status'],flush=True)


if __name__=='__main__':main()
