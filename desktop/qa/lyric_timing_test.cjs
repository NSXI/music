require('node:fs').mkdirSync('qa/results',{recursive:true});
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const {parseHTML}=require('linkedom');
const lines=[{time:0,text:'كلمات أصلية'},{time:5,text:'Music together'}];
const source=[[{text:'كلمات',start:1,end:1.4},{text:'أصلية',start:1.5,end:2}],[{text:'Music',start:6,end:6.4},{text:'together',start:6.5,end:7}]];
const chars=text=>[...text].map(text=>`<span class="lyric-char">${text}</span>`).join('');
const html=source.map((words,index)=>`<button class="lyric-line" data-lyric-index="${index}"><span class="lyric-content">${words.map(word=>`<span class="lyric-word ${index?'separate':'joined'}" data-start="${word.start}" data-end="${word.end}">${chars(word.text)}</span>`).join('')}</span><span class="lyric-translation"><span class="lyric-word joined" data-start="-1" data-end="-1">${chars('ترجمة')}</span></span></button>`).join('');
const {window,document}=parseHTML('<html><head></head><body><main id="main"><div id="lyric-stage" data-motion="letters">'+html+'</div></main><audio id="audio"></audio></body></html>');
window.HTMLElement.prototype.scrollTo=function(){};window.matchMedia=()=>({matches:false,addEventListener(){}});
const audio=document.querySelector('#audio');audio.paused=true;audio.currentTime=0;audio.ended=false;
const ctx=vm.createContext({window,document,audio,view:'lyrics',lyricData:lines,lyricWordLines:source.map((words,index)=>({start:lines[index].time,words})),lyricTimed:true,lyricActive:-2,ui:{motion:true,lyricOffset:0},duration:()=>10,$:selector=>document.querySelector(selector),MutationObserver:window.MutationObserver,requestAnimationFrame:()=>1,cancelAnimationFrame(){},updateLyrics:()=>window.madarLyricClock.tick(),console});
vm.runInContext(fs.readFileSync('frontend/lyric-clock.js','utf8'),ctx);
const clock=window.madarLyricClock,stage=document.querySelector('#lyric-stage');let checks=0;
const original=()=>[...stage.querySelectorAll('.lyric-word')].filter(word=>!word.closest('.lyric-translation'));
function at(time,offset=0){audio.currentTime=time;ctx.ui.lyricOffset=offset;clock.tick();return time+offset}
function assertFuture(pos){for(const word of original())if(+word.dataset.start>pos){assert.notEqual(word.dataset.wordState,'active');assert.equal(word.classList.contains('singing'),false);assert.equal(+word.style.getPropertyValue('--word-progress')||0,0);for(const char of word.querySelectorAll('.lyric-char')){assert.equal(+char.style.getPropertyValue('--char-progress')||0,0);checks++}checks+=3}}
function progress(word){return +word.style.getPropertyValue('--word-progress')}
const first=original()[0],second=original()[1];
for(const position of [0,.999999]){assertFuture(at(position));assert.equal(first.dataset.wordState,'upcoming');assert.equal(stage.querySelector('.lyric-translation').dataset.timing,'line-visual');assert.equal(+stage.querySelector('.lyric-line').style.getPropertyValue('--lyric-line-progress'),0);checks+=3}
at(1);assert.equal(first.dataset.wordState,'active');assert.equal(progress(first),0);assert.equal(first.querySelector('.lyric-char').dataset.charState,'active');assert.equal(+first.querySelector('.lyric-char').style.getPropertyValue('--char-progress'),0);checks+=4;
at(1.1);assert.equal(progress(first),.25);assert.equal(first.dataset.characterTiming,'visual-within-word');assert.equal(+first.querySelector('.lyric-char').style.getPropertyValue('--char-progress'),1);checks+=3;
at(1.4);assert.equal(first.dataset.wordState,'sung');assert.equal(progress(first),1);assert.equal(second.dataset.wordState,'upcoming');assertFuture(1.4);checks+=3;
at(2);assert.equal(+stage.querySelector('.lyric-line').style.getPropertyValue('--lyric-line-progress'),1);checks++;
assertFuture(at(7));assertFuture(at(0));
assertFuture(at(.699999,.3));assert.equal(first.dataset.wordState,'upcoming');checks++;
at(.7,.3);assert.equal(first.dataset.wordState,'active');assert.equal(progress(first),0);checks+=2;
assertFuture(at(1.199999,-.2));assert.equal(first.dataset.wordState,'upcoming');checks++;
at(1.2,-.2);assert.equal(first.dataset.wordState,'active');assert.equal(progress(first),0);checks+=2;
for(const translation of stage.querySelectorAll('.lyric-translation .lyric-word')){assert.equal(translation.dataset.start,'-1');assert.equal(translation.dataset.end,'-1');assert.equal(translation.dataset.timing,'line-visual');checks+=3}
const stylesheet=document.createElement('style');stylesheet.textContent=fs.readFileSync('frontend/lyric-timing.css','utf8');document.head.append(stylesheet);
function guard(element,property,value){const matching=[...stylesheet.sheet.cssRules].filter(rule=>rule.selectorText&&element.matches(rule.selectorText)&&rule.style.getPropertyValue(property)===value&&rule.style.getPropertyPriority(property)==='important');assert.ok(matching.length,'Missing timestamp CSS guard '+property+' for '+element.outerHTML.slice(0,160));checks++}
at(.5);for(const motion of ['flow','rise','fade','letters','typewriter','words','wave','float','focus','slide','cascade','glint','breathe','ripple','spring']){
 stage.dataset.motion=motion;
 for(const element of [stage.querySelector('.lyric-line'),stage.querySelector('.lyric-content'),first,first.querySelector('.lyric-char')]){guard(element,'animation','none');guard(element,'animation-delay','0s');guard(element,'transition','none')}
 guard(first,'--word-progress','0');guard(first.querySelector('.lyric-char'),'--char-progress','0');
}
const result={checks,cssMotionVariants:15,verified:'Exact before/start/end boundaries, forward/backward seeks, positive/negative offsets, translation speech window, original/translation timing labels, and parsed CSS no-delay guards.',note:'Background DOM/static CSS checks; not a visual browser frame-rate or phoneme-alignment measurement.'};
fs.writeFileSync('qa/results/lyric-timing-results.json',JSON.stringify(result,null,2));console.log(JSON.stringify(result,null,2));
