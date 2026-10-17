#!/usr/bin/env python3
"""Docker-only, mouse-only normal new-game/save/reopen verification.

Requires a candidate binary and model exported by TestMouseControlsExposeKeyboardActionsAndGeometry.
The model describes visible buttons; it is never supplied to the game.
"""
import argparse,hashlib,json,os,shutil,signal,subprocess,time
from pathlib import Path
from PIL import Image,ImageDraw

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in ('binary','model','disk','output','root'):p.add_argument('--'+n,type=Path,required=True)
 p.add_argument('--resume-fixture',type=Path,help='Verify title-screen mouse loading from a completed normal run, including the single-save fallback.')
 a=p.parse_args();assert Path('/.dockerenv').exists();assert not a.output.exists()
 out=a.output;out.mkdir();assert out.stat().st_uid==os.getuid()
 sources={n:dict(path=str(getattr(a,n)),sha256=sha(getattr(a,n))) for n in ('binary','model','disk')}
 # The candidate receives the same read-only assets as a packaged build.
 for n in ('assets','docs'):
  target=a.binary.parent/n
  if not target.exists():target.symlink_to(a.root/n,target_is_directory=True)
 cwd=out/'empty-cwd';cwd.mkdir();(cwd/'原版磁碟').symlink_to(a.disk.parent,target_is_directory=True)
 env=dict(os.environ,DISPLAY=':150',HOME='/tmp',XDG_CONFIG_HOME=str(out/'config'),LC_ALL='C.UTF-8')
 model=json.loads(a.model.read_text());events=[];states=[];shots=[];app=display=None;window=None;scale=4
 def write(n,data):(out/n).write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
 def run(*cmd):return subprocess.check_output(list(map(str,cmd)),env=env,text=True,timeout=15).strip()
 def image(name=None):
  dest=out/((name or 'current')+'.png');run('import','-window',window,dest)
  if name:shots.append(name)
  return Image.open(dest).convert('RGB')
 def card_open():return image().getpixel((20*scale,16*scale))==(74,164,240)
 def click(x,y,hold=.6,close_ack=False):
  events.append(dict(mouse=[x,y],scale=scale,button=1,holdSeconds=hold));write('inputs.json',events)
  run('xdotool','mousemove','--window',window,x*scale,y*scale);time.sleep(.08);run('xdotool','mousedown',1);time.sleep(hold)
  if close_ack:
   end=time.monotonic()+10
   while app.poll() is None and card_open() and time.monotonic()<end:time.sleep(.1)
   assert app.poll()==0 or not card_open(),'button press was not acknowledged before release'
  run('xdotool','mouseup',1);time.sleep(.65)
  assert app.poll() is None or app.returncode==0,'game crashed'
 def menu(system=False):
  deadline=time.monotonic()+10
  while not card_open() and time.monotonic()<deadline:click(299,8)
  assert card_open(),'actions card did not open'
  click(230 if system else 80,191)
 def choose(index,close_ack=False):
  for i in range(index//12):click(284,176)
  index%=12;click(90+(index%2)*140,63+(index//2)*19,close_ack=close_ack)
 def action(name,screen,hold=.6):
  menu();choices=model[screen];i=next(i for i,b in enumerate(choices) if b['action']==name)
  for page in range(i//12):click(284,176)
  keep=name.startswith(('previous-','next-','interior-')) or name in ('increase-attribute','decrease-attribute')
  i%=12;click(90+(i%2)*140,63+(i//2)*19,hold,close_ack=not keep)
 def system(name):
  menu(True)
  # In a running game, system order is shared across all presentations.
  names=['help','music','sfx','options','presentation','library','save','quit']
  choose(names.index(name),close_ack=name not in ('music','sfx','presentation'))
 def snapshot(name,screen):
  system('save');data=json.loads((out/'save.json').read_text())['state']
  assert data['schema']==45 and data['screen']==screen,(name,data['screen'],screen)
  states.append(dict(name=name,state=data));write('states.json',states);image(name);return data
 def launch(load=False):
  nonlocal app,window
  args=[str(a.binary),'-scale',str(scale),'-save',str(out/'save.json'),'-settings',str(out/'settings.json'),'-library',str(out/'library.json')]
  if load:args+=['-load']
  app=subprocess.Popen(args,cwd=cwd,env=env,stdout=(out/('game-reopen.log' if load else 'game.log')).open('w'),stderr=subprocess.STDOUT)
  for i in range(120):
   r=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,text=True,capture_output=True)
   if r.returncode==0 and r.stdout.strip():window=r.stdout.strip().splitlines()[-1];break
   assert app.poll() is None;time.sleep(.2)
  else:raise RuntimeError('game window unavailable')
  run('xdotool','windowmove',window,0,0);run('xdotool','windowfocus',window);time.sleep(1.5)
 try:
  display=subprocess.Popen(['Xvfb',':150','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],env=env,stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT);time.sleep(.7)
  if a.resume_fixture:
   fixture=a.resume_fixture
   expected=json.loads((fixture/'save.json').read_text())['state']
   fixture_hash={n:sha(fixture/(n+'.json')) for n in ('save','settings','library')}
   fields=['schema','character','attributes','attributeMax','cityId','systemId','planetId','tileset','layout','parts','stores','holds','position','groundView','fuel','bank','cash','hullCondition','cargoCityId','cargoPosition']
   for label,use_library in [('title-library',True),('title-single-save',False)]:
    for n in ('save','settings'):shutil.copyfile(fixture/(n+'.json'),out/(n+'.json'))
    if use_library:shutil.copyfile(fixture/'library.json',out/'library.json')
    elif (out/'library.json').exists():(out/'library.json').unlink()
    launch();image(label+'-before')
    action('load-game','title');loaded=snapshot(label+'-loaded','ground')
    for field in fields:assert expected[field]==loaded[field],(label,field)
    system('quit');assert app.wait(timeout=15)==0;app=None
   for n,h in fixture_hash.items():assert sha(fixture/(n+'.json'))==h
   write('receipt.json',dict(status='PASS-mouse-title-load-and-single-save-fallback',sources=sources,fixtureSHA256=fixture_hash,reopenFields=fields,keyboardEvents=0,inputs=len(events),scope='Two normal title-screen launches; click Load through Actions; existing library and absent-library single-save fallback; no -load flag or state injection'))
   return
  launch();image('01-first-disk-gate')
  # No disk flag, no settings, no keyboard. Open the path's real file picker.
  click(100,86);assert card_open();image('02-disk-browser')
  choose(1) # Unicode-named directory symlink, after Parent folder.
  files=[]
  for pth in a.disk.parent.iterdir():
   if pth.is_dir() or pth.suffix.lower()=='.st':files.append(pth)
  files.sort(key=lambda p:(not p.is_dir(),p.name.lower()))
  choose(1+files.index(a.disk));time.sleep(1)
  assert json.loads((out/'settings.json').read_text())['atariDisk']==str(a.disk.parent/'sundog.st') or Path(json.loads((out/'settings.json').read_text())['atariDisk']).resolve()==a.disk.resolve()
  # First-look card: HD is the second preset, selected with its normal row.
  click(100,90);time.sleep(1);image('03-hd-title')
  assert json.loads((out/'settings.json').read_text())['presentation']=='atari-st-hd'
  action('select-game','title');image('04-library')
  action('make-library','character-library')
  action('new-game','character-library')
  menu();choose(0);image('05-name-keyboard')
  # Enter "Quinn" using only the on-screen keyboard, including a Q.
  click(56,161) # Upper case.
  def letter(c):
   chars='ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-\'';i=chars.index(c.upper());click(35+(i%10)*28,71+(i//10)*21)
  letter('Q');click(56,161)
  for c in 'uinn':letter(c)
  click(267,161);image('06-attributes')
  action('increase-attribute','attribute-allocation',hold=1.5)
  action('next-attribute','attribute-allocation');action('increase-attribute','attribute-allocation')
  # Original confirmation explicitly allows abandoning remaining points.
  action('confirm-attributes','attribute-allocation');action('confirm-attributes','attribute-allocation')
  s=snapshot('07-new-ship','shipboard');assert s['character']=='Quinn' and s['attributes'][0]>30 and s['attributes'][1]>30
  action('open-cargo','shipboard');snapshot('08-cargo','cargo');action('back','cargo')
  action('open-engineering','shipboard');s0=snapshot('09-engineering','engineering')
  action('next-system','engineering');s1=snapshot('10-engineering-next','engineering');assert s0['selectedSystemIndex']!=s1['selectedSystemIndex']
  action('back','engineering');action('open-navigation','shipboard');snapshot('11-navigation','navigation')
  action('next-destination','navigation');snapshot('12-navigation-next','navigation');action('back','navigation')
  action('open-stores','shipboard');snapshot('13-stores','stores');action('back','stores')
  action('open-ground','shipboard');time.sleep(.5)
  menu();choose(0) # Animation's Skip button is mouse accessible.
  before=snapshot('14-ground','ground')
  click(220,100,hold=.8);after=snapshot('15-ground-moved','ground')
  assert before['groundView']!=after['groundView'],'mouse ground walk did not change view'
  action('open-status','ground');snapshot('16-ground-status','character-status');action('back','character-status')
  # Global save library entry is mouse accessible from the ground.
  system('library');action('store-game','character-library');image('17-library-stored')
  assert json.loads((out/'library.json').read_text())['slots']
  action('back','character-library')
  system('options');image('18-options')
  click(150,96);click(299,8);menu() # Same music row as F2, then actions card.
  # Close options through the card, preserving the selected music preference.
  choices=['browse','detect','locale','presentation','music','sfx','options-close'];choose(choices.index('options-close'))
  assert json.loads((out/'settings.json').read_text())['musicEnabled'] is False
  system('music');click(280,26)
  assert json.loads((out/'settings.json').read_text())['musicEnabled'] is True
  system('sfx');click(280,26)
  final=snapshot('19-final-ground','ground')
  system('quit');assert app.wait(timeout=15)==0;app=None
  launch();action('load-game','title');reopened=snapshot('20-reopened','ground')
  fields=['schema','character','attributes','attributeMax','cityId','systemId','planetId','tileset','layout','parts','stores','holds','position','groundView','fuel','bank','cash','hullCondition','cargoCityId','cargoPosition']
  for f in fields:assert final[f]==reopened[f],f
  changed={k for k in final if final[k]!=reopened.get(k)}
  assert changed<={'awakeClock','timeMinutes'},changed
  system('quit');assert app.wait(timeout=15)==0;app=None
  for n,record in sources.items():assert sha(getattr(a,n))==record['sha256']
  assert all('key' not in e for e in events)
  write('receipt.json',dict(status='PASS-mouse-only-normal-player',sources=sources,inputs=len(events),states=len(states),reopenFields=fields,unchangedFieldCount=len(final)-len(changed),elapsedClockFields=sorted(changed),screenshots=shots,keyboardEvents=0,scope='new player, mouse file browsing, first HD choice, name, attributes, ship/cargo/engineering/navigation/stores/ground/status, library, settings/music/SFX, quit and reopen; no state injection; not a full original parity or all-transaction claim'))
 finally:
  for proc in (app,display):
   if proc is not None and proc.poll() is None:
    proc.terminate()
    try:proc.wait(timeout=8)
    except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=5)
  if shots:
   sheet=Image.new('RGB',(960,((len(shots)+2)//3)*220),(24,24,24));d=ImageDraw.Draw(sheet)
   for i,name in enumerate(shots):
    im=Image.open(out/(name+'.png'));im.thumbnail((320,200));x=(i%3)*320;y=(i//3)*220;sheet.paste(im,(x,y));d.text((x+3,y+201),name,fill='white')
   sheet.save(out/'contact.png')
if __name__=='__main__':main()
