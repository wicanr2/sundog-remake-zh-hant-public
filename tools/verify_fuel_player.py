#!/usr/bin/env python3
"""schema44正常Drahew保存：購量／兩補油入口／五語HD／新程序重開。

入口：docs/atari-st/ship-rules.md。只讀保存觀測狀態，不注入遊戲資料。
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
from PIL import Image


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    for name in ('binary', 'disk', 'library-input', 'settings-input', 'output'):
        ap.add_argument('--' + name, type=Path, required=True)
    ap.add_argument('--mode', choices=('pod', 'energy'), required=True)
    ap.add_argument('--review-only', action='store_true', help='從正常購量保存只重拍五語，取消提示，不購買或耗油')
    ap.add_argument('--resume-energy', action='store_true', help='從正常戰術耗油保存接續，保留既有購買前綴')
    ap.add_argument('--resume-depot', action='store_true', help='從正常載具近depot保存接續，不重走已完成路線')
    a = ap.parse_args()
    assert Path('/.dockerenv').is_file(), 'Docker required'
    sha = lambda p: hashlib.sha256(Path(p).read_bytes()).hexdigest()
    for p in (a.binary, a.disk, a.library_input, a.settings_input):
        assert p.is_file(), p
    source_hashes = {k:sha(v) for k,v in dict(binary=a.binary, disk=a.disk, library=a.library_input, settings=a.settings_input).items()}
    initial = json.loads(a.library_input.read_text())['slots']['slot-1']
    assert initial['schema'] == 44 and initial['cityId'] == 'drahew'
    if a.review_only:
        assert (initial['screen'] == 'fuel-purchase' and initial['fuelPurchase']['kind'] in ('main', 'aux')) or (initial['screen'] == 'tactical' and sum(h.get('kind') == 'fuel' for h in initial['holds']) == 1)
    elif a.resume_energy:
        assert a.mode == 'energy' and initial['screen'] == 'tactical' and 0 < initial['fuel'] <= 16
        assert all(h.get('kind') == 'fuel' for h in initial['holds']) and initial['cargoAttached']
    elif a.resume_depot:
        assert initial['screen'] == 'ground' and initial['groundMode'] == 'pod' and initial['fuel'] == 0
        assert all(not h.get('kind') for h in initial['holds']) and not initial['cargoAttached']
    else:
        assert initial['screen'] == 'shipboard' and initial['fuel'] == 0
        assert all(not h.get('kind') for h in initial['holds'])
    out = a.output.resolve()
    assert not out.exists() and out.parent.stat().st_uid == os.getuid()
    out.mkdir()
    root = Path(__file__).resolve().parents[1]
    for name in ('assets', 'docs'):
        (out / name).symlink_to(root / name, target_is_directory=True)
    shutil.copyfile(a.library_input, out / 'library.json')
    shutil.copyfile(a.settings_input, out / 'settings.json')
    shutil.copyfile(__file__, out / 'tool-source.py')
    checks, inputs, images, reentries = [], [], [], []
    env = dict(os.environ, DISPLAY=':94', HOME='/tmp', LIBGL_ALWAYS_SOFTWARE='1', EBITENGINE_SCREENSHOT_KEY='F12')
    display = subprocess.Popen(['Xvfb', ':94', '-screen', '0', '1600x1000x24', '-nolisten', 'tcp', '-noreset'], stdout=(out / 'xvfb.log').open('w'), stderr=subprocess.STDOUT)
    app = window = None
    stable = ['schema', 'cityId', 'systemId', 'planetId', 'holds', 'warehouses', 'cash', 'bank', 'bankBalances', 'groundMode', 'cargoAttached', 'cargoCityId', 'cargoPosition', 'shipCityId', 'shipPosition', 'parts', 'stores', 'colony']
    def write(name, data):
        (out / name).write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
    def run(*cmd):
        return subprocess.check_output(list(map(str, cmd)), env=env, text=True, stderr=subprocess.STDOUT, timeout=20).strip()
    def key(value, duration=.25):
        inputs.append(dict(key=value, holdSeconds=duration))
        run('xdotool', 'keydown', '--clearmodifiers', value)
        try:
            time.sleep(duration)
        finally:
            run('xdotool', 'keyup', value)
        time.sleep(.25)
        assert app.poll() is None
    def start(load=False):
        nonlocal app, window
        cmd = [str(a.binary.resolve()), '-scale', '4', '-atari-disk', str(a.disk.resolve()), '-save', str(out / 'save.json'), '-library', str(out / 'library.json'), '-settings', str(out / 'settings.json'), '-locale', 'zh-Hant']
        if load:
            cmd.append('-load')
        app = subprocess.Popen(cmd, cwd=out, env=env, stdout=(out / f'game-{len(reentries)}.log').open('a'), stderr=subprocess.STDOUT)
        end = time.monotonic() + 25
        while time.monotonic() < end:
            assert app.poll() is None, 'early game exit'
            r = subprocess.run(['xdotool', 'search', '--onlyvisible', '--pid', str(app.pid)], env=env, text=True, capture_output=True)
            if not r.returncode:
                window = r.stdout.splitlines()[-1]
                run('xdotool', 'windowfocus', '--sync', window)
                time.sleep(1)
                return
            time.sleep(.1)
        raise RuntimeError('normal window missing')
    def capture(name):
        before = (out / 'library.json').stat().st_mtime_ns
        for value in ('F5', '1', 's'):
            key(value)
        end = time.monotonic() + 15
        while time.monotonic() < end and (out / 'library.json').stat().st_mtime_ns == before:
            time.sleep(.1)
        assert (out / 'library.json').stat().st_mtime_ns != before, 'F5 library write missing'
        raw = (out / 'library.json').read_bytes()
        (out / f'library-{name}.json').write_bytes(raw)
        state = json.loads(raw)['slots']['slot-1']
        checks.append(dict(name=name, state=state, librarySHA256=sha(out / 'library.json')))
        write('checks.json', checks)
        key('Escape')
        print(name, state['screen'], 'fuel', state['fuel'], 'tanks', sum(h.get('kind') == 'fuel' for h in state['holds']), flush=True)
        return state
    def shot(name):
        # F5's normal Escape leaves a back notice. Dismiss it with a real
        # outside click, away from every purchase option, before capturing.
        inputs.append(dict(click=[4, 4], scale=4, purpose='dismiss back notice'))
        run('xdotool', 'mousemove', '--window', window, 16, 16)
        run('xdotool', 'click', '1')
        time.sleep(.3)
        before = set(out.glob('screenshot*.png'))
        key('F12')
        end = time.monotonic() + 15
        while time.monotonic() < end and not set(out.glob('screenshot*.png')) - before:
            time.sleep(.1)
        created = set(out.glob('screenshot*.png')) - before
        assert len(created) == 1
        source = next(iter(created))
        end = time.monotonic() + 15
        while True:
            try:
                with Image.open(source) as im:
                    assert im.size == (1280, 800)
                    im.verify()
                break
            except (OSError, SyntaxError):
                if time.monotonic() >= end:
                    raise
                time.sleep(.1)
        target = out / (name + '.png')
        created.pop().rename(target)
        images.append(dict(path=target.name, sha256=sha(target)))
    def reopen(name, exact=True):
        inputs.append(dict(key='q', purpose='real autosave and process exit'))
        run('xdotool', 'keydown', '--clearmodifiers', 'q')
        time.sleep(.25)
        run('xdotool', 'keyup', 'q')
        assert app.wait(timeout=20) == 0
        before = json.loads((out / 'save.json').read_text())['state']
        start(True)
        after = capture(name)
        fields = stable + (['screen', 'fuel', 'fuelPurchase', 'groundReturn', 'shipPhase', 'shipControls'] if exact else [])
        assert all(before.get(k) == after.get(k) for k in fields), 'reopen changed saved fields'
        reentries.append(dict(name=name, fields=fields, before={k: before.get(k) for k in fields}, after={k: after.get(k) for k in fields}, dynamicFuel=dict(before=before['fuel'], after=after['fuel']), exactEnergy=exact))
        return after
    def move(x, y, duration=1.5):
        inputs.append(dict(mouse=[x, y], scale=4, holdSeconds=duration))
        run('xdotool', 'mousemove', '--window', window, x * 4, y * 4)
        run('xdotool', 'mousedown', '1')
        try:
            time.sleep(duration)
        finally:
            run('xdotool', 'mouseup', '1')
        time.sleep(.25)
    def locale_next(expected):
        key('F2')
        run('xdotool', 'keydown', '--clearmodifiers', 'Right')
        try:
            end = time.monotonic() + 15
            while time.monotonic() < end:
                if json.loads((out / 'settings.json').read_text()).get('locale') == expected:
                    break
                time.sleep(.1)
            else:
                raise RuntimeError('locale did not change: ' + expected)
        finally:
            run('xdotool', 'keyup', 'Right')
        key('Escape')
    def languages(kind, baseline):
        for locale in ('zh-Hant', 'zh-Hans', 'ja', 'ko', 'en'):
            if locale != 'zh-Hant':
                locale_next(locale)
            s = capture(f'{kind}-{locale}')
            assert s['fuelPurchase'] == baseline['fuelPurchase'] and s['holds'] == baseline['holds']
            shot(f'hd-{kind}-{locale}')
        locale_next('zh-Hant')
    def drain_energy(s):
        before_holds = s['holds']
        # Observe the actual virtual-time progress. F5 pauses gameplay, and
        # software HD rendering can run below 60 FPS in a bounded container.
        end = time.monotonic() + 240
        while time.monotonic() < end and sum(h.get('kind') == 'fuel' for h in s['holds']) == 2:
            time.sleep(5)
            s = capture(f'20-energy-gate-{len(checks)}')
        assert sum(h.get('kind') == 'fuel' for h in s['holds']) == 1 and s['fuel'] > 200
        assert not s['holds'][0].get('kind') and s['holds'][1]['kind'] == 'fuel'
        assert s['holds'][1] == before_holds[1]
        assert all(s['holds'][0].get(k, 0) == before_holds[0].get(k, 0) for k in ('quality', 'sourceCode', 'price'))
        return s
    try:
        start()
        for value in ('space', 'F5', '1', 'l'):
            key(value)
        s = capture('01-normal-loaded')
        if s['tileset'] != 'atari-st-hd-tiles':
            key('F8')
            s = capture('02-normal-hd')
        assert s['tileset'] == 'atari-st-hd-tiles'
        if a.review_only:
            if s['screen'] == 'tactical':
                shot('hd-energy-refill-reopened')
                scope = 'normal existing post-refill tactical save, F5 load, complete HD PNG capture; no new fuel-gate claim'
            else:
                languages(s['fuelPurchase']['kind'], s)
                scope = 'normal existing purchase save, F5 load, five-language HD purchase fields, dismiss back notice; no purchase or energy use in this review'
            write('receipt.json', dict(status='PASS', mode=a.mode, scope=scope, checks=checks, inputs=inputs, images=images, reentries=reentries, saveSchema=44, originalGUI='unverified', sourceSHA256={k:sha(v) for k,v in dict(binary=a.binary,disk=a.disk,library=a.library_input,settings=a.settings_input).items()}, toolSHA256=sha(out/'tool-source.py')))
            print('PASS normal five-language purchase review', flush=True)
            return
        if a.resume_energy:
            s = drain_energy(s)
            after = reopen('21-energy-refill-reopened', False)
            assert after['holds'] == s['holds']
            shot('hd-energy-refill-reopened')
            write('receipt.json', dict(status='PASS', mode='energy-resume', scope='continue unmodified normal tactical save after real depot purchases; actual attached <=0 fuel consumer and fresh-process persistent-field reentry; no replay of already completed purchases', checks=checks, inputs=inputs, images=images, reentries=reentries, saveSchema=44, originalGUI='unverified', sourceSHA256={k:sha(v) for k,v in dict(binary=a.binary,disk=a.disk,library=a.library_input,settings=a.settings_input).items()}, toolSHA256=sha(out/'tool-source.py'), limitations='Active energy, phase and time continue on reopen; only listed persistent fields compared exactly.'))
            print('PASS normal continued energy gate', flush=True)
            return
        if not a.resume_depot:
            key('c'); key('e')
            s = capture('03-normal-pod-exit')
            assert s['screen'] == 'ground' and s['groundMode'] == 'pod' and not s['cargoAttached']
            for attempt in range(3):
                move(225, 95)
                s = capture(f'04-depot-approach-{attempt}')
                anchor = s.get('groundView', {}).get('anchor', {})
                if s['screen'] == 'ground' and abs(anchor.get('x', -999) - 225) <= 1 and abs(anchor.get('y', -999) - 95) <= 1:
                    break
            else:
                raise RuntimeError('normal depot approach did not reach waypoint')
        for attempt in range(5):
            move(259, 101)
            s = capture(f'05-normal-depot-{attempt}')
            if s['screen'] == 'depot':
                break
        assert s['screen'] == 'depot' and s['fuel'] == 0
        if a.mode == 'energy':
            key('1'); quote = capture('06-main-quote')
            assert quote['fuelPurchase']['kind'] == 'main' and quote['fuelPurchase']['unitPrice'] > 0
            languages('main', quote)
            key('2'); pending = capture('07-main-one-pending')
            assert pending['fuelPurchase']['quantity'] == quote['fuelPurchase']['quantity'] + 1
            assert pending['fuelPurchase']['funds'] == quote['fuelPurchase']['funds'] - quote['fuelPurchase']['unitPrice']
            reopen('08-main-pending-reopened')
            key('3'); s = capture('09-main-settled')
            assert s['fuel'] == 16
        key('3'); quote = capture('10-aux-quote')
        assert quote['fuelPurchase']['kind'] == 'aux' and quote['fuelPurchase']['unitPrice'] > 0
        if a.mode == 'energy':
            languages('aux', quote)
        key('2'); pending = capture('11-aux-one-pending')
        assert pending['fuelPurchase']['quantity'] == quote['fuelPurchase']['quantity'] + 1
        reopen('12-aux-pending-reopened')
        key('1'); full = capture('13-aux-full-pending')
        assert full['fuelPurchase']['quantity'] == 16
        key('3'); s = capture('14-aux-settled')
        assert all(h.get('kind') == 'fuel' and h['quality'] == 7 and h['sourceCode'] == quote['fuelPurchase']['sourceCode'] for h in s['holds'])
        purchased_holds = s['holds']
        assert s['cash'] + max(0, s['bank']) == quote['cash'] + max(0, quote['bank']) - 2 * quote['fuelPurchase']['unitPrice']
        key('3'); fullquote = capture('15-full-slots-quote')
        key('1'); key('2'); unchanged = capture('16-full-slots-no-charge')
        assert unchanged['fuelPurchase'] == fullquote['fuelPurchase']
        key('3'); key('Escape')
        s = capture('17-normal-ground-return')
        for x, y in ((190, 101), (190, 100), (184, 100)):
            move(x, y, .5)
        key('Left', .6)
        s = capture('18-normal-boarded')
        assert s['screen'] == 'shipboard' and s['cargoAttached'] and s['groundMode'] == 'foot'
        if a.mode == 'pod':
            assert sum(h.get('kind') == 'fuel' for h in s['holds']) == 1 and s['fuel'] > 240
            assert s['holds'][1] == purchased_holds[1]
            assert all(s['holds'][0].get(k, 0) == purchased_holds[0].get(k, 0) for k in ('quality', 'sourceCode', 'price'))
            after = reopen('19-pod-refill-reopened', False)
            shot('hd-pod-refill-reopened')
        else:
            assert all(h.get('kind') == 'fuel' for h in s['holds']) and 0 < s['fuel'] <= 16
            key('x'); key('u'); key('r')
            first = capture('19-energy-draining')
            s = drain_energy(first)
            after = reopen('21-energy-refill-reopened', False)
            shot('hd-energy-refill-reopened')
        assert after['holds'] == s['holds']
        write('receipt.json', dict(status='PASS', mode=a.mode, scope=('schema44 normal ground route continuation' if a.resume_depot else 'schema44 normal saved-game load and pod exit') + ', depot, real purchases, pending reopens, full-slot refusal, normal reboarding and auxiliary fuel gate; distinct pod/energy branches', checks=checks, inputs=inputs, images=images, reentries=reentries, saveSchema=44, originalGUI='unverified', sourceSHA256={k:sha(v) for k,v in dict(binary=a.binary,disk=a.disk,library=a.library_input,settings=a.settings_input).items()}, toolSHA256=sha(out/'tool-source.py'), limitations='No state injection. Reopens during active ship callbacks compare persistent fields; fuel/phase/time keep running and are not claimed exact without clock replay. Five-language main/aux review only in energy branch.'))
        print('PASS schema44 normal fuel branch', a.mode, flush=True)
    except Exception as error:
        write('failure.json', dict(status='FAIL', error=str(error), checks=checks, inputs=inputs, images=images, reentries=reentries, sourceSHA256=source_hashes, toolSHA256=sha(out/'tool-source.py')))
        raise
    finally:
        if app and app.poll() is None:
            app.terminate()
            try:
                app.wait(timeout=10)
            except subprocess.TimeoutExpired:
                app.kill(); app.wait(timeout=5)
        display.terminate()
        try:
            display.wait(timeout=5)
        except subprocess.TimeoutExpired:
            display.kill(); display.wait(timeout=5)


if __name__ == '__main__':
    main()
