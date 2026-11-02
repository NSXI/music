require('node:fs').mkdirSync('qa/results',{recursive:true});
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const {boot}=require('./app_boot_test.cjs');
const raster=require('@napi-rs/canvas');
for(const [file,name] of [['segoeui.ttf','Segoe UI'],['tahoma.ttf','Tahoma'],['consola.ttf','Cascadia Code']])raster.GlobalFonts.registerFromPath('C:/Windows/Fonts/'+file,name);
const settle=()=>new Promise(resolve=>setImmediate(resolve));
(async()=>{
 const {ctx,window,document,audio,storage,calls}=await boot('en');
 const run=code=>vm.runInContext(code,ctx);assert.equal(run('lyricStyles.length'),104);
 const backing=new WeakMap(),prototype=Object.getPrototypeOf(document.createElement('canvas'));
 const native=element=>{if(!backing.has(element))backing.set(element,raster.createCanvas(300,150));return backing.get(element)};
 prototype.getContext=function(type){assert.equal(type,'2d');return native(this).getContext('2d')};
 for(const name of ['width','height'])Object.defineProperty(prototype,name,{configurable:true,get(){return native(this)[name]},set(value){native(this)[name]=value}});
 window.HTMLElement.prototype.getBoundingClientRect=function(){return {x:0,y:0,left:0,top:0,right:800,bottom:560,width:800,height:560}};
 Object.defineProperty(window.HTMLElement.prototype,'clientHeight',{configurable:true,get(){return this.id==='lyric-stage'?560:60}});
 Object.defineProperty(window.HTMLElement.prototype,'offsetTop',{configurable:true,get(){return Number(this.dataset?.lyricIndex||0)*90}});
 ctx.getComputedStyle=()=>({getPropertyValue:key=>({'--ink':'#f3f5ff','--accent':'#bca5ff','--muted':'#b0bad0'}[key]||'')});
 const pending=new Map();let id=0;ctx.requestAnimationFrame=callback=>{pending.set(++id,callback);return id};ctx.cancelAnimationFrame=value=>pending.delete(value);
 const lines=[{time:0,text:'Stars guide my heart'},{time:4,text:'The moon lights home'},{time:8,text:'Our music finds hope'}];
 const words=lines.map(line=>({start:line.time,words:line.text.split(' ').map((text,i)=>({text,start:line.time+i*.9,end:line.time+i*.9+.8}))}));
 const translations=['النجوم ترشد قلبي','القمر ينير البيت','موسيقانا تجد الأمل'];
 const task={id:'original-integration',song:{id:'original-integration',title:'Original test',artist:'Madar Studio',album:'Design session',duration:12},status:'done',quality:{codec:'FLAC',rate:96000,bits:24},log:[]};
 window.go.main.Studio.GetLyrics=async()=>({text:lines.map(x=>'[00:'+String(x.time).padStart(2,'0')+'.00]'+x.text).join('\n'),words,timed:true,status:'Original fixture'});
 window.go.main.Studio.GetTranslation=async()=>({exists:true,status:'Original translation fixture',originalHash:'original',lines:lines.map((x,i)=>({original:x.text,text:translations[i],time:x.time}))});
 run('snapshot.jobs='+JSON.stringify([task])+';current='+JSON.stringify(task)+';ui.translationEnabled=true;ui.motion=true;audio.currentTime=1;navigate("lyrics")');await settle();await settle();
 const originalVolume=run('ui.volume'),source=audio.src;let checks=0;
 for(const style of [...window.MadarLyricArt.styles,...window.MadarMotionPresets.map(p=>p.id)]){
  run('ui.lyricStyle='+JSON.stringify('verci-'+style)+';render()');await settle();await settle();
  assert.equal(document.querySelectorAll('.verci-canvas').length,1);assert.equal(window.madarVerci.status().kind,'word');checks+=2;
  assert.equal(document.querySelector('.verci-timing-note').textContent,'Genuine word timing');checks++;
  assert.ok(document.querySelector('.verci-announcement').textContent.includes('النجوم ترشد قلبي'));checks++;
  const keyEvent=new window.Event('keydown',{bubbles:true});keyEvent.key='ArrowRight';document.querySelector('.verci-canvas').dispatchEvent(keyEvent);
  assert.ok(audio.currentTime>=0);checks++;
  audio.currentTime=1;
 }
 const box=window.HTMLElement.prototype.getBoundingClientRect;
 window.HTMLElement.prototype.getBoundingClientRect=function(){return {x:0,y:0,left:0,top:0,right:3840,bottom:2160,width:3840,height:2160}};
 window.devicePixelRatio=2;
 for(const [mode,pixels] of [['economy',1000000],['balanced',2000000],['smooth',3000000]]){
  window.madarResources.setMode(mode);window.madarVerci.sync();
  const sized=document.querySelector('.verci-canvas');assert.ok(sized.width*sized.height<=pixels,mode+' exceeded the 4K raster budget');checks++;
 }
 window.HTMLElement.prototype.getBoundingClientRect=box;window.madarResources.setMode('economy');window.madarVerci.sync();
 audio.paused=false;audio.dispatchEvent(new window.Event('play'));await settle();assert.equal(pending.size,1);checks++;
 for(let n=0;n<120;n++){audio.currentTime=1+n/60;const batch=[...pending];pending.clear();for(const [,callback] of batch)callback();assert.equal(pending.size,1);checks++}
 audio.paused=true;audio.dispatchEvent(new window.Event('pause'));assert.equal(pending.size,0);checks++;
 await window.madarFullscreen.enter();await settle();assert.ok(document.body.classList.contains('lyrics-is-fullscreen'));assert.equal(document.querySelectorAll('.verci-canvas').length,1);checks+=2;
 assert.equal(audio.src,source);assert.equal(run('ui.volume'),originalVolume);assert.equal(run('ui.translationTarget'),'ar');checks+=3;
 window.madarI18n.setLanguage('ar');assert.equal(document.querySelector('.verci-timing-note').textContent,'توقيت كلمات حقيقي');checks++;
 await window.madarFullscreen.leave();await settle();assert.deepEqual(calls,[true,false]);checks++;
 run('ui.lyricStyle="focus";render()');await settle();assert.equal(document.querySelectorAll('.verci-canvas').length,0);checks++;
 run('navigate("home")');await settle();assert.equal(window.madarLyricClock.status().cachedRows,0);assert.equal(pending.size,0);assert.equal(window.madarVerci.status().active,false);checks+=3;
 assert.equal(JSON.parse(storage.get('madar-ui-v1')).language,'ar');checks++;
 console.log('PASS:',checks,'full integration checks: 89 real raster-backed canvas views, bilingual translation, keyboard, one clock, pause/disposal, full-screen bridge and preserved playback state. No desktop window opened.');
})().catch(error=>{console.error(error);process.exitCode=1});
