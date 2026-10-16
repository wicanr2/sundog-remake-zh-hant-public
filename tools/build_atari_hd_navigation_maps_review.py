#!/usr/bin/env python3
"""Extend the frozen sector-only review via Go overlays; never build a release."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    assert Path('/.dockerenv').exists(), 'Docker only'
    root = Path(__file__).resolve().parents[1]
    out = args.output.resolve()
    assert out.is_relative_to(root / 'workplace') and not out.exists()
    source = root / 'cmd/sundog-ebiten/main.go'
    main_text = source.read_text()
    start = main_text.index('func (a *app) Draw(')
    end = main_text.index('\nfunc ', start + 1)
    draw = main_text[start:end]
    needle = '\tif isMenuScreen(a.state.Screen) {'
    assert draw.count(needle) == 1
    draw = draw.replace(needle, '\tif a.drawHDNavigationReview(screen) { return }\n' + needle, 1)
    main_text = main_text[:start] + draw + main_text[end:]
    main_text = main_text.replace('ebiten.SetWindowTitle("SunDog Remake")', 'ebiten.SetWindowTitle("SunDog HD Navigation Maps Review — prototype")', 1)
    sector = root / 'tools/atari_hd_navigation_game.go.txt'
    art = sector.read_text()
    guard = ' || a.state.NavigationView != game.NavigationSector'
    assert art.count(guard) == 1
    art = art.replace(guard, '', 1)
    begin = art.index('\ttarget, hasTarget := a.reducer.World.Destination(a.state.NavigationTargetID)')
    finish = art.index('\tp.rect(141, 120, 72, 10, 0x020508)', begin)
    art = art[:begin] + '\tif a.state.NavigationView == game.NavigationSector {\n' + art[begin:finish] + '\t} else { p.drawNavigationMaps(a) }\n' + art[finish:]
    out.mkdir()
    (out / 'main.go').write_text(main_text)
    (out / 'navigation.go').write_text(art)
    maps = root / 'tools/atari_hd_navigation_maps.go.txt'
    replacement = {str(source): str(out / 'main.go'), str(root / 'cmd/sundog-ebiten/zz_hd_navigation_game.go'): str(out / 'navigation.go'), str(root / 'cmd/sundog-ebiten/zz_hd_navigation_maps.go'): str(maps)}
    overlay = out / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': replacement}, indent=2) + '\n')
    binary = out / 'sundog-hd-review'
    command = ['go', 'build', '-overlay', str(overlay), '-o', str(binary), './cmd/sundog-ebiten']
    subprocess.run(command, cwd=root, check=True, timeout=180)
    sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
    receipt = dict(status='built-navigation-maps-prototype', command=command, binarySHA256=sha(binary), sourceHashes={str(p.relative_to(root)): sha(p) for p in [source, sector, maps, Path(__file__)]}, overlayMainSHA256=sha(out/'main.go'), overlayNavigationSHA256=sha(out/'navigation.go'))
    (out / 'build-receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    print(binary)


if __name__ == '__main__':
    main()
