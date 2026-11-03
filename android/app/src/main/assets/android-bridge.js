'use strict';
(()=>{
let sequence=0,source='',position=0,length=0,paused=true,playing=false,rate=1,at=performance.now(),buffering=false,ended=false,lastID='',lastError='';
let loadToken=Date.now()*1000,seekToken=0,sampleAt=-1,loading=Promise.resolve(),frame=0,suspended=false,lastPoll=0,getState=null;
let clockFloor=0,timelineRevision=-1;
let queueSignature='';
const requests=new Map(),commands=new Map(),element=document.getElementById('audio');
const emit=name=>element.dispatchEvent(new Event(name));
function invoke(method,...args){
 if(method==='GetState'&&getState)return getState;
 const request=new Promise((resolve,reject)=>{const id='r-'+(++sequence);requests.set(id,{resolve,reject});try{NawaAndroid.dispatch(id,method,JSON.stringify(args))}catch(error){requests.delete(id);reject(error)}});
 if(method==='GetState'){getState=request;request.finally(()=>{if(getState===request)getState=null}).catch(()=>{})}
 return request;
}
window.go={main:{Studio:new Proxy({}, {get:(_,method)=>(...args)=>invoke(method,...args)})}};
function send(name,data={}){
 const id='a-'+(++sequence),token=loadToken;
 const promise=new Promise((resolve,reject)=>{
  const timer=setTimeout(()=>{commands.delete(id);reject(new Error('Audio command timed out / انتهت مهلة أمر الصوت'))},15000);
  commands.set(id,{resolve,reject,timer,token});
  try{NawaAndroid.audio(name,JSON.stringify({...data,_request:id,_loadToken:token}))}catch(error){clearTimeout(timer);commands.delete(id);reject(error)}
 });
 // FX, scrubbing and pause handlers may intentionally ignore their result.
 promise.catch(error=>{if(token===loadToken)window.NawaMobile.audioError(error.message)});
 return promise;
}
function clock(){
 // Extrapolate only a confirmed native clock; freeze if bridge delivery stalls.
 const elapsed=playing&&!paused&&!buffering&&!suspended?Math.min(.25,Math.max(0,performance.now()-at)/1000)*rate:0;
 const value=Math.max(0,Math.min(length||Infinity,position+elapsed));
 // Native timestamp corrections must not pull a running thumb backwards.
 // A real seek, track change or discontinuity explicitly resets this floor.
 if(playing&&!paused&&!buffering){clockFloor=Math.max(clockFloor,value);return Math.min(length||Infinity,clockFloor)}
 return value;
}
function metadata(url){let track;try{track=current}catch{};return {url,id:track?.id||'',title:track?.song?.title||'Nawa',artist:track?.song?.artist||'',album:track?.song?.album||'',duration:Number(track?.song?.duration)||0,seekable:!(track?.preview&&track?.decoded),artwork:track?cover(track.song,track.preview?null:track.id):''}}
async function queue(){
 if(current?.preview)return;
 const signature=playOrder.join('|')+'|'+Boolean(current?.decoded)+'|'+source;
 const token=loadToken;
 if(signature===queueSignature)return;
 const items=playOrder.map(id=>taskByID(id)).filter(t=>t?.status==='done').map(t=>({id:t.id,url:current?.id===t.id&&current.decoded?source:'/media/'+t.id,title:t.song.title,artist:t.song.artist,album:t.song.album,duration:Number(t.song.duration)||0,artwork:cover(t.song,t.id)}));
 if(items.length){await send('queue',{items});if(token===loadToken)queueSignature=signature}
}
function paint(){
 const value=clock(),total=length||Number(typeof current!=='undefined'?current?.song?.duration:0)||0,progress=total>0?Math.min(1,value/total):0;
 for(const id of ['seek','fs-seek']){const node=document.getElementById(id);if(node&&node.dataset.androidDragging!=='true')node.value=progress*1000}
 const label=t=>Math.floor(t/60)+':'+String(Math.floor(t%60)).padStart(2,'0');
 for(const [ids,text] of [[['elapsed','fs-elapsed'],label(value)],[['duration','fs-duration'],label(total)]])for(const id of ids){const node=document.getElementById(id);if(node&&node.textContent!==text)node.textContent=text}
 const bar=document.getElementById('fs-progress');if(bar){bar.style.setProperty('--fs-progress',progress*100+'%');bar.style.setProperty('--fs-has-progress',progress>0?'1':'0')}
}
function animate(){frame=0;if(suspended||document.hidden||paused||!playing||buffering)return;paint();frame=requestAnimationFrame(animate)}
function schedule(){if(suspended||document.hidden||paused||!playing||buffering){if(frame)cancelAnimationFrame(frame);frame=0;return}if(!frame)frame=requestAnimationFrame(animate)}
Object.defineProperties(element,{
 src:{get:()=>source,set:url=>{
  source=String(url);queueSignature='';loadToken++;seekToken=0;sampleAt=-1;position=0;clockFloor=0;timelineRevision=-1;length=0;paused=true;playing=false;buffering=true;at=performance.now();ended=false;lastError='';lastID='';schedule();
  loading=send('load',metadata(source));emit('loadstart');paint();
 }},
 currentSrc:{get:()=>source},currentTime:{get:clock,set:value=>{
  const target=Number(value);if(!Number.isFinite(target))return;
  position=Math.max(0,Math.min(length||Infinity,target));clockFloor=position;at=performance.now();seekToken++;playing=false;buffering=true;schedule();
  send('seek',{position,_seekToken:seekToken});emit('seeking');emit('timeupdate');paint();
 }},
 duration:{get:()=>length||NaN},paused:{get:()=>paused},ended:{get:()=>ended},readyState:{get:()=>length>0?4:0},
 playbackRate:{get:()=>rate,set:value=>{position=clock();at=performance.now();rate=Number(value)||1;emit('ratechange')}},
 preservesPitch:{get:()=>true,set:()=>{}},volume:{get:()=>{try{return Number(ui.volume)}catch{return .7}},set:()=>{}},loop:{get:()=>{try{return Boolean(ui.repeat)}catch{return false}},set:value=>{send('repeat',{repeat:Boolean(value)})}},
 play:{value:async()=>{const token=loadToken;await loading;if(token!==loadToken)return;await queue();if(token!==loadToken)return;await send('play')}},
 pause:{value:()=>{position=clock();paused=true;playing=false;at=performance.now();schedule();send('pause');emit('pause');paint()}},
 load:{value:()=>{}},error:{get:()=>lastError?{message:lastError}:null}
});
window.NawaMobile={invoke,send,
 async reply(id){const pending=requests.get(id);if(!pending)return;requests.delete(id);try{const r=await fetch('/native-response/'+encodeURIComponent(id),{cache:'no-store'}).then(r=>r.json());if(!r.ok)throw new Error(r.error||'Android operation failed');pending.resolve(r.result)}catch(error){pending.reject(error)}},
 audioReply(id,state,error){const pending=commands.get(id);if(!pending)return;commands.delete(id);clearTimeout(pending.timer);if(error){pending.reject(new Error(error));return}if(pending.token===loadToken&&state)this.state(state);pending.resolve(state)},
 fx(value){position=clock();at=performance.now();send('fx',value)},
 state(s){
  if(!source&&s.id){loadToken=Number(s.loadToken);seekToken=Number(s.seekToken)||0;source=s.url||''}
  if(Number(s.loadToken)!==loadToken||Number(s.seekToken||0)<seekToken||Number(s.sampledAt||0)<sampleAt)return;
  sampleAt=Number(s.sampledAt)||0;
  const previous=paused,wasBuffering=buffering,oldLength=length;
  const epoch=Number(s.timelineRevision)||0;if(epoch!==timelineRevision||s.id!==lastID){clockFloor=Math.max(0,Number(s.position)||0);timelineRevision=epoch}
  position=Math.max(0,Number(s.position)||0);length=Math.max(0,Number(s.duration)||0);paused=Boolean(s.paused);playing=Boolean(s.playing);buffering=Boolean(s.buffering);rate=Number(s.speed)||1;at=performance.now();
  if(s.id&&s.id!==lastID){try{const track=taskByID(s.id);if(track){lastID=s.id;if(current?.id!==track.id){current={...track};ui.lyricOffset=Number(ui.lyricOffsets?.[track.id]||0);updateNow();if(view==='lyrics'||view==='player')render();loadLyrics(false)}}}catch{}}
  if(length&&oldLength!==length){emit('loadedmetadata');emit('durationchange')}
  if(previous!==paused)emit(paused?'pause':'play');if(wasBuffering!==buffering)emit(buffering?'waiting':'playing');
  emit('timeupdate');paint();schedule();
  if(s.ended&&!ended){ended=true;emit('ended')}else if(!s.ended)ended=false;
  if(s.error&&s.error!==lastError){lastError=s.error;emit('error')}
 },
 audioError(message){lastError=message;try{toast(message)}catch{}},
 suspend(){position=clock();at=performance.now();suspended=true;schedule();try{window.madarVerci?.suspend?.();document.body.classList.add('android-suspended')}catch{}},
 resume(){suspended=false;document.body.classList.remove('android-suspended');try{poll();window.madarVerci?.sync?.()}catch{};schedule()},
 poll(){
  if(suspended||document.hidden||getState)return;
  let active=false;try{active=snapshot.jobs.some(t=>['queued','working'].includes(t.status))||snapshot.alignment?.running||snapshot.translation?.running||snapshot.folderImport?.running}catch{}
  const interval=active?950:playing?3000:2000;if(performance.now()-lastPoll<interval)return;lastPoll=performance.now();try{poll()}catch{}
 },
 back(){if(document.querySelector('.lyric-style-options,.lyric-style-list,.font-picker-list')){document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));return true}const dialog=document.querySelector('dialog[open]');if(dialog){dialog.close();return true}if(window.madarFullscreen?.active){window.madarFullscreen.leave();return true}try{if(view!=='home'){navigate('home');return true}}catch{}return false},
 modelProgress(percent){const out=document.getElementById('android-model-progress');if(out)out.textContent=percent+'%'},
 status(){return {loadToken,seekToken,position:clock(),playing,buffering,paused,pendingAudio:commands.size,pendingRPC:requests.size}}
};
for(const event of ['pointerdown','touchstart'])document.addEventListener(event,e=>{if(['seek','fs-seek'].includes(e.target.id))e.target.dataset.androidDragging='true'},{passive:true});
for(const event of ['pointerup','pointercancel','touchend','touchcancel','change','blur'])document.addEventListener(event,()=>{for(const id of ['seek','fs-seek']){const node=document.getElementById(id);if(node)delete node.dataset.androidDragging}},{capture:true,passive:true});
document.addEventListener('visibilitychange',()=>{if(document.hidden){position=clock();at=performance.now()}schedule()});
})();
