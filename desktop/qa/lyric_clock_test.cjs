require('node:fs').mkdirSync('qa/results',{recursive:true});
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict'),{performance}=require('node:perf_hooks');
const {parseHTML}=require('linkedom');
const rows=500,words=10;
const data=Array.from({length:rows},(_,i)=>({time:i*5,text:'Original test line '+i}));
function chars(text){return [...text].map(character=>`<span class="lyric-char">${character}</span>`).join('')}
const html=data.map((line,i)=>`<button class="lyric-line" data-lyric-index="${i}">${Array.from({length:words},(_,j)=>`<span class="lyric-word separate" data-start="${i*5+j*.35}" data-end="${i*5+j*.35+.3}">${chars('word'+j)}</span>`).join('')}<span class="lyric-translation" dir="rtl"><span class="lyric-word joined" data-start="-1" data-end="-1">${chars('كلمات')}</span><span class="lyric-word joined" data-start="-1" data-end="-1">${chars('أصلية')}</span></span></button>`).join('');
const {window,document}=parseHTML('<!doctype html><html><body><main id="main"><div id="lyric-stage">'+html+'</div></main><audio id="audio"></audio></body></html>');
const audio=document.querySelector('#audio');audio.paused=true;audio.currentTime=0;audio.ended=false;
let hidden=false;Object.defineProperty(document,'hidden',{get:()=>hidden});
window.HTMLElement.prototype.scrollTo=function({top}){this.scrollTop=top};
const pending=new Map();let nextID=0,canvasFrames=0;
const ctx=vm.createContext({window,document,audio,view:'lyrics',lyricData:data,lyricWordLines:data.map(line=>({start:line.time})),lyricTimed:true,lyricActive:-2,ui:{motion:true,lyricOffset:0},duration:()=>rows*5,$:s=>document.querySelector(s),MutationObserver:window.MutationObserver,requestAnimationFrame:callback=>{const id=++nextID;pending.set(id,callback);return id},cancelAnimationFrame:id=>pending.delete(id),updateLyrics:()=>window.madarLyricClock.tick(),console});
window.matchMedia=()=>({matches:false,addEventListener(){}});window.madarVerci={frame(){canvasFrames++}};
vm.runInContext(fs.readFileSync('frontend/lyric-clock.js','utf8'),ctx);
const clock=window.madarLyricClock,stage=document.querySelector('#lyric-stage');
let checks=0;
function check(position){
 audio.currentTime=position;clock.tick();
 const index=Math.min(rows-1,Math.floor(position/5)),active=document.querySelector('.lyric-line.current');
 assert.equal(active?.dataset.lyricIndex,position<0?undefined:String(index));checks++;
 if(position<0)return;
 const original=[...active.querySelectorAll('.lyric-word')].filter(word=>!word.closest('.lyric-translation'));
 for(const word of original){const start=+word.dataset.start,end=+word.dataset.end;assert.equal(word.classList.contains('sung'),position>=end);assert.equal(word.classList.contains('singing'),position>=start&&position<end);checks+=2;const nodes=[...word.querySelectorAll('.lyric-char')],progress=Math.max(0,Math.min(1,(position-start)/(end-start)));for(let i=0;i<nodes.length;i++){assert.equal(nodes[i].classList.contains('sung'),i/nodes.length<progress);checks++}}
 assert.equal(active.querySelector('.lyric-translation').dataset.timing,'line-visual');
 assert.equal(active.querySelector('.lyric-translation .lyric-word').dataset.start,'-1');checks+=2;
}
async function test(){
 assert.equal(clock.status().cachedRows,500);assert.equal(pending.size,0);checks+=2;
 for(const position of [-1,0,.1,.31,.36,2.9,4.99,5,799.7,3.01,2499.9])check(position);
 const rebuilds=clock.status().rebuilds;
 stage.append(document.createElement('canvas'));await new Promise(resolve=>setImmediate(resolve));
 assert.equal(clock.status().rebuilds,rebuilds,'Adding the independent canvas must not rebuild lyric caches');checks++;
 audio.paused=false;audio.dispatchEvent(new window.Event('play'));assert.equal(pending.size,1);checks++;
 const started=performance.now();
 for(let frame=0;frame<3600;frame++){
  audio.currentTime=frame/60;
  const queued=[...pending.entries()];assert.equal(queued.length,1);pending.delete(queued[0][0]);queued[0][1](frame*1000/60);checks++;
 }
 const elapsed=performance.now()-started;
 assert.equal(clock.status().rebuilds,rebuilds);assert.equal(canvasFrames,3600);checks+=2;
 audio.paused=true;audio.dispatchEvent(new window.Event('pause'));assert.equal(pending.size,0);assert.ok(stage.classList.contains('lyric-clock-paused'));checks+=2;
 const translationsBefore=clock.status().rebuilds,currentTranslation=stage.querySelector('.lyric-line.current .lyric-translation');currentTranslation.innerHTML='<span class="lyric-word joined" data-start="-1" data-end="-1">'+chars('ترجمة جديدة')+'</span>';
 await new Promise(resolve=>setImmediate(resolve));assert.equal(clock.status().rebuilds,translationsBefore+1);assert.equal(pending.size,0);assert.equal(currentTranslation.dataset.timing,'line-visual');checks+=3;
 audio.paused=false;audio.dispatchEvent(new window.Event('play'));hidden=true;document.dispatchEvent(new window.Event('visibilitychange'));assert.equal(pending.size,0);checks++;
 hidden=false;document.dispatchEvent(new window.Event('visibilitychange'));assert.equal(pending.size,1);checks++;
 ctx.ui.motion=false;document.body.classList.add('no-motion');await new Promise(resolve=>setImmediate(resolve));assert.equal(pending.size,0);checks++;
 check(55.16);
 ctx.view='library';document.querySelector('#main').innerHTML='<p>Library</p>';await new Promise(resolve=>setImmediate(resolve));assert.equal(clock.status().cachedRows,0);assert.equal(pending.size,0);checks+=2;
 const result={checks,rows,alignedWords:rows*words,frames:3600,domCpuMilliseconds:Math.round(elapsed*10)/10,meanDomCpuMillisecondsPerFrame:Math.round(elapsed/3600*1000)/1000,metrics:clock.status(),note:'Synthetic Node/DOM CPU test; not a GPU frame-rate or real playback measurement.'};
 fs.writeFileSync('qa/results/lyric-clock-results.json',JSON.stringify(result,null,2));console.log(JSON.stringify(result,null,2));
}
test().catch(error=>{console.error(error);process.exitCode=1});
