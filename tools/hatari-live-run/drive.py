#!/usr/bin/env python3
"""D4 宿主端控制器：閉迴路把 SunDog 遊戲內游標移到目標並點擊。"""
import subprocess, time, glob, os, sys
from PIL import Image
SP=os.path.dirname(os.path.abspath(__file__))
# 工作目錄通常不在 repo 裡（截圖含原版像素），所以兩者都可由環境變數指定。
SHOTS=os.environ.get('SUNDOG_D4_SHOTS', SP+'/shots')
CONTAINER=os.environ.get('SUNDOG_D4_CONTAINER', 'sundog-d4')
def ex(*a): subprocess.run(['docker','exec','-e','DISPLAY=:99',CONTAINER]+list(a),check=True)
def snap():
    before=set(glob.glob(SHOTS+'/grab*.png'))
    ex('sh','-c','echo "hatari-shortcut screenshot" > /tmp/hfifo')
    for _ in range(40):
        time.sleep(0.3)
        new=set(glob.glob(SHOTS+'/grab*.png'))-before
        if new:
            p=new.pop(); time.sleep(0.2)
            return Image.open(p).convert('RGB'), p
    raise RuntimeError('no screenshot')
def find_cursor(im):
    # SunDog 游標：白色箭頭，尖端在左上。找「白色像素、其左與上為非白」的候選，
    # 驗證尖端向右下延伸的白斜線。
    w,h=im.size; px=im.load()
    def white(x,y): r,g,b=px[x,y]; return r>200 and g>200 and b>200
    best=None
    for y in range(2,h-24,1):
        for x in range(2,w-16,1):
            if not white(x,y): continue
            if white(x-1,y) or white(x,y-1) or white(x-1,y-1): continue
            # 箭頭主體：往下 6 列，每列從 x 起有遞增寬度的白段
            score=0
            for d in range(2,12,2):
                if white(x+d//2,y+d) and white(x+d//4,y+d): score+=1
            if score>=4:
                # 排除星空單點：要求 (x, y+12) 附近仍有白
                if white(x+3,y+12) or white(x+2,y+12): return (x,y)
                if best is None: best=(x,y)
    return best
def move_rel(dx,dy):
    # 分小步送相對位移，模擬實體滑鼠
    step=12
    while dx or dy:
        sx=max(-step,min(step,dx)); sy=max(-step,min(step,dy))
        ex('xdotool','mousemove_relative','--','%d'%sx,'%d'%sy)
        dx-=sx; dy-=sy; time.sleep(0.06)
def click(n=1):
    for _ in range(n):
        ex('xdotool','click','1'); time.sleep(0.15)
def goto(tx,ty,tol=4,tries=10):
    """tx,ty 是 832x552 screenshot 座標的目標（游標尖端）。"""
    for i in range(tries):
        im,p=snap(); cur=find_cursor(im)
        if cur is None:
            print('cursor not found; nudge'); move_rel(15,15); continue
        dx,dy=tx-cur[0],ty-cur[1]
        print(f'cursor at {cur}, delta ({dx},{dy}) [{os.path.basename(p)}]')
        if abs(dx)<=tol and abs(dy)<=tol: return True
        # screenshot 與 host 位移比例先假設 1:1，閉迴路自然收斂
        move_rel(int(dx*0.8) or (1 if dx>0 else -1 if dx<0 else 0),
                 int(dy*0.8) or (1 if dy>0 else -1 if dy<0 else 0))
    return False

def diff_blobs(a,b,thresh=40):
    import numpy as np
    A=np.asarray(a,dtype=int); B=np.asarray(b,dtype=int)
    m=(abs(A-B).sum(axis=2)>thresh)
    ys,xs=m.nonzero()
    if len(xs)==0: return []
    # 連通成分（粗略：以 8px 網格聚類）
    pts=list(zip(xs,ys)); blobs=[]
    for x,y in pts:
        for blob in blobs:
            bx0,by0,bx1,by1=blob['box']
            if bx0-12<=x<=bx1+12 and by0-12<=y<=by1+12:
                blob['box']=(min(bx0,x),min(by0,y),max(bx1,x),max(by1,y)); blob['n']+=1; break
        else:
            blobs.append({'box':(x,y,x,y),'n':1})
    blobs.sort(key=lambda b:-b['n'])
    return blobs

def track_move(dx,dy):
    """移動 (dx,dy)（host px），回傳移動後箭尖的 screenshot 座標。"""
    a,_=snap()
    move_rel(dx,dy); time.sleep(0.3)
    b,_=snap()
    blobs=diff_blobs(a,b)
    if not blobs: return None
    # 挑沿移動方向最遠的 blob 的左上角當新箭尖
    def key(bl):
        x0,y0,x1,y1=bl['box']
        return (x0*(1 if dx>=0 else -1))+(y0*(1 if dy>=0 else -1))
    bl=max(blobs[:2],key=key)
    x0,y0,x1,y1=bl['box']
    return (x0,y0)

def abs_move(hx,hy):
    ex('xdotool','mousemove','%d'%int(hx),'%d'%int(hy)); time.sleep(0.2)

def goto2(tx,ty,tol=8,tries=6):
    # 絕對映射：tip_shot = (host_x*832/800 - 24, host_y - 48)，再閉迴路修正
    hx=(tx+24)*800/832; hy=ty+48
    abs_move(hx,hy)
    for i in range(tries):
        pos=track_move(5,5)   # 小步找目前位置（移動後）
        if pos is None: continue
        dx,dy=tx-pos[0],ty-pos[1]
        print(f'tip {pos} delta ({dx},{dy})')
        if abs(dx)<=tol and abs(dy)<=tol: return True
        hx+=dx*800/832; hy+=dy
        abs_move(hx,hy)
    return False

def goto2_old(tx,ty,tol=8,tries=14):
    pos=track_move(6,6)
    for i in range(tries):
        if pos is None:
            pos=track_move(10,10); continue
        dx,dy=tx-pos[0],ty-pos[1]
        print(f'tip {pos} delta ({dx},{dy})')
        if abs(dx)<=tol and abs(dy)<=tol: return True
        # host 與 screenshot 大致 1:1（遊戲 2x、host 事件 1:1 進 IKBD 後遊戲自縮放），閉迴路收斂
        pos=track_move(max(-60,min(60,int(dx*0.7))) or (2 if dx>0 else -2),
                       max(-60,min(60,int(dy*0.7))) or (2 if dy>0 else -2))
    return False

if __name__=='__main__':
    cmd=sys.argv[1]
    if cmd=='snap':
        im,p=snap(); print(p, im.size)
    elif cmd=='cursor':
        im,p=snap(); print(find_cursor(im), p)
    elif cmd=='goto':
        print(goto2(int(sys.argv[2]),int(sys.argv[3])))
    elif cmd=='click':
        click(int(sys.argv[2]) if len(sys.argv)>2 else 1)
    elif cmd=='goclick':
        if goto2(int(sys.argv[2]),int(sys.argv[3])):
            click(); print('clicked')
        else: print('FAILED to reach')
    elif cmd=='rel':
        move_rel(int(sys.argv[2]),int(sys.argv[3]))
