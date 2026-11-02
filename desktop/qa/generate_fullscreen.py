from pathlib import Path
import json,shutil
src=Path(__file__).resolve().parents[1]/'frontend';out=Path(__file__).resolve().parent/'fullscreen';out.mkdir(exist_ok=True)
for asset in src.iterdir():
 if asset.suffix in {'.js','.css'}:shutil.copy2(asset,out/asset.name)
(out/'cover.svg').write_text('''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 800"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#b6d6e3"/><stop offset=".45" stop-color="#568aa4"/><stop offset="1" stop-color="#14263f"/></linearGradient><radialGradient id="h"><stop stop-color="#fff5cf"/><stop offset="1" stop-color="#d8b4bf" stop-opacity="0"/></radialGradient></defs><rect width="800" height="800" fill="url(#g)"/><circle cx="540" cy="220" r="280" fill="url(#h)"/><g fill="none" stroke="#d6e9ee" opacity=".45"><ellipse cx="380" cy="430" rx="300" ry="140" transform="rotate(-24 380 430)"/><ellipse cx="380" cy="430" rx="290" ry="110" transform="rotate(-24 380 430)"/><ellipse cx="380" cy="430" rx="275" ry="80" transform="rotate(-24 380 430)"/></g><circle cx="380" cy="430" r="60" fill="#eaf2ed" opacity=".85"/><text x="72" y="110" font-family="sans-serif" font-size="34" letter-spacing="10" fill="#edf7fa">MADAR STUDIO</text><text x="72" y="714" font-family="sans-serif" font-size="65" letter-spacing="-3" fill="#eef6f7">First light</text></svg>''',encoding='utf8')
lines=['Every note finds a place','The room becomes a quiet sky','We follow the light together','And carry this melody home']
translations=['لكل نغمة مكان','تصير الغرفة سماءً هادئة','نتبع الضوء معًا','ونحمل هذا اللحن إلى البيت']
mock='''<script>
window.fixtureLines=LINES;window.fixtureTranslations=TRANSLATIONS;
window.fixtureTask={id:'original-fullscreen-fixture',song:{id:'original-fullscreen-fixture',title:'First light',artist:'Madar Studio',album:'Design session',artwork:'http://127.0.0.1:43928/cover.svg',duration:24},status:'done',quality:{codec:'FLAC',rate:96000,bits:24},log:[]};
window.nativeCalls=[];window.go={main:{Studio:{SetLyricsFullscreen:async enabled=>{window.nativeCalls.push(enabled)},GetFonts:async()=>['Segoe UI','Tahoma','Arial'],GetState:async()=>({jobs:[window.fixtureTask],preferences:{translationTarget:'ar',translationSource:'en',autoTranslate:false},translation:{ready:true}}),GetLyrics:async()=>({text:window.fixtureLines.map((text,i)=>`[00:${String(i*6).padStart(2,'0')}.00]${text}`).join('\\n'),timed:true,status:'كلمات اختبار أصلية'}),GetTranslation:async()=>({exists:true,source:'en',target:'ar',status:'ترجمة اختبار أصلية',originalHash:'fixture',lines:window.fixtureLines.map((text,i)=>({original:text,text:window.fixtureTranslations[i],time:i*6}))})}}};
</script>'''.replace('LINES',json.dumps(lines)).replace('TRANSLATIONS',json.dumps(translations,ensure_ascii=False))
setup='''<script>
ui.theme='noir';ui.lyricStyle='focus';ui.lyricMotion='flow';ui.translationEnabled=true;ui.translationAuto=false;ui.translationTarget='ar';ui.translationSource='en';ui.motion=true;ui.fsMedium='cd';ui.fsVariants={cd:'cover',vinyl:'grooved',cassette:'classic'};ui.fsBackground='aurora';ui.fsCardSize=340;ui.fsTextSize=42;snapshot.jobs=[window.fixtureTask];current=window.fixtureTask;view='lyrics';applyAppearance();nav();render();
</script>'''
html=(src/'index.html').read_text(encoding='utf8').replace('<script src="app.js"></script>',mock+'<script src="app.js"></script>').replace('</body>',setup+'</body>')
(out/'index.html').write_text(html,encoding='utf8')
print('Full-screen fixture generated with production renderer and original cover/phrases')
