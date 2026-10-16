#!/usr/bin/env python3
"""Docker內驗證三平台本機完整版；入口docs/release.md。"""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import struct
import subprocess
import tempfile
import time
import zipfile


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def mac_metadata(binary):
    data=binary.read_bytes();assert data[:4]==bytes.fromhex('cafebabe')
    count=struct.unpack_from('>I',data,4)[0];assert count==2
    arches=[]
    for i in range(count):
        cpu,sub,offset,size,align=struct.unpack_from('>IIIII',data,8+i*20)
        part=data[offset:offset+size];assert part[:4]==bytes.fromhex('cffaedfe')
        commands=struct.unpack_from('<I',part,16)[0];at=32;deps=[];signed=False
        for _ in range(commands):
            cmd,length=struct.unpack_from('<II',part,at);assert length>=8 and at+length<=len(part)
            if cmd==0x1d:signed=True
            if cmd in (0xc,0x80000018,0x8000001f,0x80000023):
                start=at+struct.unpack_from('<I',part,at+8)[0];end=part.index(0,start,at+length)
                name=part[start:end].decode();assert name.startswith(('/usr/lib/','/System/Library/')),name;deps.append(name)
            at+=length
        if cpu==0x100000c:assert signed,'arm64缺少ad-hoc簽章'
        arches.append(dict(cpu=cpu,architecture={0x1000007:'x86_64',0x100000c:'arm64'}[cpu],codeSignature=signed,dependencies=deps))
    assert {a['architecture'] for a in arches}=={'x86_64','arm64'}
    return arches


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--delivery',type=Path,required=True)
    p.add_argument('--platform',choices=['linux-x86_64','windows-amd64','macos-universal'],required=True)
    a=p.parse_args();assert Path('/.dockerenv').is_file()
    root=a.delivery.resolve(strict=True);version=root.name
    assert re.fullmatch(r'v\.[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}',version), '掛載目錄須保留完整版號'
    assert root.stat().st_uid==os.getuid()
    out=root/'smoke'/a.platform;assert not out.exists();out.mkdir(parents=True)
    archive=root/'full-local'/f'SunDog-{version}-{a.platform}-private-full.zip'
    assert archive.is_file()
    inputs=[];states=[];app=None;display=None
    with tempfile.TemporaryDirectory(prefix='sundog-full-package-') as temporary:
        temp=Path(temporary)
        with zipfile.ZipFile(archive) as z:
            assert z.testzip() is None
            for entry in z.infolist():
                r=Path(entry.filename);assert not r.is_absolute() and '..' not in r.parts
                assert entry.filename.isascii() or entry.flag_bits&0x800,'中文檔名缺UTF-8旗標'
            z.extractall(temp)
            for entry in z.infolist():
                f=temp/entry.filename
                if f.is_file():f.chmod((entry.external_attr>>16)&0o777 or 0o644)
        stage=next(temp.glob('*private-full'))
        manifest=stage/'MANIFEST.sha256';assert manifest.is_file()
        file_count=0
        for line in manifest.read_text().splitlines():
            expected,name=line.split('  ',1);r=Path(name.removeprefix('./'));assert not r.is_absolute() and '..' not in r.parts
            assert sha(stage/r)==expected,name;file_count+=1
        disk=stage/'sundog.st';assert sha(disk)=='f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6'
        assert len(list((stage/'audio').glob('*.ogg')))==33
        base=dict(version=version,platform=a.platform,archiveSHA256=sha(archive),manifestFileCount=file_count,diskSHA256=sha(disk),audioCount=33)
        if a.platform=='macos-universal':
            for name in ['sundog-ui-ko.json','apple2-text-ko.json','atari-st-dialog-ko.json']:
                catalog=next(stage.rglob(name));assert json.loads(catalog.read_text())['locale']=='ko'
            binary=next(stage.rglob('SunDog.app/Contents/MacOS/sundog-remake'))
            assert version.encode() in binary.read_bytes()
            launcher=stage/'Launch-SunDog.command';assert launcher.stat().st_mode&0o111
            subprocess.run(['sh','-n',str(launcher)],check=True,timeout=10)
            base.update(status='PASS-static',architectures=mac_metadata(binary),scope='完整ZIP逐檔manifest、原始磁碟、33音效、雙架構／系統相依／arm64簽章及啟動腳本靜態驗收；macOS實機未驗。')
            (out/'receipt.json').write_text(json.dumps(base,ensure_ascii=False,indent=2)+'\n');print('PASS macOS static');return
        windows=a.platform=='windows-amd64'
        env=dict(os.environ,DISPLAY=':113',HOME='/tmp')
        if windows:env.update(WINEPREFIX=str(out/'wine-prefix'),WINEDEBUG='-all',WINEDLLOVERRIDES='mscoree,mshtml=')
        else:env['XDG_CONFIG_HOME']=str(out/'config')
        empty=out/'empty-cwd';empty.mkdir()
        def run(*argv):return subprocess.check_output(list(map(str,argv)),env=env,text=True,timeout=20).strip()
        def press(key,pause=.65):
            inputs.append(dict(key=key,pause=pause,holdSeconds=.3));run('xdotool','keydown','--clearmodifiers',key);time.sleep(.3);run('xdotool','keyup',key);time.sleep(pause)
        def find_window():
            for _ in range(150):
                r=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,text=True,capture_output=True)
                if r.returncode==0 and r.stdout.strip():
                    w=r.stdout.strip().splitlines()[-1];run('xdotool','windowfocus','--sync',w);return w
                assert app.poll() is None,'正式封包提早退出';time.sleep(.15)
            raise RuntimeError('找不到正式封包視窗')
        def documents():
            basepath=out/'wine-prefix' if windows else out/'config'
            for path in basepath.rglob('*.json'):
                try:yield path,json.loads(path.read_text())
                except (UnicodeError,ValueError):continue
        def capture(name):
            # A software-rendered HD frame can outlast a 100ms key pulse.
            # Observe the library header before sending slot/store actions;
            # an old on-disk slot alone cannot prove the current scene saved.
            header=out/'library-current.png';started=time.monotonic()
            run('xdotool','keydown','--clearmodifiers','F5')
            try:
                while True:
                    assert app.poll() is None,'正式封包提早退出'
                    run('import','-window',window,header)
                    rgb=run('convert',header,'-format','%[hex:p{8,8}]','info:')[:6]
                    if min(int(rgb[i:i+2],16) for i in (0,2,4))>170:break
                    assert time.monotonic()-started<25,'F5未進入保存庫'
                    time.sleep(.1)
            finally:run('xdotool','keyup','F5')
            inputs.append(dict(key='F5',ready='observed-library-header',heldSeconds=time.monotonic()-started))
            time.sleep(.4)
            if not any('slots' in d for _,d in documents()):press('m')
            press('1');stored_at=time.time();press('s')
            libs=[(p,d) for p,d in documents() if 'slots' in d];assert len(libs)==1
            assert libs[0][0].stat().st_mtime>=stored_at, '本次未寫入保存庫'
            press('Escape')
            libs=[(p,d) for p,d in documents() if 'slots' in d];assert len(libs)==1
            path,d=libs[0];s=d['slots']['slot-1'];states.append(dict(name=name,state=s,sha256=sha(path)))
            time.sleep(2.2);run('import','-window',window,out/(name+'.png'));return s
        def command(load=False):
            flags=['-scale','3','-locale','zh-Hant']+(['-load'] if load else ['-start','title'])
            if windows:
                launch=stage/'Launch-SunDog.cmd';winpath='Z:'+str(launch).replace('/','\\')
                return ['/usr/lib/wine/wine64','cmd','/c',winpath]+flags
            return [str(stage/'啟動陽犬號.sh')]+flags
        try:
            display=subprocess.Popen(['Xvfb',':113','-screen','0','1280x800x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT);time.sleep(.7)
            if windows:
                with (out/'wineboot.log').open('w') as log:subprocess.run(['/usr/lib/wine/wine64','wineboot','-i'],env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=90)
            with (out/'game.log').open('w') as log:
                app=subprocess.Popen(command(),cwd=empty,env=env,stdout=log,stderr=subprocess.STDOUT);window=find_window();time.sleep(2)
                run('import','-window',window,out/'first-launch.png')
                press('Return',1);press('F5');press('p',1)
                initial=capture('newgame');assert initial['schema']==44 and initial['screen']=='shipboard' and initial['layout']=='atari-st'
                press('F8');hd=capture('hd-shipboard');assert hd['tileset']=='atari-st-hd-tiles' and hd['layout']=='atari-st'
                assert all(initial.get(k)==hd.get(k) for k in ['holds','cash','bank','colony','position','fuel','tradeRolls','combatRolls'])
                press('F2')
                for locale in ['zh-Hans','ja','ko','en','zh-Hant']:
                    press('Right')
                    settings=[d for _,d in documents() if d.get('schema')==1 and 'presentation' in d and 'locale' in d]
                    assert len(settings)==1 and settings[0]['locale']==locale,settings
                    run('import','-window',window,out/f'language-{locale}.png')
                press('Escape')
                press('F8');assert capture('apple-presentation')['layout']=='remake'
                press('F8');assert capture('atari-restored')['tileset']=='atari-st-tiles'
                if not windows:
                    press('n');assert capture('navigation')['screen']=='navigation';press('Escape');press('x')
                    assert capture('tactical')['screen']=='tactical'
                    press('u');assert capture('shields-on')['shieldSetting']==255
                    press('o');assert capture('shields-off')['shieldSetting']==0
                    press('Escape');press('g',1);press('space',1);ground=capture('ground');assert ground['screen']=='ground'
                    press('F8');hd=capture('hd-ground');assert hd['tileset']=='atari-st-hd-tiles' and hd['layout']=='atari-st'
                    assert all(ground.get(k)==hd.get(k) for k in ['holds','cash','bank','colony','position','fuel','tradeRolls','combatRolls'])
                    press('F8');other=capture('presentation');assert other['layout']=='remake'
                    assert all(ground.get(k)==other.get(k) for k in ['holds','cash','bank','colony','position','fuel','tradeRolls','combatRolls'])
                press('q');app.wait(timeout=20);assert app.returncode==0
                saves=[(p,d['state']) for p,d in documents() if d.get('format')=='sundog-remake-prototype-save-v44'];assert len(saves)==1
                saved_path,before=saves[0]
                app=subprocess.Popen(command(True),cwd=empty,env=env,stdout=log,stderr=subprocess.STDOUT);window=find_window();time.sleep(1)
                after=capture('reopened');fields=['schema','screen','holds','warehouses','cash','bank','colony','position','fuel','theme','tileset','layout','shipSystems']
                assert all(before.get(k)==after.get(k) for k in fields),'正式封包重開資料不同'
                press('q');app.wait(timeout=20);assert app.returncode==0 and not list(empty.iterdir())
                log.flush();assert '33' in (out/'game.log').read_text(),'沒有載入33音效的紀錄'
            base.update(status='PASS-Wine' if windows else 'PASS',inputs=inputs,states=states,savePath=str(saved_path),reopenFields=fields,emptyWorkingDirectory=True,scope='實際本平台完整ZIP乾淨解壓、正式啟動腳本、正常新局及預設位置保存重開；Linux另驗導航／戰術／護盾／地面；兩平台都驗三呈現切換。Wine不是Windows實機。')
            (out/'receipt.json').write_text(json.dumps(base,ensure_ascii=False,indent=2)+'\n');print('PASS',a.platform)
        finally:
            if app is not None and app.poll() is None:
                app.terminate()
                try:app.wait(timeout=5)
                except subprocess.TimeoutExpired:app.kill();app.wait(timeout=5)
            if windows:subprocess.run(['/usr/bin/wineserver','-k'],env=env,timeout=15)
            if display is not None:display.terminate();display.wait(timeout=5)


if __name__=='__main__':main()
