require('node:fs').mkdirSync('qa/results',{recursive:true});
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const {parseHTML}=require('linkedom');
let checks=0;
async function budgets(){
 const html=Array.from({length:4},(_,i)=>`<button class="lyric-line" data-lyric-index="${i}"><span class="lyric-content"><span class="lyric-word" data-start="${i*5}" data-end="${i*5+4}"><span class="lyric-char">A</span><span class="lyric-char">B</span></span></span></button>`).join('');
 const {window,document}=parseHTML('<html><head></head><body><main id="main"><div id="lyric-stage">'+html+'</div></main><audio id="audio"></audio></body></html>');
 window.HTMLElement.prototype.scrollTo=function(){};window.matchMedia=()=>({matches:false,addEventListener(){}});
 const audio=document.querySelector('#audio');audio.paused=true;audio.currentTime=0;audio.ended=false;
 let hidden=false;Object.defineProperty(document,'hidden',{get:()=>hidden});
 const pending=new Map();let id=0,rendered=0;window.madarVerci={frame(){rendered++},sync(){}};
 const saved=[],backend=[];window.go={main:{Studio:{SetResourceMode:async mode=>backend.push(mode)}}};
 const ctx=vm.createContext({window,document,audio,view:'lyrics',lyricData:Array.from({length:4},(_,i)=>({time:i*5,text:'AB'})),lyricWordLines:[],lyricTimed:true,lyricActive:-2,ui:{motion:true,lyricOffset:0,volume:.52,speed:.8,translationTarget:'ar'},persist:()=>saved.push(JSON.stringify(ctx.ui)),duration:()=>20,$:s=>document.querySelector(s),MutationObserver:window.MutationObserver,CustomEvent:window.CustomEvent,requestAnimationFrame:callback=>{const key=++id;pending.set(key,callback);return key},cancelAnimationFrame:key=>pending.delete(key),updateLyrics:()=>window.madarLyricClock.tick(),console});
 for(const name of ['resource-controls.js','lyric-clock.js'])vm.runInContext(fs.readFileSync('frontend/'+name,'utf8'),ctx);
 const resources=window.madarResources,clock=window.madarLyricClock;
 assert.equal(resources.budget().mode,'economy');assert.equal(resources.budget().fps,240);assert.equal(ctx.ui.lyricFps,'auto');assert.ok(!resources.settings().includes('value="30"'));checks+=4;
 const result={};
 for(const [mode,fps,dpr,maxPixels] of [['economy',240,1,1000000],['balanced',240,1.5,2000000],['smooth',240,2,3000000]]){
  resources.setMode(mode);await new Promise(resolve=>setImmediate(resolve));assert.deepEqual(JSON.parse(JSON.stringify(resources.budget())),{mode,fps,dpr,maxPixels});assert.equal(JSON.parse(saved.at(-1)).performanceMode,mode);assert.equal(backend.at(-1),mode);checks+=3;
  audio.paused=false;audio.dispatchEvent(new window.Event('play'));const before=clock.status().frames,painted=rendered;
  for(let frame=0;frame<600;frame++){
   audio.currentTime=frame/60;
   assert.equal(pending.size,1);checks++;
   const [key,callback]=pending.entries().next().value;pending.delete(key);callback(frame*1000/60);
  }
  const actual=clock.status().frames-before;assert.equal(actual,600);assert.equal(rendered-painted,actual);checks+=2;result[mode]={paintedFrames:actual,seconds:10,simulatedDisplayFps:60,autoCeiling:fps,dpr,maxPixels};
  audio.currentTime=.5;audio.dispatchEvent(new window.Event('seeked'));assert.equal(clock.status().running,true);assert.equal(+document.querySelector('.lyric-word').style.getPropertyValue('--word-progress'),.125);checks+=2;
  audio.paused=true;audio.dispatchEvent(new window.Event('pause'));assert.equal(pending.size,0);checks++;
 }
 result.fpsChoices={};
 for(const [setting,target] of [['60',60],['120',120],['144',144],['auto',240]]){
  resources.setFps(setting);assert.equal(resources.budget().fps,target);assert.equal(JSON.parse(saved.at(-1)).lyricFps,setting);checks+=2;
  audio.paused=false;audio.dispatchEvent(new window.Event('play'));const before=clock.status().frames;
  for(let frame=0;frame<2400;frame++){audio.currentTime=frame/240;const [key,callback]=pending.entries().next().value;pending.delete(key);callback(frame*1000/240)}
  const actual=clock.status().frames-before;assert.ok(Math.abs(actual-target*10)<=1,'Chosen FPS cadence: '+setting+' actual='+actual);checks++;
  result.fpsChoices[setting]={paintedFrames:actual,seconds:10,simulatedDisplayFps:240};
  audio.paused=true;audio.dispatchEvent(new window.Event('pause'));assert.equal(pending.size,0);checks++;
 }
 assert.equal(ctx.ui.volume,.52);assert.equal(ctx.ui.speed,.8);assert.equal(ctx.ui.translationTarget,'ar');checks+=3;
 audio.paused=false;audio.dispatchEvent(new window.Event('play'));hidden=true;document.dispatchEvent(new window.Event('visibilitychange'));assert.equal(pending.size,0);checks++;
 hidden=false;document.dispatchEvent(new window.Event('visibilitychange'));assert.equal(pending.size,1);checks++;
 audio.paused=true;audio.dispatchEvent(new window.Event('pause'));
 window.go.main.Studio.SetResourceMode=async()=>{throw new Error('test backend unavailable')};resources.setMode('economy');await new Promise(resolve=>setImmediate(resolve));assert.equal(resources.status().mode,'economy');assert.equal(resources.status().backendError,'test backend unavailable');checks+=2;
 delete window.go.main.Studio.SetResourceMode;resources.setMode('balanced');assert.equal(resources.budget().mode,'balanced');checks++;
 return result;
}
function compact(){
 const prefix=fs.readFileSync('qa/ui_logic_test.cjs','utf8').split('const song=')[0];
 const {run,documentEvents,get,ctx}=new Function('require',prefix+';return {run:s=>vm.runInContext(s,ctx),documentEvents,get,ctx}')(require);
 const samples=['Music brings us together','الموسيقى تجمعنا معًا','संगीत हमें एक साथ लाता है','كلمات English 👩‍👩‍👧‍👧 é 123','<b>literal text</b>'];
 const styles=['ship','fisheye','fisheye-visual','visual','code','code-visual','branches','web','constellation'];
 let expandedNodes=0,compactNodes=0;
 for(const text of samples){
  run('ui.lyricStyle="focus"');const full=run('renderDirectionalText('+JSON.stringify(text)+')');
  const fullDOM=parseHTML('<html><body>'+full+'</body></html>').document;expandedNodes+=fullDOM.body.querySelectorAll('*').length;
  for(const style of styles){run('ui.lyricStyle="verci-'+style+'"');const html=run('renderDirectionalText('+JSON.stringify(text)+')'),{document}=parseHTML('<html><body>'+html+'</body></html>');assert.equal(document.querySelector('.lyric-content').textContent,text);assert.equal(document.querySelector('.sr-only').textContent,text);assert.equal(document.querySelectorAll('.lyric-word,.lyric-char').length,0);checks+=3;if(style==='ship')compactNodes+=document.body.querySelectorAll('*').length}
  run('ui.lyricStyle="letters"');assert.ok(run('renderDirectionalText('+JSON.stringify(text)+')').includes('lyric-char'));checks++;
 }
 run('view="lyrics";ui.lyricStyle="focus";globalThis.resourceRenderCalls=0;render=()=>{globalThis.resourceRenderCalls++}');
 const change=documentEvents.find(event=>event.type==='change').handler;
 for(const [style,total] of [['verci-ship',1],['verci-web',1],['focus',2],['poster',2]]){change({target:{id:'lyric-style',value:style,dataset:{}}});assert.equal(run('resourceRenderCalls'),total);checks++}
 vm.runInContext(fs.readFileSync('frontend/fullscreen.js','utf8'),ctx);run('window.madarFullscreen.active=true');
 const play=get('#fs-play');let writes=0,icon=play.innerHTML;Object.defineProperty(play,'innerHTML',{get:()=>icon,set:value=>{writes++;icon=value}});
 run('window.madarFullscreen.sync();window.madarFullscreen.sync();window.madarFullscreen.sync()');assert.equal(writes,1);checks++;
 get('#audio').paused=false;run('window.madarFullscreen.sync();window.madarFullscreen.sync()');assert.equal(writes,2);checks++;
 return {samples:samples.length,canvasStyles:9,expandedDomElements:expandedNodes,compactDomElements:compactNodes,nodeReductionPercent:Math.round((1-compactNodes/expandedNodes)*1000)/10,note:'DOM element count is a memory proxy, not a process RAM measurement.'};
}
(async()=>{const result={checks:0,modes:await budgets(),dom:compact(),note:'Synthetic background DOM/RAF tests. No OS CPU/RAM or actual GPU frame rate claim.'};result.checks=checks;fs.writeFileSync('qa/results/resource-frontend-results.json',JSON.stringify(result,null,2));console.log(JSON.stringify(result,null,2));})().catch(error=>{console.error(error);process.exitCode=1});
