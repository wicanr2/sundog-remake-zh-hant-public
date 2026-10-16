import hashlib,json,os,shutil,subprocess,time,sys
from pathlib import Path
from datetime import datetime,timezone
from PIL import Image
assert Path('/.dockerenv').is_file()
mode=sys.argv[1];assert mode in ['insufficient','ride']
base=Path('/out');out=base/(mode+'-r1');assert not out.exists();out.mkdir();assert out.stat().st_uid==1000
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
binary=Path('/src/workplace/hd-full-production-20261015/sundog-hd-candidate-r3');assert sha(binary)=='3f9e31ab2b82c98497686e992584f54366e203a4c72fa9a922b7cba34b9167d0'
source=Path('/src/workplace/supply-phase44-phase6-n-ljones-r1-20261005/library-03-supply-exchange-position-step-18.json') if mode=='insufficient' else Path('/src/workplace/supply-phase44-phase3-sosai8-r1-20261004/library-03-supply-exchange-position-step-18.json')
source_sha=sha(source);assert source_sha==('453187e894fee041541bcab8ff9f5f9a8f9fa24e040bf03eab4b80b817db3229' if mode=='insufficient' else '9f70248d0c6a65f694ffe2399f1172f2694337b3347c9d9dffc1710801c1550c')
shutil.copyfile(source,out/'library.json');shutil.copyfile(__file__,out/'tool-at-start.py')
(out/'settings.json').write_text(json.dumps(dict(schema=1,theme='atari-st-original',tileset='atari-st-tiles',layout='atari-st')))
events=[];images=[];polls=[];app=None;window=None;keys=set();process=0;index=0
env=dict(os.environ,DISPLAY=':140',HOME='/tmp')
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
 cmd=['go','run','/src/workplace/hd-full-production-20261015/player/c-tube/'+out.name+'/replay.go',str(out/'resume-before.json'),str(ticks)]
 r=subprocess.run(cmd,cwd='/src',env=dict(env,GOCACHE='/cache',GOMODCACHE='/mod',GOPROXY='off',GOTOOLCHAIN='local'),capture_output=True,text=True,timeout=120);assert r.returncode==0,r.stderr
 expected=json.loads(r.stdout);fields=[k for k in before if k not in ['messageKey','saved','quitRequested']];diff=[k for k in fields if after.get(k)!=expected.get(k)];result=dict(advancedVBLs=ticks,fieldCount=len(fields),fields=fields,differences=diff,command=cmd);write('resume-result.json',result);write('resume-expected.json',expected);assert not diff,diff;return result
def facts(path,label):
 cmd=['go','run','/src/workplace/hd-full-production-20261015/player/c-tube/facts.go',str(path)]
 r=subprocess.run(cmd,cwd='/src',env=dict(env,GOCACHE='/cache',GOMODCACHE='/mod',GOPROXY='off',GOTOOLCHAIN='local'),capture_output=True,text=True,timeout=60);assert r.returncode==0,r.stderr
 value=json.loads(r.stdout);write(label+'-facts.json',value);event('formal-tube-readonly-facts',label=label,command=cmd,value=value);return value
def tube_ready(label,kind):
 deadline=time.monotonic()+20;good=0
 while time.monotonic()<deadline:
  assert app.poll() is None;r=frame(label);im=Image.open('/tmp/terminal-frame.png').convert('RGB')
  blue=sum(1 for x in range(74,231) for y in range(55,123) if (lambda c:c[2]>180 and c[0]<80 and c[1]<140)(im.getpixel((x*4,y*4))))
  red=sum(1 for x in range(76,221) for y in range(95,116) if (lambda c:c[0]>180 and c[1]<100 and c[2]<100)(im.getpixel((x*4,y*4))))
  black=sum(1 for x in range(77,227) if max(im.getpixel((x*4,110*4)))<30)
  ready=blue>100 and black>100 if kind in ['list','transport'] else red>3
  good=good+1 if ready else 0;event('tube-region-observation',label=label,contentKind=kind,blueSamples=blue,redSamples=red,actualMenuBlackSamples=black,pollIndex=r['index'])
  if good>=2:frame(label+'-content-ready',True);return
  time.sleep(.05)
 raise RuntimeError('tube actual content not ready '+kind)
def library_save():
 key('F5');visible('17-F5-library');old=(out/'library.json').stat().st_mtime_ns;key('1');down('s');deadline=time.monotonic()+20
 try:
  while time.monotonic()<deadline:
   if (out/'library.json').stat().st_mtime_ns!=old:
    s=json.loads((out/'library.json').read_text())['slots']['slot-1'];assert s['screen']=='ground';shutil.copyfile(out/'library.json',out/'18-real-arrived-library.json');event('actual-F5-S-library-write',mtimeNS=(out/'library.json').stat().st_mtime_ns,SHA256=sha(out/'library.json'),state=s);frame('18-library-saved',True);return s
   time.sleep(.05)
  raise RuntimeError('library actual mtime write missing')
 finally:up('s')
display=subprocess.Popen(['Xvfb',':140','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
try:
 time.sleep(.4);start(True);select_hd();initial=facts(source,'00-source');assert initial['city']=='n-ljones' and initial['position']==dict(x=5,y=3)
 key('b');tube_ready('07-HD-tube-destination-list','list');listed=quit_save('08-HD-tube-list-real-save');assert listed['screen']=='tube-station' and listed.get('tubePage',0)==0
 start();tube_ready('09-HD-tube-list-reopened','list');key('1');tube_ready('10-HD-tube-confirm','insufficient' if mode=='insufficient' else 'transport');confirmed=quit_save('11-HD-tube-confirm-real-save');assert confirmed['screen']=='tube-station' and confirmed['tubePage']==1
 info=facts(out/'11-HD-tube-confirm-real-save.json','12-confirm');selected=info['selected'];fare=selected['Destination']['Fare'];target=selected['Destination']['ID'];assert target==initial['destinations'][0]['ID']
 start();tube_ready('13-HD-tube-confirm-reopened','insufficient' if mode=='insufficient' else 'transport')
 if mode=='insufficient':
  assert not selected['Affordable'] and confirmed['bank']==0 and confirmed['cash']==initial['cash'];before=quit_save('14-HD-insufficient-before-reopen');start();tube_ready('15-HD-insufficient-final-reopened','insufficient');after=quit_save('16-HD-insufficient-after-reopen');clock=replay(before,after)
 else:
  assert selected['Affordable'] and info['bank']>=fare;key('t');visible('14-HD-tube-arrived');arrived=library_save();assert arrived['cityId']==target and arrived['bank']==confirmed['bank']-fare and arrived['cash']==confirmed['cash'] and arrived['shipCityId']==confirmed['shipCityId'];event('one-normal-tube-ride-verified',fare=fare,fromCity=confirmed['cityId'],toCity=arrived['cityId'],bankBefore=confirmed['bank'],bankAfter=arrived['bank'],cashUnchanged=arrived['cash']);key('Escape');visible('19-arrived-library-return');before=quit_save('20-HD-arrived-before-reopen');start();after=quit_save('21-HD-arrived-after-reopen');clock=replay(before,after)
 assert sha(source)==source_sha and sha(binary)=='3f9e31ab2b82c98497686e992584f54366e203a4c72fa9a922b7cba34b9167d0';assert not any('panic:' in q.read_text() for q in out.glob('game-*.log'))
 write('receipt.json',dict(status='PASS-HD-C-tube-'+mode,source=str(source),sourceSHA256=source_sha,binarySHA256=sha(binary),diskSHA256=sha('/full/sundog.st'),toolSHA256=sha(out/'tool-at-start.py'),events=events,images=images,replay=clock,finalSaveSHA256=sha(out/'save.json'),settingsSHA256=sha(out/'settings.json'),initialFacts=initial,confirmedFacts=info,fare=fare,target=target,limitations=['正式Tube以Bank付費，Cash不等於可搭乘；不足分支未按T，正Bank來源只搭一次。','1280繁中正常Title/Space/C/1/L/F8/B/1與真保存重開，沒有state/seed/資金注入。','只證明本候選正常車站玩家鏈，不新增原版GUI或其他平台聲明。'],visualReview='pending-single-read'))
 print('PASS',mode,len(events),len(images),fare,target,clock['advancedVBLs'],clock['fieldCount'],flush=True)
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
