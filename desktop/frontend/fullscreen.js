'use strict';
// Original Madar implementation inspired by the user's NovaTune media layouts.
// It reuses the same audio element, lyric data and translation renderer.
(()=>{
const media=[['cd','CD'],['vinyl','فينيل'],['cassette','كاسيت'],['square','غلاف الألبوم']];
const finishes={cd:[['cover','غلاف كامل'],['silver','ملصق فضي'],['prism','طيف ضوئي']],vinyl:[['grooved','أخاديد كلاسيكية'],['picture','صورة كاملة'],['smoke','دخاني شفاف']],cassette:[['classic','أسود كلاسيكي'],['clear','هيكل شفاف'],['neon','نيون']]};
const backgrounds=[['aurora','هالة الغلاف'],['blue','زجاج أزرق'],['cinema','سينما'],['noir','OLED أسود'],['album','غلاف الألبوم خلف الكلمات'],['theme','الثيم الرئيسي']];
const progressStyles=[['wave','موجة ناعمة'],['line','خط هادئ'],['glow','نبض ضوئي'],['shimmer','لمعة'],['segments','مقاطع'],['comet','مذنب']];
const defaults={fsMedium:'cd',fsVariants:{cd:'cover',vinyl:'grooved',cassette:'classic'},fsBackground:'theme',fsCardSize:340,fsTextSize:42,fsRotation:18,fsBlur:52,fsDim:.58,fsProgress:'wave',fsAmbient:true};
for(const [key,value] of Object.entries(defaults))if(ui[key]===undefined)ui[key]=typeof value==='object'?{...value}:value;
ui.fsVariants={...defaults.fsVariants,...ui.fsVariants};
if(!media.some(([id])=>id===ui.fsMedium))ui.fsMedium='cd';
if(!backgrounds.some(([id])=>id===ui.fsBackground))ui.fsBackground='aurora';
if(!progressStyles.some(([id])=>id===ui.fsProgress))ui.fsProgress='wave';
for(const [medium,options] of Object.entries(finishes))if(!options.some(([id])=>id===ui.fsVariants[medium]))ui.fsVariants[medium]=defaults.fsVariants[medium];
const clamp=(v,min,max,fallback)=>Number.isFinite(Number(v))?Math.max(min,Math.min(max,Number(v))):fallback;
let changing=false,cacheID='',cacheStatus='',savedScroll=0,mediaSignature='';
const expandIcon='<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5"/></svg>';
const fs={active:false,media,finishes,backgrounds,progressStyles,enter,leave,render,settings,afterRender,sync,applyPreferences,loaded,canReuse,repaint,setDockPinned,dockPinned:()=>dockPinned};
window.madarFullscreen=fs;

async function enter(){
 if(changing||fs.active||document.body.classList.contains('v3-mini-player'))return;
 if(!current){toast('شغّل أغنية أولًا ثم افتح ملء الشاشة');return}
 changing=true;savedScroll=$('#main').scrollTop;
 try{await call('SetLyricsFullscreen',true);fs.active=true;view='lyrics';document.body.classList.add('lyrics-is-fullscreen');nav();globalRender();$('.fs-scene')?.focus({preventScroll:true})}
 catch(e){toast(e.message||e)}finally{changing=false}
}
// A named alias avoids shadowing the application's render() with this module's
// markup renderer. The bridge is assigned by app.js before this module loads.
const globalRender=window.madarRender;
async function leave(){
 if(changing||!fs.active)return;
 changing=true;
 try{await call('SetLyricsFullscreen',false);fs.active=false;dockPinned=false;document.body.classList.remove('lyrics-is-fullscreen','fs-playing','fs-suspended');mediaSignature='';globalRender();$('#main').scrollTop=savedScroll;$('[data-action="lyrics-fullscreen"]')?.focus()}
 catch(e){toast(e.message||e)}finally{changing=false}
}
function loaded(id,status){cacheID=id;cacheStatus=status||'';sync()}
function canReuse(id){return cacheID===id&&lyricData.length>0}
function repaint(){
 const stage=$('#lyric-stage');if(!stage||!canReuse(current?.id))return;
 stage.classList.toggle('plain',!lyricTimed);
 stage.innerHTML=lyricData.map((line,i)=>`<button class="lyric-line" dir="${textDirection(line.text)}" ${lyricTimed?`data-lyric-seek="${line.time}"`:''} data-lyric-index="${i}">${renderLyricText(line,i)}${translationMarkup(line,i)}</button>`).join('');
 lyricActive=-2;if($('#lyric-status'))$('#lyric-status').textContent=cacheStatus;if($('#translation-summary'))$('#translation-summary').textContent=translationData?.status||'';updateLyrics();pollTranslation();window.madarVerci?.sync();
}
function mediumArtwork(t){
 const kind=ui.fsMedium,finish=ui.fsVariants[kind]||'cover';
 if(kind==='square')return `<div class="fs-square">${art(t.song,t.id)}</div>`;
 if(kind==='cassette')return `<div class="fs-cassette" data-finish="${finish}" aria-label="تصميم كاسيت ${esc(finishes.cassette.find(x=>x[0]===finish)?.[1]||'')}"><div class="fs-tape-label"><span dir="auto">${esc(t.song.title)}</span><small dir="auto">${esc(t.song.artist)}</small></div><div class="fs-tape-window"><i class="fs-reel"></i><span></span><i class="fs-reel reverse"></i></div><div class="fs-tape-base"></div><i class="fs-screw a"></i><i class="fs-screw b"></i><i class="fs-screw c"></i><i class="fs-screw d"></i></div>`;
 return `<div class="fs-disc fs-disc-${kind}" data-finish="${finish}" aria-label="تصميم ${kind==='cd'?'CD':'فينيل'}"><div class="fs-disc-picture">${art(t.song,t.id)}</div><i class="fs-disc-sheen"></i><i class="fs-disc-center"></i></div>`;
}
function render(t){
 const image=cover(t.song,t.id);
 return `<section class="fs-scene" tabindex="-1" role="region" aria-label="كلمات ملء الشاشة"><div class="fs-backdrop" aria-hidden="true">${image?`<img src="${esc(image)}" alt="">`:''}</div><div class="fs-veil" aria-hidden="true"></div><aside class="fs-dock" id="fs-dock" data-open="false" aria-label="أدوات التشغيل الجانبية"><button class="fs-dock-handle" id="fs-dock-toggle" aria-controls="fs-dock-content" aria-expanded="false" aria-label="إظهار أدوات التشغيل">${icon('arrow')}</button><div class="fs-dock-content" id="fs-dock-content"><header class="fs-toolbar"><button id="fs-exit" class="fs-toolbar-button" data-action="fs-exit" title="الخروج من ملء الشاشة · Esc">${icon('arrow')}<span>رجوع</span></button><div class="fs-toolbar-center"><span class="fs-brand">نَوى</span><span>مساحة للكلمات</span></div><button class="fs-toolbar-button" data-action="lyrics-settings" aria-label="إعدادات ملء الشاشة والكلمات">${icon('settings')}<span>الإعدادات</span></button></header><div class="fs-media-column"><div class="fs-media-art" id="fs-media-art">${mediumArtwork(t)}</div><section class="fs-player-card" id="fs-player-card" aria-label="مشغل ملء الشاشة"><div class="fs-track-info"><div><h2 id="fs-title" dir="auto">${esc(t.song.title)}</h2><p id="fs-artist" dir="auto">${esc(t.song.artist)}</p></div><button class="fs-icon" data-action="fs-favorite" aria-label="المفضلة" id="fs-favorite">${icon('heart')}</button></div><div class="fs-progress" id="fs-progress" data-progress="${ui.fsProgress}"><input id="fs-seek" type="range" min="0" max="1000" value="0" aria-label="موضع التشغيل في ملء الشاشة" dir="ltr"></div><div class="fs-times" dir="ltr"><span id="fs-elapsed">0:00</span><span id="fs-duration">${time(t.song.duration)}</span></div><div class="fs-transport" dir="ltr"><button class="fs-icon" data-action="fs-shuffle" id="fs-shuffle" aria-label="تشغيل عشوائي">${icon('shuffle')}</button><button class="fs-icon" data-action="previous" aria-label="المسار السابق">${icon('previous')}</button><button class="fs-play" data-action="toggle-play" id="fs-play" aria-label="تشغيل أو إيقاف">${icon(audio.paused?'play':'pause')}</button><button class="fs-icon" data-action="next" aria-label="المسار التالي">${icon('next')}</button><button class="fs-icon" data-action="fs-repeat" id="fs-repeat" aria-label="تكرار">${icon('repeat')}</button></div><div class="fs-card-bottom"><span id="fs-source">${current.preview?'بث مباشر':'من مكتبتك'}</span><label class="fs-volume-row" title="مستوى الصوت">${icon('volume')}<input id="fs-volume" type="range" min="0" max="1" step="0.01" value="${ui.volume}" aria-label="مستوى الصوت في ملء الشاشة" dir="ltr"></label></div></section></div><div class="fs-small-status"><span id="lyric-status"></span><span id="translation-summary" class="sr-only" role="status"></span></div></div></aside><div class="fs-composition"><div class="fs-lyrics-column"><div class="fs-lyrics-meta"><span id="fs-timing-kind">${lyricTimed?'كلمات متزامنة':'الكلمات'}</span><span class="fs-translation-label">${ui.translationEnabled?esc(translationLanguages.find(x=>x[0]===ui.translationTarget)?.[1]||''):''}</span></div><div class="lyric-stage fs-lyrics" id="lyric-stage" data-style="${ui.lyricStyle}" data-motion="${ui.lyricMotion}"><div class="busy"><i class="spinner"></i></div></div></div></div><div id="align-status" class="fs-align-status">${alignmentWidget()}</div></section>`;
}
function settings(){
 const choice=(values,value)=>values.map(([id,name])=>`<option value="${id}" ${value===id?'selected':''}>${name}</option>`).join('');
 return `<section id="fullscreen-theme-settings" tabindex="-1" class="lyrics-popup-section fs-design-settings"><h3>ثيمات ملء الشاشة</h3><p class="note">اختر خلفية مستقلة لشاشة الكلمات. تُحفظ تلقائيًا ويمكن تعديلها قبل الدخول إلى ملء الشاشة.</p><div class="fs-background-picker">${backgrounds.map(([id,name])=>`<button type="button" data-fs-background="${id}" class="fs-background-choice ${ui.fsBackground===id?'selected':''}" aria-pressed="${ui.fsBackground===id}"><span class="fs-background-swatch fs-swatch-${id}" aria-hidden="true">${['album','aurora'].includes(id)&&current?art(current.song,current.id):'≋'}</span><span>${name}</span></button>`).join('')}</div><h4>تصميم المشغل الجانبي</h4><div class="fs-medium-picker">${media.map(([id,name])=>`<button class="fs-medium-choice ${ui.fsMedium===id?'selected':''}" data-fs-medium="${id}" aria-pressed="${ui.fsMedium===id}"><i class="fs-mini fs-mini-${id}" aria-hidden="true"></i><span>${name}</span></button>`).join('')}</div><div class="lyrics-field-grid">${Object.entries(finishes).map(([medium,options])=>`<label data-fs-variant="${medium}" ${ui.fsMedium!==medium?'hidden':''}>تصميم ${media.find(x=>x[0]===medium)[1]}<select data-fs-pref="variant-${medium}" aria-label="تصميم ${media.find(x=>x[0]===medium)[1]}">${choice(options,ui.fsVariants[medium])}</select></label>`).join('')}<label>الخلفية<select data-fs-pref="fsBackground" aria-label="خلفية ملء الشاشة">${choice(backgrounds,ui.fsBackground)}</select></label><label>شكل شريط التقدم<select data-fs-pref="fsProgress" aria-label="شكل تقدم ملء الشاشة">${choice(progressStyles,ui.fsProgress)}</select></label><label>حجم بطاقة المشغل <output id="fs-size-label">${ui.fsCardSize}px</output><input data-fs-pref="fsCardSize" type="range" min="260" max="440" step="10" value="${ui.fsCardSize}" aria-label="حجم بطاقة ملء الشاشة"></label><label>حجم الكلمات <output id="fs-text-label">${ui.fsTextSize}px</output><input data-fs-pref="fsTextSize" type="range" min="26" max="66" step="1" value="${ui.fsTextSize}" aria-label="حجم كلمات ملء الشاشة"></label><label>دورة القرص <output id="fs-rotation-label">${ui.fsRotation}ث</output><input data-fs-pref="fsRotation" type="range" min="6" max="40" step="1" value="${ui.fsRotation}" aria-label="مدة دوران القرص"></label><label>ضبابية الغلاف<input data-fs-pref="fsBlur" type="range" min="0" max="80" step="2" value="${ui.fsBlur}" aria-label="ضبابية خلفية ملء الشاشة"></label><label>تعتيم الخلفية<input data-fs-pref="fsDim" type="range" min="0.3" max="0.85" step="0.01" value="${ui.fsDim}" aria-label="تعتيم خلفية ملء الشاشة"></label></div><label class="check-line"><input type="checkbox" data-fs-pref="fsAmbient" ${ui.fsAmbient?'checked':''}>حركة خلفية هادئة أثناء التشغيل</label><p class="note">القرص والبكرات يتوقفان مع الإيقاف أو إخفاء العرض. الأشكال والخطوط والترجمة أدناه تعمل داخل ملء الشاشة أيضًا. F11 للدخول والخروج، وEsc يقفل الإعدادات قبل الخروج.</p>${!fs.active?'<button class="secondary" data-action="fs-enter-from-settings">افتح ملء الشاشة</button>':''}</section>`;
}
function afterRender(){
 if(!fs.active)return;
 document.body.classList.add('lyrics-is-fullscreen');mediaSignature='';applyPreferences();sync();
}
function applyPreferences(){
 ui.fsCardSize=clamp(ui.fsCardSize,260,440,340);ui.fsTextSize=clamp(ui.fsTextSize,26,66,42);ui.fsRotation=clamp(ui.fsRotation,6,40,18);ui.fsBlur=clamp(ui.fsBlur,0,80,52);ui.fsDim=clamp(ui.fsDim,.3,.85,.58);
 const root=document.documentElement;for(const [key,value] of Object.entries({'--fs-card-size':ui.fsCardSize+'px','--fs-text-size':ui.fsTextSize+'px','--fs-cover-blur':ui.fsBlur+'px','--fs-dim':ui.fsDim}))root.style.setProperty(key,value);
 document.body.dataset.fsBackground=ui.fsBackground;document.body.dataset.fsAmbient=ui.fsAmbient?'on':'off';
 const currentTask=current&&(current.preview?current:taskByID(current.id));
 if(fs.active&&currentTask){const signature=current.id+'|'+ui.fsMedium+'|'+(ui.fsVariants[ui.fsMedium]||'');if(mediaSignature!==signature){const holder=$('#fs-media-art');if(holder)holder.innerHTML=mediumArtwork(currentTask);mediaSignature=signature}if($('#fs-progress'))$('#fs-progress').dataset.progress=ui.fsProgress;for(const element of $$('.fs-disc,.fs-reel'))for(const animation of element.getAnimations?.()||[])if(animation.animationName==='fsTurn')animation.updatePlaybackRate?.(18/ui.fsRotation)}
 for(const element of $$('[data-fs-variant]'))element.hidden=element.dataset.fsVariant!==ui.fsMedium;
 for(const element of $$('[data-fs-medium]')){element.classList.toggle('selected',element.dataset.fsMedium===ui.fsMedium);element.setAttribute('aria-pressed',String(element.dataset.fsMedium===ui.fsMedium))}
 for(const [id,value] of [['fs-size-label',ui.fsCardSize+'px'],['fs-text-label',ui.fsTextSize+'px'],['fs-rotation-label',ui.fsRotation+'ث']])if($('#'+id))$('#'+id).textContent=value;
 persist();
}
let dockPinned=false;
function setDockPinned(pinned){
 dockPinned=pinned;
 const dock=$('#fs-dock'),handle=$('#fs-dock-toggle'),button=$('[data-v3-stage=pin]');
 dock?.setAttribute('data-open',String(pinned));
 handle?.setAttribute('aria-expanded',String(pinned));
 button?.setAttribute('aria-pressed',String(pinned));
 if(!pinned){handle?.blur();button?.blur()}
 if($('.fs-scene'))$('.fs-scene').dataset.dockPinned=String(pinned);
}
function bindDock(){const dock=$('#fs-dock'),handle=$('#fs-dock-toggle');if(!dock||dock.dataset.bound)return;dock.dataset.bound='true';const update=()=>{const open=dockPinned||dock.matches(':hover')||dock.contains(document.activeElement);dock.dataset.open=String(open);handle?.setAttribute('aria-expanded',String(open))};for(const event of ['pointerenter','pointerleave','focusin','focusout'])dock.addEventListener(event,()=>queueMicrotask(update));handle?.addEventListener('click',()=>setDockPinned(!dockPinned))}
function sync(){
 if(!fs.active||!$('#fs-player-card')&&!$('#fs-play'))return;bindDock();
 document.body.classList.toggle('fs-playing',!audio.paused&&!audio.ended);document.body.classList.toggle('fs-suspended',document.hidden);
 const play=$('#fs-play');if(play&&play.dataset.playState!==String(audio.paused)){play.dataset.playState=String(audio.paused);play.innerHTML=icon(audio.paused?'play':'pause');play.setAttribute('aria-label',audio.paused?'تشغيل في ملء الشاشة':'إيقاف مؤقت في ملء الشاشة')}
 const length=duration(),position=Math.min(1,Math.max(0,length>0?audio.currentTime/length:0));
 if($('#fs-elapsed'))$('#fs-elapsed').textContent=time(audio.currentTime);if($('#fs-duration'))$('#fs-duration').textContent=time(length);
 const seek=$('#fs-seek');if(seek){seek.value=position*1000;seek.disabled=Boolean(current?.preview&&current.decoded);seek.title=seek.disabled?'هذا البث لا يدعم التقديم':''}
 const progress=$('#fs-progress');if(progress){progress.style.setProperty('--fs-progress',position*100+'%');progress.style.setProperty('--fs-has-progress',position>0?'1':'0')}
 for(const [id,enabled] of [['fs-repeat',ui.repeat],['fs-shuffle',ui.shuffle],['fs-favorite',ui.favorites.includes(current?.id)]]){const element=$('#'+id);if(element){element.classList.toggle('selected',enabled);element.setAttribute('aria-pressed',String(enabled))}}
 if($('#fs-volume')&&document.activeElement!==$('#fs-volume'))$('#fs-volume').value=ui.volume;
 if($('#fs-timing-kind'))$('#fs-timing-kind').textContent=lyricTimed?(lyricWordLines.length?'توقيت الكلمات':'كلمات متزامنة'):'نص غير متزامن';
 const translation=$('.fs-translation-label');if(translation)translation.textContent=ui.translationEnabled?translationLanguages.find(x=>x[0]===ui.translationTarget)?.[1]||'':'';
}
document.addEventListener('click',async event=>{
 const button=event.target.closest('button');if(!button)return;
 try{
  if(button.dataset.fsMedium){ui.fsMedium=button.dataset.fsMedium;applyPreferences();return}
  switch(button.dataset.action){case 'lyrics-fullscreen':await enter();break;case 'fs-enter-from-settings':$('#modal').close();await enter();break;case 'fs-exit':await leave();break;case 'fs-shuffle':ui.shuffle=!ui.shuffle;persist();updateNow();break;case 'fs-repeat':ui.repeat=!ui.repeat;audio.loop=ui.repeat;persist();updateNow();break;case 'fs-favorite':{const i=ui.favorites.indexOf(current?.id);if(i>=0)ui.favorites.splice(i,1);else if(current)ui.favorites.push(current.id);persist();updateNow()}break}
 }catch(e){toast(e.message||e)}
});
function preferenceInput(event){const target=event.target,key=target.dataset.fsPref;if(!key)return;if(key.startsWith('variant-')){const medium=key.slice(8);if(finishes[medium]?.some(([id])=>id===target.value))ui.fsVariants[medium]=target.value}else if(key==='fsAmbient')ui[key]=target.checked;else if(['fsBackground','fsProgress'].includes(key))ui[key]=target.value;else ui[key]=Number(target.value);applyPreferences()}
document.addEventListener('input',event=>{preferenceInput(event);const target=event.target;if(target.id==='fs-seek'&&duration()>0&&!target.disabled){audio.currentTime=Number(target.value)/1000*duration();updateLyrics();sync()}if(target.id==='fs-volume'){ui.volume=Number(target.value);applyFX();$('#volume').value=ui.volume;sync()}});
document.addEventListener('change',preferenceInput);
document.addEventListener('change',event=>{if(event.target.dataset.fsPref!=='fsBackground')return;for(const b of $$('button[data-fs-background]')){b.classList.toggle('selected',b.dataset.fsBackground===ui.fsBackground);b.setAttribute('aria-pressed',String(b.dataset.fsBackground===ui.fsBackground))}});
document.addEventListener('click',event=>{const button=event.target.closest('button[data-fs-background]');if(!button)return;ui.fsBackground=button.dataset.fsBackground;persist();applyPreferences();for(const b of $$('button[data-fs-background]')){b.classList.toggle('selected',b.dataset.fsBackground===ui.fsBackground);b.setAttribute('aria-pressed',String(b.dataset.fsBackground===ui.fsBackground))}for(const select of $$('[data-fs-pref=fsBackground]'))select.value=ui.fsBackground});
document.addEventListener('keydown',event=>{
 if(event.key==='Escape'&&window.nawaFontPicker?.isOpen()){event.preventDefault();event.stopImmediatePropagation();window.nawaFontPicker.close(true);return}if(event.key==='Escape'&&window.nawaStylePicker?.isOpen()){event.preventDefault();event.stopImmediatePropagation();window.nawaStylePicker.close();return}if(event.key==='Escape'&&fs.active){event.preventDefault();event.stopImmediatePropagation();if($('#modal').open)$('#modal').close();else leave();return}
 if(event.key==='F11'&&(view==='lyrics'||fs.active)&&!$('#modal').open){event.preventDefault();event.stopImmediatePropagation();(fs.active?leave():enter())}
},true);
$('#modal').addEventListener('click',event=>{if(!fs.active||event.target!==$('#modal'))return;const box=$('#modal').getBoundingClientRect();if(event.clientX<box.left||event.clientX>box.right||event.clientY<box.top||event.clientY>box.bottom)$('#modal').close()});
document.addEventListener('visibilitychange',()=>{if(fs.active)document.body.classList.toggle('fs-suspended',document.hidden)});
})();
