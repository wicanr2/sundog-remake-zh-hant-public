import hashlib,json,os,shutil,subprocess,time
from datetime import datetime,timezone
from pathlib import Path
from PIL import Image,ImageChops

assert Path('/.dockerenv').is_file()
base=Path('/out');out=base/'title-r5';assert not out.exists();out.mkdir();assert out.stat().st_uid==1000
binary=Path('/src/workplace/hd-full-production-20261015/sundog-hd-candidate-r5')
source=Path('/normal-input/save.json')
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
source_sha=sha(source);binary_sha=sha(binary);shutil.copyfile(binary,out/'candidate-at-start');(out/'candidate-at-start').chmod(0o755);binary=out/'candidate-at-start';shutil.copyfile(__file__,out/'tool-at-start.py')
shutil.copyfile('/out/a5/settings.json',out/'settings.json')
target_point=[62,95]
env=dict(os.environ,DISPLAY=':136',HOME='/tmp');events=[];polls=[];images=[];app=None;window=None;index=0;process_index=0;active_keys=set();mouse_held=False
def write(n,obj):(out/n).write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n')
def event(kind,**kw):
 e=dict(kind=kind,monotonicNS=time.monotonic_ns(),utc=datetime.now(timezone.utc).isoformat(),**kw);events.append(e);write('events.json',events);return e
def run(*a):return subprocess.check_output(list(map(str,a)),env=env,text=True,timeout=20).strip()
def down(k):active_keys.add(k);event('keydown',key=k);run('xdotool','keydown','--clearmodifiers',k)
def up(k):event('keyup',key=k);run('xdotool','keyup',k);active_keys.discard(k)
def normal_key(k):
 down(k);time.sleep(.25);up(k);event('normal-existing-prefix-key',key=k,holdSeconds=.25)
def classify(im):
 h=im.getpixel((8,8));bar=im.getpixel((8,75))
 if min(h)>170 and bar[0]>120 and bar[1]<130 and bar[2]<130:return 'library-or-menu'
 if (min(h)<100 or h==(167,175,184)) and sum(1 for c in im.resize((80,50)).getdata() if max(c)>100)>100:return 'in-game'
 return 'other'
def actor_mask(im):
 red,green,blue=im.split();return ImageChops.darker(ImageChops.darker(red,green),blue).point(lambda v:255 if v>240 else 0)
def target_white(im):
 x,y=target_point[0]*4,target_point[1]*4
 # HD worldPerson torso is 0xf3f6fa. Ignore animated legs for cabin targets.
 box=(x-22,y-22,x+23,y+23) if target_point==[177,100] else (x-5,y-5,x+5,y+5)
 return actor_mask(im).crop(box).histogram()[255]
def frame(label,retain=False):
 global index
 index+=1;t0=time.monotonic_ns();temp=Path('/tmp/frame.png');run('import','-window',window,temp);t1=time.monotonic_ns();im=Image.open(temp).convert('RGB');assert im.size==(1280,800)
 record=dict(index=index,label=label,startNS=t0,endNS=t1,SHA256=sha(temp),sceneClass=classify(im),actorRegionSHA256=hashlib.sha256(actor_mask(im).tobytes()).hexdigest(),targetWhiteCount=target_white(im),sentinels={str(p):im.getpixel(p) for p in [(8,8),(8,75),(800,160),(400,320),(1040,360)]})
 if retain:
  f=out/f'{index:03d}-{label}.png';shutil.copyfile(temp,f);record['image']=f.name;images.append(dict(path=f.name,SHA256=sha(f),sceneClass=record['sceneClass'],visualReview='pending-single-read'))
 polls.append(record);write('polls.json',polls);write('images.json',images);return record
def wait_scene(scene,label,bound=20):
 started=time.monotonic();consecutive=0
 while time.monotonic()-started<bound:
  assert app.poll() is None,'程式提早退出'
  r=frame(label);consecutive=consecutive+1 if (r['sceneClass']==scene or (scene=='visible' and sum(c for c,v in Image.open('/tmp/frame.png').convert('RGB').getcolors(1024001) if max(v)>100)>1000)) else 0
  if consecutive>=2:
   r=frame(label+'-ready',True);assert r['sceneClass']==scene or scene=='visible';event('scene-ready',scene=scene,label=label,elapsedSeconds=time.monotonic()-started,pollIndex=r['index']);return r
  time.sleep(.05)
 raise RuntimeError('未觀測到就緒場景 '+scene)
def key_scene(k,scene,label):
 down(k)
 try:return wait_scene(scene,label)
 finally:up(k)
def start(first=False,scene="in-game",label="reopened"):

 global app,window,process_index
 process_index+=1
 command=[str(binary),'-scale','4','-atari-disk','/full/sundog.st','-save',str(out/'save.json'),'-library',str(out/'library.json'),'-settings',str(out/'settings.json'),'-locale','zh-Hant']+([] if first else ['-load'])
 event('start',command=command,sourceSHA256=None if first else sha(out/'save.json'),normalNewGame=first)
 app=subprocess.Popen(command,cwd='/src',env=env,stdout=(out/f'game-{process_index}.log').open('w'),stderr=subprocess.STDOUT)
 deadline=time.monotonic()+20
 while time.monotonic()<deadline:
  assert app.poll() is None
  r=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,text=True,capture_output=True)
  if r.returncode==0 and r.stdout.strip():window=r.stdout.strip().splitlines()[-1];break
  time.sleep(.05)
 else:raise RuntimeError('no window')
 run('xdotool','windowfocus',window)
 geometry=run('xdotool','getwindowgeometry','--shell',window);g=dict(line.split('=',1) for line in geometry.splitlines() if '=' in line);assert g['WIDTH']=='1280' and g['HEIGHT']=='800';event('window',id=window,geometry=geometry)
 wait_scene(scene,'01-source-pilot' if first else label)
def save_library():
 old=(out/'library.json').stat().st_mtime_ns if (out/'library.json').exists() else 0
 key_scene('F5','library-or-menu','05-F5-library')
 if not (out/'library.json').exists():
  down('m');deadline=time.monotonic()+20
  try:
   while not (out/'library.json').exists() and time.monotonic()<deadline:time.sleep(.05)
   assert (out/'library.json').exists()
  finally:up('m')
  old=(out/'library.json').stat().st_mtime_ns
 normal_key('1');down('s');deadline=time.monotonic()+20
 try:
  while time.monotonic()<deadline:
   if (out/'library.json').stat().st_mtime_ns!=old:
    state=json.loads((out/'library.json').read_text())['slots']['slot-1'];assert state['screen']==expected_screen,(state['screen'],expected_screen);assert state['tileset']=='atari-st-hd-tiles';assert state['layout']=='atari-st';shutil.copyfile(out/'library.json',out/'library-after-boarding.json');event('library-write-ack',SHA256=sha(out/'library.json'),state=state);frame('06-library-saved',retain=True);return state
   time.sleep(.05)
  raise RuntimeError('保存庫未寫入')
 finally:up('s')
def quit_save(name):
 down('q')
 try:app.wait(timeout=20)
 finally:up('q')
 assert app.returncode==0
 state=json.loads((out/'save.json').read_text())['state'];shutil.copyfile(out/'save.json',out/(name+'.json'));event('quit-save',name=name,SHA256=sha(out/'save.json'),state=state);return state
xvfb=subprocess.Popen(['Xvfb',':136','-screen','0','1920x1200x24','-nolisten','tcp','-noreset'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT)
def walk_to(point,label,next_scene):
 global target_point,mouse_held
 target_point=point
 initial=frame(label+'-before',retain=True);baseline=initial['targetWhiteCount']
 coords=[v*4 for v in point];event('mousemove',logical=point,windowRelative=coords);run('xdotool','mousemove','--window',window,*coords)
 event('mousedown',button=1);run('xdotool','mousedown','1');mouse_held=True
 deadline=time.monotonic()+25;previous=None;stable=0
 while time.monotonic()<deadline:
  r=frame(label+'-held',retain=previous is None);current=r['targetWhiteCount'];stable=stable+1 if current==previous else 0
  if current>baseline+10 and stable>=2:
   frame(label+'-station-held',retain=True);event('target-white-sprite-ready',point=point,baseline=baseline,count=current,pollIndex=r['index']);break
  previous=current;time.sleep(.05)
 else:raise RuntimeError('目標站位未觀測到人物 '+repr(point))
 event('mouseup',button=1);run('xdotool','mouseup','1');mouse_held=False
 return wait_scene(next_scene,label+'-released')
# Appended to the frozen normal-player event/readiness helper.
model=json.loads(Path('/src/workplace/hd-engineering-review-20261015/model.json').read_text())
slots={('system',0):next(x for x in model['systems'][0]['slots'] if x['row']==0 and x['column']==0),('title',0):dict(x=74,y=68,w=156,h=9,cx=152,cy=72)}
source_state=json.loads(source.read_text())['state']
checks=[];replays=[]
def slot_hash(kind,index):
 s=slots[kind,index];im=Image.open('/tmp/frame.png').convert('RGB');box=tuple(v*4 for v in (s['x']+2,s['y']+2,s['x']+s['w']-2,s['y']+s['h']-2))
 return hashlib.sha256(im.crop(box).tobytes()).hexdigest()
def wait_changed(kind,index,baseline,label,retain=False):
 deadline=time.monotonic()+20;previous=None;stable=0
 while time.monotonic()<deadline:
  frame(label,retain=False);value=slot_hash(kind,index);stable=stable+1 if value==previous else 0
  event('slot-observation',label=label,slotKind=kind,index=index,SHA256=value,changed=value!=baseline,stable=stable)
  if value!=baseline and stable>=2:
   if retain:frame(label+'-ready',retain=True)
   return value
  previous=value
 raise RuntimeError('槽位畫面未改變並穩定 '+repr((kind,index,label)))
def click_slot(kind,index,label,button=1):
 global mouse_held
 frame(label+'-before',retain=False);baseline=slot_hash(kind,index);s=slots[kind,index]
 event('mousemove',logical=[s['cx'],s['cy']],windowRelative=[s['cx']*4,s['cy']*4],modelSlot=[kind,index]);run('xdotool','mousemove','--window',window,s['cx']*4,s['cy']*4)
 event('mousedown',button=button);run('xdotool','mousedown',button);mouse_held=True
 try:
  # Existing normal input pulse, not a scene readiness delay. The held icon
  # covers its source at this cursor position, so no source-ROI ack here.
  time.sleep(.25);event('normal-existing-prefix-mouse',button=button,holdSeconds=.25)
 finally:event('mouseup',button=button);run('xdotool','mouseup',button);mouse_held=False
 # Outside zoom window: held icon goes to the existing hand fallback slot.
 event('mousemove',logical=[10,100],windowRelative=[50,500]);run('xdotool','mousemove','--window',window,50,500)
 return wait_changed(kind,index,baseline,label+'-released',retain=True)
def cancel_drag(kind,index,label):
 global mouse_held
 frame(label+'-before',retain=False);baseline=slot_hash(kind,index)
 event('mousedown',button=3);run('xdotool','mousedown','3');mouse_held=True
 try:wait_changed(kind,index,baseline,label+'-pressed')
 finally:event('mouseup',button=3);run('xdotool','mouseup','3');mouse_held=False
 return wait_changed(kind,index,baseline,label+'-released',retain=True)
def check(name,state,condition):
 checks.append(dict(name=name,result=bool(condition),state=state));write('state-checks.json',checks);assert condition,name
def replay(name,before,after,sleep=False):
 ticks=(after['awakeClock']['source']['ticks']-before['awakeClock']['source']['ticks'])%2**32;assert ticks<10000
 go='''package main
import("encoding/json";"os";"strconv";"github.com/wicanr2/sundog-remake/internal/game")
func main(){b,e:=os.ReadFile(os.Args[1]);if e!=nil{panic(e)};var s game.State;if e=json.Unmarshal(b,&s);e!=nil{panic(e)};n,_:=strconv.Atoi(os.Args[2]);r,e:=game.NewReducer();if e!=nil{panic(e)};for i:=0;i<n;i++{s,e=r.Step(s,game.ActionVBLTick);if e!=nil{panic(e)};if os.Args[3]=="sleep"{s,e=r.Step(s,game.ActionSleepTick);if e!=nil{panic(e)}}};json.NewEncoder(os.Stdout).Encode(s)}
'''
 (out/'replay.go').write_text(go);write(name+'-replay-before.json',before)
 command=['go','run','/src/workplace/hd-full-production-20261015/player/a4/replay.go',str(out/(name+'-replay-before.json')),str(ticks),'sleep' if sleep else 'awake']
 r=subprocess.run(command,cwd='/src',env=dict(env,GOCACHE='/cache',GOMODCACHE='/mod',GOPROXY='off',GOTOOLCHAIN='local'),capture_output=True,text=True,timeout=120);assert r.returncode==0,r.stderr
 expected=json.loads(r.stdout);fields=sorted(k for k in set(before)|set(after)|set(expected) if k not in ['messageKey','saved','quitRequested']);diff=[k for k in fields if after.get(k)!=expected.get(k)]
 result=dict(name=name,command=command,advancedVBLs=ticks,fieldCount=len(fields),fields=fields,differences=diff);replays.append(result);write(name+'-replay-result.json',result);assert not diff,diff



expected_screen='shipboard'
assert binary_sha=='e02c50e63cfc411126ce96b7fc3af934e017c3bfff945ef89a61d54f4be06f48'
try:
 time.sleep(.4);start(True,scene='visible');normal_key('space');wait_scene('visible','02-HD-title-menu')
 normal_key('o');wait_scene('visible','03-HD-options');normal_key('Escape');wait_scene('visible','04-HD-options-return')
 normal_key('a');wait_scene('visible','05-HD-about');normal_key('space');wait_scene('visible','06-HD-about-return')
 key_scene('c','library-or-menu','07-HD-character-library-entry')
 final=quit_save('08-menu-quit')
 write('receipt.json',dict(status='PASS-HD-r5-title-about-options-library-entry',binarySHA256=binary_sha,toolSHA256=sha(out/'tool-at-start.py'),events=events,images=images,settingsSHA256=sha(out/'settings.json'),diskSHA256=sha('/full/sundog.st'),scope='僅r5貨船標題/製作名單/選項/正常角色庫入口；沒有再次N建角、載入既有遊戲或改玩家規則。新局完整鏈沿a4唯一建角→a5真保存續行。',visualReview='pending-single-read'))
 print('PASS title-r5',len(images),flush=True)
except Exception as e:
 if app is not None and app.poll() is None and window is not None:
  try:frame('failure-actual-window',retain=True)
  except Exception:pass
 write('failure.json',dict(error=repr(e),events=events,polls=polls,sourceSHA256=source_sha));raise
finally:
 for k in list(active_keys):
  try:up(k)
  except Exception:pass
 if mouse_held:
  try:run('xdotool','mouseup','1');run('xdotool','mouseup','3')
  except Exception:pass
 if app is not None and app.poll() is None:
  app.terminate()
  try:app.wait(timeout=5)
  except subprocess.TimeoutExpired:app.kill();app.wait(timeout=5)
 xvfb.terminate();xvfb.wait(timeout=5)
