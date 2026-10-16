#!/usr/bin/env python3
"""合成全套HD正常操作短片；入口 docs/promo/README.md。"""
import argparse, hashlib, json, re, subprocess
from pathlib import Path

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def run(args,log=None):
    r=subprocess.run(list(map(str,args)),capture_output=True,text=True,timeout=180)
    if r.returncode:raise RuntimeError(r.stderr)
    if log:log.write_text(r.stdout+r.stderr)
    return r.stdout

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--delivery',type=Path,required=True);p.add_argument('--scenes',type=Path,required=True);a=p.parse_args()
    assert Path('/.dockerenv').is_file();root=a.delivery.resolve(strict=True);version=root.name
    assert re.fullmatch(r'v\.[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}',version)
    promo=root/'promo';output=promo/f'SunDog-{version}-HD.mp4';parts=promo/'hd-parts'
    assert not output.exists() and not parts.exists();parts.mkdir()
    scenes=json.loads(a.scenes.read_text());font=Path(__file__).resolve().parents[1]/'assets/fonts/NotoSansCJKtc-Regular.otf'
    stage=next((root/'full-local').glob('*private-full'))
    sounds=[stage/'audio'/n for n in ['04-menu_click.ogg','08-laser.ogg','23-explosion.ogg','21-teleport_up.ogg','25-hail.ogg']];assert all(s.is_file() for s in sounds)
    run(['ffmpeg','-v','error','-f','lavfi','-i','anullsrc=r=44100:cl=stereo','-t','0.35',parts/'gap.wav'])
    sound_files=[]
    for i,s in enumerate(sounds):
        wav=parts/f'sound-{i}.wav';run(['ffmpeg','-v','error','-i',s,'-ar','44100','-ac','2',wav]);sound_files += [wav,parts/'gap.wav']
    listing=parts/'sounds.txt';listing.write_text(''.join(f"file '{s}'\n" for s in sound_files))
    run(['ffmpeg','-v','error','-f','concat','-safe','0','-i',listing,parts/'sounds.wav'])
    elapsed=0;receipts=[]
    for i,scene in enumerate(scenes):
        source=Path(scene['source']).resolve(strict=True);caption=parts/f'caption-{i}.txt';caption.write_text(scene['caption'])
        footer=parts/f'footer-{i}.txt';footer.write_text(f'{version}  ｜  本機私人展示  ｜  需自備合法 Atari ST 磁碟')
        # HD uses native capture or Lanczos reduction. Captions stay outside the game.
        vf=f'scale=1280:800:flags=lanczos,pad=1280:896:0:0:color=0x101820,fps=30,drawtext=fontfile={font}:textfile={caption}:fontsize=27:fontcolor=white:x=28:y=816,drawtext=fontfile={font}:textfile={footer}:fontsize=18:fontcolor=0xb9c8d4:x=28:y=862,format=yuv420p'
        cmd=['ffmpeg','-v','error','-threads','2'];still=source.suffix.lower()=='.png'
        cmd += ['-loop','1','-i',source] if still else ['-ss',str(scene.get('sourceStart',0)),'-i',source]
        audio=scene.get('audioDemo',False)
        cmd += ['-i',parts/'sounds.wav','-af','apad'] if audio else ['-f','lavfi','-i','anullsrc=r=44100:cl=stereo']
        duration=scene['seconds'];target=parts/f'{i:02}.mp4'
        cmd += ['-map','0:v:0','-map','1:a:0','-t',str(duration),'-vf',vf,'-threads','2','-c:v','libx264','-preset','veryfast','-crf','18','-c:a','aac','-b:a','128k','-ar','44100','-ac','2',target];run(cmd)
        receipts.append(dict(start=elapsed,seconds=duration,source=str(source),sha256=sha(source),caption=scene['caption'],intentionalStill=still,audio='獨立原版音效示範，非現場同步收音' if audio else '有意靜音，未擷取現場音效'));elapsed+=duration
    concat=parts/'concat.txt';concat.write_text(''.join(f"file '{parts/f'{i:02}.mp4'}'\n" for i in range(len(scenes))))
    run(['ffmpeg','-v','error','-f','concat','-safe','0','-i',concat,'-c','copy','-movflags','+faststart',output])
    probe=json.loads(run(['ffprobe','-v','error','-show_streams','-show_format','-of','json',output]));assert {s['codec_type'] for s in probe['streams']}=={'video','audio'};assert abs(float(probe['format']['duration'])-elapsed)<.2
    (promo/'ffprobe.json').write_text(json.dumps(probe,indent=2)+'\n')
    run(['ffmpeg','-hide_banner','-i',output,'-af','volumedetect,silencedetect=noise=-50dB:d=1','-f','null','-'],promo/'audio-validation.log');assert 'max_volume: -inf' not in (promo/'audio-validation.log').read_text()
    run(['ffmpeg','-hide_banner','-i',output,'-vf','blackdetect=d=1:pix_th=.1:pic_th=.98,freezedetect=n=-55dB:d=3','-an','-f','null','-'],promo/'video-validation.log');assert 'black_start:' not in (promo/'video-validation.log').read_text()
    run(['ffmpeg','-v','error','-i',output,'-vf','fps=1/4,scale=480:336,tile=4x5:padding=3:margin=3','-frames:v','1',promo/'contact-sheet.png'])
    data=dict(version=version,status='PASS-technical-pending-visual-review',video=output.name,sha256=sha(output),seconds=elapsed,scenes=receipts,soundSources=[dict(path=str(s),sha256=sha(s)) for s in sounds],diskSHA256=sha(stage/'sundog.st'),rights='含自備原版磁碟解出的音效及原版呈現比較，只留本機，不加入Git或Release。',scope='正常GUI錄影及已驗玩家截圖。靜態展示逐段明列，非完整原版parity或人耳同步驗收。')
    (promo/'rights-and-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');print(output,elapsed,'seconds',data['sha256'])

if __name__=='__main__':main()
