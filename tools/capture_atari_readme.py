"""Docker-only README captures through a normal Atari ST new-game path."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ('binary', 'disk', 'output'):
        p.add_argument('--' + name, type=Path, required=True)
    p.add_argument('--normal-library', type=Path, help='只複製正常地面保存庫，經標題選局UI載入後拍攝')
    args = p.parse_args()
    assert Path('/.dockerenv').exists()
    root = Path(__file__).resolve().parents[1]
    out = args.output.resolve()
    assert not out.exists() and out.parent.stat().st_uid == os.getuid()
    out.mkdir()
    if args.normal_library:
        (out / 'library.json').write_bytes(args.normal_library.read_bytes())
    settings = out / 'settings.json'
    settings.write_text(json.dumps(dict(schema=1, locale='zh-Hant', presentation='atari-st',
                                        theme='atari-st-original', tileset='atari-st-tiles', layout='atari-st')))
    env = dict(os.environ, DISPLAY=':94')
    sha = lambda f: hashlib.sha256(f.read_bytes()).hexdigest()
    inputs, checks, pictures = [], [], []
    game = None
    xvfb = subprocess.Popen(['Xvfb', ':94', '-screen', '0', '1280x800x24', '-nolisten', 'tcp', '-noreset'],
                            stdout=(out / 'xvfb.log').open('w'), stderr=subprocess.STDOUT)
    def run(*argv):
        return subprocess.check_output(list(map(str, argv)), env=env, text=True, timeout=15).strip()
    def key(k, pause=.25):
        inputs.append(dict(key=k, pause=pause))
        run('xdotool', 'keydown', '--clearmodifiers', k)
        time.sleep(.08)
        run('xdotool', 'keyup', '--clearmodifiers', k)
        time.sleep(pause)
    def shot(name):
        path = out / (name + '.png')
        run('import', '-window', window, path)
        pictures.append(dict(name=name, path=str(path), sha256=sha(path)))
    def observe(name, want):
        library = out / 'library.json'
        stamp = library.stat().st_mtime_ns if library.exists() else 0
        key('F5')
        if not library.exists(): key('m')
        for k in ('1', 's', 'Escape'): key(k)
        assert library.stat().st_mtime_ns != stamp, '正常保存庫未更新'
        s = json.loads(library.read_text())['slots']['slot-1']
        assert s['screen'] == want, (name, s['screen'], want)
        assert s['schema'] == 44 and s['layout'] == 'atari-st'
        (out / ('library-' + name + '.json')).write_bytes(library.read_bytes())
        checks.append(dict(name=name, state=s, sha256=sha(library)))
        time.sleep(2.3)
        shot(name)
        return s
    try:
        for _ in range(60):
            assert xvfb.poll() is None, 'Xvfb已退出'
            if subprocess.run(['xwininfo', '-root'], env=env, stdout=subprocess.DEVNULL,
                              stderr=subprocess.DEVNULL).returncode == 0: break
            time.sleep(.1)
        else: raise RuntimeError('Xvfb未就緒')
        argv = [str(args.binary), '-scale', '3', '-start', 'title', '-locale', 'zh-Hant',
                '-atari-disk', str(args.disk), '-settings', str(settings),
                '-save', str(out / 'save.json'), '-library', str(out / 'library.json')]
        game = subprocess.Popen(argv, cwd=root, env=env, stdout=(out / 'game.log').open('w'), stderr=subprocess.STDOUT)
        for _ in range(100):
            assert game.poll() is None, '遊戲提早退出'
            found = subprocess.run(['xdotool', 'search', '--onlyvisible', '--class', 'ebitengine-application'],
                                   env=env, text=True, capture_output=True)
            if found.returncode == 0 and found.stdout.strip():
                window = found.stdout.splitlines()[-1]
                run('xdotool', 'windowfocus', '--sync', window)
                break
            time.sleep(.1)
        else: raise RuntimeError('找不到遊戲視窗')
        time.sleep(1)
        shot('title')
        if args.normal_library:
            for k in ('space', 'c', '1', 'l'): key(k)
            observe('ground', 'ground')
            run('xdotool', 'keydown', '--clearmodifiers', 'q')
            game.wait(timeout=10)
            run('xdotool', 'keyup', 'q')
            assert game.returncode == 0
            report = dict(status='PASS', scope='目前程式經正常標題／保存庫UI載入真實地面檢查點；無狀態或座標注入。',
                          binarySHA256=sha(args.binary), diskSHA256=sha(args.disk),
                          initialLibrary=str(args.normal_library), initialLibrarySHA256=sha(args.normal_library),
                          inputs=inputs, checks=checks, pictures=pictures, saveSchema=44)
            (out / 'receipt.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
            print('PASS: current Atari ST normal-library ground capture', flush=True)
            return
        for k in ('space', 'c', 'n', 'z', 'e', 'd', 'Return', 'Return', 'Return'): key(k)
        initial = observe('shipboard', 'shipboard')
        key('e')
        observe('engineering', 'engineering')
        key('Escape')
        key('n')
        observe('navigation', 'navigation')
        key('Escape')
        key('x')
        tactical = observe('tactical', 'tactical')
        assert not tactical['combatActive'], '新局空船不應補造敵艦'
        key('Escape')
        key('g', 1)
        key('space', 1)
        ground = observe('ground', 'ground')
        key('F8')
        key('F8')
        returned = observe('ground-after-presentation-cycle', 'ground')
        for k in ('cash', 'fuel', 'holds', 'shipPosition', 'shipCityId', 'position', 'colony', 'parts'):
            assert ground[k] == returned[k], ('呈現輪替改變遊戲欄位', k)
        key('F2')
        time.sleep(.3)
        shot('options')
        key('Escape')
        run('xdotool', 'keydown', '--clearmodifiers', 'q')
        game.wait(timeout=10)
        run('xdotool', 'keyup', 'q')
        assert game.returncode == 0
        report = dict(status='PASS', scope='目前正式程式的Atari ST正常新局、工程／導航／空船戰術／地面與兩預設輪替。沒有原版GUI同狀態或完整航程／勝敗聲明。',
                      binarySHA256=sha(args.binary), diskSHA256=sha(args.disk),
                      inputs=inputs, checks=checks, pictures=pictures,
                      originalFontRequired=False, saveSchema=initial['schema'])
        (out / 'receipt.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        print('PASS: Atari ST normal new-game README captures', flush=True)
    except Exception as e:
        (out / 'failure.json').write_text(json.dumps(dict(status='FAIL', error=str(e), inputs=inputs, checks=checks), ensure_ascii=False, indent=2))
        raise
    finally:
        if game is not None and game.poll() is None:
            game.terminate()
            try: game.wait(timeout=5)
            except subprocess.TimeoutExpired: game.kill(); game.wait(timeout=5)
        xvfb.terminate()
        try: xvfb.wait(timeout=5)
        except subprocess.TimeoutExpired: xvfb.kill(); xvfb.wait(timeout=5)


if __name__ == '__main__':
    main()
