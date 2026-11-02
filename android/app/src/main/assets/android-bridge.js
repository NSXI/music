'use strict';
(()=>{
let sequence=0,source='',position=0,length=0,paused=true,rate=1,at=performance.now(),buffering=false,ended=false,lastID='',lastError='';
const requests=new Map(),element=document.getElementById('audio');
const emit=name=>element.dispatchEvent(new Event(name));
function invoke(method,...args){return new Promise((resolve,reject)=>{const id='r-'+(++sequence);requests.set(id,{resolve,reject});try{NawaAndroid.dispatch(id,method,JSON.stringify(args))}catch(error){requests.delete(id);reject(error)}})}
const api=new Proxy({}, {get:(_,method)=>(...args)=>invoke(method,...args)});
window.go={main:{Studio:api}};
function send(name,data={}){NawaAndroid.audio(name,JSON.stringify(data))}
function clock(){return Math.min(length||Infinity,position+(!paused&&!buffering?Math.max(0,performance.now()-at)/1000*rate:0))}
function metadata(url){let track;try{track=current}catch{};return {url,id:track?.id||'',title:track?.song?.title||'Nawa',artist:track?.song?.artist||'',album:track?.song?.album||'',artwork:track?cover(track.song,track.preview?null:track.id):''}}
function queue(){try{if(current?.preview)return;const items=playOrder.map(id=>taskByID(id)).filter(t=>t?.status==='done').map(t=>({id:t.id,url:current?.id===t.id&&current.decoded?source:'/media/'+t.id,title:t.song.title,artist:t.song.artist,album:t.song.album,artwork:cover(t.song,t.id)}));if(items.length)send('queue',{items})}catch{}}
Object.defineProperties(element,{
 src:{get:()=>source,set:url=>{source=String(url);position=0;length=0;at=performance.now();ended=false;lastError='';send('load',metadata(source));emit('loadstart')}},
 currentSrc:{get:()=>source},currentTime:{get:clock,set:value=>{position=Math.max(0,Number(value)||0);at=performance.now();send('seek',{position});emit('seeking');emit('timeupdate')}},
 duration:{get:()=>length||NaN},paused:{get:()=>paused},ended:{get:()=>ended},readyState:{get:()=>length>0?4:0},
 playbackRate:{get:()=>rate,set:value=>{position=clock();at=performance.now();rate=Number(value)||1;emit('ratechange')}},
 preservesPitch:{get:()=>true,set:()=>{}},volume:{get:()=>.7,set:()=>{}},loop:{get:()=>false,set:()=>{}},
 play:{value:async()=>{queue();send('play');paused=false;at=performance.now();emit('play')}},
 pause:{value:()=>{position=clock();paused=true;at=performance.now();send('pause');emit('pause')}},
 load:{value:()=>{}},error:{get:()=>lastError?{message:lastError}:null}
});
window.NawaMobile={invoke,send,
 async reply(id){const pending=requests.get(id);if(!pending)return;requests.delete(id);try{const r=await fetch('/native-response/'+encodeURIComponent(id),{cache:'no-store'}).then(r=>r.json());if(!r.ok)throw new Error(r.error||'Android operation failed');pending.resolve(r.result)}catch(error){pending.reject(error)}},
 fx(value){position=clock();at=performance.now();rate=value.bypass?1:Number(value.speed)||1;send('fx',value)},
 state(s){const previous=paused,oldLength=length;position=Number(s.position)||0;length=Number(s.duration)||0;paused=Boolean(s.paused);buffering=Boolean(s.buffering);rate=Number(s.speed)||1;at=performance.now();
  if(s.id&&s.id!==lastID){lastID=s.id;try{const track=taskByID(s.id);if(track&&current?.id!==track.id){current={...track};ui.lyricOffset=Number(ui.lyricOffsets?.[track.id]||0);updateNow();if(view==='lyrics'||view==='player')render();loadLyrics(false)}}catch{}}
  if(length&&oldLength!==length){emit('loadedmetadata');emit('durationchange')}
  if(previous!==paused)emit(paused?'pause':'play');emit('timeupdate');
  if(s.ended&&!ended){ended=true;emit('ended')}else if(!s.ended)ended=false;
  if(s.error&&s.error!==lastError){lastError=s.error;emit('error')}
 },
 audioError(message){lastError=message;try{toast(message)}catch{}},
 suspend(){try{window.madarVerci?.suspend?.();document.body.classList.add('android-suspended')}catch{}},
 resume(){document.body.classList.remove('android-suspended');try{poll();window.madarVerci?.sync?.()}catch{}},
 back(){if(document.querySelector('.lyric-style-list,.font-picker-list')){document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));return true}const dialog=document.querySelector('dialog[open]');if(dialog){dialog.close();return true}if(window.madarFullscreen?.active){window.madarFullscreen.leave();return true}try{if(view!=='home'){navigate('home');return true}}catch{}return false},
 modelProgress(percent){const out=document.getElementById('android-model-progress');if(out)out.textContent=percent+'%'}
};
})();
