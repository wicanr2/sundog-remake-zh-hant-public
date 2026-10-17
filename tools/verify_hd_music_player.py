#!/usr/bin/env python3
"""Docker正常保存入口：五語音樂選項、F6、重開及實際ALSA混音輸出。入口 assets/music/README.md。"""
import argparse,hashlib,json,os,re,shutil,signal,subprocess,time,wave
from pathlib import Path
from PIL import Image

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for name in ('binary','disk','save','settings','output'):p.add_argument('--'+name,type=Path,required=True)
 a=p.parse_args();assert Path('/.dockerenv').is_file();assert not a.output.exists()
 sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
 source={k:sha(getattr(a,k)) for k in ('binary','disk','save','settings')}
 s=json.loads(a.save.read_text())['state'];assert s['schema']==45 and s['screen']=='shipboard'
 out=a.output.resolve();out.mkdir(parents=True);assert out.stat().st_uid==os.getuid()
 for name in ('save','settings'):shutil.copy2(getattr(a,name),out/(name+'.json'))
 fifo=out/'runtime.pcm';os.mkfifo(fifo)
 config=out/'alsa.conf';config.write_text('pcm.!default { type file slave.pcm { type null } file "'+str(fifo)+'" format "raw" }\n')
 env=dict(os.environ,DISPLAY=':96',HOME='/tmp',ALSA_CONFIG_PATH=str(config),LC_ALL='C.UTF-8')
 app=display=record=None;events=[];window=None
 def run(*cmd):return subprocess.check_output(list(map(str,cmd)),env=env,text=True,timeout=15).strip()
 def press(key):
  events.append({'key':key,'at':time.monotonic()});run('xdotool','keydown','--clearmodifiers',key);time.sleep(.5);run('xdotool','keyup',key);time.sleep(.8)
 def prefs():return json.loads((out/'settings.json').read_text())
 def expect(key,value):
  until=time.monotonic()+20
  while time.monotonic()<until:
   assert app.poll() is None
   if prefs().get(key)==value:return
   time.sleep(.1)
  raise RuntimeError((key,value,prefs()))
 def capture(name):
  # Ship/nav views contain blinking indicators; only options are static.
  # Compare actual pixels, not encoded PNG bytes, after locale repaint.
  time.sleep(1)
  target=out/(name+'.png')
  if not name.startswith(('options-','music-off-')):
   run('import','-window',window,target);return
  previous=None;deadline=time.monotonic()+15;target=out/(name+'.png')
  while time.monotonic()<deadline:
   run('import','-window',window,target);current=hashlib.sha256(Image.open(target).convert('RGB').tobytes()).hexdigest()
   if current==previous:return
   previous=current;time.sleep(.35)
  raise RuntimeError('capture did not stabilize: '+name)
 def start(index):
  nonlocal app,window,record
  # Oto accepts S16 input but its ALSA driver writes FLOAT_LE after mixing.
  record=subprocess.Popen(['ffmpeg','-v','error','-re','-f','f32le','-ar','44100','-ac','2','-i',str(fifo),'-t','120','-threads','2',str(out/f'playback-{index}.wav')],stdout=subprocess.DEVNULL,stderr=(out/f'recorder-{index}.log').open('w'))
  app=subprocess.Popen([str(a.binary.resolve()),'-load','-scale','4','-locale','zh-Hant','-save',str(out/'save.json'),'-settings',str(out/'settings.json'),'-library',str(out/'library.json'),'-atari-disk',str(a.disk.resolve()),'-audio',str(out/'missing-original-sfx')],env=env,stdout=(out/f'game-{index}.log').open('w'),stderr=subprocess.STDOUT)
  until=time.monotonic()+25
  while time.monotonic()<until:
   assert app.poll() is None
   r=subprocess.run(['xdotool','search','--onlyvisible','--pid',str(app.pid)],env=env,text=True,capture_output=True)
   if r.returncode==0 and r.stdout.strip():window=r.stdout.strip().splitlines()[-1];run('xdotool','windowfocus','--sync',window);time.sleep(3);return
   time.sleep(.1)
  raise RuntimeError('game window missing')
 def finish():
  nonlocal record
  press('q');assert app.wait(timeout=15)==0
  assert record.wait(timeout=15)==0;record=None
 try:
  display=subprocess.Popen(['Xvfb',':96','-screen','0','1280x800x24','-nolisten','tcp'],stdout=(out/'xvfb.log').open('w'),stderr=subprocess.STDOUT);time.sleep(.7)
  start(1);capture('shipboard-on');time.sleep(3)
  press('F2')
  for _ in range(4):press('Down')
  press('Return');expect('musicEnabled',False);time.sleep(1.5);capture('music-off-zh-Hant')
  for _ in range(4):press('Up')
  for locale in ('zh-Hans','ja','ko','en','zh-Hant'):
   press('Right');expect('locale',locale);capture('options-'+locale)
  press('Escape');finish()
  assert prefs()['musicEnabled'] is False
  start(2);assert prefs()['musicEnabled'] is False;capture('reopened-off')
  assert 'HD配樂：' not in (out/'game-2.log').read_text()
  press('F6');expect('musicEnabled',True);time.sleep(3)
  press('n');time.sleep(3);capture('navigation-on')
  press('Escape');press('e');time.sleep(3);capture('engineering-on');press('Escape')
  press('F2');capture('options-on');press('Escape');finish()
  assert 'HD配樂：ship' in (out/'game-1.log').read_text()
  assert 'HD配樂：flight' in (out/'game-2.log').read_text()
  assert '沒有音效可播' in (out/'game-1.log').read_text()
  audio=[]
  for index in (1,2):
   path=out/f'playback-{index}.wav'
   with wave.open(str(path)) as w:
    assert w.getnchannels()==2 and w.getframerate()==44100;frames=w.getnframes();pcm=w.readframes(frames);assert any(pcm)
   check=subprocess.run(['ffmpeg','-hide_banner','-i',str(path),'-af','volumedetect,silencedetect=noise=-55dB:d=2','-f','null','-'],text=True,capture_output=True,check=True)
   (out/f'audio-{index}.log').write_text(check.stderr);assert 'max_volume: -inf' not in check.stderr
   peak=float(re.search(r'max_volume: (-?[0-9.]+) dB',check.stderr)[1]);assert peak< -6,('mixed output clipped or wrong capture format',peak)
   if index==1:assert 'silence_start:' in check.stderr,'music off did not reach silence'
   audio.append({'path':path.name,'sha256':sha(path),'frames':frames,'seconds':frames/44100})
  for k,h in source.items():assert sha(getattr(a,k))==h
  result={'status':'PASS-normal-GUI-and-ALSA-output','sourceSHA256':source,'locales':['zh-Hant','zh-Hans','ja','ko','en'],'defaultOn':True,'optionsOff':True,'reopenedOff':True,'F6On':True,'navigationAndEngineering':True,'withoutOriginalSFX':True,'audio':audio,'events':events,'listeningReview':'pending; ALSA capture proves playback, not human listening','scope':'real schema45 shipboard save loaded normally; no state injection; original sources unchanged'}
  (out/'receipt.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print('PASS normal music controls, five locales, persistence and actual PCM output')
 finally:
  for proc in (app,record,display):
   if proc is not None and proc.poll() is None:
    proc.send_signal(signal.SIGINT if proc is record else signal.SIGTERM)
    try:proc.wait(timeout=8)
    except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=5)
if __name__=='__main__':main()
