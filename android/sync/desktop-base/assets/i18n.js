'use strict';
// Interface localization only. Audio, metadata, lyrics and editable user text
// keep their original values, direction and timing.
(()=>{
const copy=window.MADAR_EN||{};
const entries=Object.entries(copy).filter(([key])=>key.length>1).sort((a,b)=>b[0].length-a[0].length);
const textHistory=new WeakMap(),attributeHistory=new WeakMap();
const protectedContent='script,style,textarea,[data-no-i18n],[data-user-content],.lyric-line,.lyric-content,.lyric-translation,.translation-editor [dir="auto"],.playlist-checks,.player-title,#now-title,#now-artist,#fs-title,#fs-artist,pre,[dir="auto"],[data-filter^="playlist:"],#lyric-font option:not([value=""]),#translation-font option:not([value=""])';
let scheduled=false,applying=false;
const valid=value=>value==='en'?'en':'ar';
ui.language=valid(ui.language);
const escapeRegex=value=>value.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
const replacements=entries.map(([from,to])=>[new RegExp('(?<![\\p{L}\\p{M}])'+escapeRegex(from)+'(?![\\p{L}\\p{M}])','gu'),to]);
function translate(value,language=ui.language){
 const text=String(value??'');
 if(language!=='en'||!/[\u0600-\u06ff]/.test(text))return text;
 const normalized=text.trim().replace(/\s+/g,' ');
 let result=copy[normalized];
 if(result===undefined){
  result=normalized;
  for(const [pattern,replacement] of replacements)result=result.replace(pattern,()=>replacement);
  result=result.replace(/(\d+(?:\.\d+)?)ث(?=$|[\s·،])/g,'$1s').replace(/\b1 tracks\b/g,'1 track').replace(/\b1 albums\b/g,'1 album');
 }
 return text.match(/^\s*/)[0]+result+text.match(/\s*$/)[0];
}
function isProtected(element){
 if(!element)return true;
 if(element.matches('.genre-card strong')&&['غير مصنّف','Uncategorized'].includes(element.textContent.trim()))return false;
 if(element.matches('.album-card strong')&&['ألبوم غير معروف','Unknown album'].includes(element.textContent.trim()))return false;
 if(element.closest('#now-title,#now-artist')&&(typeof current==='undefined'||!current))return false;
 if(element.closest(protectedContent))return true;
 return typeof view!=='undefined'&&view==='library'&&filter.startsWith('genre:')&&element.closest('.page-head h1')&&!['غير مصنّف','Uncategorized'].includes(element.textContent.trim());
}
function localizeText(node){
 if(isProtected(node.parentElement)||!node.nodeValue?.trim())return;
 const previous=textHistory.get(node),currentValue=node.nodeValue;
 const source=previous&&currentValue===previous.target?previous.source:currentValue;
 const target=translate(source);
 textHistory.set(node,{source,target});
 if(currentValue!==target)node.nodeValue=target;
}
function localizeAttribute(element,name){
 if(element.closest('[data-no-i18n],[data-user-content],.lyric-line,.lyric-content,.lyric-translation,pre,textarea'))return;
 const currentValue=element.getAttribute(name);if(!currentValue)return;
 let history=attributeHistory.get(element);if(!history){history=new Map();attributeHistory.set(element,history)}
 const previous=history.get(name),source=previous&&currentValue===previous.target?previous.source:currentValue;
 let target=source;
 if(ui.language==='en'){
  // Track titles within accessible control names are opaque user metadata.
  const preview=source.match(/^تشغيل (.+) من المصدر قبل التنزيل$/);
  const metadata=source.match(/^(غلاف|تشغيل|تحديد|تنزيل|فحص حجم وجودة) (.+)$/);
  if(copy[source]!==undefined)target=copy[source];
  else if(preview)target='Play '+preview[1]+' from the source before downloading';
  else if(metadata)target=({غلاف:'Artwork for',تشغيل:'Play',تحديد:'Select',تنزيل:'Download','فحص حجم وجودة':'Check size and quality for'}[metadata[1]])+' '+metadata[2];
  else target=translate(source);
 }
 history.set(name,{source,target});if(currentValue!==target)element.setAttribute(name,target);
}
function refresh(root=document.body){
 if(!root||applying)return;
 applying=true;
 try{
  if(root.nodeType===3){localizeText(root);return}
  const walker=document.createTreeWalker(root,NodeFilter.SHOW_TEXT);
  let node;while((node=walker.nextNode()))localizeText(node);
  const selector='[aria-label],[title],[placeholder],[alt]';
  const elements=[...(root.matches?.(selector)?[root]:[]),...root.querySelectorAll(selector)];
  for(const element of elements)for(const name of ['aria-label','title','placeholder','alt'])if(element.hasAttribute(name))localizeAttribute(element,name);
  for(const picker of document.querySelectorAll('[data-app-language]'))if(picker.value!==ui.language)picker.value=ui.language;
 }finally{applying=false}
}
function applyLanguage(){
 document.documentElement.lang=ui.language;
 document.documentElement.dir=ui.language==='en'?'ltr':'rtl';
 document.title=ui.language==='en'?'Nawa Music':'نَوى Music';
 refresh();
}
function setLanguage(language){ui.language=valid(language);persist();applyLanguage()}
document.addEventListener('change',event=>{if(event.target.matches('[data-app-language]'))setLanguage(event.target.value)});
const pending=new Set();
const observer=new MutationObserver(records=>{
 // Ignore the localization layer's own writes. For new render nodes, only
 // process their subtree, avoiding full-page work during audio time updates.
 for(const record of records){
  if(record.type==='characterData'){
   const saved=textHistory.get(record.target);if(!saved||record.target.nodeValue!==saved.target)pending.add(record.target);
  }else if(record.type==='attributes'){
   const saved=attributeHistory.get(record.target)?.get(record.attributeName);
   if(!saved||record.target.getAttribute(record.attributeName)!==saved.target)pending.add(record.target);
  }else for(const node of record.addedNodes)if(node.nodeType===1||node.nodeType===3){
   const saved=textHistory.get(node);if(!saved||node.nodeValue!==saved.target)pending.add(node);
  }
 }
 if(!pending.size||scheduled)return;
 scheduled=true;queueMicrotask(()=>{scheduled=false;const roots=[...pending];pending.clear();for(const node of roots)if(node.isConnected)refresh(node)});
});
observer.observe(document.body,{childList:true,subtree:true,characterData:true,attributes:true,attributeFilter:['aria-label','title','placeholder','alt']});
window.madarI18n={setLanguage,translate,refresh,get language(){return ui.language}};
applyLanguage();
})();
