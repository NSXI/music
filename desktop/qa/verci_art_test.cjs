require('node:fs').mkdirSync('qa/results',{recursive:true});
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),crypto=require('node:crypto');
const {performance}=require('node:perf_hooks');
const {createCanvas,GlobalFonts}=require('@napi-rs/canvas');
for(const [file,family] of [['segoeui.ttf','Segoe UI'],['tahoma.ttf','Tahoma'],['consola.ttf','Cascadia Code']])assert.ok(GlobalFonts.registerFromPath('C:/Windows/Fonts/'+file,family));
const art=require('../frontend/lyric-art.js');
const out='qa/previews';fs.mkdirSync(out,{recursive:true});
const lines=['Stars guide my heart','The moon lights home','Our music finds hope','We dream together again'].map((text,i)=>({time:i*4,text}));
const aligned=lines.map(line=>({start:line.time,words:line.text.split(' ').map((text,i)=>({text,start:line.time+i*.9,end:line.time+i*.9+.8}))}));
const translations=['النجوم ترشد قلبي','القمر ينير البيت','موسيقانا تجد الأمل','نحلم معًا من جديد'];
const data=art.prepare(lines,aligned);assert.equal(data.kind,'word');assert.equal(data.units.length,16);
assert.deepEqual(data.units.map(w=>[w.start,w.end]),aligned.flatMap(r=>r.words.map(w=>[w.start,w.end])));
const fallback=art.prepare(lines,[]);assert.equal(fallback.kind,'line');assert.equal(fallback.units.length,4);
assert.equal(art.prepare([{text:'كلمات أصلية بلا توقيت',time:null}]).units[0].start,null);
assert.equal(art.prepare(lines,[{...aligned[0],words:[{text:'wrong text',start:0,end:1}]}]).kind,'line');
assert.equal(art.iconFor('أَحِبُّك'),'♥');assert.equal(art.direction('كلمات عربية'),'rtl');assert.equal(art.direction('Music 2026'),'ltr');
const timings={},hashes=new Set(),sheet=createCanvas(1800,1260),sheetCtx=sheet.getContext('2d');
const canvas=createCanvas(960,640),ctx=canvas.getContext('2d');
function background(c,W,H){const g=c.createLinearGradient(0,0,W,H);g.addColorStop(0,'#141d31');g.addColorStop(1,'#090e19');c.fillStyle=g;c.fillRect(0,0,W,H)}
for(let i=0;i<art.styles.length;i++){
 const style=art.styles[i],state=art.makeState(),frame={width:960,height:640,data,time:5.1,style,colors:{text:'#f5f2ff',accent:'#bca5ff',secondary:'#b3bdd4'},font:'Segoe UI',translationFont:'Tahoma',translations,motion:'flow',textScale:.9};
 const image=createCanvas(960,640),imageCtx=image.getContext('2d');
 // The production layer is transparent over the app theme; add a backdrop here.
 art.paint(ctx,frame,state);background(imageCtx,960,640);imageCtx.drawImage(canvas,0,0);
 const png=image.toBuffer('image/png');hashes.add(crypto.createHash('sha256').update(png).digest('hex'));fs.writeFileSync(path.join(out,'verci-'+style+'.png'),png);
 background(sheetCtx,1800,1260); // reset below only on the first tile
}
// Compose after all PNGs were saved, retaining each independent raster result.
const {loadImage}=require('@napi-rs/canvas');
(async()=>{
 background(sheetCtx,1800,1260);
 for(let i=0;i<art.styles.length;i++){const x=(i%3)*600,y=Math.floor(i/3)*420;sheetCtx.drawImage(await loadImage(path.join(out,'verci-'+art.styles[i]+'.png')),x,y+28,600,392);sheetCtx.font='16px Segoe UI';sheetCtx.fillStyle='#d5ddf0';sheetCtx.fillText(art.styles[i],x+20,y+23)}
 fs.writeFileSync(path.join(out,'verci-nine-designs.png'),sheet.toBuffer('image/png'));
 assert.equal(hashes.size,9,'Each design must produce a distinct image');
 for(const style of art.styles){
  const state=art.makeState(),samples=[];
  for(let n=0;n<130;n++){const start=performance.now();const r=art.paint(ctx,{width:960,height:640,data,time:4+n/60,style,translations,font:'Segoe UI',translationFont:'Tahoma',motion:'flow'},state);if(n>=10)samples.push(performance.now()-start);assert.ok(r.hits.length<=15);assert.ok(state.positions.size<=40)}
  samples.sort((a,b)=>a-b);timings[style]={medianMs:+samples[60].toFixed(3),p95Ms:+samples[114].toFixed(3)};
 }
 const huge=Array.from({length:10000},(_,i)=>({text:'Original line '+i,time:i*4}));let start=performance.now();const hugeData=art.prepare(huge);const prepareMs=performance.now()-start;
 start=performance.now();for(let i=0;i<100000;i++)assert.equal(art.upperBound(hugeData.units,i%40000),Math.floor((i%40000)/4));const lookupMs=performance.now()-start;
 const arabic=art.prepare([{time:0,text:'الموسيقى تجمعنا معًا'},{time:4,text:'نكتب ضوءًا جديدًا'}]);
 for(const style of art.styles)assert.doesNotThrow(()=>art.paint(ctx,{width:960,height:640,data:arabic,time:1,style,translations:['Music brings us together','We write a new light'],font:'Tahoma',translationFont:'Segoe UI'},art.makeState()));
 const report={designs:9,distinctImages:hashes.size,frameSize:'960x640',samplesPerStyle:120,timings,prepare10000LinesMs:+prepareMs.toFixed(3),lookup100000TimesMs:+lookupMs.toFixed(3),scope:'CPU/software raster and functional checks; not measured WebView2 GPU FPS. Original test text only.'};
 fs.writeFileSync('qa/results/verci-art-performance.json',JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2));
})().catch(error=>{console.error(error);process.exitCode=1});
