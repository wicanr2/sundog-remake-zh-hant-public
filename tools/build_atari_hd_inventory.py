#!/usr/bin/env python3
"""Docker內重生Atari ST HD來源及玩家畫面清單；入口docs/art/atari-st-hd-plan.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

PROBE = r'''package main
import("encoding/json";"os";"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia")
func main(){
 d,e:=originalmedia.Open(os.Args[1]);if e!=nil {panic(e)}
 dir,e:=d.Directory();if e!=nil {panic(e)}
 type item struct {Index,Width,Height,Pixels int; Error string}
 rows:=[]item{}
 for i:=range dir {r:=item{Index:i};p,e:=d.DecodeImage(i);if e!=nil {r.Error=e.Error()} else {r.Width,r.Height,r.Pixels=p.Width,p.Height,len(p.Pixels)};rows=append(rows,r)}
 icons,e:=d.ItemIcons();if e!=nil {panic(e)}
 _,e=d.Font();if e!=nil {panic(e)}
 _,e=d.GroundSprites();if e!=nil {panic(e)}
 json.NewEncoder(os.Stdout).Encode(struct{DirectoryCount,ItemIcons int;Images []item;FontValid,GroundSpritesValid bool}{len(dir),icons.Count(),rows,true,true})
}'''


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for key in ('repo', 'disk', 'output', 'work'):
        p.add_argument('--'+key, type=Path, required=True)
    a=p.parse_args()
    if not Path('/.dockerenv').is_file():
        p.error('只能在Docker執行')
    repo,disk,output,work=[getattr(a,k).resolve() for k in ('repo','disk','output','work')]
    for target in (output,work):
        if target.exists() or target.parent.stat().st_uid!=os.getuid():
            p.error('輸出已存在或父目錄擁有者不符')
    directory=repo/'docs/atari-st/image-directory.json'
    source=json.loads(directory.read_text())
    if sha(disk)!=source['source']['diskSha256']:
        p.error('磁碟身分不同')
    work.mkdir(mode=0o700)
    probe=work/'probe.go';probe.write_text(PROBE)
    runtime=json.loads(subprocess.check_output(['go','run',str(probe),str(disk)],cwd=repo,text=True,timeout=120))
    (work/'runtime-metadata.json').write_text(json.dumps(runtime,indent=2)+'\n')
    assert runtime['DirectoryCount']==len(source['images'])==75 and runtime['ItemIcons']==31
    known={
        0:('title','docs/atari-st/title-screen.md'),
        1:('logo','docs/atari-st/title-screen.md'),
        2:('credits','docs/atari-st/title-screen.md'),
        4:('item-icons','docs/atari-st/item-icons.md'),
        8:('shipboard','docs/atari-st/ship-rules.md'),
        10:('interface-frame','docs/ui/atari-st-layout-plan.md'),
        11:('planet-map','docs/atari-st/font-strip.md'),
        12:('system-map','docs/atari-st/font-strip.md'),
        13:('ship-room','internal/source/atarist/originalmedia/interiors.go'),
        30:('ground-backdrop','docs/atari-st/ground-rules.md'),
    }
    for start,end,label in [(14,20,'bar'),(20,24,'parts-shop'),(24,27,'equipment-shop'),(27,30,'hotel')]:
        for i in range(start,end):known[i]=(label,'docs/atari-st/city-subsystems.json')
    strips={r['index']:r for r in source['tileStrips']}
    images=[];items=[]
    for row,decoded in zip(source['images'],runtime['Images']):
        i=row['index'];assert i==decoded['Index']
        if not decoded['Error']:
            assert [row['declaredWidth'],row['declaredHeight']]==[decoded['Width'],decoded['Height']]
            assert decoded['Pixels']==decoded['Width']*decoded['Height']
        role,reference=known.get(i,('unknown','docs/atari-st/image-directory.json'))
        images.append(dict(sourceIndex=i,rawOffset=row['rawOffset'],byteLength=row['byteLength'],sourceSize=[row['declaredWidth'],row['declaredHeight']],decode=decoded,role=role,reference=reference,usageGrade='unknown' if role=='unknown' else '既有來源consumer證據；本輪不重新升格',productionApproved=False))
        if i in strips:
            strip=strips[i];assert row['declaredWidth']==strip['tileCount']*32 and row['declaredHeight']==32
            for cell in range(strip['tileCount']):
                items.append(dict(id=f'image-{i}-cell-{cell}',kind='source-tile-candidate',sourceImage=i,cell=cell,sourceSize=[32,32],sourceAnchor='待按正式consumer凍結',status='DRAFT; 未製作／未核定去重',reference='docs/atari-st/ground-rules.md'))
        elif i==4:
            for code in range(1,32):
                items.append(dict(id=f'item-icon-{code}',kind='item-icon',sourceImage=4,cell=code-1,itemCode=code,sourceSize=[16,16],status='DRAFT; 未製作',reference='docs/atari-st/item-icons.md'))
        else:
            items.append(dict(id=f'image-{i}',kind='image-candidate',sourceImage=i,role=role,status='DRAFT; 待用途／範圍審查' if role=='unknown' else 'DRAFT; 未製作',reference=reference))
    for code in list(range(32))+[60,64]:
        items.append(dict(id=f'marker-{code}',kind='non-text-glyph-candidate',sourceRawOffset='0x06600',glyph=code,sourceCell=[code*6,0,6,8],status='DRAFT; 用途／重畫映射待審',reference='docs/atari-st/font-strip.md'))
    for glyph in range(25):
        items.append(dict(id=f'small-figure-{glyph}',kind='small-figure',sourceRawOffset='0x06600',sourceCell=[0x300+glyph*7,0,7,8],enemyType=glyph//3 if glyph<21 else None,animationFrame=glyph%3 if glyph<21 else None,status='DRAFT; 未製作' if glyph<21 else 'DRAFT; unknown用途，不補造',reference='cmd/sundog-ebiten/stlayout.go#drawSTViewportEnemy'))
    for index in range(10):
        items.append(dict(id=f'ground-pattern-{index}',kind='ground-sprite-candidate',sourceRawOffset=f'0x{0x30802+index*8:05x}',sourceSize=[7,7],status='DRAFT; 依目前正式圖樣／碰撞映射核定，不由歷史註解猜用途',reference='docs/atari-st/walker-timing-spec.md'))
    screens={};sources={directory.relative_to(repo).as_posix():sha(directory)}
    for folder in ('internal/game','cmd/sundog-ebiten','internal/source/atarist/originalmedia'):
        for file in sorted((repo/folder).glob('*.go')):
            if file.name.endswith('_test.go'):continue
            text=file.read_text();sources[file.relative_to(repo).as_posix()]=sha(file)
            for match in re.finditer(r'\b(Screen\w+)\s+Screen\s*=\s*"([^"]+)"',text):
                screens[match.group(2)]=dict(id=match.group(1),source=file.relative_to(repo).as_posix(),status='DRAFT; 正常玩家畫面／子狀態全量HD待製作')
    report=dict(schema=1,status='DRAFT',scope='來源與目前正式consumer清單；候選數量不等於最終重畫數量，不宣稱全量HD或原版完整GUI完成。',sourceDisk=dict(name=source['source']['disk'],sha256=sha(disk)),tool=dict(path=Path(__file__).resolve().relative_to(repo).as_posix(),sha256=sha(Path(__file__)),go=subprocess.check_output(['go','version'],text=True).strip()),sourceHashes=sources,images=images,workItems=items,screens=screens,counts=dict(directory=75,tileStrips=len(strips),tileCandidates=sum(x['tileCount'] for x in strips.values()),itemIcons=31,nonTextMarkerCandidates=34,smallFigures=25,groundPatterns=10,totalCandidateWorkItems=len(items),formalScreens=len(screens)),boundaries=['未要求原始文字字型；其餘94個文字字元可用已授權清晰字型，不逐格重畫。','330格來源圖塊仍需按用途／鄰接／錨點去重；不能直接宣稱330件獨立正式資產。','每個screen仍須補齊子選單、事件、動畫與勝敗分支；來源75張影像不是完整玩家畫面全集。','正式資產獨立繪製；不從生成全畫面切atlas，不更改碰撞、秘密資訊或命中區。','原始磁碟／解碼像素留本機，本清單不含原版像素。'])
    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(report['counts'],ensure_ascii=False))


if __name__=='__main__':
    main()
