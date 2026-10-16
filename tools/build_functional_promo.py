#!/usr/bin/env python3
"""合成機制抽樣版短片；入口及素材邊界見 docs/promo/README.md。"""
import argparse
import hashlib
import json
import re
from pathlib import Path
import subprocess


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(args, log=None):
    result = subprocess.run(list(map(str, args)), capture_output=True, text=True, timeout=180)
    if result.returncode:
        raise RuntimeError(f'命令失敗 {args}:\n{result.stderr}')
    if log:
        log.write_text(result.stdout + result.stderr)
    return result.stdout


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--root', type=Path, required=True)
    p.add_argument('--version', required=True)
    p.add_argument('--battle-shot', type=Path, required=True)
    p.add_argument('--formal-v1', action='store_true', help='正式v1分鏡，使用本版實際錄影及已驗玩家圖片')
    p.add_argument('--repair-shot', type=Path)
    p.add_argument('--score-shot', type=Path)
    args = p.parse_args()
    assert Path('/.dockerenv').exists(), '只能在Docker執行'
    if not re.fullmatch(r'v\.[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}', args.version):
        p.error('版號不符合 v.<主>.<次>.<修訂>-YYYYMMDD')
    root = args.root.resolve()
    assert root.name == args.version
    promo = root / 'promo'
    output = promo / f'SunDog-{args.version}-mechanisms.mp4'
    assert not output.exists(), '正式成片禁止覆寫'
    parts = promo / 'functional-parts'
    parts.mkdir(exist_ok=True)
    assert not any(parts.iterdir()), '中間產物禁止覆寫'
    stage = next((root / 'full-local').glob('*private-full'))
    audio = stage / 'audio'
    font = Path(__file__).resolve().parents[1] / 'assets/fonts/NotoSansCJKtc-Regular.otf'
    source = promo / 'remake/clips/seg1.mp4'
    clips = promo / ('remake/clips' if args.formal_v1 else 'remake-r2/clips')
    cabin = promo / 'remake-r3/clips/seg14.mp4'
    sources = [source, clips/'seg13.mp4', args.battle_shot]
    if args.formal_v1:
        assert args.repair_shot and args.score_shot
        sources += [clips/'seg6.mp4',args.repair_shot,args.score_shot]
    else:
        sources.append(cabin)
    assert all(s.is_file() for s in sources)
    sound_names = ['04-menu_click.ogg', '08-laser.ogg', '23-explosion.ogg', '21-teleport_up.ogg', '25-hail.ogg']
    sounds = [audio / name for name in sound_names]
    assert all(s.is_file() for s in sounds)
    run(['ffmpeg','-v','error','-f','lavfi','-i','anullsrc=r=44100:cl=stereo','-t','0.4',parts/'gap.wav'])
    sound_list = parts / 'sound-list.txt'
    normalized = []
    for i,s in enumerate(sounds):
        wav = parts / f'sound-{i}.wav'
        run(['ffmpeg','-v','error','-i',s,'-ar','44100','-ac','2','-c:a','pcm_s16le',wav])
        normalized.append(wav)
    sound_list.write_text(''.join(f"file '{s}'\nfile '{parts/'gap.wav'}'\n" for s in normalized))
    run(['ffmpeg','-v','error','-f','concat','-safe','0','-i',sound_list,'-af','aresample=44100','-ac','2',parts/'sound-demo.wav'])
    scenes = [
        (source,0,3,'陽犬號 SunDog｜繁體中文重製'),
        (source,20,8,'Atari ST 原版船形與城市地面'),
        (clips/'seg13.mp4',0,21,'駕駛台、戰術顯示器與工程艙'),
        (cabin,3,20,'城市地面移動與原版飛船輪廓'),
        (args.battle_shot,0,5,'正常有貨交戰抽樣｜命中與保存重開通過'),
        (root/'smoke/linux-player-r3/linux-ground.png',0,9,'原版音效示範｜選單、雷射、爆炸、傳送、通訊'),
    ]
    if args.formal_v1:
        scenes = [
            (source,0,4,'陽犬號 SunDog｜正式 v1.0.0'),
            (clips/'seg13.mp4',0,17,'Atari ST 船艙剖面與導航介面'),
            (source,5,8,'正常角色建立｜繁體中文介面'),
            (clips/'seg6.mp4',7,10,'Atari ST 主介面｜F8 切換 Apple II 呈現'),
            (args.battle_shot,0,5,'有貨交戰抽驗｜勝敗與保存重開通過'),
            (args.repair_shot,0,5,'船體維修｜付費修理與待結算保存重開'),
            (args.score_shot,0,7,'九期主線正常完成｜結算 175 分'),
            (root/'smoke/linux-x86_64/ground.png',0,9,'原版音效示範｜獨立接入，非現場同步收音'),
            (source,0,4,'三平台引擎｜需玩家自備合法 Atari ST 磁碟'),
        ]
    receipts = []
    elapsed = 0
    for i,(input_path,start,duration,caption) in enumerate(scenes):
        caption_path = parts / f'caption-{i}.txt'
        caption_path.write_text(caption)
        footer_path = parts / f'footer-{i}.txt'
        footer_path.write_text(f'{args.version}   ·   只支援新存檔   ·   本機私人示範')
        vf = f'scale=1280:800:flags=neighbor,pad=1280:896:0:0:color=0x101820,fps=30,drawtext=fontfile={font}:textfile={caption_path}:fontsize=28:fontcolor=white:x=32:y=818,drawtext=fontfile={font}:textfile={footer_path}:fontsize=18:fontcolor=0xacc2cf:x=32:y=862,format=yuv420p'
        cmd = ['ffmpeg','-v','error','-threads','2']
        if input_path.suffix == '.png':
            cmd += ['-loop','1','-i',input_path]
        else:
            cmd += ['-ss',str(start),'-i',input_path]
        sound_scene = i == (7 if args.formal_v1 else 5)
        if sound_scene:
            cmd += ['-i',parts/'sound-demo.wav','-af','apad']
        else:
            cmd += ['-f','lavfi','-i','anullsrc=r=44100:cl=stereo']
        part = parts / f'{i:02}.mp4'
        cmd += ['-map','0:v:0','-map','1:a:0','-t',str(duration),'-vf',vf,'-threads','2','-c:v','libx264','-preset','veryfast','-crf','18','-c:a','aac','-ar','44100','-ac','2','-b:a','128k',part]
        run(cmd)
        receipts.append(dict(start=elapsed,duration=duration,source=str(input_path),sourceStart=start,sourceSHA256=sha(input_path),caption=caption,intentionalStill=input_path.suffix=='.png',audio='原版音效示範，另行接入且不冒稱現場同步' if sound_scene else '有意靜音；來源為正常GUI錄影或驗證靜態圖'))
        elapsed += duration
    concat = parts/'concat.txt'
    concat.write_text(''.join(f"file '{parts/f'{i:02}.mp4'}'\n" for i in range(len(scenes))))
    run(['ffmpeg','-v','error','-f','concat','-safe','0','-i',concat,'-c','copy','-movflags','+faststart',output])
    probe = json.loads(run(['ffprobe','-v','error','-show_streams','-show_format','-of','json',output]))
    (promo/'ffprobe.json').write_text(json.dumps(probe,ensure_ascii=False,indent=2)+'\n')
    assert abs(float(probe['format']['duration'])-elapsed)<.2
    assert {s['codec_type'] for s in probe['streams']} == {'video','audio'}
    run(['ffmpeg','-hide_banner','-i',output,'-af','volumedetect,silencedetect=noise=-50dB:d=1','-f','null','-'],promo/'audio-validation.log')
    log = promo.joinpath('audio-validation.log').read_text()
    assert 'max_volume: -inf' not in log
    run(['ffmpeg','-hide_banner','-i',output,'-vf','blackdetect=d=1:pix_th=.10:pic_th=.98,freezedetect=n=-55dB:d=3','-an','-f','null','-'],promo/'video-validation.log')
    assert 'black_start:' not in promo.joinpath('video-validation.log').read_text()
    run(['ffmpeg','-v','error','-i',output,'-vf','fps=1/4,scale=480:336,tile=4x5:padding=3:margin=3','-frames:v','1',promo/'contact-sheet.png'])
    metadata = dict(version=args.version,status='PASS-technical-pending-visual-review',video=output.name,videoSHA256=sha(output),durationSeconds=elapsed,scenes=receipts,soundSources=[dict(path=str(s),sha256=sha(s)) for s in sounds],audioProvenance='sundog-sound-export從玩家合法自備Atari ST磁碟原始音效命令匯出；非新增配樂；示範段獨立標示，GUI段無現場收音',rights='含原版畫面與音效，只供本機私人觀看，不加入Git、公開雲端或Release',diskSHA256=sha(stage/'sundog.st'),saveSchema=44,scope='正常玩家錄影與正常有貨交戰截圖。只陳述已抽驗機制，不宣稱全遊戲逐像素／逐幀一致。')
    (promo/'rights-and-validation.json').write_text(json.dumps(metadata,ensure_ascii=False,indent=2)+'\n')
    print(f'{output}: {elapsed}秒，SHA256 {sha(output)}')


if __name__ == '__main__':
    main()
