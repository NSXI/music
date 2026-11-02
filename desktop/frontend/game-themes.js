'use strict';
// Approved theme integration. All lyric movement is supplied by the existing audio clock.
(()=>{
const presets={
 kingdom:{style:'game-kingdom',label:'Kingdom Hearts · حوار النور',font:'Nawa KHMenu',translationFont:'Nawa Arabic',character:'Sora',symbol:'crown',palette:{background:'#09172d',panel:'#182c46',text:'#f4f7ff',muted:'#9db5ce',accent:'#f8d45b'}},
 bloodborne:{style:'game-bloodborne',label:'Bloodborne · حبر الصياد',font:'Nawa Hunter',translationFont:'Nawa Amiri',character:'The Hunter',symbol:'rune',palette:{background:'#101112',panel:'#191b1d',text:'#e3ddca',muted:'#a7a396',accent:'#c8b993'}},
 reload:{style:'game-reload',label:'Persona 3 Reload · قصّ أزرق',font:'Nawa Reload',translationFont:'Nawa Arabic',character:'Makoto',symbol:'moon',palette:{background:'#083df2',panel:'#061c67',text:'#f5fcff',muted:'#a7cfff',accent:'#54ebff'}}
};
const keys=['lyricStyle','lyricFont','translationFont'],fonts=['Nawa KHMenu','Nawa Hunter','Nawa Reload','Nawa Arabic','Nawa Amiri'];
if(typeof ui.gameThemeLyrics!=='boolean')ui.gameThemeLyrics=true;
if(typeof ui.gameThemeArt!=='boolean')ui.gameThemeArt=true;
if(!['portrait','symbol','none'].includes(ui.gameThemeAvatar))ui.gameThemeAvatar='portrait';
if(!ui.gameThemeChoices||typeof ui.gameThemeChoices!=='object'||Array.isArray(ui.gameThemeChoices))ui.gameThemeChoices={};
let applied='',decorated='',ready=false;
for(const p of Object.values(presets))if(!lyricStyles.some(([id])=>id===p.style))lyricStyles.push([p.style,p.label]);
function snapshot(){return Object.fromEntries(keys.map(k=>[k,ui[k]||'']))}
function apply(){
 const p=presets[ui.theme],wanted=p&&ui.gameThemeLyrics?ui.theme:'';
 if(wanted!==applied){
  if(wanted){
   if(!ui.gameThemePrevious||typeof ui.gameThemePrevious!=='object')ui.gameThemePrevious=snapshot();
   const chosen=ui.gameThemeChoices[wanted]||{};
   ui.lyricStyle=lyricStyles.some(([id])=>id===chosen.lyricStyle)?chosen.lyricStyle:p.style;
   ui.lyricFont=typeof chosen.lyricFont==='string'?chosen.lyricFont:p.font;
   ui.translationFont=typeof chosen.translationFont==='string'?chosen.translationFont:p.translationFont;
  }else if(ui.gameThemePrevious){for(const k of keys)if(typeof ui.gameThemePrevious[k]==='string')ui[k]=ui.gameThemePrevious[k];delete ui.gameThemePrevious}
  applied=wanted;
 }
 document.body.dataset.gameArt=String(Boolean(p&&ui.gameThemeArt));
 document.body.dataset.gameAvatar=p?ui.gameThemeAvatar:'none';
 document.body.dataset.gameLyrics=String(Boolean(wanted));
 if(ready)decorate();
}
function decorate(){
 const host=document.querySelector('.brand-lockup'),p=presets[ui.theme];if(!host)return;
 const key=p?ui.theme+':'+ui.gameThemeAvatar+':'+document.documentElement.lang:'';
 let badge=host.querySelector('.nawa-game-avatar');
 if(!p||ui.gameThemeAvatar==='none'){badge?.remove();decorated='';return}
 if(badge&&decorated===key)return;badge?.remove();badge=document.createElement('span');badge.className='nawa-game-avatar game-avatar-'+ui.theme;badge.setAttribute('role','img');badge.setAttribute('aria-label',ui.gameThemeAvatar==='symbol'?p.label:p.character);badge.title=ui.gameThemeAvatar==='symbol'?p.label:p.character;host.append(badge);decorated=key;
}
function settings(){
 const en=document.documentElement.lang==='en',p=presets[ui.theme];
 return `<section class="lyrics-popup-section game-theme-settings settings-panel"><h3>${en?'Game theme details':'تفاصيل ثيمات الألعاب'}</h3><label class="check-line"><input type="checkbox" data-game-theme-lyrics ${ui.gameThemeLyrics?'checked':''}>${en?'Match lyric style and fonts to the game theme':'شكل الكلمات وخطوطها حسب ثيم اللعبة'}</label><label class="check-line"><input type="checkbox" data-game-theme-art ${ui.gameThemeArt?'checked':''}>${en?'Game atmosphere and symbols':'أجواء اللعبة ورموزها'}</label><label class="field">${en?'Theme avatar':'رمزية الثيم'}<select data-game-theme-avatar><option value="portrait" ${ui.gameThemeAvatar==='portrait'?'selected':''}>${en?'Character portrait':'رمزية الشخصية'}</option><option value="symbol" ${ui.gameThemeAvatar==='symbol'?'selected':''}>${en?'Game symbol':'رمز اللعبة'}</option><option value="none" ${ui.gameThemeAvatar==='none'?'selected':''}>${en?'Hidden':'مخفية'}</option></select></label>${p?`<p class="note" data-user-content>${esc(p.label)} · ${esc(p.font)}</p>`:''}<p class="note">${en?'Each game remembers your lyric style and independent font choices. Other themes keep your previous settings.':'يحفظ كل ثيم اختيارات الشكل وخط الأصل والترجمة بشكل مستقل، وتعود اختياراتك السابقة عند مغادرة ثيمات الألعاب.'}</p><button class="quiet" data-game-theme-reset ${p?'':'disabled'}>${en?'Restore this game’s lyric defaults':'استعادة كلمات الثيم الافتراضية'}</button><p class="note">${en?'KHMenu is a faithful recreation. Reload uses Barlow as a visual alternative, not the original Rodin font.':'KHMenu خط معاد رسمه. بيرسونا يستخدم Barlow بديلًا بصريًا، وليس خط Rodin الأصلي.'}</p></section>`;
}
document.addEventListener('change',event=>{
 const target=event.target;
 if(target.matches('[data-game-theme-lyrics],[data-game-theme-art],[data-game-theme-avatar]')){
  if(target.hasAttribute('data-game-theme-lyrics'))ui.gameThemeLyrics=target.checked;
  if(target.hasAttribute('data-game-theme-art'))ui.gameThemeArt=target.checked;
  if(target.hasAttribute('data-game-theme-avatar'))ui.gameThemeAvatar=target.value;
  const lyricsDialog=Boolean(document.querySelector('#modal')?.open&&document.querySelector('#lyric-font'));
  applyAppearance();render();if(lyricsDialog)openLyricsSettings().catch(error=>toast(error.message||String(error)));if(view==='lyrics')window.madarLyricClock?.sync();return;
 }
 const key={'lyric-font':'lyricFont','translation-font':'translationFont','lyric-style':'lyricStyle','lyric-style-settings':'lyricStyle'}[target.id];
 if(key&&applied){const previous=ui.gameThemeChoices[applied]||{};ui.gameThemeChoices[applied]={...previous,[key]:ui[key]};persist()}
});
document.addEventListener('click',event=>{if(!event.target.closest('[data-game-theme-reset]')||!presets[ui.theme])return;const lyricsDialog=Boolean(document.querySelector('#modal')?.open&&document.querySelector('#lyric-font'));delete ui.gameThemeChoices[ui.theme];applied='';applyAppearance();render();if(lyricsDialog)openLyricsSettings().catch(error=>toast(error.message||String(error)));window.madarVerci?.sync();window.madarLyricClock?.sync()});
const nawa=window.madarNawa;if(nawa){const oldSettings=nawa.settings,oldFont=nawa.font,oldPalette=nawa.palette;nawa.settings=()=>oldSettings()+settings();nawa.font=id=>presets[id]?.font||oldFont(id);nawa.palette=id=>presets[id]?.palette||oldPalette(id)}
function fontLoaded(){window.madarVerci?.invalidateFonts?.();window.madarLyricClock?.invalidate();if(view==='lyrics')window.madarLyricClock?.tick()}
document.fonts?.addEventListener?.('loadingdone',fontLoaded);
window.madarGameThemes={presets,fonts,apply,decorate,settings,status:()=>({theme:ui.theme,applied,art:ui.gameThemeArt,avatar:ui.gameThemeAvatar,style:ui.lyricStyle,font:ui.lyricFont,translationFont:ui.translationFont})};ready=true;apply();applyTypography();decorate();persist();
})();
