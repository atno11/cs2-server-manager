'use strict';

class SSEParser {
  constructor(onEvent) { this.onEvent=onEvent; this.decoder=new TextDecoder(); this.buffer=''; this.event=''; this.id=''; this.data=[]; }
  push(bytes) {
    this.buffer += this.decoder.decode(bytes,{stream:true});
    let cut;
    while ((cut=this.buffer.indexOf('\n'))>=0) {
      let line=this.buffer.slice(0,cut); this.buffer=this.buffer.slice(cut+1);
      if (line.endsWith('\r')) line=line.slice(0,-1);
      if (!line) { if(this.data.length) this.onEvent({event:this.event||'message',id:this.id,data:this.data.join('\n')}); this.event='';this.id='';this.data=[]; continue; }
      if (line.startsWith(':')) continue;
      const colon=line.indexOf(':'); const field=colon<0?line:line.slice(0,colon);
      let value=colon<0?'':line.slice(colon+1); if(value.startsWith(' '))value=value.slice(1);
      if(field==='data')this.data.push(value);
      if(field==='event')this.event=value;
      if(field==='id'&&!value.includes('\0'))this.id=value;
      if(this.data.join('\n').length>16384)throw Error('Stream event too large');
    }
    if (this.buffer.length>65536) throw Error('Stream frame too large');
  }
  finish() { const tail=this.decoder.decode(); if(tail)this.push(new TextEncoder().encode(tail)); }
}

function applySnapshot(oldItems,payload) {
  if(!payload||!Array.isArray(payload.instances)||payload.instances.length>32)throw Error('Invalid snapshot');
  const next=new Map();
  for(const item of payload.instances) {
    if(typeof item.id!=='string'||typeof item.state!=='string'||!Number.isInteger(item.port)||next.has(item.id))throw Error('Invalid instance');
    next.set(item.id,item);
  }
  return next;
}

function reconcileLog(current,event) {
  if(!Number.isSafeInteger(event.start)||!Number.isSafeInteger(event.end)||event.end<event.start||event.end-event.start>2048||typeof event.bytes!=='string')throw Error('Invalid log event');
  const raw=atob(event.bytes); const bytes=Uint8Array.from(raw,c=>c.charCodeAt(0));
  if(bytes.length!==event.end-event.start)throw Error('Invalid log length');
  if(event.end<=current.end)return current;
  if(event.start>current.end)throw Error('Log gap');
  const extra=bytes.slice(current.end-event.start);
  const joined=new Uint8Array(current.bytes.length+extra.length);
  joined.set(current.bytes);joined.set(extra,current.bytes.length);
  const retained=joined.slice(Math.max(0,joined.length-65536));
  return {bytes:retained,start:event.end-retained.length,end:event.end};
}

function advanceSequence(last,id,isSnapshot) {
  if(!/^\d+$/.test(id))throw Error('Invalid event sequence');
  const sequence=BigInt(id);
  if(isSnapshot)return sequence;
  if(last===null||sequence!==last+1n)throw Error('Event gap');
  return sequence;
}

function nextBackoff(current,connectedMs) { return connectedMs>=15000?1000:Math.min(current*2,30000); }

if(typeof module!=='undefined')module.exports={SSEParser,applySnapshot,reconcileLog,advanceSequence,nextBackoff};

if(typeof document!=='undefined') {
  let token='',generation=0,controller=null,items=new Map(),selected='',log={bytes:new Uint8Array(),start:0,end:0},loading=false,pending=[],logLoadID=0,lastSeq=null;
  const output=document.getElementById('output'),status=document.getElementById('connection');
  const setStatus=value=>{status.textContent=value};
  const logText=()=>{output.textContent=new TextDecoder().decode(log.bytes)};
  async function api(path,options={}) { const headers={...(options.headers||{}),Authorization:'Bearer '+token};const response=await fetch(path,{...options,headers});if(!response.ok)throw Error('Request failed ('+response.status+')');return response; }
  function render() {
    const body=document.getElementById('instances');body.replaceChildren();
    for(const item of [...items.values()].sort((a,b)=>a.id.localeCompare(b.id))) {
      const tr=document.createElement('tr');
      for(const value of [item.id,item.port,item.state]) {const td=document.createElement('td');td.textContent=value;tr.append(td)}
      const td=document.createElement('td');
      for(const action of ['start','stop','restart','logs']) {const button=document.createElement('button');button.textContent=action;button.onclick=async()=>{try{if(action==='logs'){selected=item.id;await loadLogs(generation)}else await api('/api/instances/'+encodeURIComponent(item.id)+'/'+action,{method:'POST'})}catch(error){output.textContent=error.message}};td.append(button)}
      const remove=document.createElement('button');remove.textContent='remove';remove.onclick=async()=>{try{await api('/api/instances/'+encodeURIComponent(item.id),{method:'DELETE'})}catch(error){output.textContent=error.message}};td.append(remove);tr.append(td);body.append(tr);
    }
  }
  async function loadLogs(version) {
    if(!selected)return;
    loading=true;pending=[];const id=selected,loadID=++logLoadID;
    try {
      const response=await api('/api/instances/'+encodeURIComponent(id)+'/logs');
      const bytes=new Uint8Array(await response.arrayBuffer());
      if(version!==generation||id!==selected||loadID!==logLoadID)return;
      const end=Number(response.headers.get('X-Log-End'));const start=Number(response.headers.get('X-Log-Start'));
      if(!Number.isSafeInteger(start)||!Number.isSafeInteger(end)||end-start!==bytes.length)throw Error('Invalid log snapshot');
      log={bytes:bytes.slice(Math.max(0,bytes.length-65536)),start:Math.max(start,end-65536),end};
      for(const event of pending)log=reconcileLog(log,event);
      logText();
    } catch(error) {if(version===generation&&loadID===logLoadID){output.textContent=error.message;controller?.abort()}}
    finally {if(loadID===logLoadID){loading=false;pending=[]}}
  }
  function handleEvent(message,version) {
    if(version!==generation)return;
    let data;try{data=JSON.parse(message.data)}catch{controller?.abort();return}
    try {
      if(message.event==='snapshot') {lastSeq=advanceSequence(lastSeq,message.id,true);items=applySnapshot(items,data);render();if(selected&&items.has(selected))void loadLogs(version);else{logLoadID++;selected='';log={bytes:new Uint8Array(),start:0,end:0};output.textContent=''};return}
      lastSeq=advanceSequence(lastSeq,message.id,false);
      if(message.event==='instance.registered'||message.event==='instance.state') {if(!data.instance||typeof data.instance.id!=='string')throw Error('Invalid instance event');items.set(data.instance.id,data.instance);render();return}
      if(message.event==='instance.removed') {if(typeof data.id!=='string')throw Error('Invalid removal');items.delete(data.id);if(selected===data.id){logLoadID++;selected='';output.textContent=''}render();return}
      if(message.event==='instance.log'&&data.id===selected) {if(loading){pending.push(data);if(pending.length>128)throw Error('Log recovery backlog exceeded')}else{log=reconcileLog(log,data);logText()}}
    }catch{controller?.abort()}
  }
  const pause=(ms,signal)=>new Promise(resolve=>{const finish=()=>{clearTimeout(timer);signal.removeEventListener('abort',finish);resolve()};const timer=setTimeout(finish,ms);signal.addEventListener('abort',finish,{once:true})});
  async function stream(version,signal) {
    let backoff=1000;
    while(version===generation&&!signal.aborted) {
      const attempt=new AbortController();controller=attempt;
      const stop=()=>attempt.abort();signal.addEventListener('abort',stop,{once:true});
      let connectedAt=0;
      try {
        setStatus('Connecting');
        const response=await fetch('/api/events',{headers:{Authorization:'Bearer '+token},cache:'no-store',signal:attempt.signal});
        if(response.status===401){setStatus('Authentication required');return}
        if(!response.ok||!response.body)throw Error('Stream unavailable');
        setStatus('Connected');connectedAt=Date.now();lastSeq=null;
        const reader=response.body.getReader();const parser=new SSEParser(message=>handleEvent(message,version));
        while(version===generation&&!attempt.signal.aborted){const result=await reader.read();if(result.done)break;parser.push(result.value)}
      }catch(error){if(version===generation&&!signal.aborted)setStatus('Disconnected')}
      finally{signal.removeEventListener('abort',stop);attempt.abort();if(controller===attempt)controller=null}
      if(version!==generation||signal.aborted)return;
      const delay=connectedAt&&Date.now()-connectedAt>=15000?1000:backoff;
      setStatus('Reconnecting');await pause(delay,signal);backoff=nextBackoff(delay,connectedAt?Date.now()-connectedAt:0);
    }
  }
  let pageController=null;
  document.getElementById('connect').onclick=()=>{pageController?.abort();generation++;logLoadID++;lastSeq=null;token=document.getElementById('token').value;document.getElementById('token').value='';items=new Map();render();selected='';output.textContent='';pageController=new AbortController();void stream(generation,pageController.signal)};
  document.getElementById('form').onsubmit=async event=>{event.preventDefault();const definition=Object.fromEntries(new FormData(event.target));definition.port=Number(definition.port);definition.args=[];try{await api('/api/instances',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(definition)})}catch(error){output.textContent=error.message}};
  window.addEventListener('pagehide',()=>pageController?.abort());
}
