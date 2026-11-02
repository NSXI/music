const fs=require('fs'),vm=require('vm'),assert=require('assert/strict'),crypto=require('crypto');
const raster=require('@napi-rs/canvas');
for(const [f,n] of [['segoeui.ttf','Segoe UI'],['tahoma.ttf','Tahoma']])raster.GlobalFonts.registerFromPath('C:/Windows/Fonts/'+f,n);
const window={},context=vm.createContext({window,console});for(const n of ['motion-presets.js','motion-library.js','lyric-art.js'])vm.runInContext(fs.readFileSync('frontend/'+n,'utf8'),context);
const art=window.MadarLyricArt,canvas=raster.createCanvas(800,520),c=canvas.getContext('2d'),hashes=new Set();let checks=0;
(async()=>{
const sheet=raster.createCanvas(1600,1560),sc=sheet.getContext('2d');let tile=0;
for(const p of window.MadarMotionPresets){
 const mixed=art.prepare([{time:0,text:'نور music جديد'}],[{start:0,words:[{text:'نور',start:0,end:.5},{text:'music',start:1,end:1.5},{text:'جديد',start:2,end:2.5}]}]),mixedState=art.makeState();
 for(const time of [.2,.7,1.2,.2]){art.paint(c,{width:800,height:520,data:mixed,time,duration:3,style:p.id,paused:true,colors:{}},mixedState);assert.equal(mixedState.library.lang,'ar');assert.equal(mixedState.library.hot,time!==.7);assert.equal(mixedState.library.dt,10000)}
 const plain=art.prepare([{time:null,text:'نص music بلا توقيت'}]);art.paint(c,{width:800,height:520,data:plain,time:0,duration:0,style:p.id,colors:{}},art.makeState());

 for(const [lang,text,tr] of [['en','Stars guide my heart home','النجوم ترشد قلبي'],['ar','النجوم ترشد قلبي إلى البيت','Stars guide my heart']]){
  const lines=[{time:0,text},{time:5,text}],words=lines.map(l=>({start:l.time,words:l.text.split(' ').map((text,i)=>({text,start:l.time+i*.8,end:l.time+i*.8+.6}))})),data=art.prepare(lines,words),state=art.makeState();
  for(const [w,h] of [[800,520],[320,620]]){canvas.width=w;canvas.height=h;
   for(const time of [-.1,0,.25,.59,.7,.8,1.2,3.8,5.1,1.2]){
    c.resetTransform();const result=art.paint(c,{width:w,height:h,data,time,duration:10,style:p.id,colors:{text:'#f5f2ff',accent:'#bca5ff',secondary:'#aab6cf'},translations:[tr,tr],font:'Segoe UI',translationFont:'Tahoma',textScale:1},state);
    assert.equal(result.line,time>=5?1:0);assert.ok(Number.isFinite(result.progress));assert.ok(state.library.items.size<100);checks+=3;
   }
  }
  canvas.width=800;canvas.height=520;for(let j=0;j<10;j++)art.paint(c,{width:800,height:520,data,time:1.1+j*.016,duration:10,style:p.id,colors:{text:'#f5f2ff',accent:'#bca5ff',secondary:'#aab6cf'},translations:[tr,tr],font:'Segoe UI'},state);
  const png=canvas.toBuffer('image/png');if(p.mode===0)fs.writeFileSync('qa/previews/motion-'+p.group+'-'+lang+'.png',png);hashes.add(crypto.createHash('sha256').update(png).digest('hex'));
  if(lang==='en'&&[0,4,9].includes(p.mode)&&p.group!=='studio'){const x=tile%4*400,y=Math.floor(tile/4)*390;sc.fillStyle='#111522';sc.fillRect(x,y,400,390);const img=await raster.loadImage(png);sc.drawImage(img,x,y+25,400,350);sc.fillStyle='#fff';sc.font='14px Segoe UI';sc.fillText(p.group+' / '+p.mode,x+10,y+18);tile++}
  art.paint(c,{width:800,height:520,data,time:1.2,duration:10,style:p.id,reduced:true,colors:{},translations:[tr]},state);checks++;
 }
}
fs.mkdirSync('qa/previews',{recursive:true});fs.writeFileSync('qa/previews/motion-library-integrated.png',sheet.toBuffer('image/png'));
console.log(JSON.stringify({presets:window.MadarMotionPresets.length,checks,distinctImages:hashes.size,scope:'Software raster; English/Arabic, narrow/wide, real timing boundaries, gaps, seeks, translation and reduced motion. No native GPU FPS claim.'}));


})().catch(e=>{console.error(e);process.exitCode=1});
