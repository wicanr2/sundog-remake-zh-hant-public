import hashlib,json,os,shutil,subprocess,time,sys
from pathlib import Path
from datetime import datetime,timezone
from PIL import Image
assert Path('/.dockerenv').is_file()
mode=sys.argv[1];assert mode in ['score','defeat']
base=Path('/out');out=base/(mode+('-r2' if mode=='score' else '-r1'));assert not out.exists();out.mkdir();assert out.stat().st_uid==1000
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
binary=base/'candidate-r3-at-start';assert sha(binary)=='3f9e31ab2b82c98497686e992584f54366e203a4c72fa9a922b7cba34b9167d0'
source=Path('/src/workplace/atari-v1-verification-20261014/final-score-reopened-r2/save.json') if mode=='score' else Path('/src/workplace/combat-outcomes-20261015/defeat/library-turn-236.json')
source_sha=sha(source);assert source_sha==('5f9c5e04bd93ed03691d0e9ebbed3f030b959e6400c75589fbdffe99a5e00d65' if mode=='score' else 'b4b1e21f9c397fc3dc0a26336b7602f6276aeb591c2e19fd7d166895911ef771')
shutil.copyfile(source,out/('save.json' if mode=='score' else 'library.json'));shutil.copyfile(__file__,out/'tool-at-start.py')
(out/'settings.json').write_text(json.dumps(dict(schema=1,theme='atari-st-original',tileset='atari-st-tiles',layout='atari-st')))
if mode=='score':
 source=base/'score-r1/12-HD-before-reopen.json';source_sha=sha(source)
 shutil.copyfile(source,out/'save.json');shutil.copyfile(base/'score-r1/settings.json',out/'settings.json')
events=[];images=[];polls=[];app=None;window=None;keys=set();process=0;index=0
env=dict(os.environ,DISPLAY=':138',HOME='/tmp')
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
 count=sum(max(c)>110 for c in im.resize((80,50)).getdata());r=dict(index=index,label=label,startNS=t0,endNS=time.monotonic_ns(),SHA256=sha(p),brightSampleCount=count)
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
 process+=1;library_start=first and mode=='defeat'
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
 old=(out/'save.json').stat().st_mtime_ns if (out/'save.json').exists() else 0;down('s');deadline=time.monotonic()+20
 try:
  while time.monotonic()<deadline:
   if (out/'save.json').exists() and (out/'save.json').stat().st_mtime_ns!=old:
    s=json.loads((out/'save.json').read_text())['state'];assert s['tileset']=='atari-st-hd-tiles';shutil.copyfile(out/'save.json',out/(label+'.json'));event('normal-S-write-ack',name=label,SHA256=sha(out/'save.json'),state=s);return s
   time.sleep(.05)
  raise RuntimeError('S save mtime unchanged')
 finally:up('s')
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
 cmd=['go','run','/src/workplace/hd-full-production-20261015/player/f-tests/'+out.name+'/replay.go',str(out/'resume-before.json'),str(ticks)]
 r=subprocess.run(cmd,cwd='/src',env=dict(env,GOCACHE='/cache',GOMODCACHE='/mod',GOPROXY='off',GOTOOLCHAIN='local'),capture_output=True,text=True,timeout=120);assert r.returncode==0,r.stderr
 expected=json.loads(r.stdout);fields=[k for k in before if k not in ['messageKey','saved','quitRequested']];diff=[k for k in fields if after.get(k)!=expected.get(k)];result=dict(advancedVBLs=ticks,fieldCount=len(fields),fields=fields,differences=diff,command=cmd);write('resume-result.json',result);write('resume-expected.json',expected);assert not diff,diff;return result
display=subprocess.Popen(['Xvfb',':138','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
try:
 time.sleep(.4);start(True)
 if mode=='defeat':select_hd()
 if mode=='defeat':
  s=save_probe('07-HD-before-defeat');assert s['combatActive'] and s['hullCondition']>0 and s['screen']=='tactical'
  # The original saved branch already has shields off. Preserve its RNG.
  assert s['shieldLevel']==0
  deadline=time.monotonic()+100
  for turn in range(1,101):
   assert time.monotonic()<deadline,'bounded defeat route did not terminate'
   for _ in range(10):key('r')
   s=save_probe(f'08-combat-turn-{turn:03d}');event('combat-observation',turn=turn,hull=s['hullCondition'],enemyHull=s['enemyHull'],screen=s['screen'])
   if s['screen']=='outcome':break
  assert s['screen']=='outcome' and s['outcomeKey']=='ui.outcome.failure' and s['hullCondition']<=0 and not s['combatActive']
  visible('10-HD-defeat-terminal');key('Escape');key('x');visible('11-HD-defeat-terminal-locked')
 before=quit_save('12-HD-before-reopen')
 if mode=='score':assert before['screen']=='score' and before['missionComplete'] and before['colony']['phase']==9
 else:assert before['screen']=='outcome' and before['outcomeKey']=='ui.outcome.failure'
 start();after=quit_save('21-HD-after-reopen');clock=replay(before,after)
 assert sha(source)==source_sha and sha(binary)=='3f9e31ab2b82c98497686e992584f54366e203a4c72fa9a922b7cba34b9167d0'
 assert not any('panic:' in p.read_text() for p in out.glob('game-*.log'))
 write('receipt.json',dict(status='PASS-HD-'+mode,sourceSHA256=source_sha,binarySHA256=sha(binary),diskSHA256=sha('/full/sundog.st'),toolSHA256=sha(out/'tool-at-start.py'),events=events,images=images,replay=clock,finalSaveSHA256=sha(out/'save.json'),settingsSHA256=sha(out/'settings.json'),limitations=['正常GUI、1280×800繁中、真F8偏好寫入，重開沿同一settings；無state或seed注入。','只證明本候選正常終局呈現與重開，不新增原版GUI對拍或跨平台聲明。'],visualReview='pending-single-read'))
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
