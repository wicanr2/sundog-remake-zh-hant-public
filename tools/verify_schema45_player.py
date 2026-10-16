#!/usr/bin/env python3
"""Docker 正常新局及戰鬥保存；入口 docs/atari-st/ship-rules.md。"""
import argparse
from collections import deque
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
from PIL import Image


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ('binary', 'disk', 'output'):
        p.add_argument('--'+name, type=Path, required=True)
    p.add_argument('--library-input', type=Path, help='接續本工具正常建立的新角色庫，不重抽世界種子')
    a = p.parse_args()
    assert Path('/.dockerenv').is_file()
    root = Path(__file__).resolve().parents[1]
    a.binary = a.binary.resolve(strict=True); a.disk = a.disk.resolve(strict=True)
    out = a.output.resolve()
    assert not out.exists() and out.parent.stat().st_uid == os.getuid()
    out.mkdir()
    if a.library_input:
        a.library_input = a.library_input.resolve(strict=True)
        initial = json.loads(a.library_input.read_text())['slots']['slot-1']
        assert initial['schema'] == 45 and initial['screen'] in ('shipboard', 'trade')
        assert initial['character'] == 'Schema Pilot' and not any(h.get('kind') for h in initial['holds'])
        shutil.copyfile(a.library_input, out/'library.json')
    sha = lambda f: hashlib.sha256(f.read_bytes()).hexdigest()
    for name in ('assets', 'docs'):
        (out/name).symlink_to(root/name, target_is_directory=True)
    (out/'settings.json').write_text(json.dumps(dict(schema=1, theme='atari-st-original', tileset='atari-st-hd-tiles', layout='atari-st')))
    env = dict(os.environ, DISPLAY=':133', HOME='/tmp', LIBGL_ALWAYS_SOFTWARE='1')
    app = window = None
    inputs, checks, images = [], [], []
    def write(name, data):
        (out/name).write_text(json.dumps(data, ensure_ascii=False, indent=2)+'\n')
    def run(*cmd):
        return subprocess.check_output(list(map(str, cmd)), env=env, text=True, timeout=20).strip()
    def key(value):
        inputs.append(dict(key=value))
        run('xdotool', 'keydown', '--clearmodifiers', value); time.sleep(.25)
        run('xdotool', 'keyup', value); time.sleep(.4)
    def start(load=False):
        nonlocal app, window
        cmd = [str(a.binary), '-scale', '4', '-atari-disk', str(a.disk), '-save', str(out/'save.json'), '-library', str(out/'library.json'), '-settings', str(out/'settings.json'), '-locale', 'zh-Hant']
        if load: cmd.append('-load')
        app = subprocess.Popen(cmd, cwd=out, env=env, stdout=(out/f'game-{len(checks)}.log').open('w'), stderr=subprocess.STDOUT)
        end = time.monotonic()+30
        while time.monotonic() < end:
            assert app.poll() is None, '遊戲提早退出'
            r = subprocess.run(['xdotool', 'search', '--onlyvisible', '--pid', str(app.pid)], env=env, text=True, capture_output=True)
            if r.returncode == 0 and r.stdout.strip():
                window = r.stdout.splitlines()[-1]; run('xdotool', 'windowfocus', '--sync', window); time.sleep(1); return
            time.sleep(.1)
        raise RuntimeError('遊戲視窗未就緒')
    def observe(name):
        key('F5')
        if not (out/'library.json').exists(): key('m')
        key('1'); key('s')
        raw = (out/'library.json').read_bytes(); (out/f'library-{name}.json').write_bytes(raw)
        s = json.loads(raw)['slots']['slot-1']; assert s['schema'] == 45
        checks.append(dict(name=name, state=s)); write('checks.json', checks); write('inputs.json', inputs)
        key('Escape')
        print(name, s['screen'], s.get('tradePage', 0), s['holds'], s.get('enemyCourse'), flush=True)
        return s
    def shot(name):
        f = out/(name+'.png'); run('import', '-window', window, f)
        with Image.open(f) as im: assert im.size == (1280, 800); im.verify()
        images.append(dict(path=f.name, sha256=sha(f)))
    def close():
        key('q'); assert app.wait(timeout=20) == 0
        return json.loads((out/'save.json').read_text())['state']
    def grid_walk(s, goal, label):
        # Existing verified alternate presentation provides semantic grid input.
        # Keep this boundary explicit: this is not an Atari pixel-route oracle.
        for attempt in range(3):
            if s['layout'] == 'remake' and s['tileset'] == 'apple-ii-tiles': break
            key('F8'); s = observe(label+f'-grid-{attempt}')
        assert s['layout'] == 'remake'
        city = json.loads((root/'internal/source/atarist/assets/city-maps.json').read_text())['cities'][0]
        buildings = {(b['row'], b['column']) for b in city['buildings']}
        start = (s['position']['x'], s['position']['y'])
        ship = (s['shipPosition']['x'], s['shipPosition']['y'])
        queue, prev = deque([start]), {start: None}
        while queue and goal not in prev:
            current = queue.popleft()
            for value, dx, dy in (('Up', 0, -1), ('Right', 1, 0), ('Down', 0, 1), ('Left', -1, 0)):
                x, y = current[0]+dx, current[1]+dy
                if not (-1 <= x < 30 and -1 <= y < 18): continue
                pos = (x, y)
                if pos in prev or pos == ship and pos != goal: continue
                if 0 <= x < 20 and 0 <= y < 12 and pos not in buildings and city['grid'][x][y] in (6, 7, 12, 13, 14): continue
                prev[pos] = (current, value); queue.append(pos)
        assert goal in prev
        route, cursor = [], goal
        while cursor != start:
            cursor, value = prev[cursor]; route.append(value)
        for index, value in enumerate(reversed(route)):
            key(value); s = observe(label+f'-step-{index}')
            assert s['screen'] in ('ground', 'shipboard'), '正常格移動遇到其他入口，停止而不注入狀態'
        assert s['position'] == dict(x=goal[0], y=goal[1])
        for attempt in range(3):
            if s['tileset'] == 'atari-st-hd-tiles': break
            key('F8'); s = observe(label+f'-hd-{attempt}')
        assert s['tileset'] == 'atari-st-hd-tiles'
        return s
    display = subprocess.Popen(['Xvfb', ':133', '-screen', '0', '1600x1000x24', '-nolisten', 'tcp', '-noreset'], stdout=(out/'xvfb.log').open('w'), stderr=subprocess.STDOUT)
    try:
        time.sleep(.5); start()
        if a.library_input:
            for value in ('space', 'c', '1', 'l'): key(value)
        else:
            for value in ('space', 'c', 'n'): key(value)
            run('xdotool', 'type', '--clearmodifiers', '--delay', '100', 'Schema Pilot'); inputs.append(dict(text='Schema Pilot'))
            for value in ('Return', 'Return', 'Return'): key(value)
        s = observe('01-normal-loaded'); assert s['character'] == 'Schema Pilot'
        if s['screen'] == 'shipboard':
            shot('hd-new-character')
            key('c'); key('e')
            s = observe('02-pod-exit')
            s = grid_walk(s, (15, 1), '02-to-exchange')
            key('t')
            s = observe('02-exchange-entry'); assert s['screen'] == 'trade', '正常交易入口未到，保留前綴定位路徑'
            key('e'); key('b')
            s = observe('03-buy-quote')
        assert s['tradePage'] == 1
        for attempt in range(32):
            quote = s['quotes'][s.get('quoteIndex', 0)]
            if 2 <= quote['goods'] < 31 and quote['price']*100 <= s['tradeSession']['funds']: break
            key('p'); s = observe(f'03-pass-quote-{attempt}')
        else: raise AssertionError('沒有可負擔的一般貨物，不注入或重抽新局')
        key('b'); s = observe('04-buy-destination'); assert s['tradePage'] == 10
        key('2'); s = observe('05-real-goods')
        assert any(h.get('kind') == 'goods' and 2 <= h.get('goods', 0) < 31 for h in s['holds'])
        key('Escape'); key('Escape'); s = observe('06-ground-return')
        s = grid_walk(s, (s['shipPosition']['x'], s['shipPosition']['y']), '06-to-ship')
        assert s['screen'] == 'shipboard'
        key('x'); s = observe('07-combat-entry'); assert s['screen'] == 'tactical' and s['combatActive']
        assert 1 <= s['enemyCourseMode'] <= 7, '模式0不屬持久排程驗收範圍，不重抽'
        key('u'); key('r'); s = observe('08-manual-combat'); shot('hd-combat')
        time.sleep(4); after = observe('09-background-combat')
        assert after['awakeClock']['source']['ticks'] > s['awakeClock']['source']['ticks']
        before = close(); (out/'before-reopen.json').write_bytes((out/'save.json').read_bytes())
        start(True); after = close()
        ticks = (after['awakeClock']['source']['ticks']-before['awakeClock']['source']['ticks']) % 2**32
        assert ticks < 600
        replay = '''package main
import("encoding/json";"os";"strconv";"github.com/wicanr2/sundog-remake/internal/game")
func main(){var e struct{State game.State `json:"state"`};b,err:=os.ReadFile(os.Args[1]);if err!=nil{panic(err)};if err=json.Unmarshal(b,&e);err!=nil{panic(err)};n,_:=strconv.Atoi(os.Args[2]);r,err:=game.NewReducer();if err!=nil{panic(err)};for i:=0;i<n;i++{e.State,err=r.Step(e.State,game.ActionVBLTick);if err!=nil{panic(err)}};json.NewEncoder(os.Stdout).Encode(e.State)}
'''
        (out/'replay.go').write_text(replay)
        expected = json.loads(subprocess.check_output(['go', 'run', str(out/'replay.go'), str(out/'before-reopen.json'), str(ticks)], cwd=root, env=env, text=True, timeout=120))
        fields = [k for k in before if k not in ('messageKey', 'saved', 'quitRequested')]
        assert all(after.get(k) == expected.get(k) for k in fields), [k for k in fields if after.get(k) != expected.get(k)]
        write('receipt.json', dict(status='PASS', saveSchema=45, checks=checks, inputs=inputs, images=images,
            reentry=dict(advancedVBLs=ticks, fields=fields), originalGUI='unverified',
            scope='正常標題新角色、Apple II格移動往返來源交易所、Atari ST HD真購貨／戰術初始化／手動與背景更新、F5寫庫及Q保存後新程序續行；不注入或轉換舊存檔，不宣稱Atari地面像素路徑對拍。',
            sourceSHA256=dict(binary=sha(a.binary), disk=sha(a.disk), tool=sha(Path(__file__)),
                library=sha(a.library_input) if a.library_input else None), finalSaveSHA256=sha(out/'save.json')))
        print('PASS schema45 normal new game/combat/reopen', flush=True)
    except Exception as error:
        write('failure.json', dict(status='FAIL', error=str(error), inputs=inputs, checks=checks))
        if app is not None and app.poll() is None: shot('failure')
        raise
    finally:
        if app is not None and app.poll() is None:
            app.terminate()
            try: app.wait(timeout=5)
            except subprocess.TimeoutExpired: app.kill(); app.wait(timeout=5)
        display.terminate(); display.wait(timeout=5)


if __name__ == '__main__': main()
