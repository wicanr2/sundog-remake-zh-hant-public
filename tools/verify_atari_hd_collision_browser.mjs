// Local diagnostic page only, in node:22-bookworm sharing network-none Chromium.
import fs from 'node:fs/promises';
let target;
for(let i=0;i<80;i++){
  try{target=(await(await fetch('http://127.0.0.1:9222/json/list')).json()).find(p=>p.type==='page');if(target)break;}catch{}
  await new Promise(r=>setTimeout(r,100));
}
if(!target)throw Error('Chromium unavailable');
const ws=new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve,reject)=>{ws.onopen=resolve;ws.onerror=reject;});
let sequence=0;const pending=new Map(),events=new Map();
ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result);}else if(events.has(m.method)){events.get(m.method)(m.params);events.delete(m.method);}};
function call(method,params={}){return new Promise((resolve,reject)=>{const id=++sequence;pending.set(id,{resolve,reject});ws.send(JSON.stringify({id,method,params}));});}
function event(name){return new Promise(resolve=>events.set(name,resolve));}
async function evaluate(expression){const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;}
const folder=process.argv[2];if(!folder)throw Error('local diagnostic folder required');
const model=JSON.parse(await fs.readFile(folder+'/local-model.json','utf8'));
const points=[[62,95],[74,112],[137,33],[135,106],[270,70],[184,101],[0,0],[319,199]];
const fixtures={};
for(const name of ['cabin','ground'])fixtures[name]=points.map(([x,y])=>({x,y,legal:model[name].legalAnchors[y*320+x],reachable:model[name].reachableAnchors?.[y*320+x]}));
try{
  await call('Page.enable');await call('Runtime.enable');const results=[];
  for(const width of [1280,1700]){
    await call('Emulation.setDeviceMetricsOverride',{width,height:1400,deviceScaleFactor:1,mobile:false});
    const loaded=event('Page.loadEventFired');await call('Page.navigate',{url:'file://'+folder+'/local-review.html'});await loaded;
    const result=await evaluate(`(async()=>{
      const fixtures=${JSON.stringify(fixtures)},view=document.querySelector('#view'),result=document.querySelector('#result');let count=0;
      for(const name of ['cabin','ground']){
        document.getElementById(name).click();await view.decode();
        for(const withOverlay of [true,false]){
          if(view.src.endsWith('-diagnostic.png')!==withOverlay){document.querySelector('#toggle').click();await view.decode();}
          if(withOverlay){const b=view.getBoundingClientRect();view.dispatchEvent(new MouseEvent('click',{clientX:b.left+20*b.width/view.naturalWidth,clientY:b.top+20*b.height/view.naturalHeight,bubbles:true}));if(result.textContent!=='標題區')throw Error('header classified as ground');}
          for(const f of fixtures[name]){
            const b=view.getBoundingClientRect();const clientX=b.left+(f.x*5+2.5)*b.width/view.naturalWidth,clientY=b.top+(f.y*5+2.5+(withOverlay?80:0))*b.height/view.naturalHeight;
            view.dispatchEvent(new MouseEvent('click',{clientX,clientY,bubbles:true}));
            const expected=!f.legal?'受阻':name==='cabin'&&!f.reachable?'合法，但不與起點連通':'合法，可達性依場景圖樣判定';
            if(!result.textContent.includes('('+f.x+','+f.y+')：'+expected))throw Error(JSON.stringify({name,withOverlay,f,actual:result.textContent}));count++;
          }
        }
      }
      return {status:'PASS',checks:count,viewportWidth:innerWidth,imageWidth:view.getBoundingClientRect().width,headerExcluded:true,diagnosticToggle:true};
    })()`);
    if(result.status!=='PASS'||result.checks!==32)throw Error(JSON.stringify(result));results.push(result);
  }
  await fs.writeFile(folder+'/browser-validation.json',JSON.stringify({status:'PASS-local-Chromium-diagnostic',node:process.version,results,scope:'DOM click coordinates only; not game input or original parity'},null,2)+'\n');
  console.log(JSON.stringify(results));
}finally{ws.close();}
