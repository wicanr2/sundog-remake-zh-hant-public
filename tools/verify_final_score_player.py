#!/usr/bin/env python3
"""正常最終Q自動保存重開；入口banville-restoration-spec.md的最終期閘門。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

from verify_banville_growth_player import verify_awake_resume


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    p=argparse.ArgumentParser(description=__doc__)
    for key in ('binary','disk','settings-input','save-input','output'):
        p.add_argument('--'+key,type=Path,required=True)
    a=p.parse_args()
    assert Path('/.dockerenv').is_file()
    root=Path(__file__).resolve().parents[1]
    for key in ('binary','disk','settings_input','save_input'):
        setattr(a,key,getattr(a,key).resolve(strict=True))
    before=json.loads(a.save_input.read_text())['state']
    assert before['schema']==44 and before['screen']=='score' and before['missionComplete'] and before['outcomeKey']=='ui.outcome.success'
    assert before['colony']['phase']==9 and before['colony']['nextPhase']==10 and before['colony']['remaining']==[0]*32
    out=a.output.resolve();assert not out.exists() and out.parent.stat().st_uid==os.getuid();out.mkdir()
    shutil.copyfile(a.save_input,out/'save.json');shutil.copyfile(a.settings_input,out/'settings.json')
    for name in ('assets','docs'):(out/name).symlink_to(root/name,target_is_directory=True)
    env=dict(os.environ,DISPLAY=':110',EBITENGINE_SCREENSHOT_KEY='F12')
    display=subprocess.Popen(['Xvfb',':110','-screen','0','1280x800x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
    app=None;inputs=[]
    def run(*argv):return subprocess.check_output(list(map(str,argv)),env=env,text=True,timeout=15).strip()
    try:
        time.sleep(.5)
        command=[str(a.binary),'-scale','3','-start','title','-load','-atari-disk',str(a.disk),'-settings',str(out/'settings.json'),'-save',str(out/'save.json'),'-library',str(out/'library.json'),'-locale','zh-Hant']
        app=subprocess.Popen(command,cwd=out,env=env,stdout=(out/'game.log').open('w'),stderr=subprocess.STDOUT)
        for _ in range(100):
            assert app.poll() is None,'結局重開提早退出'
            r=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,text=True,capture_output=True)
            if r.returncode==0 and r.stdout.strip():window=r.stdout.strip().splitlines()[-1];break
            time.sleep(.1)
        else:raise RuntimeError('找不到結局視窗')
        run('xdotool','windowfocus',window);time.sleep(2.5)
        run('import','-window',window,out/'reopened-score.png')
        inputs.append(dict(key='q',purpose='正常Q自動保存並離開'))
        run('xdotool','keydown','--clearmodifiers', 'q');time.sleep(.1);run('xdotool','keyup','q')
        app.wait(timeout=20);assert app.returncode==0
        after=json.loads((out/'save.json').read_text())['state']
        assert after['screen']=='score' and after['missionComplete'] and after['outcomeKey']=='ui.outcome.success'
        stable=['schema','screen','missionComplete','outcomeKey','colony','holds','warehouses','cash','bank','bankBalances','jumpRange','banvilleCargo','deaths','cityId','systemId','shipCityId','shipPosition','cargoCityId','cargoPosition','fuel','tradeSession','tradeSaleSlot']
        assert all(before.get(k)==after.get(k) for k in stable),'結局重開改動核心狀態'
        clock=verify_awake_resume(root,out,before,after,'final-score')
        receipt=dict(status='PASS',scope='從實際最後交付Q自動保存正常-load重開Score，再以Q保存；沒有F5保存庫、沒有force-win或狀態注入。',command=command,inputs=inputs,stableFields=stable,before={k:before.get(k) for k in stable},after={k:after.get(k) for k in stable},clockReentry=clock,originalGUI='unverified',inputSHA256={k:sha(getattr(a,k)) for k in ('binary','disk','settings_input','save_input')},outputSaveSHA256=sha(out/'save.json'),screenshotSHA256=sha(out/'reopened-score.png'),toolSHA256=sha(Path(__file__)))
        (out/'receipt.json').write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+'\n');print(f'PASS 正常結局單檔重開／{len(stable)}核心欄位／完整VBL重播')
    finally:
        if app is not None and app.poll() is None:
            app.terminate()
            try:app.wait(timeout=5)
            except subprocess.TimeoutExpired:app.kill();app.wait(timeout=5)
        display.terminate();display.wait(timeout=5)


if __name__=='__main__':main()
