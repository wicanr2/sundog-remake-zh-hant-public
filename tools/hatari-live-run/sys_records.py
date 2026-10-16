import gzip, json, sys, os
R=0x1b089; BASE=0x21368; G11F=BASE+0x11f*2
REPO='/work' if os.path.isdir('/work/docs') else '/home/anr2/cht/sundog'
wd=json.load(open(REPO+'/docs/atari-st/world-data.json'))['tables']['systems']['records']
b=gzip.open(sys.argv[1],'rb').read()
for i,s in enumerate(wd):
    o=R+G11F+i*20
    words=[b[o+2*k]<<8|b[o+2*k+1] for k in range(10)]
    cw=words[8]; X,Y,Z,rt=cw&15,(cw>>4)&15,(cw>>8)&15,cw>>12
    fx,fy,fz=s['coordinates']
    print('%-10s 出廠 %s 0x%04x  執行時 word8=0x%04x → (X,Y,Z)=(%2d,%2d,%2d) rating=%d  預期畫面 (%d,%d) r=%d' % (s['name'],(fx,fy,fz),int(s['coordinateWord'],16),cw,X,Y,Z,rt,X*12+8+96,Y*10+8+6,6-Z//2))
print('record0 words:', ['%04x'%w for w in [b[R+G11F+2*k]<<8|b[R+G11F+2*k+1] for k in range(10)]])
