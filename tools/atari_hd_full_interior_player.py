import hashlib,json,os,shutil,subprocess,time,sys
from pathlib import Path
from datetime import datetime,timezone
from PIL import Image
assert Path('/.dockerenv').is_file()
mode=sys.argv[1];assert mode in ['arena','dialog']
base=Path('/out');out=base/(mode+'-r2');assert not out.exists();out.mkdir();assert out.stat().st_uid==1000
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
binary=Path('/src/workplace/hd-full-production-20261015/sundog-hd-candidate-r3');assert sha(binary)=='3f9e31ab2b82c98497686e992584f54366e203a4c72fa9a922b7cba34b9167d0'
source=Path('/src/workplace/supply-phase44-phase2-einkolt-return-player-r1-20261004/library-01-start.json') if mode=='arena' else Path('/src/workplace/supply-phase44-phase4-shoothan-r1-20261005/library-02-depart-city-ship-position-step-13.json')
source_sha=sha(source);assert source_sha==('c81faf0e9a9b7f3ceb79e8ae24763806536e2b8580940924661188621c734bf5' if mode=='arena' else '4269cca1eae9977da8bfb9e488bb4f18bb58b573130b102b8c77305f6d7d15e5')
shutil.copyfile(source,out/'library.json');shutil.copyfile(__file__,out/'tool-at-start.py')
(out/'settings.json').write_text(json.dumps(dict(schema=1,theme='atari-st-original',tileset='atari-st-tiles',layout='atari-st')))
events=[];images=[];polls=[];app=None;window=None;keys=set();process=0;index=0
env=dict(os.environ,DISPLAY=':139',HOME='/tmp')
def write(name,d):(out/name).write_text(json.dumps(d,ensure_ascii=False,indent=2)+'\n')
def event(kind,**kw):
 events.append(dict(kind=kind,monotonicNS=time.monotonic_ns(),utc=datetime.now(timezone.utc).isoformat(),**kw));write('events.json',events)
def run(*a):return subprocess.check_output(list(map(str,a)),env=env,text=True,timeout=20).strip()
def down(k):keys.add(k);event('keydown',key=k);run('xdotool','keydown','--clearmodifiers',k)
def up(k):run('xdotool','keyup',k);keys.discard(k);event('keyup',key=k)
def key(k):
 down(k)
 try:time.sleep(.25)
 finally:up(k)
 event('existing-normal-input-pulse',key=k,holdSeconds=.25)
def frame(label,retain=False):
 global index
 index+=1;t0=time.monotonic_ns();p=Path('/tmp/terminal-frame.png');run('import','-window',window,p);im=Image.open(p).convert('RGB');assert im.size==(1280,800)
 count=sum(max(c)>110 for c in im.resize((80,50)).getdata()); bands=sum(1 for x in [154,162] for y in range(90,199) if (lambda c:c[2]>180 and c[0]<80 and c[1]<140)(im.getpixel((x*4,y*4))));r=dict(index=index,label=label,startNS=t0,endNS=time.monotonic_ns(),SHA256=sha(p),brightSampleCount=count,blueBandSamples=bands,corner=im.getpixel((0,0)),contentSHA256=hashlib.sha256(im.crop((640,352,1276,748)).tobytes()).hexdigest())
 if retain:
  f=out/f'{index:03d}-{label}.png';shutil.copyfile(p,f);r['image']=f.name;images.append(dict(path=f.name,SHA256=sha(f),visualReview='pending-single-read'));write('images.json',images)
 polls.append(r);write('polls.json',polls);return r
def visible(label):
 deadline=time.monotonic()+20;stable=0
 while time.monotonic()<deadline:
  assert app.poll() is None,'early exit';r=frame(label);stable=stable+1 if r['brightSampleCount']>100 else 0
  if stable>=2:
   r=frame(label+'-ready',True);event('visible-frame-ready',label=label,pollIndex=r['index']);return r
  time.sleep(.05)
 raise RuntimeError('visible frame timeout '+label)
def start(first=False):
 global app,window,process
 process+=1;library_start=first
 command=[str(binary),'-scale','4','-atari-disk','/full/sundog.st','-save',str(out/'save.json'),'-library',str(out/'library.json'),'-settings',str(out/'settings.json'),'-locale','zh-Hant']+(['-start','title'] if library_start else ['-load'])
 event('start',command=command,inputSHA256=source_sha if first else sha(out/'save.json'),settingsSHA256=sha(out/'settings.json'))
 app=subprocess.Popen(command,cwd='/src',env=env,stdout=(out/f'game-{process}.log').open('w'),stderr=subprocess.STDOUT)
 deadline=time.monotonic()+20
 while time.monotonic()<deadline:
  assert app.poll() is None
  r=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,text=True,capture_output=True)
  if r.returncode==0 and r.stdout.strip():window=r.stdout.strip().splitlines()[-1];break
  time.sleep(.05)
 else:raise RuntimeError('no window')
 run('xdotool','windowfocus',window);geometry=run('xdotool','getwindowgeometry','--shell',window);assert 'WIDTH=1280' in geometry and 'HEIGHT=800' in geometry;event('window-ready',geometry=geometry)
 visible('01-title' if library_start else ('01-source' if first else '20-reopened'))
 if library_start:
  for k,label in [('space','02-play'),('c','03-select-library'),('1','04-select-slot'),('l','05-load-tactical')]:key(k);visible(label)
def select_hd():
 old=(out/'settings.json').stat().st_mtime_ns;down('F8');deadline=time.monotonic()+20
 try:
  while time.monotonic()<deadline:
   if (out/'settings.json').stat().st_mtime_ns!=old:
    pref=json.loads((out/'settings.json').read_text());assert pref['tileset']=='atari-st-hd-tiles';event('HD-preferences-write-ack',settings=pref,SHA256=sha(out/'settings.json'));break
   time.sleep(.05)
  else:raise RuntimeError('F8 did not persist HD preferences')
 finally:up('F8')
 visible('06-HD-selected')
def save_probe(label):
 old=(out/'save.json').stat().st_mtime_ns if (out/'save.json').exists() else 0
 # Tactical has no S binding. S only probes the failure terminal; otherwise
 # observe through the normal F5 library and its actual file-write ack.
 key('s')
 if (out/'save.json').exists() and (out/'save.json').stat().st_mtime_ns!=old:
  s=json.loads((out/'save.json').read_text())['state'];assert s['screen']=='outcome';shutil.copyfile(out/'save.json',out/(label+'.json'));event('terminal-S-write-ack',name=label,SHA256=sha(out/'save.json'),state=s);return s
 key('F5');visible(label+'-library');key('1');old=(out/'library.json').stat().st_mtime_ns;down('s');deadline=time.monotonic()+20
 try:
  while time.monotonic()<deadline:
   if (out/'library.json').stat().st_mtime_ns!=old:
    s=json.loads((out/'library.json').read_text())['slots']['slot-1'];assert s['tileset']=='atari-st-hd-tiles' and s['screen']=='tactical';shutil.copyfile(out/'library.json',out/(label+'-library.json'));event('normal-F5-library-write-ack',name=label,SHA256=sha(out/'library.json'),state=s);break
   time.sleep(.05)
  else:raise RuntimeError('F5 library mtime unchanged')
 finally:up('s')
 key('Escape');visible(label+'-return-tactical');return s
def quit_save(label):
 down('q')
 try:app.wait(timeout=20)
 finally:up('q')
 assert app.returncode==0;s=json.loads((out/'save.json').read_text())['state'];assert s['tileset']=='atari-st-hd-tiles';shutil.copyfile(out/'save.json',out/(label+'.json'));event('normal-Q-save-exit-ack',name=label,SHA256=sha(out/'save.json'),state=s);return s
def replay(before,after):
 ticks=(after['awakeClock']['source']['ticks']-before['awakeClock']['source']['ticks'])%2**32;assert ticks<600
 go='''package main
import("encoding/json";"os";"strconv";"github.com/wicanr2/sundog-remake/internal/game")
func main(){b,e:=os.ReadFile(os.Args[1]);if e!=nil{panic(e)};var s game.State;if e=json.Unmarshal(b,&s);e!=nil{panic(e)};n,_:=strconv.Atoi(os.Args[2]);r,e:=game.NewReducer();if e!=nil{panic(e)};for i:=0;i<n;i++{s,e=r.Step(s,game.ActionVBLTick);if e!=nil{panic(e)}};json.NewEncoder(os.Stdout).Encode(s)}
'''
 (out/'replay.go').write_text(go);write('resume-before.json',before)
 cmd=['go','run','/src/workplace/hd-full-production-20261015/player/d-tests/'+out.name+'/replay.go',str(out/'resume-before.json'),str(ticks)]
 r=subprocess.run(cmd,cwd='/src',env=dict(env,GOCACHE='/cache',GOMODCACHE='/mod',GOPROXY='off',GOTOOLCHAIN='local'),capture_output=True,text=True,timeout=120);assert r.returncode==0,r.stderr
 expected=json.loads(r.stdout);fields=[k for k in before if k not in ['messageKey','saved','quitRequested']];diff=[k for k in fields if after.get(k)!=expected.get(k)];result=dict(advancedVBLs=ticks,fieldCount=len(fields),fields=fields,differences=diff,command=cmd);write('resume-result.json',result);write('resume-expected.json',expected);assert not diff,diff;return result
def bands_ready(label):
 deadline=time.monotonic()+20;good=0
 while time.monotonic()<deadline:
  assert app.poll() is None;r=frame(label);good=good+1 if r['blueBandSamples']>=15 else 0
  if good>=2:
   r=frame(label+'-buttons-ready',True);event('normal-blue-action-bands-ready',label=label,pollIndex=r['index'],samples=r['blueBandSamples']);return
  time.sleep(.05)
 raise RuntimeError('temporary message did not clear to action bands '+label)
def checkpoint(label,screen):
 s=quit_save(label);assert s['screen']==screen,(label,s['screen']);return s
def reopen(label,screen):
 start();s=None;event('expected-reopened-screen',screen=screen,label=label)
def step(k,label):
 frame(label+'-before');baseline=polls[-1]['contentSHA256'];key(k);deadline=time.monotonic()+20
 while time.monotonic()<deadline:
  r=frame(label)
  if r['contentSHA256']!=baseline:event('normal-interior-visible-change',key=k,pollIndex=r['index']);return
  time.sleep(.05)
 raise RuntimeError('interior input caused no visible change '+label)
display=subprocess.Popen(['Xvfb',':139','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
try:
 time.sleep(.4);start(True);select_hd()
 if mode=='arena':
  first=checkpoint('07-HD-arena-before-reopen','arena');assert first['arena']['active']
  reopen('08-HD-arena-reopened','arena');key('x');visible('09-HD-arena-fled-ground')
  key('e');bands_ready('10-HD-bar-door');s=checkpoint('11-HD-bar-real-save','interior');assert s['interior']['kind']==12 and s['interior']['position']==dict(x=5,y=6)
  reopen('12-HD-bar-reopened','interior')
  for i,k in enumerate(['Right']*3+['Up']*5):step(k,'13-interior-step-'+str(i))
  bands_ready('14-HD-near-slots');s=checkpoint('15-HD-at-slots-real-save','interior');assert s['interior']['position']==dict(x=8,y=1) and s['interior']['hasSlots']
  reopen('16-HD-at-slots-reopened','interior');key('g');bands_ready('17-HD-slots-open');before=checkpoint('18-HD-slots-before-spin','slots');assert not before['slots']['spun']
  reopen('19-HD-slots-reopened','slots');key('space');bands_ready('20-HD-first-natural-spin-result');after=checkpoint('21-HD-slots-first-spin-real-save','slots')
  assert after['slots']['spun'] and after['slots']['rolls']==before['slots']['rolls']+6 and after['cash']==before['cash']-before['slots']['bet']+after['slots']['payout']
  event('one-natural-spin-result',before=before['slots'],after=after['slots'],cashBefore=before['cash'],cashAfter=after['cash'])
  reopen('22-HD-spin-result-reopened','slots');bands_ready('23-HD-spin-result-reopened-content');key('Escape');visible('24-HD-slots-return-interior')
  for i,k in enumerate(['Down']*5+['Left']*3):step(k,'25-interior-return-step-'+str(i))
  bands_ready('26-HD-back-at-door');key('e');visible('27-HD-bar-left-ground');before=checkpoint('28-HD-final-before-reopen','ground')
  assert not before['interior']['active'] and before['position']==dict(x=15,y=7)
  reopen('29-HD-ground-reopened','ground');after=checkpoint('30-HD-final-after-reopen','ground');clock=replay(before,after)
 else:
  bands_ready('07-HD-dialog-content');key('Right');bands_ready('08-HD-dialog-next-verb');before=checkpoint('09-HD-dialog-selected-real-save','dialog');assert before['dialogVerb']==1
  reopen('10-HD-dialog-selected-reopened','dialog');bands_ready('11-HD-dialog-selected-reopened-content');key('Left');bands_ready('12-HD-dialog-previous-verb');s=checkpoint('13-HD-dialog-restored-verb','dialog');assert s['dialogVerb']==0
  reopen('14-HD-dialog-restored-reopened','dialog');key('Escape');visible('15-HD-dialog-return-ground');before=checkpoint('16-HD-dialog-ground-before-reopen','ground');reopen('17-HD-dialog-ground-reopened','ground');after=checkpoint('18-HD-dialog-ground-after-reopen','ground');clock=replay(before,after)
 assert sha(source)==source_sha and sha(binary)=='3f9e31ab2b82c98497686e992584f54366e203a4c72fa9a922b7cba34b9167d0'
 assert not any('panic:' in q.read_text() for q in out.glob('game-*.log'))
 write('receipt.json',dict(status='PASS-HD-D-'+mode,source=str(source),sourceSHA256=source_sha,binarySHA256=sha(binary),diskSHA256=sha('/full/sundog.st'),toolSHA256=sha(out/'tool-at-start.py'),events=events,images=images,replay=clock,finalSaveSHA256=sha(out/'save.json'),settingsSHA256=sha(out/'settings.json'),limitations=['1280×800繁中正常Title/Space/C/1/L、F8真寫偏好、後續同settings與真Q保存重開。','arena正常X逃離，當下原座標本身terrain11直接E進酒吧；正常室內鍵盤走到機台、只旋轉一次不挑結果。','沒有注入state/座標/seed，不新增原版GUI對拍或其他平台聲明。'],visualReview='pending-single-read'))
 print('PASS',mode,len(events),len(images),clock['advancedVBLs'],clock['fieldCount'],flush=True)
except Exception as e:
 if app is not None and app.poll() is None and window is not None:
  try:frame('failure-actual-window',True)
  except Exception:pass
 write('failure.json',dict(error=repr(e),events=events,polls=polls));raise
finally:
 for k in list(keys):
  try:up(k)
  except Exception:pass
 if app is not None and app.poll() is None:
  app.terminate()
  try:app.wait(timeout=5)
  except subprocess.TimeoutExpired:app.kill();app.wait(timeout=5)
 display.terminate();display.wait(timeout=5)
