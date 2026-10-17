#!/usr/bin/env python3
"""Docker-only mouse card capture in five languages and three presentations."""
import argparse,hashlib,json,os,shutil,subprocess,time
from pathlib import Path
from PIL import Image,ImageDraw

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser(description=__doc__)
 for n in ('binary','disk','save','settings','output'):p.add_argument('--'+n,type=Path,required=True)
 a=p.parse_args();assert Path('/.dockerenv').exists();assert not a.output.exists();a.output.mkdir()
 out=a.output;source={n:sha(getattr(a,n)) for n in ('binary','disk','save','settings')}
 for n in ('save','settings'):shutil.copyfile(getattr(a,n),out/(n+'.json'))
 env=dict(os.environ,DISPLAY=':152',HOME='/tmp',LC_ALL='C.UTF-8',XDG_CONFIG_HOME=str(out/'config'))
 app=display=None;events=[];shots=[];scale=4;window=None
 def run(*args):return subprocess.check_output(list(map(str,args)),env=env,text=True,timeout=15).strip()
 def click(x,y):
  events.append(dict(mouse=[x,y]));(out/'inputs.json').write_text(json.dumps(events)+'\n')
  run('xdotool','mousemove','--window',window,x*scale,y*scale);time.sleep(.08);run('xdotool','mousedown',1);time.sleep(.6);run('xdotool','mouseup',1);time.sleep(.65)
 def shot(name=None):
  path=out/((name or 'current')+'.png');run('import','-window',window,path)
  if name:shots.append(name)
  return Image.open(path).convert('RGB')
 def card():return shot().getpixel((20*scale,16*scale))==(74,164,240)
 def menu(system=False):
  end=time.monotonic()+10
  while not card() and time.monotonic()<end:click(299,8)
  assert card();click(230 if system else 80,191)
 def choose(i):click(90+(i%2)*140,63+(i//2)*19)
 def prefs():return json.loads((out/'settings.json').read_text())
 try:
  display=subprocess.Popen(['Xvfb',':152','-screen','0','1600x1000x24','-nolisten','tcp','-noreset'],env=env,stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT);time.sleep(.7)
  app=subprocess.Popen([str(a.binary),'-load','-scale','4','-atari-disk',str(a.disk),'-save',str(out/'save.json'),'-settings',str(out/'settings.json'),'-library',str(out/'library.json')],env=env,stdout=(out/'game.log').open('w'),stderr=subprocess.STDOUT)
  for i in range(120):
   result=subprocess.run(['xdotool','search','--onlyvisible','--name','SunDog'],env=env,text=True,capture_output=True)
   if result.returncode==0 and result.stdout.strip():window=result.stdout.strip().splitlines()[-1];break
   assert app.poll() is None;time.sleep(.2)
  else:raise RuntimeError('no game window')
  run('xdotool','windowmove',window,0,0);run('xdotool','windowfocus',window);time.sleep(2)
  assert prefs()['locale']=='zh-Hant' and prefs()['presentation']=='atari-st-hd'
  cases=[]
  for locale in ('zh-Hant','zh-Hans','ja','ko','en'):
   assert prefs()['locale']==locale
   for presentation in ('atari-st-hd','apple-ii','atari-st'):
    assert prefs()['presentation']==presentation
    menu();name=locale+'-'+presentation;shot(name)
    cases.append(dict(locale=locale,presentation=presentation,screenshot=name+'.png',sha256=sha(out/(name+'.png'))))
    menu(True);choose(4) # Cycle presentation; the card stays open.
   menu(True);choose(3) # Options, with a mouse entry from a running game.
   click(150,40) # Next locale.
   menu();choose(6) # Close the options via its own card.
  # Resize with an odd border, then use the visible action launcher at its
  # new mapped position. The static test covers all button centres at each size.
  run('xdotool','windowsize',window,1400,900);time.sleep(1)
  click(299+15,8+12) # centred 1280x800 canvas: origin 60,50 pixels.
  im=shot('resized-card');assert im.size==(1400,900) and im.getpixel((140,114))==(74,164,240)
  # A mouse-only quit; no synthetic key is used in this verifier.
  run('xdotool','mousemove','--window',window,980,814);run('xdotool','mousedown',1);time.sleep(.6);run('xdotool','mouseup',1);time.sleep(.65)
  run('xdotool','mousemove','--window',window,980,530);run('xdotool','mousedown',1);time.sleep(.6);run('xdotool','mouseup',1)
  assert app.wait(timeout=15)==0;app=None
  for n,h in source.items():assert sha(getattr(a,n))==h
  (out/'receipt.json').write_text(json.dumps(dict(status='PASS-mouse-locales-and-presentations',sourceSHA256=source,cases=cases,keyboardEvents=0,resize=[1400,900],inputs=len(events),visualReview='pending'),ensure_ascii=False,indent=2)+'\n')
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
