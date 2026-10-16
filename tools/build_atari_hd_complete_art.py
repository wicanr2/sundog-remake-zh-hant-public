#!/usr/bin/env python3
"""Independent HD vectors for every ST tile strip and room variant.

Creation references: local source contact sheets, image-directory.json and the
approved A vocabulary. This generator reads no original image, pixel or mask.
Source indices identify art only; they never supply gameplay semantics.
"""
import argparse
import hashlib
import json
from pathlib import Path
from build_atari_hd_world_art import Art, tile


PALETTES = [
    ['ca35c4','9c8b2d','20c7cc','cf35c2','9c8b2d','df342b','bfcbd2','657885','38a839','e83631','2456cf','d8c52a','cb32c9'],
    ['2456cf','1cbd37','20c7cc','dcc229','d4e4d8','8dbb3f','6ebfc9','ddbd31','c42c28','24c7cd','d52c2b','cd35cb','d52d2c'],
    ['20cf3e','d5c12a','bfcbd2','e1c52a','11c8d2','2456cf','cf30cf','d52e2b','daca29','22c5c8','d6b927','bb2a37','24a93b'],
    ['21bf3b','1dc531','a8bdc8','d7c322','2360ce','23bdc4','d02cce','d5332d','d12aca','d5c124','dd3330','29c2c8','d7ce27'],
    ['da39c9','1fc83f','20b9c4','5753d6','e6bc2b','da342b','ce32be','20bfce','3857d6','e23c31','dfc329','20b7ba','cc32c6'],
    ['dc3dc5','1bcb45','e5c02e','d33ac1','28b94b','d62e2b','20c3cb','dbbf2a','27a936','2dc6cb','d52f2c','cc36bf','d82f2b'],
    ['2453d6','34d852','d9c43b','df3239','dec529','d72c2b','20c7cf','b0b795','8d729a','d73331','289d35','d7bf2c','22bdd4'],
    ['d62ac9','32bb46','dce4e7','28c6ce','b0b8be','d12bc5','4254cc','24b7d3','d6a72c','9936c9','d42d27','d0c541','daca27'],
    ['218ece','b6c5cc','aab9c2','d73b34','3265dc','d64b2b','bd4dc7','d63832','1ca43d','4f41d0','dd302d','cf35c9','2cc7cc'],
    ['d6c029','a6b4bd','aab7bf','db342c','26b547','dbe5e6','a22f30','34bdcc','25a23c','a52bb9','c7322c','aebdc5','d3e3e5'],
    ['e93933','19bc3d','29c6c8','7730c7','c2cfd5','cf2c2b','c2cfd5','c635c5','26b7c9','d8c42a','31b6c5','ce35ba','26c3ce'],
    ['d7c52c','1fbe3c','e3b629','9b32c2','d83333','dd5e64','22c7d6','d133ca','26b33f','d7bc28','20bfd3','a7b3ba','cf302b'],
]

# Visual construction recipes transcribed by eye from the reference atlas.
# No opaque semantic names are inferred from unknown source pictures.
RECIPES = [
    ['scatter','garden','scatter','step-r','scatter','disk','ruin','ruin','garden','step-r','step-r','canopy','step-r'],
    ['slab','garden','hook','step-l','cluster','cluster','ruin','gate','dome','step-r','step-r','step-r','hook'],
    ['cluster','garden','tier','blocks','tier','diagonal','tier','cluster','canopy','tier','tier','diamond','maze'],
    ['garden','garden','tier','diagonal','pyramid','slab','cross','tier','canopy','tier','tier','tier','tier'],
    ['cross','maze','slab','step-r','blade','diagonal','fork','slab','lens','loop','slab','hook','step-r'],
    ['cross','garden','canopy','step-r','lens','tripod','cluster','hook','dome','step-r','cross','hook','loop'],
    ['blocks','garden','cluster','step-r','canopy','pyramid','spoke','ruin','garden','crescent','maze','chevron','cross'],
    ['pyramid','spoke','cluster','cluster','spider','blade','cross','diamond','dome','cluster','diagonal','cluster','spoke'],
    ['blocks','ruin','blocks','step-l','book','cross','scatter','disk','loop','slab','step-l','canopy','slab'],
    ['blocks','ruin','ruin','hook','diagonal','pyramid','scatter','portal','garden','cross','loop','slab','cross'],
    ['ring','garden','cluster','ring','lens','cross','slab','cluster','diagonal','canopy','chevron','cross','chevron'],
    ['cross','garden','diagonal','hook','hook','cross','dome','tier','hook','cross','chevron','portal','hook'],
]

PROFILES = {
    'step-r': [[3,2],[22,2],[29,7],[28,12],[19,15],[28,17],[29,24],[23,29],[3,29],[3,22],[18,22],[21,20],[11,19],[11,12],[21,10],[19,8],[3,8]],
    'step-l': [[28,2],[9,2],[2,7],[3,12],[12,15],[3,17],[2,24],[8,29],[28,29],[28,22],[13,22],[10,20],[20,19],[20,12],[10,10],[12,8],[28,8]],
    'hook': [[3,3],[25,3],[29,8],[24,15],[14,17],[10,23],[28,23],[28,29],[2,29],[2,20],[9,13],[20,11],[22,8],[3,8]],
    'gate': [[2,2],[30,2],[30,9],[20,9],[20,29],[13,29],[13,9],[2,9]],
    'blade': [[3,2],[29,2],[29,7],[12,22],[9,29],[3,29]],
    'fork': [[3,2],[11,2],[14,12],[22,3],[30,3],[23,14],[28,29],[20,29],[16,20],[10,29],[2,26],[9,15]],
    'loop': [[3,2],[25,2],[30,8],[30,15],[23,21],[11,21],[11,29],[3,29]],
    'chevron': [[2,3],[12,3],[29,15],[29,20],[13,30],[2,30],[19,17]],
    'diamond': [[15,1],[22,10],[30,15],[23,21],[16,31],[9,22],[1,16],[9,10]],
    'book': [[2,2],[28,2],[28,29],[17,26],[12,30],[2,29]],
}


def city_cell(strip, index):
    # Drahew's reviewed geometry is retained wherever its source index occurs.
    approved = {31: {0: (15, 0), 1: (5, 0), 2: (34, 0), 3: (35, 0), 4: (36, 0)},
                36: {i: (i+2, 1) for i in range(13)},
                37: {i: (i+2, 0) for i in range(13)}}
    if strip in approved and index in approved[strip]:
        code, look = approved[strip][index]
        if strip==36 and index==3: code,look=11,2
        if strip==36 and index==9: code,look=11,3
        if strip==37 and index==3: code,look=11,0
        if strip==37 and index==9: code,look=11,1
        return tile(code, look)
    a = Art(32, 32)
    if strip in (32, 33):
        if index == 1:
            return tile(5, 0)
        c = ['1ac6cf','c92ab9','df342b','20cad2','2456cf'][index]
        shape = ['tier','blocks','ring','tier','cross'][index]
    else:
        family = strip-34
        c = PALETTES[family][index]
        shape = RECIPES[family][index]
    # The tile's own neutral foundation keeps silhouettes readable against
    # the city grass. It is separate from the collision field.
    def block(x, y, w, h, ink=c):
        a.poly([[x+1.5,y+h],[x+w,y+h],[x+w+2,y+h+2],[x+2,y+h+2]], '222b32')
        a.panel(x, y, w, h, ink, 'd6e2e8')
        a.line([[x+1,y+1],[x+w-1,y+1]], 'f4f8fa', .5)
        if w > 7 and h > 6:
            a.rect(x+2,y+2,w-4,1.1,'75dce2')
            a.rect(x+2,y+h-3,w-4,1,'243946')
    if shape in PROFILES:
        points=PROFILES[shape]
        a.poly([[x+1.5,y+1.5] for x,y in points],'28343d')
        a.poly(points,c)
        a.line(points[:4],'e7f0f3',.65)
        # Deliberate individual panel joints, hardware and light strips.
        if shape in ['step-r','step-l','hook']:
            a.line([[7,5],[18,5]],'76dce8',.7)
            a.line([[7,26],[18,26]],'243e51',1)
            a.rect(5,4,1.5,1.5,'edf5f7');a.rect(6,25,1.5,1.5,'edf5f7')
        elif shape=='loop':
            a.panel(12,7,10,8,'173647');a.rect(14,9,5,2,'49c8d2')
        elif shape=='book':
            a.line([[13,4],[13,26]],'eff5f8',.8)
            for y in [8,13,18,23]:a.line([[17,y],[25,y-1]],'f1e7b7',.5)
        elif shape=='gate':
            for x in [5,12,19,26]:a.line([[x,3],[x,7]],'f5ebbe',.5)
        elif shape=='blade':
            a.line([[6,4],[23,4],[9,22]],'f7e8b7',.8)
        else:
            a.line([[10,8],[21,15],[12,24]],'8ee6ed',.75)
    elif shape == 'tier':
        for n in range(3): block(3+n*.8,2+n*9,25-n*2,7,c)
    elif shape == 'blocks':
        for x,y in [(2,2),(17,2),(2,17),(17,17)]:block(x,y,11,11)
    elif shape == 'cluster':
        for x,y,w,h in [(2,3,12,10),(17,4,11,9),(5,19,10,9),(19,17,10,12)]:block(x,y,w,h)
    elif shape == 'slab':
        a.poly([[3,4],[20,2],[29,10],[27,27],[12,30],[3,23]], c)
        a.line([[4,5],[19,3],[27,10],[26,25]],'e5edf2',.7)
        for n in range(4): a.line([[7,10+n*4],[23,8+n*4]],'263c4e',1)
    elif shape == 'canopy':
        a.poly([[3,3],[28,3],[25,14],[7,14]],c)
        a.line([[4,4],[27,4]],'eef5f8',.7)
        a.rect(5,15,3,14,'868e98');a.rect(23,15,3,14,'868e98')
        a.rect(8,20,15,8,'493e2d')
    elif shape == 'pyramid':
        for n in range(5):a.panel(2+n*2.4,2+n*2.4,28-n*4.8,28-n*4.8,c,'91e2e8')
    elif shape == 'ring':
        a.ellipse(16,16,14,14,'253844');a.ellipse(15,15,13,13,c)
        a.ellipse(15,15,9.5,9.5,'dce6eb',.7);a.ellipse(15,15,6,6,'102735')
        a.line([[9,16],[21,12]],'e8f7fa',1.4)
    elif shape == 'dome':
        a.ellipse(16,17,13,12,'253844');a.ellipse(15,14,12,12,c)
        a.ellipse(12,10,6,4,'83e9df');a.ellipse(15,15,11,10,'e5f3f4',.55)
    elif shape=='disk':
        a.ellipse(16,17,14,12,'26343e');a.ellipse(15,14,14,12,c)
        a.ellipse(15,14,9,7,'f1e8db',.7);a.ellipse(15,14,4,4,'263e4c')
        a.line([[15,14],[24,4]],'e1f0f2',1.2)
    elif shape=='lens':
        a.ellipse(15,13,10,10,c);a.ellipse(15,13,7,7,'c5e9e3',.8)
        a.line([[9,21],[3,28]],c,4);a.line([[10,22],[4,28]],'d1e7ec',.7)
        a.line([[10,13],[20,13]],'e6f2ec',1.8);a.line([[15,8],[15,18]],'e6f2ec',1.8)
    elif shape=='portal':
        a.ellipse(16,16,15,15,'283640');a.ellipse(15,15,14,14,c)
        a.ellipse(15,15,11,11,'e1ebee',1);a.ellipse(15,15,8,10,'94232c')
        a.line([[15,6],[15,24]],'0b1f2b',1.5)
    elif shape=='crescent':
        a.ellipse(16,15,14,14,c);a.ellipse(10,12,11,11,'020508')
        a.line([[19,3],[25,8],[27,15],[24,23]],'e7edf1',.7)
    elif shape=='tripod':
        a.poly([[3,1],[16,9],[29,2],[30,9],[20,16],[20,29],[12,29],[12,16],[2,9]],c)
        a.line([[5,3],[16,10],[27,4]],'f0e0da',.7);a.line([[16,15],[16,27]],'6ae3e5',1)
    elif shape=='spider':
        a.ellipse(16,13,7,7,'18303e');a.ellipse(16,12,5,5,c)
        for n in range(4):
            a.line([[12,13],[6-n,10+n*5],[1+n,13+n*5]],'c5ced4',1.3)
            a.line([[20,13],[26+n,10+n*5],[31-n,13+n*5]],'c5ced4',1.3)
    elif shape == 'cross':
        a.poly([[3,2],[12,8],[16,12],[24,3],[30,6],[21,17],[30,27],[24,30],[15,22],[5,30],[1,25],[10,16],[1,7]],c)
        a.line([[4,5],[15,15],[27,27]],'f0e5d7',1)
        a.line([[5,27],[15,16],[26,5]],'49dae0',1)
    elif shape == 'spoke':
        for n in range(8):
            import math
            angle=n*math.pi/4
            a.line([[16,16],[16+13*math.cos(angle),16+13*math.sin(angle)]],c,3)
        a.ellipse(16,16,4,4,'deedf2');a.ellipse(16,16,2,2,'274c69')
    elif shape == 'diagonal':
        a.poly([[1,2],[8,2],[30,24],[30,31],[23,31],[1,9]],'c5d5dd')
        a.poly([[12,2],[30,2],[30,17]],c)
        a.poly([[1,15],[17,31],[1,31]],c)
        a.line([[2,4],[28,29]],'273d49',2)
    elif shape == 'maze':
        a.rect(1,1,30,30,c)
        for n in range(4):a.line([[3+n*4,4+n*3],[28-n*3,4+n*3],[28-n*3,28-n*3],[5+n*3,28-n*3]],'dfe73e',1)
    elif shape == 'garden':
        a.rect(1,1,30,30,'1f9232')
        for n in range(12):
            x,y=4+(n*11+index*3)%25,4+(n*7+index)%25
            a.ellipse(x,y,2.4,2.4,'51c650');a.line([[x-1,y-1],[x+1,y]],'b9df63',.5)
    elif shape in ('scatter','ruin'):
        for n in range(11):
            x,y=2+(n*13+index*2)%25,2+(n*7+index*3)%25
            a.poly([[x,y+3],[x+1,y],[x+4,y+1],[x+3,y+5]],c if n%3 else 'bcc9d0')
            a.line([[x+1,y+1],[x+3,y+2]],'eff5f8',.45)
    else:
        raise ValueError((strip,index,shape))
    return a.nodes


def surface_cell(strip, index):
    a=Art(32,32)
    if strip in (46,47,54): base,detail='8f17a5','382335'
    elif strip in (48,49,55): base,detail='6a741e','257a36'
    elif strip in (50,51,56): base,detail='a523a9','2aaf45'
    elif strip in (52,57): base,detail='751995','e1e8ef'
    else: base,detail='c9bd23','a027a9'
    a.rect(0,0,32,32,base)
    if strip in (46,47,50,51,52,54,56,57):
        a.nodes[-1]['material']='planet-ground'
    if strip in (48,49,50,51,52):
        # Thirteen coastline motifs: independently authored corner, channel,
        # island and inlet curves. No source bitmap boundary is traced.
        shores=[[[0,0],[8,0],[4,9],[0,12]],[[24,0],[32,0],[32,15],[26,9]],
                [[0,0],[32,0],[32,12],[21,7],[10,10],[0,17]],
                [[0,0],[18,0],[12,12],[0,20]],[[0,0],[32,0],[32,32],[0,32]],
                [[0,19],[9,14],[15,20],[21,17],[32,21],[32,32],[0,32]],
                [[0,0],[10,0],[8,18],[0,24]],[[19,0],[32,0],[32,23],[26,17],[23,8]],
                [[0,0],[10,0],[7,8],[0,13]],[[23,0],[32,0],[32,10],[28,14]],
                [[0,21],[9,19],[15,25],[25,23],[32,27],[32,32],[0,32]],
                [[0,0],[32,0],[32,12],[25,16],[20,10],[11,15],[0,11]],
                [[0,0],[10,0],[11,8],[22,14],[32,10],[32,32],[0,32]]]
        points=shores[index]
        a.poly(points,'184dba')
        a.line(points+[points[0]],'3987d9',.7)
        if index==4:
            a.poly([[8,17],[12,8],[21,6],[27,15],[23,24],[13,27]],base)
            a.line([[9,17],[13,9],[21,7]],'c7d369',.6)
    elif strip in (46,47,54):
        for n in range(3):
            x,y=5+(index*7+n*11)%23,5+(index*3+n*13)%23
            a.ellipse(x,y,3+n,2.7+n,detail)
            a.ellipse(x-.5,y-.5,2.6+n,2.3+n,'c455ca',.6)
        if strip==54 and index<2:a.ellipse(15,15,10-index*3,10-index*3,'58cddb')
    else:
        for n in range(6):
            x,y=(n*13+index*3)%32,(n*7+index*5)%32
            a.line([[x,y],[x+3,y-4],[x+8,y-3],[x+11,y-8]],detail,1.4)
    if strip in (55,56,58) and index<2:
        a.panel(5,6,21,21,'899ba9');a.panel(9,9,14,13,base)
        for n in range(3):a.rect(10+n*4,10,2,9,'dae9ed')
    for n in range(18):
        x,y=1+(n*17+index*5)%30,1+(n*11+index*7)%30
        a.line([[x,y],[x+.5,y-.5]],detail,.35)
    return a.nodes


def room(index):
    a=Art(160,100)
    floors=['32404b','1740a0','a12626','15397d','20762b','173489',
            '15959b','39434b','173e9c','7c218e','34414a','3b454e','248536',
            '414852','a82c2c','1c3793']
    floor=floors[index-14] if index>=14 else '485a68'
    a.rect(0,0,160,100,'020508');a.panel(1,1,158,98,floor,'d5e0e7')
    # Exact formal walk/door/counter regions; floor decoration stays visual.
    for y in range(31,89,12):a.line([[2,y],[157,y]],'657685',.25)
    a.panel(1,51,5,15,'718d9a');a.rect(2,54,2,9,'38c8d4')
    if 14<=index<=19:
        a.panel(2,2,155,13,'273740')
        a.line([[7,26],[135,26],[135,29],[7,29]],'d6b943',1.5)
        for n in range(12):
            x=10+n*11;a.rect(x,6,7,3,'d6aa39');a.rect(x+1,4,3,2,'eae3a1')
        for x in [10,32,44,66,78,100,112,134]:
            a.panel(x-3,90,15,8,'9350aa' if index%2 else '808d98')
            a.rect(x,92,9,4,'e5e4da');a.ellipse(x+4,94,1.2,1,'42b4c2')
        if index in (16,17):
            for x,y in [(27,48),(77,62),(121,43)]:a.line([[x-5,y],[x+5,y]],'68cbd7',.7);a.line([[x,y-5],[x,y+5]],'68cbd7',.7)
    elif 20<=index<=23:
        a.panel(2,2,155,9,'293d49');a.line([[7,59],[148,59],[148,61],[7,61]],'daa739',1)
        for y in [16,33,72,86]:
            a.panel(99,y,55,10,'697d8d')
            for x in [103,115,127,139]:a.panel(x,y+2,8,5,'d9bd40');a.line([[x+1,y+3],[x+5,y+3]],'f4edb1',.4)
        a.panel(9,4,44,49,'205766' if index%2==0 else '1f3b7c')
    elif 24<=index<=26:
        a.panel(2,2,155,10,'283b48');a.line([[54,11],[54,93],[56,93],[56,11]],'c1a741',1)
        for x,y in [(76,19),(112,18),(139,21),(78,40),(118,47),(141,66),(81,84),(114,85)]:
            a.panel(x,y,10,6,'d9bb37');a.line([[x+2,y+2],[x+7,y+2]],'f7e68d',.5)
        for y in [27,57,77]:a.line([[72,y],[152,y]],'c3d2d9',.8)
    elif 27<=index<=29:
        a.panel(2,2,155,12,'283c4a');a.line([[7,27],[148,27],[148,29],[7,29]],'d4b343',1)
        for x in [8,86]:
            a.panel(x,69,65,28,'607587');a.panel(x+3,72,59,22,'9eb4bd')
            a.rect(x+6,73,17,9,'d7e5e8');a.rect(x+25,73,33,20,'31546b' if index!=28 else '2d8c7e')
    else:
        a.panel(16,18,128,64,'afbcc5');a.panel(19,21,30,58,'1b8547')
        for y in [23,51]:a.panel(53,y,85,23,'d4dfe5');a.panel(69,y+8,25,8,'667d8d')
    return a.nodes


def main():
    assert Path('/.dockerenv').exists()
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--output',type=Path,required=True);args=p.parse_args()
    root=Path(__file__).resolve().parents[1]
    inventory=json.loads((root/'docs/art/atari-st-hd-asset-inventory.json').read_text())
    sizes={x['sourceIndex']:x['sourceSize'][0]//32 for x in inventory['images'] if 31<=x['sourceIndex']<=58}
    strips={f'{strip}/{cell}':city_cell(strip,cell) if strip<46 else surface_cell(strip,cell)
            for strip,count in sorted(sizes.items()) for cell in range(count)}
    assert len(strips)==330
    data={'schema':'sundog-atari-hd-complete-vectors-1','status':'independent-art-production-candidate',
          'sourceIdentity':'image31..58 cells; image13..29 room variants; indices and sizes from verified inventory',
          'evidence':'Artwork is an authored reinterpretation. Indices/anchors proven by existing consumers; no pixel-exact or visual-collision-exact claim.',
          'creation':'Own polygon/curve recipes; source contact sheets inspected by eye; no original pixel/mask input to this tool',
          'stripSizes':sizes,'strips':strips,'rooms':{str(i):room(i) for i in range(13,30)}}
    args.output.write_text(json.dumps(data,ensure_ascii=False,separators=(',',':'))+'\n')
    print('Created 330 individual tile vectors and 17 complete room variants',hashlib.sha256(args.output.read_bytes()).hexdigest())


if __name__=='__main__':main()
