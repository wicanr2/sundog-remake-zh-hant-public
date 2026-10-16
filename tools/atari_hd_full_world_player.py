import hashlib,json,os,shutil,subprocess,time,sys
from pathlib import Path
from datetime import datetime,timezone
from PIL import Image,ImageChops
assert Path('/.dockerenv').is_file()
mode=sys.argv[1];assert mode=='world'
base=Path('/out');out=base/(mode+'-r5');assert not out.exists();out.mkdir();assert out.stat().st_uid==1000
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
binary=Path('/src/workplace/hd-full-production-20261015/sundog-hd-candidate-r6');assert sha(binary)=='d5e047512c919957903799a28a8591ecc4fb30cc971bc1d992650f4e25d72976'
source=Path('/src/workplace/hd-full-production-20261015/player/c-world/world-r3/07-initial-library.json')
source_sha=sha(source);assert source_sha=='5ac0735b2ef949b45360cff75fe095ec13332b93e2e18fa25caf3f0d03f11a0c'
shutil.copyfile(source,out/'library.json');shutil.copyfile(__file__,out/'tool-at-start.py')
shutil.copyfile('/src/workplace/hd-full-production-20261015/player/c-world/world-r3/settings.json',out/'settings.json')
events=[];images=[];polls=[];app=None;window=None;keys=set();process=0;index=0
env=dict(os.environ,DISPLAY=':142',HOME='/tmp')
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
 white=ImageChops.darker(ImageChops.darker(*im.split()[:2]),im.split()[2]).point(lambda v:255 if v>240 else 0).histogram()[255];count=sum(max(c)>110 for c in im.resize((80,50)).getdata()); bands=sum(1 for x in [154,162] for y in range(90,199) if (lambda c:c[2]>180 and c[0]<80 and c[1]<140)(im.getpixel((x*4,y*4))));r=dict(index=index,label=label,startNS=t0,endNS=time.monotonic_ns(),SHA256=sha(p),brightSampleCount=count,rawWhitePixelCount=white,libraryHeader=(min(im.getpixel((8,8)))>170 and (lambda c:c[0]>120 and c[1]<130 and c[2]<130)(im.getpixel((8,75)))),blueBandSamples=bands,corner=im.getpixel((0,0)),contentSHA256=hashlib.sha256(im.crop((640,352,1276,748)).tobytes()).hexdigest())
 if retain:
  f=out/f'{index:03d}-{label}.png';shutil.copyfile(p,f);r['image']=f.name;images.append(dict(path=f.name,SHA256=sha(f),visualReview='pending-single-read'));write('images.json',images)
 polls.append(r);write('polls.json',polls);return r
def visible(label):
 deadline=time.monotonic()+20;stable=0
 while time.monotonic()<deadline:
  exitcode=app.poll();event('unexpected-runtime-exit',exitCode=exitcode) if exitcode is not None else None;assert exitcode is None,'early exit';r=frame(label);stable=stable+1 if (r['rawWhitePixelCount']>0 or r['libraryHeader']) else 0
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
 event('normal-runtime-exit',exitCode=app.returncode);assert app.returncode==0;s=json.loads((out/'save.json').read_text())['state'];assert s['tileset']=='atari-st-hd-tiles';shutil.copyfile(out/'save.json',out/(label+'.json'));event('normal-Q-save-exit-ack',name=label,SHA256=sha(out/'save.json'),state=s);return s
def replay(before,after):
 ticks=(after['awakeClock']['source']['ticks']-before['awakeClock']['source']['ticks'])%2**32;assert ticks<600
 go='''package main
import("encoding/json";"os";"strconv";"github.com/wicanr2/sundog-remake/internal/game")
func main(){b,e:=os.ReadFile(os.Args[1]);if e!=nil{panic(e)};var s game.State;if e=json.Unmarshal(b,&s);e!=nil{panic(e)};n,_:=strconv.Atoi(os.Args[2]);r,e:=game.NewReducer();if e!=nil{panic(e)};for i:=0;i<n;i++{s,e=r.Step(s,game.ActionVBLTick);if e!=nil{panic(e)}};json.NewEncoder(os.Stdout).Encode(s)}
'''
 (out/'replay.go').write_text(go);write('resume-before.json',before)
 cmd=['go','run','/src/workplace/hd-full-production-20261015/player/c-world/'+out.name+'/replay.go',str(out/'resume-before.json'),str(ticks)]
 r=subprocess.run(cmd,cwd='/src',env=dict(env,GOCACHE='/cache',GOMODCACHE='/mod',GOPROXY='off',GOTOOLCHAIN='local'),capture_output=True,text=True,timeout=120);assert r.returncode==0,r.stderr
 expected=json.loads(r.stdout);fields=[k for k in before if k not in ['messageKey','saved','quitRequested']];diff=[k for k in fields if after.get(k)!=expected.get(k)];result=dict(advancedVBLs=ticks,fieldCount=len(fields),fields=fields,differences=diff,command=cmd);write('resume-result.json',result);write('resume-expected.json',expected);assert not diff,diff;return result
source_receipt=Path('/src/workplace/hd-full-production-20261015/player/c-world/world-r3/failure.json')
steps=[]
def library_probe(label):
 key('F5');visible(label+'-F5-library');
 if not (out/'library.json').exists():
  down('m');deadline=time.monotonic()+20
  try:
   while time.monotonic()<deadline and not (out/'library.json').exists():time.sleep(.05)
   assert (out/'library.json').exists()
  finally:up('m')
 key('1');old=(out/'library.json').stat().st_mtime_ns;down('s');deadline=time.monotonic()+20
 try:
  while time.monotonic()<deadline:
   if (out/'library.json').stat().st_mtime_ns!=old:
    raw=json.loads((out/'library.json').read_text());s=raw['slots']['slot-1'];shutil.copyfile(out/'library.json',out/(label+'-library.json'));event('actual-library-step-probe',label=label,mtimeNS=(out/'library.json').stat().st_mtime_ns,SHA256=sha(out/'library.json'),state=s);break
   time.sleep(.05)
  else:raise RuntimeError('library step probe not written '+label)
 finally:up('s')
 key('Escape');visible(label+'-normal-return');return s
display=subprocess.Popen(['Xvfb',':142','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
try:
 time.sleep(.4);gui_deadline=time.monotonic()+110;start(True);initial=library_probe('07-initial');assert initial['screen']=='ground' and initial['groundMode']=='pod' and not initial['groundWorld'] and initial['position']==dict(x=2,y=-1)
 previous=initial;status='BOUNDED-NOT-WORLD';reason='six inputs exhausted'
 plan=['Up','Up','Up','Up']
 for n,k in enumerate(plan,1):
  assert time.monotonic()<gui_deadline,'110s GUI input bound reached';key(k);visible('08-'+k+'-'+str(n)+'-actual-scene');s=library_probe('09-step-'+str(n));change=any(s.get(k)!=previous.get(k) for k in ['position','groundView','groundWorld']);steps.append(dict(number=n,key=k,state=s,visiblePositionChanged=change));write('steps.json',steps)
  if s['screen']!='ground':status='STOPPED-NATURAL-EVENT';reason=s['screen'];break
  if s['groundWorld']:status='PASS-HD-groundWorld-pod-north';reason='normal city exit';break
  if not change:status='STOPPED-NORMAL-COLLISION';reason='unchanged position and groundView';break
  previous=s
 before=quit_save('10-final-before-reopen');start();after=quit_save('11-final-after-reopen');clock=replay(before,after)
 assert sha(source)==source_sha and sha(binary)=='d5e047512c919957903799a28a8591ecc4fb30cc971bc1d992650f4e25d72976';assert not any('panic:' in q.read_text() for q in out.glob('game-*.log'))
 write('receipt.json',dict(status=status,reason=reason,source=str(source),sourceSHA256=source_sha,sourceReceiptSHA256=sha(source_receipt),binarySHA256=sha(binary),diskSHA256=sha('/full/sundog.st'),toolSHA256=sha(out/'tool-at-start.py'),events=events,images=images,replay=clock,steps=steps,initial=initial,finalSaveSHA256=sha(out/'save.json'),settingsSHA256=sha(out/'settings.json'),limitations=['Normal Title/Space/C/1/L/F8, at most six Up inputs, each observed through actual F5/1/S library writes; no coordinate/seed/state injection.','First actual event or collision stops the route; no fabricated fallback or forced world entry.','Only 1280 Traditional Chinese one north direction, not full exploration or new original GUI parity.'],visualReview='pending-single-read'))
 print(status,reason,len(steps),len(events),len(images),clock['advancedVBLs'],clock['fieldCount'],flush=True)
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
