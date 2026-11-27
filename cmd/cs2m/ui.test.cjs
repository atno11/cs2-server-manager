const {test}=require('node:test');
const assert=require('node:assert/strict');
const {SSEParser,applySnapshot,reconcileLog,advanceSequence,nextBackoff}=require('./ui.js');

test('fragmented UTF-8, multiline data, comments and several events',()=>{
  const got=[];const parser=new SSEParser(event=>got.push(event));
  const bytes=new TextEncoder().encode(': heartbeat\n\nid: 4\nevent: snapshot\ndata: {"text":"é"}\ndata: second\n\nid: 5\nevent: instance.state\ndata: {}\n\n');
  for(const byte of bytes)parser.push(Uint8Array.of(byte));
  assert.deepEqual(got,[{event:'snapshot',id:'4',data:'{"text":"é"}\nsecond'},{event:'instance.state',id:'5',data:'{}'}]);
});

test('snapshot replacement validates uniqueness',()=>{
  const old=new Map([['stale',{id:'stale'}]]);
  const next=applySnapshot(old,{instances:[{id:'one',port:27015,state:'running'}]});
  assert.equal(next.has('stale'),false);assert.equal(next.get('one').state,'running');
  assert.throws(()=>applySnapshot(old,{instances:[{id:'one',port:1,state:'a'},{id:'one',port:2,state:'b'}]}));
});

test('log offsets suppress duplicates and reject gaps',()=>{
  const empty={bytes:new Uint8Array(),start:0,end:0};
  const event={start:0,end:3,bytes:Buffer.from('abc').toString('base64')};
  const first=reconcileLog(empty,event);
  assert.equal(new TextDecoder().decode(first.bytes),'abc');
  assert.equal(reconcileLog(first,event),first);
  const overlap=reconcileLog(first,{start:2,end:5,bytes:Buffer.from('cde').toString('base64')});
  assert.equal(new TextDecoder().decode(overlap.bytes),'abcde');
  assert.throws(()=>reconcileLog(overlap,{start:7,end:8,bytes:Buffer.from('x').toString('base64')}),/gap/);
});

test('malformed and oversized frames cannot grow without bound',()=>{
  const parser=new SSEParser(()=>{});
  assert.throws(()=>parser.push(new TextEncoder().encode('data: '+ 'x'.repeat(17000)+'\n')),/too large/);
  const other=new SSEParser(()=>{});
  assert.throws(()=>other.push(new Uint8Array(70000).fill(65)),/too large/);
});

test('sequence gaps force a fresh snapshot and backoff stays bounded',()=>{
  let sequence=advanceSequence(null,'8',true);
  sequence=advanceSequence(sequence,'9',false);
  assert.equal(sequence,9n);
  assert.throws(()=>advanceSequence(sequence,'11',false),/gap/);
  assert.equal(advanceSequence(sequence,'20',true),20n);
  assert.throws(()=>advanceSequence(sequence,'token',false),/Invalid/);
  let delay=1000;for(let i=0;i<20;i++)delay=nextBackoff(delay,0);
  assert.equal(delay,30000);assert.equal(nextBackoff(delay,15000),1000);
});

function startMockPage(fetchMock) {
  const saved={document:global.document,window:global.window,fetch:global.fetch};
  const elements=new Map();
  const listeners=new Map();
  const element=()=>({value:'',textContent:'',children:[],replaceChildren(){this.children=[]},append(child){this.children.push(child)}});
  for(const id of ['token','connect','instances','output','connection','form'])elements.set(id,element());
  global.document={getElementById:id=>elements.get(id),createElement:element};
  global.window={addEventListener:(name,fn)=>listeners.set(name,fn)};
  global.fetch=fetchMock;
  delete require.cache[require.resolve('./ui.js')];require('./ui.js');
  return {elements,restore(){listeners.get('pagehide')?.();global.document=saved.document;global.window=saved.window;global.fetch=saved.fetch;delete require.cache[require.resolve('./ui.js')]}};
}

function deferred() {let resolve;const promise=new Promise(r=>{resolve=r});return {promise,resolve}}
const tick=()=>new Promise(resolve=>setImmediate(resolve));
function logResponse(text) {const bytes=new TextEncoder().encode(text);return {ok:true,headers:{get:name=>name==='X-Log-Start'?'0':String(bytes.length)},arrayBuffer:async()=>bytes.buffer}}
function frame(id,type,data) {return new TextEncoder().encode(`id: ${id}\nevent: ${type}\ndata: ${JSON.stringify(data)}\n\n`)}
function streamFeed() {
  const waiting=[];const queued=[];
  return {body:{getReader:()=>({read:()=>queued.length?Promise.resolve(queued.shift()):new Promise(resolve=>waiting.push(resolve))})},
    push(bytes){const next={done:false,value:bytes};if(waiting.length)waiting.shift()(next);else queued.push(next)},
    close(){const next={done:true};if(waiting.length)waiting.shift()(next);else queued.push(next)}};
}
function clickLogs(page,id) {
  const row=page.elements.get('instances').children.find(row=>row.children[0].textContent===id);
  assert.ok(row,`missing ${id}`);
  const button=row.children.at(-1).children.find(button=>button.textContent==='logs');
  button.onclick();
}

test('delayed log responses and queued events cannot cross removal, same-ID recreation or selection',async()=>{
  const stream=streamFeed(),old=deferred(),fresh=deferred(),other=deferred();
  const calls=[];
  const page=startMockPage((path)=>{
    if(path==='/api/events')return Promise.resolve({status:200,ok:true,body:stream.body});
    calls.push(path);return calls.length===1?old.promise:calls.length===2?fresh.promise:other.promise;
  });
  try {
    page.elements.get('token').value='test-token';page.elements.get('connect').onclick();await tick();
    stream.push(frame(0,'snapshot',{instances:[{id:'one',port:1,state:'stopped'},{id:'two',port:2,state:'stopped'}]}));await tick();
    clickLogs(page,'one');await tick();
    stream.push(frame(1,'instance.log',{id:'one',start:0,end:3,bytes:Buffer.from('old').toString('base64')}));
    stream.push(frame(2,'instance.removed',{id:'one'}));
    stream.push(frame(3,'instance.registered',{instance:{id:'one',port:1,state:'stopped'}}));await tick();
    clickLogs(page,'one');await tick();
    old.resolve(logResponse('old'));await tick();
    assert.equal(page.elements.get('output').textContent,'');
    stream.push(frame(4,'instance.log',{id:'one',start:3,end:4,bytes:Buffer.from('!').toString('base64')}));await tick();
    fresh.resolve(logResponse('new'));await tick();
    assert.equal(page.elements.get('output').textContent,'new!');
    clickLogs(page,'two');await tick();
    other.resolve(logResponse('two'));await tick();
    assert.equal(page.elements.get('output').textContent,'two');
  } finally {page.restore()}
});

test('fresh reconnect snapshot invalidates an older pending log response',async()=>{
  const streams=[streamFeed(),streamFeed()],old=deferred(),fresh=deferred();
  let streamCalls=0,logCalls=0;
  const realSetTimeout=global.setTimeout,realClearTimeout=global.clearTimeout;
  global.setTimeout=(fn)=>{setImmediate(fn);return 1};global.clearTimeout=()=>{};
  const page=startMockPage(path=>{
    if(path==='/api/events')return Promise.resolve({status:200,ok:true,body:streams[streamCalls++].body});
    return ++logCalls===1?old.promise:fresh.promise;
  });
  try {
    page.elements.get('token').value='test-token';page.elements.get('connect').onclick();await tick();
    streams[0].push(frame(0,'snapshot',{instances:[{id:'one',port:1,state:'stopped'}]}));await tick();
    clickLogs(page,'one');await tick();
    streams[0].close();await tick();await tick();
    assert.equal(streamCalls,2);
    streams[1].push(frame(0,'snapshot',{instances:[{id:'one',port:1,state:'stopped'}]}));await tick();
    old.resolve(logResponse('stale'));await tick();
    assert.equal(page.elements.get('output').textContent,'');
    fresh.resolve(logResponse('fresh'));await tick();
    assert.equal(page.elements.get('output').textContent,'fresh');
  } finally {page.restore();global.setTimeout=realSetTimeout;global.clearTimeout=realClearTimeout}
});

test('switching selected instances ignores a delayed response for the old selection',async()=>{
  const stream=streamFeed(),old=deferred(),fresh=deferred();let loads=0;
  const page=startMockPage(path=>path==='/api/events'?Promise.resolve({status:200,ok:true,body:stream.body}):++loads===1?old.promise:fresh.promise);
  try {
    page.elements.get('token').value='test-token';page.elements.get('connect').onclick();await tick();
    stream.push(frame(0,'snapshot',{instances:[{id:'one',port:1,state:'stopped'},{id:'two',port:2,state:'stopped'}]}));await tick();
    clickLogs(page,'one');clickLogs(page,'two');await tick();
    old.resolve(logResponse('old'));await tick();
    assert.equal(page.elements.get('output').textContent,'');
    fresh.resolve(logResponse('two'));await tick();
    assert.equal(page.elements.get('output').textContent,'two');
  } finally {page.restore()}
});

test('authentication failure stops automatic retries',async()=>{
  let calls=0;const page=startMockPage(async()=>{calls++;return {status:401,ok:false}});
  try {page.elements.get('token').value='test-token';page.elements.get('connect').onclick();await new Promise(resolve=>setImmediate(resolve));
    assert.equal(calls,1);assert.equal(page.elements.get('connection').textContent,'Authentication required');
  } finally {page.restore()}
});

test('reconnect button aborts the previous stream before opening another',async()=>{
  let active=0,maxActive=0,aborted=0;
  const page=startMockPage((_,options)=>new Promise((resolve,reject)=>{active++;maxActive=Math.max(maxActive,active);options.signal.addEventListener('abort',()=>{active--;aborted++;reject(Error('aborted'))},{once:true})}));
  try {page.elements.get('token').value='first';page.elements.get('connect').onclick();page.elements.get('token').value='second';page.elements.get('connect').onclick();await new Promise(resolve=>setImmediate(resolve));
    assert.equal(maxActive,1);assert.equal(aborted,1);
  } finally {page.restore()}
});
