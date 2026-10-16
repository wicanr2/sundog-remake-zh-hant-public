#!/usr/bin/env python3
"""Docker 內以原始 SYSTEM.INTERP 核對 FTL 航距提升。入口：ftl-boost-spec.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for key in ('repo', 'talos', 'disk', 'output'):
        p.add_argument('--' + key, type=Path, required=True)
    p.add_argument('--depot-services', action='store_true', help='另驗原版depot父選單與修船完整caller')
    a = p.parse_args()
    if not Path('/.dockerenv').is_file():
        p.error('只能在 Docker 執行')
    repo, talos, disk, output = [getattr(a, k).resolve() for k in ('repo', 'talos', 'disk', 'output')]
    if output.exists() or output.parent.stat().st_uid != os.getuid():
        p.error('輸出已存在或父目錄擁有者不同')
    interp = talos / 'workplace/sundog/SYSTEM.INTERP'
    assert disk.is_file() and interp.is_file()
    output.mkdir(mode=0o700)
    names = ['combat_' + n for n in ('aim', 'cadence', 'course', 'init', 'parent', 'display', 'levels', 'movement', 'fuel')]
    names += ['fuel_depot']
    sources = [repo / ('tools/talos_' + n + '_test.go.txt') for n in names]
    overlay = output / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': {str(talos / ('internal/m68k/sundog_' + n + '_test.go')): str(s) for n, s in zip(names, sources)}}, indent=2))
    env = os.environ.copy()
    env.update(SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(output), TALOS_UCSD_INTERP=str(interp.parent), GOPROXY='off', GOTOOLCHAIN='local')
    selection = '^TestSunDog(FTLBoost|DepotMenu|DepotRepair)$' if a.depot_services else '^TestSunDogFTLBoost$'
    command = ['go', 'test', '-overlay', str(overlay), './internal/m68k', '-run', selection, '-count=1', '-v']
    with (output / 'talos.log').open('w') as log:
        result = subprocess.run(command, cwd=talos, env=env, stdout=log, stderr=subprocess.STDOUT)
    native = output / 'native-ftl-boost.json'
    report = dict(status='PASS' if result.returncode == 0 else 'FAIL', command=command,
                  go=subprocess.check_output(['go', 'version'], text=True).strip(),
                  diskSHA256=hashlib.sha256(disk.read_bytes()).hexdigest(),
                  interpSHA256=hashlib.sha256(interp.read_bytes()).hexdigest(),
                  sourceSHA256={str(s.relative_to(repo)): hashlib.sha256(s.read_bytes()).hexdigest() for s in sources + [Path(__file__).resolve()]})
    if native.is_file():
        report['original'] = json.loads(native.read_text())
    if a.depot_services:
        for name in ('depot-menu', 'depot-repair'):
            evidence = output / ('native-' + name + '.json')
            if evidence.is_file():
                report[name] = json.loads(evidence.read_text())
            else:
                report['status'] = 'FAIL'
        if report['status'] == 'PASS':
            env.update(SUNDOG_DEPOT_REPAIR_NATIVE=str(output/'native-depot-repair.json'),SUNDOG_FTL_NATIVE=str(native))
            formal = ['go','test','./internal/game','-run','^(TestDepotRepairNativeMatrix|TestFTLBoostNativeMatrix)$','-count=1','-v']
            with (output/'formal.log').open('w') as log:
                checked = subprocess.run(formal,cwd=repo,env=env,stdout=log,stderr=subprocess.STDOUT)
            report['formalCommand'] = formal
            report['formalExitCode'] = checked.returncode
            rows = report['depot-repair']['Native']
            report['formalRepairCases'] = dict(reducer=sum(row['Hull'] > 0 for row in rows), callerOnly=sum(row['Hull'] == 0 for row in rows))
            report['formalScope'] = '有效船體經正式Reducer，Hull0死亡狀態僅比較caller helper；組數由formalRepairCases按實際原生輸入計算。FTL400組經正式Reducer，非正常GUI。'
            if checked.returncode:
                report['status'] = 'FAIL'
    (output / 'receipt.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(report['status'])
    raise SystemExit(0 if report['status'] == 'PASS' else 1)


if __name__ == '__main__':
    main()
