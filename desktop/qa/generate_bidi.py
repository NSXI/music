from pathlib import Path
import json,shutil
src=Path(__file__).resolve().parents[1]/'frontend';out=Path(__file__).resolve().parent/'bidi';out.mkdir(exist_ok=True)
for asset in src.iterdir():
 if asset.suffix in {'.js','.css'}:shutil.copy2(asset,out/asset.name)
samples=[('en','Music brings us together'),('ar','الموسيقى تجمعنا معًا'),('fr','La musique nous rassemble'),('es','La música nos une'),('de','Musik verbindet uns'),('ru','Музыка объединяет нас'),('ja','音楽は私たちをつなぐ'),('ko','음악은 우리를 하나로 모읍니다'),('zh','音乐让我们走到一起'),('hi','संगीत हमें एक साथ लाता है'),('tr','Müzik bizi bir araya getirir'),('pt','A música nos une'),('it','La musica ci unisce'),('fa','موسیقی ما را به هم پیوند می‌دهد'),('ur','موسیقی ہمیں اکٹھا کرتی ہے'),('id','Musik menyatukan kita'),('nl','Muziek brengt ons samen'),('uk','Музика об’єднує нас'),('mixed-en','Music موسيقى 2026 (Live)'),('mixed-ar','موسيقى Music 2026 (مباشر)'),('emoji','✨ Music 🎵 مرحبًا'),('marks','اَلْمُوسِيقَى تُجَمِّعُنَا')]
mock='''<script>
window.qaSamples=SAMPLES;
window.qaTask={id:'own-language-fixture',song:{id:'own-language-fixture',title:'فحص اتجاه اللغات',artist:'Madar QA',duration:60},status:'done',quality:{codec:'FLAC'},log:[]};
window.go={main:{Studio:{GetState:async()=>({jobs:[window.qaTask],preferences:{translationTarget:'ar',translationSource:'en',autoTranslate:false},translation:{ready:true}}),GetLyrics:async()=>({text:window.qaSamples.map(([lang,text],i)=>`[00:${String(i+1).padStart(2,'0')}.00]${text}`).join('\\n'),timed:true,status:'نص اختبار أصلي'}),GetTranslation:async()=>({exists:true,originalHash:'fixture',source:'en',target:'ar',status:'فحص محلي',lines:window.qaSamples.map(([lang,text],i)=>({original:text,text:'الموسيقى تجمعنا معًا',time:i+1}))})}}};
</script>'''.replace('SAMPLES',json.dumps(samples,ensure_ascii=False))
test='''<script>
ui.theme='noir';ui.lyricStyle='focus';ui.lyricMotion='letters';ui.translationAuto=false;ui.translationTarget='ar';ui.translationSource='en';ui.motion=true;current=window.qaTask;snapshot.jobs=[current];view='lyrics';nav();applyAppearance();render();
async function inspectBidi(){await loadLyrics(false);const results=[];const check=(name,pass,detail='')=>{results.push({name,pass,detail});if(!pass)console.error(name,detail)};
const buttons=[...document.querySelectorAll('.lyric-line')];check('22 language and mixed text rows',buttons.length===qaSamples.length);
for(let i=0;i<buttons.length;i++){const content=buttons[i].querySelector(':scope > .lyric-content');check('logical text '+qaSamples[i][0],content.textContent===qaSamples[i][1]);check('direction '+qaSamples[i][0],getComputedStyle(content).direction===textDirection(qaSamples[i][1]));check('translated characters '+qaSamples[i][0],buttons[i].querySelectorAll('.lyric-translation .lyric-char').length>0)}
const probe=document.createElement('button');probe.className='lyric-line current';probe.style.cssText='position:fixed;top:0;left:0;width:max-content;white-space:nowrap;opacity:1;animation:none;transform:none;z-index:-1';document.body.append(probe);
for(const text of ['Music','Madar','Français','Русский','日本語','한국어','中文','العربية','فارسی','اردو','संगीत']){probe.innerHTML=renderDirectionalText(text);const w=probe.querySelector('.lyric-word');const chars=[...w.querySelectorAll('.lyric-char')];const xs=chars.map(x=>x.getBoundingClientRect().x);const rtl=textDirection(text)==='rtl';check('visual order '+text,xs.every((x,i)=>i===0||(rtl?x<=xs[i-1]+.5:x>=xs[i-1]-.5)),JSON.stringify(xs));if(joiningScript(text)){const plain=document.createElement('span');plain.textContent=text;plain.dir=textDirection(text);plain.style.unicodeBidi='isolate';probe.append(plain);check('joined width '+text,Math.abs(w.getBoundingClientRect().width-plain.getBoundingClientRect().width)<1,`${w.getBoundingClientRect().width}/${plain.getBoundingClientRect().width}`)}}
probe.remove();const stage=document.querySelector('#lyric-stage');for(const [style] of lyricStyles){stage.dataset.style=style;for(const [motion] of lyricMotions){stage.dataset.motion=motion;const b=buttons[0];b.classList.add('current');check('paired motion '+style+'/'+motion,!!b.querySelector('.lyric-translation .lyric-content')&&getComputedStyle(b.querySelector('.lyric-translation')).display==='block')}}
stage.dataset.style='focus';stage.dataset.motion='letters';buttons.forEach((x,i)=>x.classList.toggle('current',i<4));
const failed=results.filter(x=>!x.pass);const report=document.createElement('div');report.id='bidi-report';report.setAttribute('role','status');report.style.cssText='position:fixed;bottom:0;left:0;right:0;padding:8px;background:#073b23;color:white;z-index:9999;font-size:14px';report.textContent=`BIDI QA: ${results.length-failed.length}/${results.length} PASS; ${failed.length} FAIL`+(failed.length?' '+JSON.stringify(failed):'');document.body.append(report);window.qaResults=results;
}
inspectBidi().catch(e=>{document.body.insertAdjacentHTML('beforeend','<pre id="bidi-report">QA ERROR '+esc(e.stack)+'</pre>')});
</script>'''
html=(src/'index.html').read_text(encoding='utf8').replace('<script src="app.js"></script>',mock+'<script src="app.js"></script>').replace('</body>',test+'</body>')
assert 'window.qaSamples=' in html
(out/'index.html').write_text(html,encoding='utf8')
print('Generated language layout fixture using production CSS and renderer')
