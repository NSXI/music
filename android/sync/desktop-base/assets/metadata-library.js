'use strict';
(()=>{
let folderSignature='',artRequest=0;
const keyProviders=[['vagalume','Vagalume'],['audd','AudD'],['betterlyrics','Better Lyrics · اختياري'],['fanart','Fanart.tv'],['audiodb','TheAudioDB · اختياري']];
function folderProgress(){const state=snapshot.folderImport||{};if(!state.running&&!state.processed&&!state.failed)return '<p class="note">تُقرأ المجلدات الفرعية، وتُتخطى النسخ المكررة. ملفاتك الأصلية تبقى في مكانها.</p>';return `<div class="folder-import-status"><strong>${state.running?(state.scanning?'فحص المجلدات…':'استيراد الملفات…'):(state.cancelled?'أُلغي الفحص':'اكتمل فحص المجلدات')}</strong><div class="row wrap"><span>تمت معالجة: ${state.processed||0} / ${state.total||0}</span><span>أضيف أو حُدث: ${state.added||0}</span><span>مكرر أو موجود: ${state.skipped||0}</span><span>متعذر: ${state.failed||0}</span></div>${state.current?`<p dir="auto" class="folder-current">${esc(state.current)}</p>`:''}${state.running?'<button class="quiet" data-action="folder-import-cancel">إيقاف الفحص</button>':''}${state.errors?.length?`<details><summary>تفاصيل الملفات المتعذرة</summary>${state.errors.map(error=>`<p dir="auto">${esc(error)}</p>`).join('')}</details>`:''}</div>`}
function folders(){return (snapshot.preferences.libraryFolders||[]).map(path=>`<div class="library-folder-row"><span dir="auto">${esc(path)}</span><button class="quiet" data-folder-remove="${esc(path)}">إزالة من القائمة</button></div>`).join('')||'<p class="note">لم تُضف مجلدات بعد</p>'}
function settings(){return `<div class="settings-grid metadata-settings"><section class="settings-panel"><h3>مجلدات الموسيقى</h3><div id="library-folder-list">${folders()}</div><div class="row wrap"><button class="primary" data-action="import-folder">إضافة مجلد</button><button class="quiet" data-action="folder-rescan">إعادة فحص المجلدات</button></div><div id="folder-import-progress">${folderProgress()}</div><p class="note">إزالة المجلد من القائمة تبقي الأغاني المستوردة وملفاتها.</p></section><section class="settings-panel"><h3>مصادر الكلمات والأغلفة الإضافية</h3><p class="note">مصادر كلمات متعددة اللغات تشمل اليابانية والعربية والروسية، مع خمسة مصادر أغلفة إضافية. تُحفظ النتائج المطابقة محليًا وتُقرأ قبل الشبكة. بعض المصادر تحتاج مفتاحك الخاص.</p><label class="field" for="lrcapi-url">عنوان خادم LrcAPI</label><input id="lrcapi-url" type="url" dir="ltr" value="${esc(snapshot.preferences.lrcapiURL||'')}" placeholder="https://your-lrcapi-server.example"><button class="quiet" data-action="save-lrcapi-server">حفظ خادم الكلمات</button><p class="note">LrcAPI يحتاج خادمًا متوافقًا متاحًا لك حاليًا. AudD اختياري ويستهلك رصيد خدمتك ولا يُفعّل تلقائيًا.</p>${keyProviders.map(([id,name])=>`<label class="field" for="metadata-key-${id}">${esc(name)} · <span data-metadata-state="${id}">${snapshot.metadataConfigured?.[id]?'مفتاح محفوظ':'دون مفتاح محفوظ'}</span></label><div class="row metadata-key-row"><input type="password" id="metadata-key-${id}" autocomplete="off" aria-label="مفتاح ${esc(name)}" placeholder="اتركه فارغًا للحفاظ على المفتاح"><button class="quiet" data-metadata-save="${id}">حفظ المفتاح</button><button class="quiet" data-metadata-clear="${id}">مسح المفتاح</button></div>`).join('')}<p class="note">المفاتيح محفوظة مشفرة لمستخدم Windows الحالي؛ لا تُرفق حسابات أو مفاتيح خاصة.</p></section></div>`}
function artworkActions(){return current?`<button class="quiet" data-artwork="${esc(current.id)}">أغلفة الأغنية والألبوم</button>`:''}
function renderArtwork(id,result){
 return `<h2>أغلفة الأغنية والألبوم</h2><p>${result.cached?'نتائج محفوظة محليًا':'حُفظت الصور المطابقة محليًا'} · ${result.candidates?.length||0}</p><div class="artwork-candidates">${(result.candidates||[]).map(candidate=>`<button class="artwork-choice ${result.selectedId===candidate.id?'selected':''}" data-artwork-apply="${esc(candidate.id)}" data-task="${esc(id)}"><img loading="lazy" decoding="async" src="${esc(candidate.previewUrl+'?size=512')}" alt="غلاف من ${esc(candidate.providerName)}"><strong>${esc(candidate.providerName)}</strong><span>${candidate.width} × ${candidate.height} · ${esc(candidate.format)} · ${size(candidate.bytes)}</span><span>${candidate.cached?'محفوظ محليًا':'صورة متحققة'}</span></button>`).join('')}</div>${!result.candidates?.length?'<p>لم يتوفر غلاف مطابق من المصادر المتاحة</p>':''}<details class="artwork-source-status"><summary>حالة مصادر الأغلفة</summary>${Object.entries(result.statuses||{}).map(([provider,status])=>`<p><strong>${esc(provider.replace('artwork-',''))}</strong>: <span>${esc(status)}</span></p>`).join('')}</details><button class="secondary" data-artwork-refresh="${esc(id)}">بحث جديد في مصادر الأغلفة</button>`;
}
async function openArtwork(id,refresh=false){
 const token=++artRequest;modal(`<h2>أغلفة الأغنية والألبوم</h2><p>البحث عن صور مطابقة وحفظها محليًا…</p><div id="artwork-results" data-request="${token}"></div>`);
 const result=await call('SearchArtwork',id,refresh);if(!$('#modal').open||$('#artwork-results')?.dataset.request!==String(token))return;
 $('#modal-content').innerHTML=renderArtwork(id,result);
}
function update(){
 const list=$('#library-folder-list'),progress=$('#folder-import-progress');if(!list&&!progress)return;
 for(const state of $$('[data-metadata-state]')){const value=snapshot.metadataConfigured?.[state.dataset.metadataState]?'مفتاح محفوظ':'دون مفتاح محفوظ';if(state.dataset.current!==value){state.textContent=value;state.dataset.current=value}}
 const signature=JSON.stringify(snapshot.preferences.libraryFolders||[]);if(list&&signature!==folderSignature){list.innerHTML=folders();folderSignature=signature}
 if(progress){const html=folderProgress();if(progress.dataset.snapshot!==JSON.stringify(snapshot.folderImport||{})){progress.innerHTML=html;progress.dataset.snapshot=JSON.stringify(snapshot.folderImport||{})}}
}
async function cachePreview(id){
 try{const result=await call('CacheSourceArtwork',id);if(!result?.previewUrl)return;ui.artworkSelections??={};ui.artworkSelections[id]=result.previewUrl;persist();if(current?.id===id){updateNow();if(view==='lyrics'||view==='player')render()}}
 catch{} // Audio preview remains usable if the source has no available artwork.
}
document.addEventListener('click',async event=>{
 const button=event.target.closest('button');if(!button)return;
 try{
  if(button.dataset.action==='import-folder'){const result=await call('AddLibraryFolder');await poll();if(result?.running)toast('بدأ استيراد المجلد في الخلفية');update()}
  else if(button.dataset.action==='folder-rescan'){await call('RescanLibraryFolders');await poll();update()}
  else if(button.dataset.action==='folder-import-cancel'){await call('CancelFolderImport');await poll();update()}
  else if(button.dataset.folderRemove){await call('RemoveLibraryFolder',button.dataset.folderRemove);await poll();update()}
  else if(button.dataset.action==='save-lrcapi-server'){await call('SetPreferences',{...snapshot.preferences,lrcapiURL:$('#lrcapi-url').value.trim()});await poll();toast('حُفظ خادم الكلمات')}
  else if(button.dataset.metadataSave){const input=$('#metadata-key-'+button.dataset.metadataSave);if(!input.value.trim()){toast('أدخل المفتاح أو استخدم زر المسح');return}await call('SetMetadataKey',button.dataset.metadataSave,input.value.trim());input.value='';await poll();toast('حُفظ المفتاح مشفرًا')}
  else if(button.dataset.metadataClear){await call('SetMetadataKey',button.dataset.metadataClear,'');await poll();toast('مُسح المفتاح')}
  else if(button.dataset.artwork){await openArtwork(button.dataset.artwork)}
  else if(button.dataset.artworkRefresh){await openArtwork(button.dataset.artworkRefresh,true)}
  else if(button.dataset.artworkApply){button.disabled=true;const candidate=await call('ApplyArtwork',button.dataset.task,button.dataset.artworkApply);ui.artworkSelections??={};ui.artworkSelections[button.dataset.task]=candidate.previewUrl;persist();await poll();$('#modal').close();render();updateNow();toast('حُفظ الغلاف محليًا')}
 }catch(error){toast(error.message||error);button.disabled=false}
});
window.madarMetadata={settings,artworkActions,update,openArtwork,cachePreview};
})();
