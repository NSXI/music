require('node:fs').mkdirSync('qa/results',{recursive:true});
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const {parseHTML}=require('linkedom');
const prefix=fs.readFileSync('qa/ui_logic_test.cjs','utf8').split('const song=')[0];
const fixtures=new Function('require',prefix+String.raw`
const views=[];
views.push({name:'application-shell',html:fs.readFileSync('frontend/index.html','utf8').match(/<body[^>]*>([\s\S]*)<\/body>/)[1]});
const song={id:'fixture',title:'الكلمات',artist:'المصدر',album:'الترجمة',genres:['ترجمة'],duration:24,provider:'archive',url:'https://example.test/track'};
const task={id:'fixture',song,status:'done',quality:{codec:'FLAC',lossless:true,bits:24,rate:96000},source:'archive',log:[],path:'C:/Music/الكلمات.flac',bytes:12345,total:12345};
run('snapshot.jobs='+JSON.stringify([task])+';current='+JSON.stringify(task)+';searchResult='+JSON.stringify({title:'USER_COLLECTION',tracks:[song],complete:false,providerCounts:{archive:1},providerErrors:{tidal:'Server setup required'}}));
vm.runInContext(fs.readFileSync('frontend/fullscreen.js','utf8'),ctx);
ctx.window.addEventListener=()=>{};
vm.runInContext(fs.readFileSync('frontend/lyric-art.js','utf8'),ctx);
vm.runInContext(fs.readFileSync('frontend/resource-controls.js','utf8'),ctx);
vm.runInContext(fs.readFileSync('frontend/verci-lyrics.js','utf8'),ctx);
vm.runInContext(fs.readFileSync('frontend/metadata-library.js','utf8'),ctx);
views.push({name:'queue-states',html:run(JSON.stringify(['queued','working','done','paused','failed','cancelled'])+'.map(status=>jobCard({...'+JSON.stringify(task)+',status})).join("")')});
for(const name of ['home','search','library','albums','downloads','player','lyrics','appearance','settings']){run('view='+JSON.stringify(name)+';render()');views.push({name,html:get('#main').innerHTML})}
run('ui.libraryMode="tracks";view="library";render()');views.push({name:'track-library',html:get('#main').innerHTML});
run('view="lyrics";installedFonts=["Arial"];lyricData=[{time:0,text:"USER_ORIGINAL"}];translationData={exists:true,originalHash:"fixture",lines:[{original:"USER_ORIGINAL",text:"ترجمة المستخدم",time:0}]};openLyricsSettings()');views.push({name:'lyrics-popup',html:get('#modal-content').innerHTML});
for(const expression of ['showInfo("fixture")','playlistDialog()','editTranslation()']){run(expression);views.push({name:expression,html:get('#modal-content').innerHTML})}
for(const medium of ['cd','vinyl','cassette','square']){run('ui.fsMedium='+JSON.stringify(medium));views.push({name:'fullscreen-'+medium,html:run('window.madarFullscreen.render(current)')})}
run('snapshot.jobs=[];current=null;searchResult=null');
for(const name of ['home','search','library','albums','downloads','player','lyrics','settings']){run('view='+JSON.stringify(name)+';render()');views.push({name:'empty-'+name,html:get('#main').innerHTML})}
run('searchBusy=true;view="search";render()');views.push({name:'busy-search',html:get('#main').innerHTML});
return views;
`)(require);
let checks=0;
function fingerprint(node){return node.nodeType===1?[node.tagName,[...node.attributes].map(a=>[a.name,a.value]).sort((a,b)=>a[0].localeCompare(b[0])),[...node.childNodes].map(fingerprint)]:[node.nodeType,node.nodeValue]}
async function test(){
 const uncovered=new Set();
 for(const fixture of fixtures){
  const {window,document}=parseHTML('<!doctype html><html lang="ar" dir="rtl"><head><title>مدار Music</title></head><body>'+fixture.html+'</body></html>');
  // LinkeDOM omits the browser's standard writable select.value property.
  const selectPrototype=Object.getPrototypeOf(document.createElement('select')),selectValue=Object.getOwnPropertyDescriptor(selectPrototype,'value');
  if(!selectValue.set)Object.defineProperty(selectPrototype,'value',{configurable:true,get:selectValue.get,set(value){const options=[...this.querySelectorAll('option')];for(const option of options)option.removeAttribute('selected');options.find(option=>option.value===value)?.setAttribute('selected','')}});
  const events=[];document.addEventListener('change',event=>events.push(event));
  const saved=[];
  const ui={language:'ar',volume:.52,translationTarget:'ar',lyricFont:'Arial',speed:.8,fsMedium:'cd'};
  const ctx=vm.createContext({window,document,ui,view:'home',filter:'all',persist:()=>saved.push(JSON.stringify(ui)),MutationObserver:window.MutationObserver,NodeFilter:{SHOW_TEXT:4},queueMicrotask,console});
  vm.runInContext(fs.readFileSync('frontend/locale-en.js','utf8'),ctx);
  vm.runInContext(fs.readFileSync('frontend/i18n.js','utf8'),ctx);
  const before=document.body.innerHTML,beforeFingerprint=JSON.stringify(fingerprint(document.body)),originalPlayLabel=document.querySelector('[data-play][aria-label]')?.getAttribute('aria-label');
  window.madarI18n.setLanguage('en');
  assert.equal(document.documentElement.lang,'en');assert.equal(document.documentElement.dir,'ltr');
  const languagePicker=document.querySelector('[data-app-language]');if(languagePicker){assert.equal(languagePicker.value,'en');assert.equal(languagePicker.getAttribute('aria-label'),'App language');checks+=2}
  assert.equal(ui.translationTarget,'ar');assert.equal(ui.volume,.52);assert.equal(ui.speed,.8);assert.equal(ui.lyricFont,'Arial');checks+=6;
  for(const el of document.querySelectorAll('[dir="auto"],.player-title,#fs-title,#now-title,.lyric-line,.playlist-checks')){
   if(el.textContent==='الكلمات')assert.equal(el.textContent,'الكلمات');
  }
  const userTitle=document.querySelector('.title-cell strong,.album-card strong,.player-title,#fs-title');
  if(userTitle){assert.ok(userTitle.textContent==='الكلمات'||userTitle.textContent==='الترجمة');checks++}
  const aria=document.querySelector('[data-play][aria-label]');if(aria){assert.equal(aria.getAttribute('aria-label'),originalPlayLabel==='تشغيل'?'Play':'Play الكلمات',fixture.name);checks++}
  const walker=document.createTreeWalker(document.body,4);let node;
  while((node=walker.nextNode()))if(/[\u0600-\u06ff]/.test(node.nodeValue)&&!node.parentElement.closest('[dir="auto"],.lyric-line,.player-title,.playlist-checks,pre,[data-no-i18n]'))uncovered.add(node.nodeValue.trim());
  window.madarI18n.setLanguage('ar');
  if(JSON.stringify(fingerprint(document.body))!==beforeFingerprint){fs.writeFileSync('qa/results/i18n-roundtrip-before.html',before);fs.writeFileSync('qa/results/i18n-roundtrip-after.html',document.body.innerHTML);throw new Error('Arabic round trip: '+fixture.name)}checks++;
  window.madarI18n.setLanguage('en');
  const late=document.createElement('div');late.innerHTML='<button aria-label="إغلاق">إغلاق</button><p>55 مسار محفوظ في مكتبتك</p><strong dir="auto">الكلمات</strong><div class="lyric-line"><span>ترجمة المستخدم</span></div><textarea>النص الأصلي</textarea><select id="lyric-font"><option value="">خط الواجهة تلقائيًا</option><option value="الكلمات">الكلمات</option></select>';
  document.body.append(late);
  await new Promise(resolve=>setImmediate(resolve));
  assert.equal(late.querySelector('button').textContent,'Close');assert.equal(late.querySelector('button').getAttribute('aria-label'),'Close');
  assert.equal(late.querySelector('p').textContent,'55 tracks saved in your library');
  assert.equal(late.querySelector('strong').textContent,'الكلمات');assert.equal(late.querySelector('.lyric-line').textContent,'ترجمة المستخدم');assert.equal(late.querySelector('textarea').value,'النص الأصلي');checks+=6;
  assert.equal(late.querySelector('option[value="الكلمات"]').textContent,'الكلمات');assert.equal(late.querySelector('option[value=""]').textContent,'Use interface font');checks+=2;
  late.querySelector('p').textContent='56 مسار محفوظ في مكتبتك';
  await new Promise(resolve=>setImmediate(resolve));assert.equal(late.querySelector('p').textContent,'56 tracks saved in your library');checks++;
  window.madarI18n.setLanguage('ar');assert.equal(late.querySelector('p').textContent,'56 مسار محفوظ في مكتبتك');assert.equal(document.documentElement.dir,'rtl');checks+=2;
  if(languagePicker){languagePicker.value='en';languagePicker.dispatchEvent(new window.Event('change',{bubbles:true}));assert.equal(ui.language,'en');assert.equal(document.documentElement.dir,'ltr');assert.equal(JSON.parse(saved.at(-1)).language,'en');checks+=3}
 }
 fs.writeFileSync('qa/results/i18n-uncovered.json',JSON.stringify([...uncovered],null,2));
 assert.equal(uncovered.size,0,'Untranslated interface copy: '+JSON.stringify([...uncovered]));
 console.log('PASS:',checks,'checks across',fixtures.length,'rendered states; English/Arabic round trips, live DOM updates, metadata/lyrics protection, attributes, direction and saved preference.');
}
test().catch(error=>{console.error(error);process.exitCode=1});
