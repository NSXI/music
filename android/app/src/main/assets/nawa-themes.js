'use strict';
// Theme lifecycle only: finite entrances; no independent RAF/audio or network.
(()=>{
const palettes={bloodborne:{background:'#101113',panel:'#242123',text:'#e8dfce',muted:'#b8ad9e',accent:'#bd7b71'},kingdom:{background:'#091b37',panel:'#152c4d',text:'#f0f7ff',muted:'#adc6e2',accent:'#82e6eb'},reload:{background:'#071d67',panel:'#103da3',text:'#f6fcff',muted:'#b1d6f5',accent:'#69edff'}};
const entrances={glass:1350,orchard:1250,pulse:1250,cloud:1450,prism:1500,noir:1300,bloodborne:1850,kingdom:1800,reload:1500};
const gameLogos={bloodborne:{file:'bloodborne.svg',name:'Bloodborne'},kingdom:{file:'kingdom-hearts.png',name:'KINGDOM HEARTS HD 1.5 + 2.5 ReMIX'},reload:{file:'persona-3-reload.svg',name:'Persona 3 Reload'}};
if(typeof ui.themeEntrance!=='boolean')ui.themeEntrance=true;
function gameLogo(id,extra=''){const game=gameLogos[id];return game?'<img class="theme-game-logo game-logo-'+id+' '+extra+'" src="theme-logos/'+game.file+'" alt="'+esc(game.name)+'" decoding="async">':''}
function themePreview(id){return gameLogo(id,'theme-card-game-logo')}
function ornaments(id){return '<div class="entry-ornaments" aria-hidden="true">'+Array.from({length:id==='pulse'?12:id==='prism'?5:id==='kingdom'?7:4},(_,i)=>'<i style="--piece:'+i+'"></i>').join('')+'</div>'}
const reduced=window.matchMedia('(prefers-reduced-motion: reduce)');let lastTheme='',lastView='',entry=null,timer=0;
const logoID=()=>window.NAWA_LOGOS.some(p=>p.id===Number(ui.nawaLogo))?Number(ui.nawaLogo):1;
function dismiss(){if(timer)clearTimeout(timer);timer=0;entry?.remove();entry=null}
function entrance(){
 dismiss();const id=ui.theme;
 if(!entrances[id]||!ui.themeEntrance||!ui.motion||reduced.matches||document.hidden)return;
 const node=document.createElement('div');entry=node;node.className='nawa-entry entry-'+id;node.dataset.entryTheme=id;node.setAttribute('aria-hidden','true');node.style.setProperty('--entry-duration',entrances[id]+'ms');
 const logo=gameLogo(id,'entry-game-logo')||window.nawaLogoSVG(logoID());
 node.innerHTML=ornaments(id)+'<div class="entry-stage"><div class="entry-emblem '+(gameLogos[id]?'entry-emblem-game':'')+'">'+logo+'</div><span class="entry-signature" data-no-i18n>Nawa · نَوى</span></div>';
 document.body.append(node);
 node.addEventListener('animationend',event=>{if(entry===node&&event.target===node)dismiss()});
 node.addEventListener('animationcancel',event=>{if(entry===node&&event.target===node)dismiss()});
 timer=setTimeout(()=>{if(entry===node)dismiss()},entrances[id]+120);
}
function route(){if(view===lastView)return;lastView=view;const main=document.querySelector('#main');if(main){main.classList.remove('nawa-route-enter');if(entrances[ui.theme]&&ui.motion&&!reduced.matches&&!document.hidden){void main.offsetWidth;main.classList.add('nawa-route-enter')}}}
function apply(){
 ui.nawaLogo=logoID();const english=document.documentElement.lang==='en',name=english?'Nawa':'نَوى';document.title='Nawa · نَوى';
 const word=document.querySelector('.wordmark');if(word){word.innerHTML='<span class="nawa-wordmark-icon">'+window.nawaLogoSVG(ui.nawaLogo)+'</span><span data-no-i18n>'+name+'</span><small>Music</small>'}
 const lockup=document.querySelector('.brand-lockup');if(lockup){lockup.innerHTML='<div class="nawa-brand-emblem">'+window.nawaLogoSVG(ui.nawaLogo)+'</div><div><strong data-no-i18n>'+name+'</strong><span>'+(english?'Music, close to you.':'موسيقاك، أقرب إليك.')+'</span></div>'}
 document.body.dataset.nawaBrandLogo=String(ui.nawaLogo);
 if(lastTheme!==ui.theme){lastTheme=ui.theme;lastView='';entrance();route();window.madarVerci?.sync()}
 if(!ui.themeEntrance||!ui.motion||reduced.matches||document.hidden)dismiss();
 window.madarGameThemes?.decorate();
}
function settings(){return '<section class="nawa-logo-settings settings-panel"><h3>شعار نَوى</h3><p>عشرة شعارات أصلية. اختر شعارك؛ يُحفظ اختيارك تلقائيًا.</p><div class="nawa-logo-picker">'+window.NAWA_LOGOS.map(p=>'<button class="nawa-logo-choice '+(logoID()===p.id?'selected':'')+'" data-nawa-logo="'+p.id+'" aria-pressed="'+(logoID()===p.id)+'"><span style="color:'+p.color+'">'+window.nawaLogoSVG(p.id)+'</span><strong>'+esc(p.id+' · '+p.name)+'</strong></button>').join('')+'</div><label class="check-line nawa-entrance-toggle"><input type="checkbox" data-nawa-entrance '+(ui.themeEntrance?'checked':'')+'>دخولية خاصة لكل ثيم</label><button class="quiet" data-nawa-replay '+(!ui.motion||!ui.themeEntrance?'disabled':'')+'>إعادة عرض دخول الثيم</button><p class="note">تظهر الدخولية عند فتح التطبيق وتغيير الثيم. شعارات الألعاب تظهر في دخوليات ثيماتها؛ يبقى اختيار شعار نَوى محفوظًا.</p><p class="note">يتغير الشعار داخل التطبيق؛ أيقونة ملف Windows تستخدم الشعار الأول.</p></section>'}
function replay(){if(entrances[ui.theme])entrance()}
document.addEventListener('click',e=>{const button=e.target.closest('[data-nawa-logo]');if(button){ui.nawaLogo=Number(button.dataset.nawaLogo);persist();apply();for(const b of document.querySelectorAll('[data-nawa-logo]')){const selected=Number(b.dataset.nawaLogo)===ui.nawaLogo;b.classList.toggle('selected',selected);b.setAttribute('aria-pressed',String(selected))}}if(e.target.closest('[data-nawa-replay]'))replay()});
document.addEventListener('change',event=>{if(!event.target.matches('input[data-nawa-entrance]'))return;ui.themeEntrance=event.target.checked;persist();if(!ui.themeEntrance)dismiss();const replay=document.querySelector('[data-nawa-replay]');if(replay)replay.disabled=!ui.themeEntrance||!ui.motion});
document.addEventListener('visibilitychange',()=>{document.body.classList.toggle('nawa-hidden',document.hidden);if(document.hidden)dismiss()});
reduced.addEventListener?.('change',()=>{dismiss();apply()});
if(typeof MutationObserver!=='undefined')new MutationObserver(apply).observe(document.documentElement,{attributes:true,attributeFilter:['lang']});
window.madarNawa={apply,route,settings,font:id=>({bloodborne:'Palatino Linotype',kingdom:'Trebuchet MS',reload:'Bahnschrift'}[id]||''),palette:id=>palettes[id]||null,themePreview,entrances:Object.keys(entrances),gameLogos,replay,dismiss,status:()=>({theme:ui.theme,logo:logoID(),intro:Boolean(entry),entryTheme:entry?.dataset.entryTheme||'',entranceEnabled:ui.themeEntrance,reduced:reduced.matches})};apply();route();
})();

