'use strict';
(()=>{
const menu=document.createElement('button');menu.className='android-menu icon-button';menu.setAttribute('aria-label','كل الصفحات / All pages');menu.textContent='☰';document.querySelector('.titlebar').append(menu);
menu.addEventListener('click',()=>{modal('<h2>نَوى · Nawa</h2><div class="android-nav-menu">'+document.getElementById('navigation').innerHTML+'</div>');document.querySelector('.android-nav-menu').addEventListener('click',e=>{if(e.target.closest('[data-nav]'))document.getElementById('modal').close()})});
let enhancements=false;
async function androidSettings(){
 const host=document.getElementById('main');if(view!=='settings'||host.querySelector('#android-settings'))return;
 const panel=document.createElement('section');panel.id='android-settings';panel.className='settings-panel';
 const english=document.documentElement.lang==='en';
 panel.innerHTML=`<h3>${english?'Android · On this device':'أندرويد · على جهازك'}</h3><p>${english?'Music and cached lyrics stay in the app. Import folders, export files, or add your own fonts.':'الموسيقى والكلمات المحفوظة تبقى داخل التطبيق. أضف مجلداتك أو صدّر نسخة إلى مجلد تختاره، واستورد خطوطك.'}</p><div class="row wrap"><button class="secondary" id="android-export">${english?'Export library':'تصدير المكتبة'}</button><button class="secondary" id="android-font">${english?'Import TTF / OTF font':'استيراد خط TTF / OTF'}</button><button class="secondary" id="android-model">${english?'Download timing model (148 MB)':'تنزيل نموذج التوقيت (148 MB)'}</button><span id="android-model-progress"></span></div><p class="note">${english?'Translation downloads language models on first use, then works offline. Audio effects and Dolby/Hi-Res output depend on the device. Ten EQ sliders map to the available hardware bands; width uses the platform virtualizer. A timing candidate needs review; character motion is visual.':'الترجمة تُنزّل نماذج اللغة أول مرة ثم تعمل بلا إنترنت. المؤثرات وإخراج Dolby وHi-Res تعتمد على الجهاز. منزلقات EQ العشرة تُطابق نطاقات الجهاز المتاحة؛ الاتساع يستخدم مؤثر النظام. نتيجة التوقيت تحتاج مراجعة، وتحريك الحروف بصري.'}</p>`;
 host.prepend(panel);
 const output=document.createElement('label');output.className='resource-settings-row';
 output.innerHTML=`<span>${english?'Audio output':'إخراج الصوت'}</span><select id="android-audio-output"><option value="compatible">${english?'Compatible · recommended':'متوافق · موصى به'}</option><option value="float">${english?'High precision · supported devices':'دقة عالية · للأجهزة الداعمة'}</option></select>`;
 panel.append(output);
 const explanation=document.createElement('p');explanation.className='note';explanation.textContent=english?'Compatible output uses the standard Android audio path. High precision requests float output; speed effects use the compatible path. Original files stay unchanged.':'الإخراج المتوافق يستخدم مسار أندرويد القياسي. الدقة العالية تطلب إخراج Float؛ مؤثرات السرعة تستخدم المسار المتوافق. تبقى الملفات الأصلية كما هي.';panel.append(explanation);
 output.querySelector('select').onchange=async e=>{const picker=e.currentTarget;picker.disabled=true;try{await NawaMobile.send('output',{mode:picker.value});toast(english?'Audio output updated':'تم تغيير إخراج الصوت')}catch(error){toast(error.message)}finally{picker.disabled=false}};
 panel.querySelector('#android-export').onclick=()=>call('ExportLibrary').then(n=>toast(english?`Exported ${n||0} files`:`صُدّر ${n||0} ملفًا`)).catch(e=>toast(e.message));
 panel.querySelector('#android-font').onclick=()=>call('ImportFont').then(async name=>{if(!name)return;await loadFonts();installedFonts=await call('GetFonts');toast(english?'Font added. Select it in lyrics settings.':'أُضيف الخط. اختره من إعدادات الكلمات.');}).catch(e=>toast(e.message));
 panel.querySelector('#android-model').onclick=async e=>{const b=e.currentTarget;b.disabled=true;try{await call('DownloadTimingModel');toast(english?'Timing model ready offline':'نموذج التوقيت جاهز محليًا');await poll()}catch(e){toast(e.message)}finally{b.disabled=false}};
 try{const s=await call('AndroidStatus');output.querySelector('select').value=s.audioOutput||'compatible';if(s.modelReady)panel.querySelector('#android-model-progress').textContent=english?'Model ready':'النموذج جاهز'}catch{}
}
async function loadFonts(){
 const names=await call('GetFonts');let style=document.getElementById('android-font-faces');if(!style){style=document.createElement('style');style.id='android-font-faces';document.head.append(style)}
 // Native font IDs contain only hexadecimal characters. Both supported formats
 // are tried, while the original files remain untouched.
 style.textContent=names.filter(n=>/^NawaFont-[a-f0-9]{32}$/.test(n)).map(n=>{const id=n.slice(9);return `@font-face{font-family:"${n}";src:url("/fonts/${id}.ttf") format("truetype"),url("/fonts/${id}.otf") format("opentype");font-display:swap}`}).join('');
}
const originalRender=window.madarRender;
new MutationObserver(()=>{if(enhancements)return;enhancements=true;requestAnimationFrame(()=>{enhancements=false;androidSettings()})}).observe(document.getElementById('main'),{childList:true});
document.addEventListener('click',e=>{if(e.target.closest('[data-action="about"]'))setTimeout(()=>{const target=document.getElementById('modal-content');if(target)target.innerHTML='<h2>نَوى · Nawa for Android</h2><p>مكتبة مستقلة، مشغل Media3 بالخلفية، FFmpeg وyt-dlp لأندرويد، ترجمة ML Kit محلية وتوقيت Whisper.cpp. المصدر والتراخيص مرفقة.</p><p>Independent Android music library, Media3 background playback, FFmpeg, yt-dlp, ML Kit translation and Whisper.cpp timing.</p>'},0)});
loadFonts().catch(()=>{});
call('SetRefreshRate',ui.lyricFps||'auto').catch(()=>{});
document.addEventListener('change',e=>{if(e.target.matches('[data-lyric-fps]'))call('SetRefreshRate',e.target.value).catch(error=>toast(error.message))});
})();
