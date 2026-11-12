'use strict';
(()=>{
const choices=['word','character','line'];if(!choices.includes(ui.lyricReveal))ui.lyricReveal='word';
function apply(){document.body.dataset.lyricReveal=window.nawaV3Stage?.activeSettings()?.reveal||ui.lyricReveal;window.madarLyricClock?.invalidate();window.madarLyricClock?.tick();window.madarVerci?.sync()}
function settings(){return `<section class="lyrics-popup-section"><h3>وضع التزامن</h3><label>طريقة إظهار الكلمات<select data-lyric-reveal aria-label="طريقة إظهار الكلمات"><option value="word" ${ui.lyricReveal==='word'?'selected':''}>كلمة بكلمة</option><option value="character" ${ui.lyricReveal==='character'?'selected':''}>حرف بحرف · تدرّج بصري</option><option value="line" ${ui.lyricReveal==='line'?'selected':''}>سطر كامل</option></select></label><p class="note">عند توفر توقيت الكلمات يُستخدم كما هو. توقيت السطور يعطي حركة كلمات تقديرية؛ استخدم المطابقة من الصوت للدقة. حركة الحروف بصرية داخل زمن الكلمة.</p><button class="secondary" data-action="align-lyrics">مطابقة توقيت الكلمات من الصوت</button></section>`}
document.addEventListener('change',e=>{if(!e.target.matches('select[data-lyric-reveal]'))return;ui.lyricReveal=choices.includes(e.target.value)?e.target.value:'word';persist();apply()});
audio.addEventListener('loadedmetadata',()=>{if(view==='lyrics'&&lyricData.length){visualTimingCache=null;const stage=$('#lyric-stage');if(stage&&!ui.lyricStyle?.startsWith('verci-'))stage.innerHTML=lyricData.map((line,i)=>`<button class="lyric-line" dir="${textDirection(line.text)}" data-lyric-index="${i}" ${lyricTimed?`data-lyric-seek="${line.time}"`:''}>${renderLyricText(line,i)}${translationMarkup(line,i)}</button>`).join('');apply()}});
window.nawaLyrics={apply,settings};apply();
})();
