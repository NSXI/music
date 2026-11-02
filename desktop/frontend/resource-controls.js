'use strict';
// Visual/resource budgets only: no audio rate, codec, DSP or quality changes.
(()=>{
const modes={economy:{dpr:1,maxPixels:1000000},balanced:{dpr:1.5,maxPixels:2000000},smooth:{dpr:2,maxPixels:3000000}};
const valid=mode=>Object.hasOwn(modes,mode)?mode:'economy';
const validFps=value=>['60','120','144','auto'].includes(String(value))?String(value):'auto';
ui.performanceMode=valid(ui.performanceMode);
ui.lyricFps=validFps(ui.lyricFps);
let backendError='',backendRequest=0;
function budget(){const value=validFps(ui.lyricFps);return {mode:valid(ui.performanceMode),...modes[valid(ui.performanceMode)],fps:value==='auto'?240:Number(value)}}
function settings(){return `<section class="lyrics-popup-section resource-settings"><h3>الأداء واستهلاك الموارد</h3><div class="resource-settings-row"><div><label for="resource-mode">وضع الأداء</label><p>تبقى جودة الصوت وتوقيت الكلمات كما هما. وضع الأداء لا يخفض معدل الإطارات المختار.</p></div><select id="resource-mode" data-resource-mode aria-label="وضع الأداء"><option value="economy" ${ui.performanceMode==='economy'?'selected':''}>اقتصادي · رسم أخف</option><option value="balanced" ${ui.performanceMode==='balanced'?'selected':''}>متوازن · سلاسة وتفاصيل</option><option value="smooth" ${ui.performanceMode==='smooth'?'selected':''}>سلس · تفاصيل بصرية أعلى</option></select></div><div class="resource-settings-row resource-fps-row"><label for="lyric-fps">معدل إطارات الكلمات</label><select id="lyric-fps" data-lyric-fps aria-label="معدل إطارات الكلمات">${[['auto','تلقائي · يتبع الشاشة'],['60','60'],['120','120'],['144','144']].map(([value,name])=>`<option value="${value}" ${ui.lyricFps===value?'selected':''}>${name}</option>`).join('')}</select></div><p class="note">الوضع الاقتصادي يقلل دقة الرسم فقط، مع إبقاء معدل الإطارات الذي اخترته. التلقائي يتبع تحديث الشاشة؛ النتيجة الفعلية تعتمد على الشاشة والجهاز. الحركة تتوقف عند الإيقاف أو إخفاء الصفحة.</p><p id="resource-mode-status" class="resource-mode-status" role="status"></p></section>`}
function apply(){
 document.body.dataset.performanceMode=ui.performanceMode;
 for(const picker of document.querySelectorAll('[data-resource-mode]'))if(picker.value!==ui.performanceMode)picker.value=ui.performanceMode;
 for(const picker of document.querySelectorAll('[data-lyric-fps]'))if(picker.value!==ui.lyricFps)picker.value=ui.lyricFps;
 window.dispatchEvent(new CustomEvent('madar-resource-change',{detail:budget()}));
 window.madarVerci?.sync?.();if(view==='player')drawSpectrum();
}
function adopt(mode){backendRequest++;backendError='';const status=document.querySelector('#resource-mode-status');if(status)status.textContent='';ui.performanceMode=valid(mode);persist();apply();return ui.performanceMode}
function setFps(value){ui.lyricFps=validFps(value);persist();apply();return ui.lyricFps}
function setMode(mode){
 const chosen=adopt(mode),request=++backendRequest;backendError='';
 const status=document.querySelector('#resource-mode-status');if(status)status.textContent='';
 const api=window.go?.main?.Studio?.SetResourceMode;
 if(typeof api==='function')Promise.resolve().then(()=>api(chosen)).catch(error=>{
  if(request!==backendRequest)return;backendError=String(error?.message||error);
  const status=document.querySelector('#resource-mode-status');if(status)status.textContent='حُفظ وضع الرسم؛ تعذّر حفظ إعداد المعالجة الآن.';
 });
 return chosen;
}
document.addEventListener('change',event=>{if(event.target.matches('[data-resource-mode]'))setMode(event.target.value);else if(event.target.matches('[data-lyric-fps]'))setFps(event.target.value)});
// Spectrum shares the same view lifecycle and refresh preference.
for(const name of ['play','pause','ended'])audio.addEventListener(name,()=>{if(view==='player')drawSpectrum()});
document.addEventListener('visibilitychange',()=>{if(view==='player')drawSpectrum()});
window.madarResources={settings,budget,setMode,setFps,adopt,compactLyrics:()=>Boolean(ui.lyricStyle?.startsWith('verci-')),status:()=>({mode:ui.performanceMode,lyricFps:ui.lyricFps,backendError})};
document.body.dataset.performanceMode=ui.performanceMode;
})();
