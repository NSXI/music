'use strict';
// One visible-playback clock shared by DOM lyrics and the optional visual renderer.
// Translated progress is a line-based visual effect, never independent word timing.
(()=>{
let cache=null,dirty=true,frameID=0,lastTickPosition=NaN,lastFrameStamp=NaN,stageObserver=null,suspendedStage=null,suspendedState=null;
const metrics={frames:0,skippedFrames:0,ticks:0,rebuilds:0,lineTransitions:0,wordWrites:0,charWrites:0,progressWrites:0};
const reduced=window.matchMedia?.('(prefers-reduced-motion: reduce)');
const clamp=value=>Math.max(0,Math.min(1,value));
const position=()=>Number(audio.currentTime||0)+Number(ui.lyricOffset||0);
function upperBound(values,value){let low=0,high=values.length;while(low<high){const mid=(low+high)>>>1;if(values[mid]<=value)low=mid+1;else high=mid}return low}
function canRun(){return view==='lyrics'&&lyricData.length>0&&!audio.paused&&!audio.ended&&!document.hidden&&ui.motion!==false&&!reduced?.matches}
function setSuspended(){const stage=cache?.stage||$('#lyric-stage'),suspended=!canRun();if(stage&&(stage!==suspendedStage||suspended!==suspendedState)){stage.classList.toggle('lyric-clock-paused',suspended);suspendedStage=stage;suspendedState=suspended}}
function frameRate(){return Math.max(60,Math.min(240,Number(window.madarResources?.budget?.().fps)||240))}
function stop(){if(frameID){cancelAnimationFrame(frameID);frameID=0}lastFrameStamp=NaN;setSuspended()}
function dispose(){stop();stageObserver?.disconnect();stageObserver=null;cache=null;dirty=true;lastTickPosition=NaN;suspendedStage=null;suspendedState=null}
function schedule(){if(canRun()){if(!frameID)frameID=requestAnimationFrame(onFrame)}else stop();setSuspended()}
function onFrame(stamp){frameID=0;if(!canRun()){stop();return}const now=Number.isFinite(stamp)?stamp:Date.now(),interval=1000/frameRate();if(Number.isFinite(lastFrameStamp)&&now+.01<lastFrameStamp){metrics.skippedFrames++;schedule();return}const late=Number.isFinite(lastFrameStamp)?Math.max(0,now-lastFrameStamp):0;lastFrameStamp=now+interval-(late%interval);metrics.frames++;updateLyrics();window.madarVerci?.frame?.(position());schedule()}
function invalidate(){dirty=true;lastTickPosition=NaN;schedule()}
function observeStage(stage){
 stageObserver?.disconnect();stageObserver=null;
 if(typeof MutationObserver==='undefined'||!stage)return;
 stageObserver=new MutationObserver(records=>{
  for(const record of records){
   if(record.type!=='childList')continue;
   // A canvas attached next to the lyric rows does not change cached text nodes.
   const target=record.target.nodeType===1?record.target:record.target.parentElement;
   if(target?.closest?.('.lyric-line')||[...record.addedNodes,...record.removedNodes].some(node=>node.nodeType===1&&(node.matches?.('.lyric-line')||node.closest?.('.lyric-line')||node.querySelector?.('.lyric-line')))){invalidate();if(view==='lyrics')tick();return}
  }
 });
 stageObserver.observe(stage,{childList:true,subtree:true});
}
function makeWord(element){
 const start=Number(element.dataset.start),end=Number(element.dataset.end);
 const chars=[...element.querySelectorAll('.lyric-char')];
 return {element,start,end,chars,charValues:Array(chars.length).fill(null),charStates:Array(chars.length).fill(null),sung:null,singing:null,boundary:null,progress:null,state:null};
}
function ensureCache(){
 const stage=$('#lyric-stage');
 if(!stage){if(cache){stageObserver?.disconnect();stageObserver=null;cache=null;suspendedStage=null;suspendedState=null}return null}
 if(!dirty&&cache?.stage===stage&&cache.data===lyricData&&cache.alignment===lyricWordLines)return cache;
 const rows=[...stage.querySelectorAll('.lyric-line')].map((element,index)=>{
  const original=[...element.querySelectorAll('.lyric-word')].filter(word=>!word.closest('.lyric-translation')).map(makeWord);
  const words=original.filter(word=>Number.isFinite(word.start)&&Number.isFinite(word.end)&&word.start>=0&&word.end>word.start);
  element.dataset.timing=words.length?'word':'line-visual';
  const timedWords=new Set(words);
  for(const word of original){word.element.dataset.timing=words.length?(timedWords.has(word)?'word':'untimed'):'line-visual';word.element.dataset.characterTiming=words.length?'visual-within-word':'visual-within-line'}
  const translations=[...element.querySelectorAll('.lyric-translation')].map(translation=>{
   translation.dataset.timing='line-visual';
   const translatedWords=[...translation.querySelectorAll('.lyric-word')].map(makeWord);
   let offset=0;for(const word of translatedWords){word.visualOffset=offset;offset+=word.chars.length}
   for(const word of translatedWords){word.element.dataset.timing='line-visual';word.element.dataset.characterTiming='visual-within-line'}
   return {element:translation,words:translatedWords,total:offset};
  });
  const ordered=words.every((word,i)=>!i||word.start>=words[i-1].end);
  let originalOffset=0;for(const word of original){word.visualOffset=originalOffset;originalOffset+=word.chars.length}
  return {element,index,words,original,originalTotal:originalOffset,hadTimingState:original.some(word=>Boolean(word.element.dataset.wordState)),starts:words.map(word=>word.start),alignedStart:words.reduce((start,word)=>Math.min(start,word.start),Infinity),alignedEnd:words.reduce((end,word)=>Math.max(end,word.end),-1),ordered,wordIndex:-2,translations,progress:null,initialized:false};
 });
 cache={stage,rows,times:lyricData.map(line=>Number(line.time)),data:lyricData,alignment:lyricWordLines,active:-2};
 dirty=false;lastTickPosition=NaN;metrics.rebuilds++;observeStage(stage);return cache;
}
function changeWord(word,sung,singing,boundary,progress,started){
 if(word.sung!==sung){word.element.classList.toggle('sung',sung);word.sung=sung;metrics.wordWrites++}
 if(word.singing!==singing){word.element.classList.toggle('singing',singing);word.singing=singing;metrics.wordWrites++}
 const state=sung?'sung':started?'active':'upcoming';
 if(word.state!==state){word.element.dataset.wordState=state;word.state=state;metrics.wordWrites++}
 const value=progress.toFixed(6);
 if(word.progress!==value){word.element.style.setProperty('--word-progress',value);word.element.style.setProperty('--word-motion',Math.sin(progress*Math.PI).toFixed(6));word.progress=value;metrics.progressWrites++}
 const next=Math.max(0,Math.min(word.chars.length,boundary));
 const from=word.boundary===null?0:Math.max(0,Math.min(word.boundary,next)-1),to=word.boundary===null?word.chars.length:Math.min(word.chars.length,Math.max(word.boundary,next)+1);
 const complete=progress*word.chars.length;
 for(let i=from;i<to;i++){
  const fraction=clamp(complete-i),charValue=fraction.toFixed(6),charState=fraction>=1?'sung':started&&complete>=i?'active':'upcoming';
  if(word.charValues[i]!==charValue){word.chars[i].style.setProperty('--char-progress',charValue);word.chars[i].style.setProperty('--char-motion',Math.sin(fraction*Math.PI).toFixed(6));word.charValues[i]=charValue;metrics.progressWrites++}
  if(word.charStates[i]!==charState){word.chars[i].dataset.charState=charState;word.charStates[i]=charState;metrics.charWrites++}
  if(word.boundary===null||(i<word.boundary)!==(i<next)){word.chars[i].classList.toggle('sung',i<next);metrics.charWrites++}
 }
 word.boundary=next;
}
function updateOriginalWord(word,pos){
 const fraction=clamp((pos-word.start)/(word.end-word.start));
 changeWord(word,pos>=word.end,pos>=word.start&&pos<word.end,Math.ceil(fraction*word.chars.length),fraction,pos>=word.start);
}
function updateRowWords(row,pos,force=false){
 const next=upperBound(row.starts,pos)-1;
 if(force||!row.ordered){for(const word of row.words)updateOriginalWord(word,pos)}
 else{
  const from=Math.max(0,Math.min(row.wordIndex,next)),to=Math.min(row.words.length-1,Math.max(row.wordIndex,next));
  for(let i=from;i<=to;i++)updateOriginalWord(row.words[i],pos);
 }
 row.wordIndex=next;
 const start=row.words.length?row.alignedStart:cache.times[row.index],following=cache.times[row.index+1];
 const trackEnd=duration();
 const end=row.words.length?row.alignedEnd:Number.isFinite(following)&&following>start?following:trackEnd>start?trackEnd:NaN;
 const progress=Number.isFinite(end)&&Number.isFinite(start)&&end>start?clamp((pos-start)/(end-start)):0;
 const displayed=progress.toFixed(5);
 if(row.progress!==displayed){row.element.style.setProperty('--lyric-line-progress',displayed);row.progress=displayed;metrics.progressWrites++}
 if(!row.words.length){
  const complete=progress*row.originalTotal;
  for(const word of row.original){const amount=clamp((complete-word.visualOffset)/Math.max(1,word.chars.length));changeWord(word,amount>=1,amount>0&&amount<1,Math.ceil(amount*word.chars.length),amount,pos>=start&&complete>=word.visualOffset)}
 }
 for(const translation of row.translations){
  // Share the original words' phase, including pauses between words. This is
  // visual progress only; translated words have no independent audio times.
  const source=row.words[Math.max(0,next)],gap=row.words.length&&next>=0&&next<row.words.length-1&&pos>=source.end&&pos<row.words[next+1].start;
  const spoken=row.words.length?(next<0?0:clamp((next+clamp((pos-source.start)/(source.end-source.start)))/row.words.length-(gap?1e-9:0))):progress;
  const completed=spoken*translation.words.length;
  for(let j=0;j<translation.words.length;j++){
   const word=translation.words[j],amount=clamp(completed-j);
   changeWord(word,amount>=1,amount>0&&amount<1,Math.ceil(amount*word.chars.length),amount,pos>=start&&completed>=j);
  }
 }
}
function moveActive(next,pos){
 const state=cache,previous=state.active;
 if(previous===next)return;
 // Initial cache construction writes each row once. Normal playback touches two.
 if(previous===-2){for(let i=0;i<state.rows.length;i++){state.rows[i].element.classList.toggle('current',i===next);state.rows[i].element.classList.toggle('past',i<next);if(i!==next&&state.rows[i].hadTimingState)updateRowWords(state.rows[i],pos,true)}}
 else{
  if(previous>=0)state.rows[previous]?.element.classList.remove('current');
  if(next>=0)state.rows[next]?.element.classList.add('current');
  const from=Math.max(0,Math.min(previous,next)),to=Math.min(state.rows.length-1,Math.max(previous,next));
  for(let i=from;i<=to;i++){state.rows[i].element.classList.toggle('past',i<next);if(i!==next)updateRowWords(state.rows[i],pos,true)}
 }
 state.active=next;lyricActive=next;metrics.lineTransitions++;
 const row=state.rows[next];
 if(row){
  row.initialized=false;
  if(state.stage.dataset?.manualFollow==='true')return;
  // Read geometry once per line transition, never for the per-word frame work.
  const top=row.element.offsetTop-state.stage.clientHeight/2+row.element.clientHeight/2;
  state.stage.scrollTo({top,behavior:ui.motion&&!reduced?.matches?'smooth':'auto'});
 }
}
function tick(){
 metrics.ticks++;
 if(view!=='lyrics'||document.hidden||!lyricTimed||!lyricData.length){schedule();return}
 const state=ensureCache();if(!state){schedule();return}
 const pos=position();
 if(pos!==lastTickPosition||state.active===-2){
  const next=upperBound(state.times,pos)-1;moveActive(next,pos);
  const row=state.rows[next];if(row){updateRowWords(row,pos,!row.initialized);row.initialized=true}
  lastTickPosition=pos;
  window.nawaV3Stage?.tick(audio.currentTime,next);
 }
 schedule();
}
function updateWords(pos=position()){
 const state=ensureCache();if(!state||!lyricTimed)return;
 const index=upperBound(state.times,pos)-1,row=state.rows[index];if(row){updateRowWords(row,pos,!row.initialized);row.initialized=true}
}
function mediaChanged(){lastTickPosition=NaN;lastFrameStamp=NaN;tick()}
for(const event of ['play','pause','ended','seeking','seeked','ratechange','loadedmetadata'])audio.addEventListener(event,mediaChanged);
document.addEventListener('visibilitychange',()=>{if(document.hidden)stop();else mediaChanged()});
reduced?.addEventListener?.('change',mediaChanged);
window.addEventListener?.('madar-resource-change',mediaChanged);
if(typeof MutationObserver!=='undefined'){
 const main=$('#main');if(main)new MutationObserver(records=>{if(!records.some(record=>record.target===main))return;if(view==='lyrics'){invalidate();tick()}else dispose()}).observe(main,{childList:true});
 new MutationObserver(()=>{if(!canRun())stop();else schedule()}).observe(document.body,{attributes:true,attributeFilter:['class']});
}
window.madarLyricClock={invalidate,tick,updateWords,sync:mediaChanged,stop,dispose,status:()=>({...metrics,fps:frameRate(),running:Boolean(frameID),cachedRows:cache?.rows.length||0})};
invalidate();tick();
})();
