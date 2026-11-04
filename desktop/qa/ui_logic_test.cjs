require('node:fs').mkdirSync('qa/results',{recursive:true});
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
class El{constructor(){this.innerHTML='';this.textContent='';this.value='0';this.style={setProperty(){},removeProperty(){}};this.dataset={};this.classList={toggle(){},add(){},remove(){}};this.scrollTop=0;this.clientHeight=500;this.duration=24;this.currentTime=4;this.paused=true;this.loop=false;this.open=false}getContext(){return {clearRect(){},fillRect(){}}}setAttribute(){}addEventListener(){}scrollTo(){}focus(){}showModal(){this.open=true}close(){this.open=false}play(){this.paused=false;return Promise.resolve()}pause(){this.paused=true}}
const els=new Map(),get=s=>{if(!els.has(s))els.set(s,new El);return els.get(s)};
const documentEvents=[];
const document={querySelector:get,querySelectorAll:()=>[],documentElement:get('root'),body:get('body'),addEventListener(type,handler,capture){documentEvents.push({type,handler,capture})}};
let state={jobs:[],preferences:{folder:'C:/Music',lyricProviders:['lrclib','qq','netease','kugou','musixmatch'],autoAlign:true},alignment:{ready:true}};
const ctx={document,window:{go:{main:{Studio:{GetState:async()=>state,GetLyrics:async()=>({text:'[00:00.00]Test one\n[00:03.00]Test two',timed:true,status:'offline'})}}},matchMedia:()=>({matches:false})},localStorage:{getItem(){return null},setItem(){}},setInterval(){},setTimeout(){return 1},clearTimeout(){},requestAnimationFrame(){return 1},cancelAnimationFrame(){},CSS:{escape:x=>x},URL,Intl,console,Uint8Array,Math,Date};
vm.createContext(ctx);vm.runInContext(fs.readFileSync('frontend/app.js','utf8'),ctx);vm.runInContext(fs.readFileSync('frontend/lyrics-workflow.js','utf8'),ctx);
function run(s){return vm.runInContext(s,ctx)}
const song={id:'fixture',title:'<img src=x onerror=alert(1)>',artist:'Madar Tests',album:'QA',duration:24,provider:'archive',url:'https://archive.org/details/test'};
const task={id:'12345678901234567890',song,status:'done',quality:{codec:'FLAC',lossless:true,bits:24,rate:96000},source:'archive',log:[],path:'C:/Music/test.flac',bytes:12345,total:12345};
run(`snapshot=${JSON.stringify({...state,jobs:[task]})};current=${JSON.stringify(task)};searchResult={title:'QA',query:'QA',tracks:[${JSON.stringify(song)}],complete:false,providerCounts:{archive:1},providerErrors:{tidal:'needs setup'}};selected=new Set(['fixture']);`);
assert.equal(run('themes.length'),9);assert.equal(run('lyricStyles.length'),27);assert.equal(run('lyricMotions.length'),15);assert.equal(run('lyricSources.length'),16);
for(const view of ['home','search','library','albums','downloads','player','lyrics','appearance','settings']){run(`view='${view}';render()`);const out=get('#main').innerHTML;assert.ok(out.includes('<h1>'),view);assert.ok(!out.includes('<img src=x onerror'),`escaped metadata in ${view}`);assert.ok(!out.includes('undefined'),`undefined template in ${view}`)}
for(const theme of ['glass','orchard','pulse','cloud','prism','noir'])run(`ui.theme='${theme}';applyAppearance()`);
const lyrics=run(`parseLyrics(${JSON.stringify('[offset:250]\n[00:01.00][00:05.50]One line\n[00:10.00]Two')},true)`);assert.equal(lyrics.length,3);assert.equal(lyrics[0].time,1.25);assert.equal(lyrics[1].time,5.75);
assert.ok(run(`renderLyricText({text:'كلمات عربية متصلة',time:0},0)`).includes('class="lyric-content arabic"'));
assert.ok(run(`renderLyricText({text:'Hello World',time:0},0)`).includes('lyric-char'));
assert.equal(run('translationLanguages.length'),18);
run(`ui.translationEnabled=true;translationData={exists:true,lines:[{original:'Original',text:'<img src=x onerror=alert(1)>'}]};`);
assert.ok(run(`translationMarkup({text:'Original'},0)`).includes('&lt;img'));
assert.equal(run(`translationMarkup({text:'Changed'},0)`),'');
run('ui.translationEnabled=false');assert.equal(run(`translationMarkup({text:'Original'},0)`),'');
console.log('PASS: all views render; 9 themes; 27 base styles + 15 motions; escaped metadata; LRC multiple tags and offsets; Arabic and character markup.');

run("current={id:'stream-one',preview:true};ui.shuffle=false;playOrder=['saved-one','saved-two'];searchResult={tracks:[{id:'stream-one',provider:'ytmusic'}]};filter='all'");assert.equal(run('shouldAdvanceAfterEnd()'),false,'one preview must stop at end even after library playback');
run("searchResult.tracks.push({id:'stream-two',provider:'audius'})");assert.equal(run('shouldAdvanceAfterEnd()'),true,'advance to next streaming result');
run("current.id='stream-two'");assert.equal(run('shouldAdvanceAfterEnd()'),false,'last preview must stop');
run("current.id='missing'");assert.equal(run('shouldAdvanceAfterEnd()'),false,'unknown current result must not wrap');
console.log('PASS: preview end does not repeat or use the saved-library queue.');

(async()=>{
 await new Promise(resolve=>setImmediate(resolve));
 const nativeCalls=[];ctx.window.go.main.Studio.SetLyricsFullscreen=async enabled=>{nativeCalls.push(enabled)};
 state={...state,jobs:[task]};run(`snapshot=${JSON.stringify(state)};current=${JSON.stringify(task)};view='lyrics';ui.translationEnabled=true;`);
 vm.runInContext(fs.readFileSync('frontend/fullscreen.js','utf8'),ctx);
 await run('loadLyrics(false)');
 const originalAudioSource=get('#audio').src,originalVolume=run('ui.volume');
 await run('window.madarFullscreen.enter()');
 assert.equal(run('window.madarFullscreen.active'),true);assert.ok(get('#main').innerHTML.includes('fs-scene'));
 assert.equal(get('#audio').src,originalAudioSource,'opening full-screen changed the audio source');assert.equal(run('ui.volume'),originalVolume);
 assert.ok(!get('#main').innerHTML.includes('<img src=x onerror'),'metadata escaped in full-screen');
 for(const [medium,finishes] of Object.entries({cd:['cover','silver','prism'],vinyl:['grooved','picture','smoke'],cassette:['classic','clear','neon'],square:['cover']})){
  for(const finish of finishes){run(`ui.fsMedium='${medium}';ui.fsVariants['${medium}']='${finish}';window.madarFullscreen.applyPreferences()`);const markup=get('#fs-media-art').innerHTML;assert.ok(markup.includes(medium==='cassette'?'fs-cassette':medium==='square'?'fs-square':'fs-disc-'+medium),medium+'/'+finish);assert.ok(!markup.includes('undefined'))}
 }
 assert.equal(run('window.madarFullscreen.progressStyles.length'),6);
 assert.equal(run('window.madarFullscreen.backgrounds.length'),6);
 const capture=documentEvents.findLast(x=>x.type==='keydown'&&x.capture===true).handler;
 get('#modal').open=true;let prevented=0;capture({key:'Escape',preventDefault(){prevented++},stopImmediatePropagation(){}});
 assert.equal(get('#modal').open,false);assert.equal(run('window.madarFullscreen.active'),true,'Escape closed full-screen before settings');assert.equal(nativeCalls.length,1);
 capture({key:'Escape',preventDefault(){prevented++},stopImmediatePropagation(){}});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(run('window.madarFullscreen.active'),false);assert.deepEqual(nativeCalls,[true,false]);assert.equal(prevented,2);assert.ok(get('#main').innerHTML.includes('الكلمات، بإيقاعك.'));
 assert.equal(get('#audio').src,originalAudioSource,'leaving full-screen changed the audio source');assert.equal(run('ui.volume'),originalVolume);
 console.log('PASS: 10 full-screen media designs; 6 backgrounds; 6 progress styles; Esc closes settings before native full-screen; playback and volume preserved.');
})().catch(error=>{console.error(error);process.exitCode=1});
