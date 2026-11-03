'use strict';
// One compact control strip; full-screen keeps it inside the settings popup.
function lyricWorkflowPanel(popup=false){
 const en=ui.language==='en',timed=lyricTimed&&lyricData.length>0,complete=lyricWordLines?.some(line=>line.words?.length);
 const quality=complete?(en?'Word timing':'توقيت كلمات'):timed?(en?'Line timing':'توقيت سطور'):(en?'No timing yet':'بدون توقيت');
 return `<section class="lyrics-workflow ${popup?'in-popup':''}" aria-label="${en?'Lyrics and timing':'الكلمات والتوقيت'}"><div class="lyrics-workflow-copy"><div class="lyrics-workflow-heading"><span class="lyrics-workflow-mark" aria-hidden="true">≋</span><strong>${en?'Lyrics, in time':'الكلمات، على وقتك'}</strong><span class="lyrics-workflow-quality">${quality}</span></div><p>${en?'Best matching word timing first. Audio alignment runs when you choose.':'نفضّل المصدر المطابق بتوقيت الكلمات. تحليل الصوت يبدأ باختيارك.'}</p><small>${en?'Saved lyrics first · Word timing · Line timing · Plain text':'المحفوظ أولًا · توقيت الكلمات · توقيت السطور · النص'}</small></div><div class="lyrics-workflow-buttons"><button class="secondary" data-action="lyrics-best">${en?'Find best lyrics':'جلب أفضل كلمات'}</button><button class="primary" data-action="align-lyrics" ${!current||current.preview?'disabled':''}>${en?'Align timing now':'اضبط التوقيت الآن'}</button>${!popup?`<button class="quiet" data-action="lyrics-settings" aria-label="${en?'Lyrics appearance settings':'إعدادات مظهر الكلمات'}">${icon('settings')}<span>${en?'Appearance':'المظهر'}</span></button>`:''}</div></section>`;
}
document.addEventListener('click',async event=>{
 const button=event.target.closest('[data-action="lyrics-best"]');if(!button||!current)return;
 button.disabled=true;button.setAttribute('aria-busy','true');const previous=button.textContent;button.textContent=ui.language==='en'?'Finding best match…':'جارٍ اختيار الأفضل…';const id=current.id;
 try{
  const result=await call('FetchLyricsFrom',id,'auto-refresh');
  if(current?.id===id){ui.lyricSource='auto';persist();await poll();if(view==='lyrics')await loadLyrics(false);toast(ui.language==='en'?'Best matching lyrics saved':'حُفظت أفضل كلمات مطابقة')}
 }catch(error){toast(error.message||String(error))}
 finally{if(button.isConnected){button.disabled=false;button.removeAttribute('aria-busy');button.textContent=previous}}
});
window.nawaLyricsWorkflow={refresh(){const quality=lyricWordLines?.some(line=>line.words?.length)?(ui.language==='en'?'Word timing':'توقيت كلمات'):lyricTimed&&lyricData.length?(ui.language==='en'?'Line timing':'توقيت سطور'):(ui.language==='en'?'No timing yet':'بدون توقيت');for(const node of document.querySelectorAll('.lyrics-workflow-quality'))if(node.textContent!==quality)node.textContent=quality}};
