require('node:fs').mkdirSync('qa/results',{recursive:true});
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const {parseHTML}=require('linkedom');
const front='frontend/';
async function boot(language,override={}){
 const {window,document}=parseHTML(fs.readFileSync(front+'index.html','utf8'));
 const prototype=Object.getPrototypeOf(document.createElement('select')),value=Object.getOwnPropertyDescriptor(prototype,'value');
 if(!value.set)Object.defineProperty(prototype,'value',{configurable:true,get:value.get,set(v){const options=[...this.querySelectorAll('option')];for(const o of options)o.removeAttribute('selected');options.find(o=>o.value===v)?.setAttribute('selected','')}});
 window.HTMLElement.prototype.scrollTo=function({top}){this.scrollTop=top};
 const dialog=document.querySelector('#modal');dialog.showModal=()=>{dialog.open=true};dialog.close=()=>{dialog.open=false};
 const audio=document.querySelector('#audio');audio.paused=true;audio.currentTime=7;audio.src='original-audio.flac';let audioActions=0;
 audio.play=async()=>{audio.paused=false;audioActions++};audio.pause=()=>{audio.paused=true;audioActions++};
 window.matchMedia=()=>({matches:false});
 const calls=[];window.go={main:{Studio:{GetState:async()=>({jobs:[],preferences:{},alignment:{ready:true},translation:{ready:true}}),SetLyricsFullscreen:async enabled=>calls.push(enabled)}}};
 const storage=new Map([['madar-ui-v1',JSON.stringify({language,volume:.52,translationTarget:'ar',lyricStyle:'verci-ship',...override})]]);
 const ctx=vm.createContext({window,document,console,URL,Intl,CSS:{escape:x=>x},setInterval:()=>1,setTimeout:()=>1,clearTimeout(){},requestAnimationFrame:()=>1,cancelAnimationFrame(){},localStorage:{getItem:key=>storage.get(key),setItem:(key,v)=>storage.set(key,v)},MutationObserver:window.MutationObserver,NodeFilter:{SHOW_TEXT:4},CustomEvent:window.CustomEvent,Event:window.Event,performance,queueMicrotask,Uint8Array});
 for(const script of ['motion-presets.js','motion-library.js','lyric-sequence.js','app.js','fullscreen.js','lyric-art.js','resource-controls.js','verci-lyrics.js','lyric-clock.js','metadata-library.js','locale-en.js','i18n.js','nawa-logos.js','nawa-themes.js','lyric-display.js','lyric-style-picker.js','font-previews.js'])vm.runInContext(fs.readFileSync(front+script,'utf8'),ctx,{filename:script});
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(document.documentElement.lang,language);
 assert.equal(vm.runInContext('ui.lyricStyle',ctx),override.lyricStyle||'verci-ship');
 assert.equal(document.querySelector('[data-nav=home] span').textContent,language==='en'?'Home':'الرئيسية');
 assert.equal(document.querySelector('#app-language').value,language);
 const initialSource=audio.src,initialTime=audio.currentTime;
 window.madarI18n.setLanguage(language==='en'?'ar':'en');
 assert.equal(audioActions,0);assert.equal(audio.src,initialSource);assert.equal(audio.currentTime,initialTime);assert.equal(calls.length,0);
 assert.equal(vm.runInContext('ui.volume',ctx),.52);assert.equal(vm.runInContext('ui.translationTarget',ctx),'ar');
 assert.equal(JSON.parse(storage.get('madar-ui-v1')).language,language==='en'?'ar':'en');
 // Event-driven switching also works after a real application render.
 vm.runInContext('navigate("settings")',ctx);await new Promise(resolve=>setImmediate(resolve));
 const picker=document.querySelector('#language-panel-picker');picker.value=language;picker.dispatchEvent(new window.Event('change',{bubbles:true}));
 assert.equal(document.documentElement.lang,language);assert.equal(document.querySelector('#app-language').value,language);
 assert.equal(document.querySelector('h1').textContent,language==='en'?'Settings and sources':'الإعدادات والمصادر');
 assert.equal(audioActions,0);
 return {ctx,window,document,audio,storage,calls};
}
module.exports={boot};
if(require.main===module)(async()=>{await boot('en');await boot('ar');console.log('PASS: full application boot in both saved languages; real render and language-change events; no playback, position, volume or lyrics-language changes. No desktop UI opened.');})().catch(error=>{console.error(error);process.exitCode=1});

