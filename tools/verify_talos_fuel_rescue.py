"""Docker內重生#17救援producer；入口見ship-rules.md，不以局部切片稱GUI parity。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--disk',type=Path,required=True)
    p.add_argument('--talos',type=Path,required=True)
    p.add_argument('--output',type=Path,required=True)
    a=p.parse_args()
    if not Path('/.dockerenv').is_file():p.error('只能在Docker執行')
    root=Path(__file__).resolve().parents[1];out=a.output.resolve();talos=a.talos.resolve();disk=a.disk.resolve(strict=True)
    interp=talos/'workplace/sundog/SYSTEM.INTERP'
    if not interp.is_file() or not disk.is_file():p.error('原始輸入缺漏')
    if out.exists() or out.parent.stat().st_uid!=os.getuid():p.error('須使用目前使用者所屬的全新輸出目錄')
    assert sha(disk)=='f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6'
    assert sha(interp)=='a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38'
    out.mkdir()
    names=['combat_'+n for n in ['aim','cadence','course','init','parent','display','levels','movement','fuel']]
    names+=['fuel_depot','fuel_entry','walker_slices','banville','banville_price','banville_site','trade_classes','food','flight_clock','fuel_rescue']
    sources=[root/f'tools/talos_{n}_test.go.txt' for n in names]
    replace={str(talos/f'internal/m68k/sundog_{n}_test.go'):str(s) for n,s in zip(names,sources)}
    overlay=out/'overlay.json';overlay.write_text(json.dumps({'Replace':replace},indent=2)+'\n')
    env=dict(os.environ,SUNDOG_PARITY_DISK=str(disk),SUNDOG_PARITY_OUTPUT=str(out),TALOS_UCSD_INTERP=str(interp.parent),GOPROXY='off',GOTOOLCHAIN='local')
    formal_cmd=['go','test','./internal/game','-run','^TestExportRescueResume$','-count=1','-v']
    formal_status=subprocess.run(formal_cmd,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
    (out/'formal-inputs.log').write_text(formal_status.stdout)
    if formal_status.returncode:raise RuntimeError(formal_status.stdout)
    cmd=['go','test','-overlay',str(overlay),'./internal/m68k','-run','^TestSunDogFuelRescue(Producer|Resume)$','-count=1','-v']
    with (out/'talos.log').open('w') as log:
        status=subprocess.run(cmd,cwd=talos,env=env,stdout=log,stderr=subprocess.STDOUT).returncode
    report=dict(status='FAIL' if status else 'PASS-native-rescue-producer',issue=17,command=cmd,
        diskSHA256=sha(disk),interpSHA256=sha(interp),sourceSHA256={str(s.relative_to(root)):sha(s) for s in sources+[Path(__file__).resolve()]},
        go=subprocess.check_output(['go','version'],text=True).strip(),
        talosHEAD=subprocess.check_output(['git','-C',str(talos),'rev-parse','HEAD'],text=True).strip())
    native=out/'native-fuel-rescue.json'
    if not status:
        r=json.loads(native.read_text());assert len(r['Gates'])==48 and len(r['SpaceGates'])==6 and len(r['Rows'])==112
        report.update(native=r,nativeSHA256=sha(native),gateCases=48,spaceGateCases=6,producerCases=112)
        comparison_cmd=['go','test','./internal/game','-run','^TestExportFuelRescue$','-count=1','-v']
        checked=subprocess.run(comparison_cmd,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        (out/'formal-comparison.log').write_text(checked.stdout)
        status=checked.returncode
        if not status:
            comparison=json.loads((out/'remake-fuel-rescue.json').read_text())
            native_resume=json.loads((out/'native-rescue-resume.json').read_text())
            formal_resume=json.loads((out/'remake-rescue-resume.json').read_text())
            assert len(native_resume)==len(formal_resume)==16
            differences=[]
            for n,f in zip(native_resume,formal_resume):
                for key in ('Input','TimeAfter','Tick','Ticks','DeltaAngle','DeltaRadius','Travel'):
                    if n[key]!=f[key]:differences.append([n['Input'],key,n[key],f[key]])
                assert f['Fuel']==255
            report.update(status='PASS-rescue-same-state' if not differences else 'FAIL-resume',
                comparison=comparison,nativeResume=native_resume,formalResume=formal_resume,resumeDifferences=differences,
                formalCommands=[formal_cmd,comparison_cmd],baseline=json.loads((root/'docs/atari-st/fuel-rescue-parity-20261005.json').read_text())['baseline'])
            status=int(bool(differences))
        else:report['status']='FAIL-formal-producer'
    report['sourceSHA256'].update({str(s.relative_to(root)):sha(s) for s in [root/'internal/game/distress.go',root/'internal/game/reducer.go',root/'internal/game/state.go',root/'internal/game/fuel_rescue_native_parity_test.go']})
    (out/'receipt.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(report['status'],flush=True)
    raise SystemExit(status)


if __name__=='__main__':main()
