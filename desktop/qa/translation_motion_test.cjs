const fs=require('fs'),vm=require('vm'),assert=require('assert/strict'),r=require('@napi-rs/canvas');
const window={},ctx=vm.createContext({window,Intl,console});for(const f of ['lyric-sequence.js','motion-presets.js','motion-library.js','lyric-art.js'])vm.runInContext(fs.readFileSync('frontend/'+f,'utf8'),ctx);
const art=window.MadarLyricArt,lines=[{text:'FIRST LATER LAST',time:0},{text:'NEXT OTHER FINAL',time:6}],aligned=lines.map(l=>({start:l.time,text:l.text,words:l.text.split(' ').map((text,i)=>({text,start:l.time+i*2,end:l.time+i*2+.8}))})),data=art.prepare(lines,aligned),before=JSON.stringify(data);let checks=0;
for(const style of [...art.styles,...window.MadarMotionPresets.map(x=>x.id)]){
 const canvas=r.createCanvas(800,560),c=canvas.getContext('2d'),fill=c.fillText.bind(c),state=art.makeState();let drawn=[];c.fillText=(text,...args)=>{drawn.push(String(text));fill(text,...args)};
 const base={width:800,height:560,data,duration:12,style,translations:['UNO DOS TRES','CUATRO CINCO SEIS'],font:'Segoe UI',translationFont:'Segoe UI',colors:{},revealMode:'word'};
 for(const time of [-.1,.2,1.2,2.2,4.2,.2]){drawn=[];art.paint(c,{...base,time},state);const banned=time<0?['UNO','DOS','TRES']:time<2?['DOS','TRES']:time<4?['TRES']:[];for(const value of drawn)for(const future of banned)assert.ok(!value.includes(future),`${style} translated ${future} too soon at ${time}`);assert.equal(state.translationVisual.kind,'visual-within-original');checks++}
 const translated=state.translationVisual;
 assert.ok(translated.state.frames>0,style+' did not use the selected renderer for translation');checks++;
 art.paint(c,{...base,time:.3,revealMode:'word'},state);const word=canvas.toBuffer('image/png');art.paint(c,{...base,time:.3,revealMode:'character'},state);assert.ok(!word.equals(canvas.toBuffer('image/png')),style+' ignores translated character mode');checks++;
 art.paint(c,{...base,time:1.2},state);const gap=state.translationVisual.progress;art.paint(c,{...base,time:1.8},state);assert.equal(state.translationVisual.progress,gap,style+' translated motion advanced through a source gap');checks++;
}
assert.equal(JSON.stringify(data),before,'translation changed source timing');console.log('PASS:',checks,'translation checks across89 layouts: same renderer, future-word gates, source gaps, backward seeks, character mode and unchanged source timing.');
