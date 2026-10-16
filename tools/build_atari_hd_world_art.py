#!/usr/bin/env python3
"""Independent vector review art. Reads no original images or bitmap masks."""
import argparse
import json
from pathlib import Path


class Art:
    def __init__(self, w, h):
        self.nodes = []
        self.clip = [0, 0, w, h]

    def node(self, kind, colour, **extra):
        self.nodes.append(dict(kind=kind, colour=int(colour, 16), clip=self.clip, **extra))

    def poly(self, points, colour):
        self.node('polygon', colour, points=points)

    def rect(self, x, y, w, h, colour):
        self.poly([[x, y], [x+w, y], [x+w, y+h], [x, y+h]], colour)

    def line(self, points, colour, width=.6):
        self.node('line', colour, points=points, width=width)

    def ellipse(self, x, y, rx, ry, colour, width=0):
        self.node('ellipse', colour, box=[x-rx, y-ry, 2*rx, 2*ry], width=width)

    def panel(self, x, y, w, h, colour, edge='dce5ec'):
        self.rect(x, y, w, h, edge)
        self.rect(x+.8, y+.8, w-1.6, h-1.6, colour)
        self.line([[x+1, y+h-1], [x+w-1, y+h-1], [x+w-1, y+1]], '525e69', .7)


def cabin(lit):
    a = Art(320, 200)
    a.rect(0, 0, 320, 200, '020508')
    outline = [[13,84],[17,74],[42,65],[83,54],[91,41],[110,30],[124,24],[290,24],[297,28],[297,48],[282,52],[282,137],[296,144],[296,165],[288,170],[122,170],[103,160],[91,145],[86,135],[47,139],[18,122],[13,115]]
    a.poly(outline, 'e6edf0')
    a.poly([[18,85],[24,77],[47,71],[88,60],[98,46],[116,35],[128,29],[289,29],[289,45],[274,51],[274,140],[289,147],[289,163],[125,164],[109,154],[98,139],[91,130],[49,133],[23,119],[18,112]], '7d8992')
    a.poly([[49,75],[90,62],[98,50],[113,37],[144,34],[156,43],[161,55],[154,63],[146,60],[133,56],[114,55],[105,65],[105,78],[116,86],[132,88],[149,83],[165,84],[174,96],[172,111],[157,120],[145,122],[129,117],[115,116],[105,126],[103,142],[115,155],[142,156],[152,150],[158,157],[151,166],[123,165],[106,157],[96,145],[91,124],[86,117],[51,118]], '099f35')
    a.line([[52,78],[88,67],[99,52],[115,40],[140,39]], '5bfa7a', 1)
    a.line([[99,124],[100,143],[114,159],[144,160]], '5bfa7a', 1)
    # Nose and cockpit retain the left-facing hull and original station anchors.
    a.panel(16,81,7,34,'c7d0d6')
    a.poly([[51,77],[83,67],[88,76],[85,112],[51,118]],'0dc43a')
    for x,y in [(52,76),(55,89),(56,104),(69,73),(77,72)]:
        a.panel(x,y,5,7,'164dcb');a.rect(x+1,y+1,2,2,'29e7ef')
    a.panel(60,90,5,11,'e9efef');a.rect(61,91,3,7,'545f66')
    a.panel(68,108,12,8,'88949c');a.line([[70,110],[78,110]],'f73c32',1)
    # The stores and engineering stations use the existing window rectangles.
    a.panel(68,68,12,8,'1b4be4');a.rect(70,70,6,3,'32dcec')
    for x,y,w,h in [(129,29,16,8),(146,34,8,12),(129,46,16,5),(129,145,16,8),(146,150,8,12),(129,161,16,6),(103,101,8,11)]:
        a.panel(x,y,w,h,'11b5bd');a.rect(x+1.8,y+1.8,max(1,w-4),max(1,h-4),'174fc1')
    a.poly([[107,55],[120,51],[131,53],[137,69],[133,81],[121,82],[111,76]],'020508')
    a.line([[109,58],[118,55],[128,57]],'e1e8ed',1)
    a.poly([[110,123],[120,118],[133,122],[139,137],[130,151],[118,149],[109,140]],'020508')
    a.line([[113,128],[124,122],[132,127]],'e1e8ed',1)
    # Runtime uses image 8 in both phases. Only palette-index-5 lights blink.
    for y in [54,81,108]:
        a.panel(173,y,106,24,'b5bfc7')
        a.panel(190,y+7,25,10,'65717b')
        a.rect(195,y+10,16,4,'27333d')
        a.line([[176,y+2],[276,y+2]],'f3f6f8',.9)
    for y in [25,148]:
        a.panel(164,y,129,23,'dce3e7')
        for x in [182,204,226,248,270]:a.line([[x,y+1],[x,y+21]],'919da6',.4)
        a.poly([[222,y+7],[232,y+3],[232,y+15]],'293b47')
    for x,y in [(169,29),(169,156)]:a.ellipse(x,y,3,3,'eff4f6');a.ellipse(x,y,1.6,1.6,'77838b')
    a.line([[155,82],[169,82],[175,75],[250,75]],'222d36',1.4)
    a.line([[157,117],[176,117],[180,130],[252,130]],'222d36',1.4)
    for x,y,w,h in [(56,93,1,1),(59,104,1,3),(143,120,1,1),(283,26,9,2),(283,162,9,2)]:
        a.rect(x,y,w,h,'ff342d' if lit else '020508')
    return a.nodes


def tile(code, look):
    a = Art(32, 32)
    look = look % (4 if code == 11 else 2)
    if code in [0,1]:return a.nodes
    if code == 5:
        for x in [2,10,18,26]:a.line([[x,2],[x,12],[x+5,12],[x+5,2]],'8c979d',.6)
    elif code == 3:
        a.rect(0,0,32,32,'0ab51e')
        if look:
            a.panel(4,3,13,12,'c6a418');a.line([[6,6],[9,4],[12,8],[15,6]],'fff16a')
        else:
            for x,y in [(5,6),(15,8),(24,6),(9,20),(22,22)]:a.ellipse(x,y,4,4,'42d630');a.line([[x-2,y],[x+2,y]],'b9e844',.5)
    elif code == 2:
        for x,y in [(1,1),(18,1),(1,18),(18,18)]:a.panel(x,y,12,12,'16ab27');a.rect(x+1,y+8,10,3,'e52bd9')
    elif code == 4:
        for x in [2,17]:a.panel(x,2,12,27,'c8d0d5');a.rect(x+2,5,8,6,'1c65e5');a.line([[x+2,18],[x+10,18]],'28d6df',1)
    elif code == 6:
        if not look:
            for i in range(0,11,3):a.panel(2+i,2+i,28-2*i,28-2*i,'1251cb','1de2e7')
        else:
            for y in [2,10,18,26]:a.poly([[2,y],[27,y],[31,y+4],[5,y+4]],'0dd6d8')
    elif code == 7:
        a.poly([[3,5],[11,1],[11,25],[3,30]],'0bafb8');a.poly([[12,2],[17,6],[17,29],[12,25]],'718690')
        a.poly([[20,1],[30,8],[30,28],[20,20]],'1644bd');a.line([[21,4],[28,10],[28,24]],'2be7ef',1)
    elif code == 8:
        a.poly([[2,1],[15,8],[30,1],[20,16],[30,30],[15,24],[1,30],[10,16]],'1aef47' if not look else 'e8df1b')
        a.line([[2,2],[28,29]],'f32be2',2);a.line([[28,2],[2,29]],'f32be2',2)
    elif code in [9,12]:
        for y in [2,18]:a.panel(2,y,27,12,'e22d22');a.rect(5,y+3,11,6,'7c1d1d');a.rect(20,y+2,7,8,'a7b4bd')
    elif code == 10 or code == 11:
        colours=['8c2bd8','dfbc1c','1cbdcc','ca35ba'];c=colours[look if code==11 else look]
        for x,y in [(2,3),(17,17)]:a.panel(x,y,13,12,c);a.rect(x+2,y+2,7,3,'d9e7eb');a.rect(x+2,y+8,8,2,'1e3241')
    elif code == 13:
        for x,y in [(1,3),(17,18)]:a.panel(x,y,14,11,'e2de19');a.rect(x+2,y+2,9,7,'617d9d');a.line([[x+3,y+3],[x+11,y+3]],'e8f3fb',.8)
    elif code == 14:
        a.poly([[2,3],[27,3],[30,14],[18,27],[3,27]],'20b8c4');a.rect(6,7,14,3,'dbeaf0');a.rect(20,19,8,5,'cb2bd7')
    elif code == 15:
        a.poly([[8,1],[23,1],[31,15],[24,30],[7,30],[1,16]],'af1631');a.poly([[10,6],[22,6],[26,16],[21,24],[9,24],[5,16]],'26343d')
        for x,y in [(10,11),(20,11),(15,20)]:a.rect(x,y,3,3,'e9a4a4')
    elif code == 34:
        a.poly([[2,25],[2,10],[11,2],[29,2],[29,24],[23,30],[7,30]],'a8b6c0');a.poly([[5,22],[5,12],[13,5],[26,5],[26,12],[13,12],[13,22]],'234ba1');a.rect(6,26,16,2,'3bd9e0')
    elif code == 35:
        a.panel(3,2,25,28,'d6c91e');a.poly([[4,11],[20,3],[28,8],[24,27],[6,29]],'12c2d2');a.line([[7,24],[24,7]],'f4ed76',1.5);a.rect(12,10,5,7,'1f3448')
    elif code == 36:
        a.poly([[1,3],[13,3],[16,12],[21,3],[30,3],[23,17],[26,29],[17,29],[15,21],[9,30],[1,26],[7,17]],'e52b28');a.line([[4,6],[11,8],[16,17],[19,26]],'dfe7eb',2)
    else:raise ValueError(code)
    return a.nodes


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output',type=Path,required=True);args=p.parse_args()
    assert Path('/.dockerenv').exists() and not args.output.exists()
    grass=Art(40,40);grass.rect(0,0,40,40,'101708')
    # Local art-only sequence. Independent x/y samples avoid diagonal lanes.
    seed=17
    for i in range(46):
        seed=(1664525*seed+1013904223)&0xffffffff;x=1+(seed>>16)%38
        seed=(1664525*seed+1013904223)&0xffffffff;y=1+(seed>>16)%38
        grass.line([[x,y],[x+.7,y-.9]],'94a326' if i%3==0 else '266b16',.35)
    data=dict(schema='sundog-hd-world-review-art-1',status='DRAFT-independent-vector-reinterpretation',sourceIdentity=dict(shipImages=[8],animation='same image8; palette5 dark/red, beat8 every fifth beat; source image9 is not used by current runtime',city='drahew',cityNumber=1,strips=[31,36,37],tileSize=32,pitch=40),collision='Existing source masks only; these nodes never feed passability.',cabinClosed=cabin(False),cabinLit=cabin(True),groundBackdrop=grass.nodes,tiles={str(code):[tile(code,look) for look in range(4)] for code in list(range(16))+[34,35,36]})
    args.output.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');print('Independent review art: 2 cabin frames, backdrop, 19 codes ×4 look slots')


if __name__=='__main__':main()
