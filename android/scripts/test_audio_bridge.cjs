const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict'),path=require('node:path');
let now=0,native={loadToken:0,seekToken:0,sampledAt:0,position:0,duration:20,paused:true,playing:false,buffering:false,id:''};
const commands=[],rpc=[],frames=new Map(),listeners=new Map();let nextFrame=0;
const audio=new EventTarget(),nodes=new Map(['audio','seek','elapsed','duration','fs-seek','fs-progress'].map(id=>[id,id==='audio'?audio:{dataset:{},style:{setProperty(){}},textContent:'',value:0}]));
const track={id:'one',status:'done',song:{title:'Original fixture',duration:20}};
const context={console,Event,EventTarget,Map,Date,Number,Promise,Math,JSON,setTimeout,clearTimeout,performance:{now:()=>now},requestAnimationFrame:fn=>{frames.set(++nextFrame,fn);return nextFrame},cancelAnimationFrame:id=>frames.delete(id),
 document:{hidden:false,getElementById:id=>nodes.get(id),addEventListener:(name,fn)=>listeners.set(name,fn),body:{classList:{add(){},remove(){}}}},
 current:track,playOrder:['one'],taskByID:id=>id==='one'?track:null,cover:()=>'',ui:{volume:.7},snapshot:{jobs:[]},view:'home',poll(){},toast(){},updateNow(){},
 NawaAndroid:{audio:(name,json)=>commands.push({name,data:JSON.parse(json)}),dispatch:(id,method,args)=>rpc.push({id,method,args})}};
context.window=context;vm.createContext(context);
vm.runInContext(fs.readFileSync(path.join(__dirname,'../app/src/main/assets/android-bridge.js'),'utf8'),context);
function reply(command,patch={}){if(command.name==='load')native={...native,loadToken:command.data._loadToken,id:command.data.id,position:0,paused:true,playing:false};native={...native,...patch,sampledAt:native.sampledAt+1};context.NawaMobile.audioReply(command.data._request,native,null)}
async function flush(){for(let i=0;i<8;i++)await Promise.resolve()}
(async()=>{
 audio.src='/media/one';const firstToken=commands.at(-1).data._loadToken;
 const playing=audio.play();now=2500;assert.equal(audio.currentTime,0,'No fake progress while native load is pending');
 reply(commands[0]);await flush();assert.equal(commands.at(-1).name,'queue');reply(commands.at(-1));await flush();assert.equal(commands.at(-1).name,'play');
 reply(commands.at(-1),{paused:false,playing:false,buffering:true});await playing;now+=1500;assert.equal(audio.currentTime,0,'Buffering stays frozen');assert.equal(audio.paused,false,'Pending play keeps pause control available');
 context.NawaMobile.state({...native,sampledAt:10,position:2,playing:true,buffering:false});now+=50;assert.ok(Math.abs(audio.currentTime-2.05)<.0001);
 now+=1000;assert.equal(audio.currentTime,2.25,'Stalled delivery has bounded extrapolation');assert.ok(frames.size===1,'One transport frame scheduled');
 context.NawaMobile.state({...native,sampledAt:11,position:3,playing:true,paused:false,buffering:false});const [frameID,callback]=frames.entries().next().value;frames.delete(frameID);callback();assert.ok(Number(nodes.get('seek').value)>100);
 audio.currentTime=7;const seek=commands.at(-1);assert.equal(seek.name,'seek');context.NawaMobile.state({...native,sampledAt:12,position:3.1,playing:true,paused:false,buffering:false});assert.equal(audio.currentTime,7,'Old seek echo cannot snap the thumb back');reply(seek,{seekToken:seek.data._seekToken,position:7,playing:false,buffering:true,paused:false});
 assert.equal(audio.currentTime,7);context.NawaMobile.state({...native,sampledAt:20,position:7,playing:true,buffering:false,paused:false});audio.pause();const frozen=audio.currentTime;now+=2000;assert.equal(audio.currentTime,frozen);assert.equal(frames.size,0);reply(commands.at(-1),{paused:true,playing:false});
 audio.src='/media/one';assert.notEqual(commands.at(-1).data._loadToken,firstToken);context.NawaMobile.state({...native,loadToken:firstToken,sampledAt:1000,position:12,playing:true,paused:false});assert.equal(audio.currentTime,0,'Previous load cannot take over the new song');reply(commands.at(-1),{seekToken:0,position:0,paused:true,playing:false});
 const a=context.NawaMobile.invoke('GetState'),b=context.NawaMobile.invoke('GetState');assert.equal(a,b);assert.equal(rpc.length,1,'Overlapping full library requests coalesced');
 // Settle the test RPC directly; app responses are fetched asynchronously.
 context.fetch=async()=>({json:async()=>({ok:true,result:{jobs:[]}})});await context.NawaMobile.reply(rpc[0].id);await a;
 assert.equal(context.NawaMobile.status().pendingAudio,0);console.log('Android audio bridge: load/play acknowledgements, buffering, smooth clock, stale loads/seeks, pause, frame cleanup and RPC coalescing passed.');
})().catch(error=>{console.error(error);process.exitCode=1});
