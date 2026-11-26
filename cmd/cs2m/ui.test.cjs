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
  for(const id of ['token','connect','instances','output','connection','form'])elements.set(id,{value:'',textContent:'',replaceChildren(){},append(){}});
  global.document={getElementById:id=>elements.get(id)};
  global.window={addEventListener:(name,fn)=>listeners.set(name,fn)};
  global.fetch=fetchMock;
  delete require.cache[require.resolve('./ui.js')];require('./ui.js');
  return {elements,restore(){listeners.get('pagehide')?.();global.document=saved.document;global.window=saved.window;global.fetch=saved.fetch;delete require.cache[require.resolve('./ui.js')]}};
}

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
