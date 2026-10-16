// Runs in node:22-bookworm, sharing the network-none Chromium container's
// loopback only. See docs/art/atari-st-hd-plan.md for the review workflow.
import fs from 'node:fs/promises';

let target;
for (let i=0;i<80;i++) {
  try { target=(await (await fetch('http://127.0.0.1:9222/json/list')).json()).find(p=>p.type==='page'); if(target)break; } catch {}
  await new Promise(r=>setTimeout(r,100));
}
if(!target)throw Error('Chromium target unavailable');
const ws=new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve,reject)=>{ws.onopen=resolve;ws.onerror=reject;});
let sequence=0;
const pending=new Map();const events=new Map();
ws.onmessage=e=>{
  const m=JSON.parse(e.data);
  if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result);}
  else if(events.has(m.method)){events.get(m.method)(m.params);events.delete(m.method);}
};
function call(method,params={}){return new Promise((resolve,reject)=>{const id=++sequence;pending.set(id,{resolve,reject});ws.send(JSON.stringify({id,method,params}));});}
function event(name){return new Promise(resolve=>events.set(name,resolve));}
async function evaluate(expression){const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value;}
const results=[];
const pagePath=process.argv[2]||'/out/atari-st-hd-navigation-maps-20261015.html';
const checkExpression=`(async()=>{
  await preview.decode();let count=0;
  const toggle=document.querySelector('#toggle');
  toggle.click();if(getComputedStyle(overlay).display==='none')throw Error('overlay not shown');
  toggle.click();if(getComputedStyle(overlay).display!=='none')throw Error('overlay not hidden');
  for(const [v,view] of Object.keys(data.images).entries()){
    document.querySelectorAll('#views button')[v].click();await preview.decode();
    for(const [i,lang] of Object.keys(data.images[view]).entries()){
    document.querySelectorAll('#languages button')[i].click();await preview.decode();
    if(preview.src!==data.images[view][lang])throw Error('locale '+lang);
    for(const f of data.fixtures){
      const b=preview.getBoundingClientRect();
      const clientX=b.left+(f.x+.5)*b.width/320,clientY=b.top+(f.y+.5)*b.height/200;
      preview.dispatchEvent(new MouseEvent('click',{clientX,clientY,bubbles:true}));
      if(readout.dataset.action!==f.action)throw Error(JSON.stringify({lang,f,actual:readout.dataset.action}));
      count++;
    }
  }
  }
  return {status:'PASS',checks:count,viewportWidth:innerWidth,imageWidth:preview.getBoundingClientRect().width,languages:4,views:3,overlayToggle:true};
})()`;
try {
  await call('Page.enable');await call('Runtime.enable');
  for(const width of [1280,1700]){
    await call('Emulation.setDeviceMetricsOverride',{width,height:1400,deviceScaleFactor:1,mobile:false});
    const loaded=event('Page.loadEventFired');
    await call('Page.navigate',{url:'file://'+pagePath});await loaded;
    const result=await evaluate(checkExpression);
    if(result?.status!=='PASS'||result.checks!==420)throw Error(JSON.stringify(result));
    await evaluate("show('sector','zh-Hant'); preview.decode().then(()=>true)");
    const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});
    await fs.writeFile(`/out/browser-cdp-${width}.png`,Buffer.from(shot.data,'base64'));
    results.push(result);
  }
  await fs.writeFile('/out/browser-validation.json',JSON.stringify({status:'PASS-Chromium',node:process.version,results},null,2)+'\n');
  console.log(JSON.stringify(results));
} finally {ws.close();}
