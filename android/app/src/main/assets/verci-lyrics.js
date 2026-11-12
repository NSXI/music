'use strict';
(()=>{
const art=window.MadarLyricArt;
let stage=null,canvas=null,context=null,observer=null,data=null,state=art.makeState(),result=null,width=0,height=0,ratio=1,manualIndex=0;
let linesRef=null,wordsRef=null,translationRef=null,translationSignature='',translationRows=[],lastLine=-1,lastStyle='',colors=null,tabHistory=new Map();
const reducedQuery=window.matchMedia('(prefers-reduced-motion: reduce)');
const enabled=()=>view==='lyrics'&&ui.lyricStyle?.startsWith('verci-')&&lyricData.length>0&&window.nawaV3Stage?.allowCanvas()!==false;
const defaults={verciLens:.75,verciText:'',verciAccent:'',verciSecondary:''};for(const [k,v] of Object.entries(defaults))if(ui[k]===undefined)ui[k]=v;
function dispose(){
 observer?.disconnect();observer=null;canvas?.remove();stage?.querySelector('.verci-timing-note')?.remove();stage?.querySelector('.verci-announcement')?.remove();
 stage?.classList.remove('verci-renderer');for(const [el,tab] of tabHistory)if(el.isConnected){if(tab===null)el.removeAttribute('tabindex');else el.setAttribute('tabindex',tab)}tabHistory.clear();
 stage=null;canvas=null;context=null;data=null;linesRef=null;wordsRef=null;translationRef=null;lastLine=-1;state=art.makeState();
}
function resize(){
 if(!stage||!canvas)return;
 const box=stage.getBoundingClientRect();width=Math.max(0,Math.round(box.width));height=Math.max(0,Math.round(box.height));
 const budget=window.madarResources?.budget()||{dpr:1,maxPixels:1000000};ratio=Math.min(window.devicePixelRatio||1,budget.dpr,Math.sqrt(budget.maxPixels/Math.max(1,width*height)));
 const w=Math.max(1,Math.floor(width*ratio)),h=Math.max(1,Math.floor(height*ratio));
 if(canvas.width!==w||canvas.height!==h){canvas.width=w;canvas.height=h;state.positions.clear()}
 frame(audio.currentTime+Number(ui.lyricOffset));
}
function translations(){return lyricData.map((line,i)=>{const row=translationData?.lines?.[i];return ui.translationEnabled&&row?.original===line.text?row.text:''})}
function activate(index){
 const unit=data?.units[index];if(!unit)return;
 if(unit.start!==null){audio.currentTime=Math.max(0,unit.start-Number(ui.lyricOffset));updateLyrics()}
 else manualIndex=index;
 frame(audio.currentTime+Number(ui.lyricOffset));
}
function attach(target){
 dispose();stage=target;stage.classList.add('verci-renderer');canvas=document.createElement('canvas');canvas.className='verci-canvas';canvas.tabIndex=0;canvas.setAttribute('role','group');canvas.setAttribute('aria-label','عرض الكلمات المتحرك؛ الأسهم للتنقل بين الكلمات أو السطور');
 context=canvas.getContext('2d',{alpha:true,desynchronized:true});if(!context){dispose();return false}
 const note=document.createElement('span');note.className='verci-timing-note';
 const announcement=document.createElement('span');announcement.className='verci-announcement sr-only';announcement.dataset.userContent='';announcement.setAttribute('aria-live','polite');
 stage.append(canvas,note,announcement);
 canvas.addEventListener('pointermove',e=>{const b=canvas.getBoundingClientRect();state.pointer={x:Math.max(-1,Math.min(1,(e.clientX-b.left)/Math.max(1,b.width)*2-1)),y:Math.max(-1,Math.min(1,(e.clientY-b.top)/Math.max(1,b.height)*2-1))};if(audio.paused)frame()});
 canvas.addEventListener('pointerleave',()=>{state.pointer={x:0,y:0};if(audio.paused)frame()});
 canvas.addEventListener('click',event=>{const box=canvas.getBoundingClientRect(),x=event.clientX-box.left,y=event.clientY-box.top;const match=result?.hits?.find(h=>Math.abs(x-h.x)<=h.width/2+14&&Math.abs(y-h.y)<=h.height/2+10);if(match)activate(match.index)});
 canvas.addEventListener('keydown',event=>{let index=result?.index??manualIndex;if(event.key==='ArrowLeft')index--;else if(event.key==='ArrowRight')index++;else if(event.key==='Home')index=0;else if(event.key==='End')index=data.units.length-1;else return;event.preventDefault();event.stopPropagation();activate(Math.max(0,Math.min(data.units.length-1,index)))});
 canvas.addEventListener('wheel',event=>{if(data?.timed)return;event.preventDefault();activate(Math.max(0,Math.min(data.units.length-1,manualIndex+Math.sign(event.deltaY))))},{passive:false});
 if(typeof ResizeObserver==='function'){observer=new ResizeObserver(resize);observer.observe(stage)}
 return true;
}
function sync(){
 const target=$('#lyric-stage');if(!enabled()||!target){if(stage)dispose();return}
 if(stage!==target||!canvas?.isConnected)if(!attach(target))return;
 const displayRows=typeof lyricTimingRows==='function'?lyricTimingRows():lyricWordLines;const changed=linesRef!==lyricData||wordsRef!==displayRows;
 if(changed){data=art.prepare(lyricData,displayRows);if(typeof lyricTimingPlan==='function')data.kind=lyricTimingPlan().kind;linesRef=lyricData;wordsRef=displayRows;manualIndex=0;lastLine=-1;state.positions.clear()}
 const signature=String(ui.translationEnabled)+'|'+ui.translationTarget;
 if(changed||translationRef!==translationData||translationSignature!==signature){translationRef=translationData;translationSignature=signature;translationRows=translations();lastLine=-1}
 for(const line of stage.querySelectorAll('.lyric-line'))if(!tabHistory.has(line)){tabHistory.set(line,line.getAttribute('tabindex'));line.tabIndex=-1}
 const computed=getComputedStyle(document.body);colors={text:ui.verciText||computed.getPropertyValue('--ink').trim(),accent:ui.verciAccent||computed.getPropertyValue('--accent').trim(),secondary:ui.verciSecondary||computed.getPropertyValue('--muted').trim()};
 const note=stage.querySelector('.verci-timing-note'),label={estimated:'حركة كلمات تقديرية من السطور؛ طابق من الصوت للدقة','mixed-estimated':'توقيت كلمات مع تقدير للسطور المتبقية',word:'توقيت كلمات حقيقي',mixed:'توقيت كلمات عند توفره؛ وإلا توقيت السطر',line:'توقيت سطور؛ الحركة تتبع السطر',plain:'نص بلا توقيت؛ استخدم الأسهم للتنقل'}[data.kind];if(note&&note.textContent!==label)note.textContent=label;
 if(lastStyle!==ui.lyricStyle){state=art.makeState();lastStyle=ui.lyricStyle}
 resize();
}
function frame(position){
 if(!stage||!context||!canvas?.isConnected||document.hidden||!enabled()||!data||width<1||height<1)return;
 const pos=Number.isFinite(position)?position:audio.currentTime+Number(ui.lyricOffset);
 const design=window.nawaV3Stage?.activeSettings();
 context.setTransform(ratio,0,0,ratio,0,0);
 result=art.paint(context,{width,height,data,time:pos,duration:duration(),style:ui.lyricStyle.slice(6),colors,font:design?.font||ui.lyricFont||window.madarNawa?.font(ui.theme)||'',translationFont:design?.translationFont||ui.translationFont||design?.font||ui.lyricFont||window.madarNawa?.font(ui.theme)||'',translations:window.nawaV2Lyrics?.canvasTranslations(translationRows)||translationRows,hideOriginal:ui.showOriginal===false,translationScale:Number(ui.translationScale)||1,reduced:!ui.motion||reducedQuery.matches,motion:design?.motion||ui.lyricMotion,lens:Number(ui.verciLens),textScale:Math.max(.65,Math.min(1.5,Number(ui.fsTextSize||42)/42)),manualIndex,revealMode:design?.reveal||ui.lyricReveal||'word',paused:audio.paused,pointer:state.pointer},state);
 if(result.line!==lastLine){lastLine=result.line;const line=lyricData[lastLine],announcement=stage.querySelector('.verci-announcement');if(announcement)announcement.textContent=(line?.text||'')+(translationRows[lastLine]?' — '+translationRows[lastLine]:'')}
}
function settings(){return `<section class="lyrics-popup-section verci-options"><h3>مكتبة الكلمات · Verci وMotion</h3><p class="note">اختر نمطًا يبدأ باسم Verci من شكل الكلمات. عند توفر توقيت الكلمات يتحرك العرض كلمة بكلمة؛ وإلا يتبع توقيت السطر. الترجمة تتبع السطر الأصلي ولا يُختلق لها توقيت كلمات.</p><p class="note">حركة الحروف بصرية داخل مدة الكلمة؛ لا تمثل توقيت نطق مستقل لكل حرف.</p><p class="note">أضيفت موشنات الملفين: 60 من المكتبة و20 من Lyric Motion. اختر Motion من شكل الكلمات.</p><div class="lyrics-field-grid"><label>لون الكلمات<input type="color" data-verci-color="verciText" value="${esc(ui.verciText||'#f3f5ff')}"></label><label>لون التمييز<input type="color" data-verci-color="verciAccent" value="${esc(ui.verciAccent||'#bca5ff')}"></label><label>لون الكلمات الثانوية<input type="color" data-verci-color="verciSecondary" value="${esc(ui.verciSecondary||'#b0bad0')}"></label><label>شدة العدسة<input type="range" data-verci-lens min="0" max="1" step="0.05" value="${ui.verciLens}" aria-label="شدة عدسة الكلمات"></label></div><button class="quiet" data-action="verci-reset-colors">اتباع ألوان الثيم</button><p class="note">أنماط Visual تضيف رموزًا لبعض الكلمات العربية والإنجليزية من قاموس محلي.</p></section>`}
document.addEventListener('input',event=>{const el=event.target;if(el.dataset.verciColor){ui[el.dataset.verciColor]=el.value;persist();sync()}else if(el.hasAttribute('data-verci-lens')){ui.verciLens=Math.max(0,Math.min(1,Number(el.value)));persist();sync()}});
document.addEventListener('click',event=>{if(event.target.closest('[data-action="verci-reset-colors"]')){ui.verciText=ui.verciAccent=ui.verciSecondary='';persist();sync()}});
document.addEventListener('change',()=>queueMicrotask(sync));
for(const name of ['timeupdate','seeking','seeked','loadedmetadata','play','pause','ended'])audio.addEventListener(name,()=>{if(enabled()){if(!stage||linesRef!==lyricData)sync();else if(name==='seeking'||name==='seeked'||audio.paused||!ui.motion||reducedQuery.matches||!window.madarLyricClock)frame(audio.currentTime+Number(ui.lyricOffset))}});
document.addEventListener('visibilitychange',()=>{if(!document.hidden)sync()});
reducedQuery.addEventListener?.('change',sync);
window.addEventListener('resize',()=>{if(stage)resize()});
window.madarVerci={sync,frame,settings,dispose,invalidateFonts:()=>{state.measure.clear();state.wrap.clear();state.positions.clear();if(stage)resize()},status:()=>({active:Boolean(stage),kind:data?.kind,units:data?.units.length||0,width,height,pixelRatio:ratio,frames:state.frames})};
})();
