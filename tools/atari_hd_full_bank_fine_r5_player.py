import hashlib,json,os,shutil,subprocess,time,sys
from pathlib import Path
from datetime import datetime,timezone
from PIL import Image
assert Path('/.dockerenv').is_file()
case=sys.argv[1];base=Path('/out');out=base/'bank-r5';assert not out.exists();out.mkdir();assert out.stat().st_uid==1000
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
binary=base/'candidate-r5-at-start';assert sha(binary)=='e02c50e63cfc411126ce96b7fc3af934e017c3bfff945ef89a61d54f4be06f48'
cases={
 'bank':('workplace/supply-phase44-phase3-hepa-funding-r1-20261005/library-04-funding-teller.json','8b7c160b267e099ca97a16e2bc0027c0de55b3342c9a53278121777d179739bb','uniteller'),
 'parts':('workplace/cryo-gate-player-r3-20261004/library-02-cryo-parts.json','f63b21b4e01907837bb88f73a1ca9cd650efbc9540d9363a3500dcbd7bc27bd3','parts'),
 'depot':('workplace/formal-v1-gate-20261015/depot-repair/library-11-depot.json','0e1ab051b78e3b15ae2cc983073dd2ab67b1cc2194e97044cf613612ccaff74a','depot'),
 'ftl':('workplace/atari-v1-verification-20261014/normal-ftl-service-r6/save.json','a343c01093636820dcd4cadd68dd3bc654d13667be3c1b85227283dc7a0cc2e4','ftl-boost')}
name,source_sha,expected_screen=cases[case];source=Path('/src')/name;assert sha(source)==source_sha;source_library=case!='ftl';envelope=json.loads(source.read_text());initial=envelope['slots']['slot-1'] if source_library else envelope['state'];assert initial['schema']==44 and initial['screen']==expected_screen
shutil.copyfile(source,out/('library.json' if source_library else 'save.json'));shutil.copyfile(__file__,out/'tool-at-start.py')
(out/'settings.json').write_text(json.dumps(dict(schema=1,theme='atari-st-original',tileset='atari-st-tiles',layout='atari-st')))
source=base/'bank-r1/save.json';source_sha=sha(source);source_library=False;initial=json.loads(source.read_text())['state'];assert initial['screen']=='uniteller' and initial['tileset']=='atari-st-hd-tiles'
shutil.copyfile(source,out/'save.json');shutil.copyfile(base/'bank-r1/settings.json',out/'settings.json')

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
 process+=1;library_start=first and source_library
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
 cmd=['go','run','/src/workplace/hd-full-production-20261015/player/c-services/'+out.name+'/replay.go',str(out/'resume-before.json'),str(ticks)]
 r=subprocess.run(cmd,cwd='/src',env=dict(env,GOCACHE='/cache',GOMODCACHE='/mod',GOPROXY='off',GOTOOLCHAIN='local'),capture_output=True,text=True,timeout=120);assert r.returncode==0,r.stderr
 expected=json.loads(r.stdout);fields=sorted((set(before)|set(after)|set(expected))-{'messageKey','saved','quitRequested'});diff=[k for k in fields if after.get(k)!=expected.get(k)];result=dict(advancedVBLs=ticks,fieldCount=len(fields),fields=fields,differences=diff,command=cmd);write('resume-result.json',result);write('resume-expected.json',expected);assert not diff,diff;return result
checks=[]
def service_ready(label,screen):
 if screen!='ground':
  event('safe-message-dismiss',logical=[10,100],knownScreen=screen)
  run('xdotool','mousemove','--window',window,40,400);run('xdotool','mousedown','1')
  try:time.sleep(.25)
  finally:run('xdotool','mouseup','1')
 deadline=time.monotonic()+20;previous=None;stable=0
 while time.monotonic()<deadline:
  frame(label);im=Image.open('/tmp/terminal-frame.png').convert('RGB');digest=hashlib.sha256(im.crop((280,200,1000,700)).tobytes()).hexdigest();stable=stable+1 if digest==previous else 0
  if stable>=2:
   frame(label+'-ready',True);event('service-window-stable',screen=screen,regionSHA256=digest);return
  previous=digest
 raise RuntimeError('service window did not stabilize '+label)
def observe(screen,label,**expected):
 key('F5');key('1');old=(out/'library.json').stat().st_mtime_ns;down('s');deadline=time.monotonic()+20
 try:
  while time.monotonic()<deadline:
   if (out/'library.json').stat().st_mtime_ns!=old:
    state=json.loads((out/'library.json').read_text())['slots']['slot-1'];assert state['screen']==screen,(label,state['screen'],screen);assert state['tileset']=='atari-st-hd-tiles'
    for field,value in expected.items():assert state.get(field,0)==value,(label,field,state.get(field),value)
    shutil.copyfile(out/'library.json',out/(label+'-library.json'));event('normal-F5-library-write-ack',name=label,SHA256=sha(out/'library.json'),state=state);checks.append(dict(label=label,expected=dict(screen=screen,**expected),state=state));write('checks.json',checks);break
   time.sleep(.05)
  else:raise RuntimeError('library write timeout '+label)
 finally:up('s')
 key('Escape');service_ready(label,screen);return state
def step(k,screen,label,**expected):key(k);return observe(screen,label,**expected)
display=subprocess.Popen(['Xvfb',':138','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
try:
 time.sleep(.4);start(True);assert json.loads((out/'settings.json').read_text())['tileset']=='atari-st-hd-tiles';s=observe(expected_screen,'10-HD-initial-service')
 if case=='bank':
  for k,page,title in [('d',1,'deposit')]:
   step(k,'uniteller','11-HD-'+title,tellerPage=page)
   s=step('Escape','uniteller','12-HD-back-'+title,tellerPage=0)
 elif case=='parts':
  original=s['selectedPartIndex'];right=step('Right','parts','11-HD-next-part');assert right['selectedPartIndex']!=original
  s=step('Left','parts','12-HD-original-part',selectedPartIndex=original)
  step('Escape','ground','13-HD-ground');s=step('y','parts','14-HD-normal-Y-parts')
 elif case=='depot':
  s=step('1','fuel-purchase','11-HD-main-fuel-quote');assert s['fuelPurchase']['kind']=='main' and s['fuelPurchase']['quantity']==s['fuelPurchase']['initialQuantity']
  step('3','depot','12-HD-fuel-cancel')
  s=step('2','fuel-purchase','13-HD-hull-quote');assert s['fuelPurchase']['kind']=='hull' and s['fuelPurchase']['quantity']==s['fuelPurchase']['initialQuantity']
  step('3','depot','14-HD-hull-cancel');step('Escape','ground','15-HD-ground');s=step('f','depot','16-HD-normal-F-depot')
 else:
  step('3','depot','11-HD-FTL-cancel');s=step('4','ftl-boost','12-HD-normal-4-FTL')
 financial=['cash','bank','bankBalances','holds','parts','stores','jumpRange','fuel','hullCondition']
 assert all(s.get(k)==initial.get(k) for k in financial),[k for k in financial if s.get(k)!=initial.get(k)]
 before=quit_save('18-HD-before-reopen');assert before['screen']==expected_screen
 start();service_ready('20-HD-reopened-service',expected_screen);after=quit_save('21-HD-after-reopen');clock=replay(before,after)
 assert sha(source)==source_sha and sha(binary)=='e02c50e63cfc411126ce96b7fc3af934e017c3bfff945ef89a61d54f4be06f48'
 assert all(after.get(k)==initial.get(k) for k in financial),[k for k in financial if after.get(k)!=initial.get(k)]
 assert not any('panic:' in p.read_text() for p in out.glob('game-*.log'))
 write('receipt.json',dict(status='PASS-HD-service-'+case,sourcePath=str(source),sourceSHA256=source_sha,binarySHA256=sha(binary),diskSHA256=sha('/full/sundog.st'),toolSHA256=sha(out/'tool-at-start.py'),events=events,images=images,checks=checks,replay=clock,financialFieldsUnchanged=financial,finalSaveSHA256=sha(out/'save.json'),settingsSHA256=sha(out/'settings.json'),limitations=['1280×800繁中Linux正常GUI，真F8偏好重用，正常F5庫mtime寫入觀測，Q真保存與新程序重開。','未確認交易、不安裝FTL、不購入零件或油料，不外推原版GUI或全部服務規則。'],visualReview='pending-single-read'))
 print('PASS',case,len(events),len(images),clock['advancedVBLs'],clock['fieldCount'],flush=True)
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
