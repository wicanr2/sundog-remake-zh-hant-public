#!/usr/bin/env python3
"""以正常鍵鼠及未修改的玩家保存，驗證購油失敗分支。

證據入口：docs/atari-st/ship-rules.md。只讀保存來確認操作結果，
不寫入遊戲狀態。須在專案 Docker／Xvfb 內執行。
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ('binary', 'disk', 'normal-depot', 'normal-return', 'output'):
        ap.add_argument('--' + name, required=True, type=Path)
    ap.add_argument('--resume-funds', action='store_true',
                    help='normal-return改用先前正常操作產生的不足額購油保存，只補驗末段與重開')
    args = ap.parse_args()
    if not Path('/.dockerenv').is_file():
        ap.error('只能在 Docker／Xvfb 執行')
    for path in (args.binary, args.disk, args.normal_depot, args.normal_return):
        if not path.is_file():
            ap.error('輸入檔不存在：' + str(path))
    if args.output.parent.stat().st_uid != os.getuid():
        ap.error('輸出父目錄擁有者不符')
    args.output.mkdir(exist_ok=False)
    records = []

    def state(path):
        return json.loads(path.read_text())['slots']['slot-1']

    def sha(path):
        return hashlib.sha256(path.read_bytes()).hexdigest()

    def run(argv):
        return subprocess.run(argv, check=True, stdout=subprocess.PIPE,
                              stderr=subprocess.STDOUT, text=True).stdout

    def key(name, duration=.12):
        run(['xdotool', 'keydown', name]); time.sleep(duration)
        run(['xdotool', 'keyup', name]); time.sleep(.15)

    scenarios = [('no-oil', args.normal_depot), ('capacity-and-funds', args.normal_return),
                 ('no-oil-reload', args.output / 'no-oil/library-recharge-without-oil.json'),
                 ('funds-reload', args.output / 'capacity-and-funds/library-insufficient-settled.json')]
    if args.resume_funds:
        scenarios = [('insufficient-resume', args.normal_return),
                     ('funds-reload', args.output / 'insufficient-resume/library-insufficient-settled.json'),
                     ('no-oil-reload', args.normal_depot)]
    for scenario, source in scenarios:
        p = args.output / scenario
        p.mkdir()
        library = p / 'library.json'
        shutil.copyfile(source, library)
        assert sha(library) == sha(source), 'initial save was modified'
        initial = state(source)
        assert initial['schema'] == 33
        settings = p / 'settings.json'
        settings.write_text(json.dumps({'schema': 1, 'locale': 'zh-Hant',
                                       'presentation': 'atari-st',
                                       'atariDisk': str(args.disk)}))
        with (p / 'game.log').open('w') as log:
            game = subprocess.Popen([str(args.binary), '-settings', str(settings),
                                     '-save', str(p / 'save.json'), '-library', str(library),
                                     '-locale', 'zh-Hant', '-scale', '3'],
                                    stdout=log, stderr=subprocess.STDOUT)
            try:
                for _ in range(100):
                    assert game.poll() is None, 'game exited before window'
                    found = subprocess.run(['xdotool', 'search', '--name', 'SunDog'],
                                           stdout=subprocess.PIPE, text=True)
                    if found.returncode == 0 and found.stdout.strip():
                        break
                    time.sleep(.1)
                else:
                    raise RuntimeError('window missing')
                window = found.stdout.strip().splitlines()[-1]
                run(['xdotool', 'windowfocus', window]); time.sleep(.8)
                key('Escape'); key('l'); time.sleep(.3)

                def mouse(x, y, duration=.4):
                    run(['xdotool', 'mousemove', '--window', window, str(x * 3), str(y * 3)])
                    run(['xdotool', 'mousedown', '1']); time.sleep(duration)
                    run(['xdotool', 'mouseup', '1']); time.sleep(.2)

                def save(name, image=False):
                    if image:
                        run(['import', '-window', window, str(p / (name + '-notice.png'))])
                    # 角落只關閉 delay_button 提示，沒有選購油項目。
                    run(['xdotool', 'mousemove', '--window', window, '10', '10'])
                    run(['xdotool', 'click', '1']); time.sleep(.2)
                    if image:
                        run(['import', '-window', window, str(p / (name + '.png'))])
                    before = library.stat().st_mtime_ns
                    for k in ['F5', '1', 's', 'Escape']:
                        key(k)
                    assert library.stat().st_mtime_ns != before, 'save did not change'
                    target = p / ('library-' + name + '.json')
                    shutil.copyfile(library, target)
                    s = state(target)
                    records.append({'scenario': scenario, 'name': name,
                                    'library': str(target), 'sha256': sha(target), 'state': s})
                    print(scenario, name, s['screen'], 'fuel', s['fuel'],
                          'bank/cash', s['bank'], s['cash'], flush=True)
                    return s

                def reboard(name):
                    for x, y in [(190, 101), (190, 100), (184, 100)]:
                        mouse(x, y)
                    key('Left', .6)
                    s = save(name)
                    assert s['screen'] == 'shipboard', ('reboard', s['screen'], s.get('groundView'))
                    return s

                def depot(name):
                    key('c'); key('e'); time.sleep(.3)
                    for attempt in range(3):
                        mouse(225, 95, 1.6)
                        approach = save(name + '-approach-' + str(attempt))
                        anchor = approach.get('groundView', {}).get('anchor', {})
                        assert approach['screen'] == 'ground', ('unexpected approach screen', approach['screen'])
                        if abs(anchor.get('x', -1000) - 225) <= 1 and abs(anchor.get('y', -1000) - 95) <= 1:
                            break
                    else:
                        raise RuntimeError('approach incomplete: ' + str(anchor))
                    mouse(259, 101, 1.6)
                    s = save(name)
                    assert s['screen'] == 'fuel-purchase', ('depot', s['screen'], s.get('groundView'))
                    key('3'); key('o')
                    return save(name + '-aux')

                def insufficient(s):
                    assert s['screen'] == 'fuel-purchase' and s['fuelPurchase']['kind'] == 'aux'
                    assert 0 <= s['fuelPurchase']['funds'] < s['fuelPurchase']['unitPrice']
                    key('2'); after = save('insufficient-one', image=True)
                    assert after['fuelPurchase'] == s['fuelPurchase']
                    # ActionSave會改MessageKey，購油提示以存檔前截圖檢查。
                    key('1'); after = save('insufficient-full', image=True)
                    assert after['fuelPurchase'] == s['fuelPurchase']
                    key('3'); after = save('insufficient-settled')
                    assert (after['bank'], after['cash']) == (s['bank'], s['cash'])
                    assert after['holds'] == s['holds']

                if scenario.endswith('reload'):
                    s = save('fresh-process', image=True)
                    for k in ('fuel', 'bank', 'cash', 'holds', 'groundMode', 'shipControls',
                              'weaponCharge', 'shieldLevel', 'shipPhase', 'shipSystems'):
                        assert s[k] == initial[k], ('fresh-process drift', k)
                elif scenario == 'insufficient-resume':
                    s = save('initial')
                    assert s['fuelPurchase'] == initial['fuelPurchase']
                    insufficient(s)
                elif scenario == 'no-oil':
                    s = save('initial')
                    assert s['screen'] == 'fuel-purchase' and s['fuel'] == 0
                    assert all(not h.get('kind') for h in s['holds'])
                    key('3'); reboard('returned-empty')
                    key('x'); s = save('tactical-empty')
                    assert not s['combatActive']
                    key('r'); after = save('recharge-without-oil', image=True)
                    assert after['fuel'] == 0 and all(not h.get('kind') for h in after['holds'])
                    assert after['weaponCharge'] <= s['weaponCharge']
                    assert (after['bank'], after['cash']) == (s['bank'], s['cash'])
                else:
                    assert initial['fuel'] == 255 and all(h.get('kind') == 'fuel' for h in initial['holds'])
                    s = depot('full-slots')
                    assert s['fuelPurchase']['quantity'] == 16
                    key('2'); after = save('full-slots-one', image=True)
                    assert after['fuelPurchase'] == s['fuelPurchase']
                    key('1'); after = save('full-slots-full')
                    assert after['fuelPurchase'] == s['fuelPurchase']
                    key('3'); reboard('full-slots-returned')
                    for cycle in range(1, 8):
                        key('x'); key('b'); key('Escape')
                        s = depot('cycle-' + str(cycle))
                        before = s['fuelPurchase']
                        key('1'); bought = save('cycle-' + str(cycle) + '-bought')
                        assert bought['fuelPurchase']['quantity'] - before['quantity'] == 2
                        assert before['funds'] - bought['fuelPurchase']['funds'] == 4400
                        key('3'); reboard('cycle-' + str(cycle) + '-returned')
                    key('x'); key('b'); key('Escape')
                    s = depot('insufficient')
                    insufficient(s)
                try:
                    key('q')
                except subprocess.CalledProcessError:
                    if game.poll() is None:
                        raise
                game.wait(timeout=10)
            finally:
                if game.poll() is None:
                    game.terminate()
                    try:
                        game.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        game.kill(); game.wait(timeout=5)
                (args.output / 'checkpoints.json').write_text(
                    json.dumps(records, ensure_ascii=False, indent=2) + '\n')
    print('PASS insufficient-funds continuation and two fresh-process reloads'
          if args.resume_funds else 'PASS normal no-oil, full slots, insufficient funds', flush=True)


if __name__ == '__main__':
    main()
