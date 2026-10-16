import hashlib,json,os,shutil,subprocess,time,sys
from pathlib import Path
from datetime import datetime,timezone
from PIL import Image
assert Path('/.dockerenv').is_file()
mode=sys.argv[1];assert mode=='trade'
base=Path('/out');out=base/(mode+'-r1');assert not out.exists();out.mkdir();assert out.stat().st_uid==1000
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
binary=Path('/src/workplace/hd-full-production-20261015/sundog-hd-candidate-r3');assert sha(binary)=='3f9e31ab2b82c98497686e992584f54366e203a4c72fa9a922b7cba34b9167d0'
source=Path('/src/workplace/supply-phase44-phase7-drahew-stock-r1-20261005/save.json')
source_sha=sha(source);assert source_sha=='786c9debce4f9b5ecb32af1ac35a6e0724b1b284e3a71f67633d5d395ca6fa6f'
shutil.copyfile(source,out/'save.json');shutil.copyfile(__file__,out/'tool-at-start.py')
(out/'settings.json').write_text(json.dumps(dict(schema=1,theme='atari-st-original',tileset='atari-st-tiles',layout='atari-st')))
events=[];images=[];polls=[];app=None;window=None;keys=set();process=0;index=0
env=dict(os.environ,DISPLAY=':141',HOME='/tmp')
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
 process+=1;library_start=False
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
 cmd=['go','run','/src/workplace/hd-full-production-20261015/player/c-trade/'+out.name+'/replay.go',str(out/'resume-before.json'),str(ticks)]
 r=subprocess.run(cmd,cwd='/src',env=dict(env,GOCACHE='/cache',GOMODCACHE='/mod',GOPROXY='off',GOTOOLCHAIN='local'),capture_output=True,text=True,timeout=120);assert r.returncode==0,r.stderr
 expected=json.loads(r.stdout);fields=[k for k in before if k not in ['messageKey','saved','quitRequested']];diff=[k for k in fields if after.get(k)!=expected.get(k)];result=dict(advancedVBLs=ticks,fieldCount=len(fields),fields=fields,differences=diff,command=cmd);write('resume-result.json',result);write('resume-expected.json',expected);assert not diff,diff;return result
source_state=json.loads(source.read_text())['state']
def trade_ready(page,label):
 # Measured formal stMenuStyles rectangles, using inside base (153,81).
 row={0:97,1:160,3:97,4:93,5:129,6:165,7:83}[page]
 deadline=time.monotonic()+20;good=0
 while time.monotonic()<deadline:
  assert app.poll() is None;r=frame(label);im=Image.open('/tmp/terminal-frame.png').convert('RGB');blue=im.getpixel((180*4,row*4));black=im.getpixel((180*4,(row+3)*4));ready=blue[2]>180 and blue[0]<80 and blue[1]<140 and max(black)<80
  good=good+1 if ready else 0;event('formal-trade-button-observation',expectedPage=page,label=label,topPixel=blue,insidePixel=black,good=good,pollIndex=r['index'])
  if good>=2:frame(label+'-actual-menu-ready',True);return
  time.sleep(.05)
 raise RuntimeError('trade actual menu did not become ready '+label)
def page_key(k,page,label):key(k);trade_ready(page,label)
def library_save():
 key('F5');visible('19-F5-library')
 if not (out/'library.json').exists():
  down('m');deadline=time.monotonic()+20
  try:
   while time.monotonic()<deadline and not (out/'library.json').exists():time.sleep(.05)
   assert (out/'library.json').exists()
  finally:up('m')
  event('actual-M-library-created',mtimeNS=(out/'library.json').stat().st_mtime_ns,SHA256=sha(out/'library.json'))
 key('1');old=(out/'library.json').stat().st_mtime_ns;down('s');deadline=time.monotonic()+20
 try:
  while time.monotonic()<deadline:
   if (out/'library.json').stat().st_mtime_ns!=old:
    s=json.loads((out/'library.json').read_text())['slots']['slot-1'];assert s['screen']=='ground';shutil.copyfile(out/'library.json',out/'20-real-ground-library.json');event('actual-F5-S-library-write',mtimeNS=(out/'library.json').stat().st_mtime_ns,SHA256=sha(out/'library.json'),state=s);frame('20-library-write',True);return s
   time.sleep(.05)
  raise RuntimeError('library actual mtime write missing')
 finally:up('s')
display=subprocess.Popen(['Xvfb',':141','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
try:
 time.sleep(.4);start(True);select_hd();trade_ready(0,'03-HD-trade-main')
 page_key('b',1,'04-HD-buy-quote');page_key('Escape',0,'05-HD-buy-back-main');page_key('s',7,'06-HD-sell-source');page_key('Escape',0,'07-HD-sell-back-main');page_key('Escape',3,'08-HD-trade-entrance');page_key('w',4,'09-HD-warehouse');page_key('s',5,'10-HD-warehouse-store');page_key('Escape',4,'11-HD-store-back-warehouse');page_key('f',6,'12-HD-warehouse-fetch');page_key('Escape',4,'13-HD-fetch-back-warehouse');page_key('Escape',3,'14-HD-warehouse-back-entrance');key('Escape');visible('15-HD-trade-return-ground')
 lib=library_save();fields=['cash','bank','bankBalances','gems','holds','warehouses','markets','quotes','tradeRolls','colony','colonySite','cargoCityId','cargoPosition','cargoAttached','groundMode'];diff=[k for k in fields if lib.get(k)!=source_state.get(k)];write('unchanged-business-fields.json',dict(fields=fields,differences=diff,source={k:source_state.get(k) for k in fields},after={k:lib.get(k) for k in fields}));assert not diff,diff
 key('Escape');visible('21-ground-library-return');before=quit_save('22-HD-trade-before-reopen');assert before['screen']=='ground' and before['cityId']=='banville';start();after=quit_save('23-HD-trade-after-reopen');clock=replay(before,after)
 assert sha(source)==source_sha and sha(binary)=='3f9e31ab2b82c98497686e992584f54366e203a4c72fa9a922b7cba34b9167d0';assert not any('panic:' in q.read_text() for q in out.glob('game-*.log'))
 write('receipt.json',dict(status='PASS-HD-C-trade-page-sampling',source=str(source),sourceSHA256=source_sha,binarySHA256=sha(binary),diskSHA256=sha('/full/sundog.st'),toolSHA256=sha(out/'tool-at-start.py'),events=events,images=images,replay=clock,unchangedBusinessFields=fields,finalSaveSHA256=sha(out/'save.json'),settingsSHA256=sha(out/'settings.json'),limitations=['正常F8HD，交易主頁/報價/賣出來源/入口/倉庫/存放/取回頁只切頁；無選貨、買賣、倉儲消耗或重抽報價。','1280繁中，Ground F5/M/1/S真庫與Q新程序重開，沒有注入state/seed/資金。','只證明本候選正常頁面抽樣，不新增原版GUI或其他平台聲明。'],visualReview='pending-single-read'))
 print('PASS TRADE',len(events),len(images),clock['advancedVBLs'],clock['fieldCount'],flush=True)
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
