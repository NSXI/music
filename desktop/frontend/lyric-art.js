'use strict';
// Independent Canvas designs informed by the supplied Verci Windows layouts.
// No provider timestamps are generated or interpolated here.
((root)=>{
const styles=['ship','fisheye','fisheye-visual','visual','code','code-visual','branches','web','constellation'];
const finite=value=>Number.isFinite(value);
const clamp=(x,a=0,b=1)=>Math.max(a,Math.min(b,x));
const direction=text=>{for(const c of String(text))if(/\p{L}/u.test(c))return /[\u0590-\u08ff\ufb1d-\ufeff]/u.test(c)?'rtl':'ltr';return 'ltr'};
const normal=text=>String(text).replace(/\s+/g,'').normalize('NFC');
function upperBound(items,time,key='start'){let lo=0,hi=items.length;while(lo<hi){const mid=(lo+hi)>>>1;if(items[mid][key]<=time)lo=mid+1;else hi=mid}return lo-1}
function prepare(lines,aligned=[]){
 const rows=aligned.filter(x=>finite(x.start)).slice().sort((a,b)=>a.start-b.start),units=[],ends=[],speech=[];
 let wordCount=0,lineCount=0,nextGroup=null,nextGreater=null;
 for(let i=lines.length-1;i>=0;i--){const t=lines[i].time;if(finite(t)){if(nextGroup!==null&&t<nextGroup)nextGreater=nextGroup;nextGroup=t;ends[i]=nextGreater}else ends[i]=nextGroup}
 for(let i=0;i<lines.length;i++){
  const line=lines[i],start=finite(line.time)?line.time:null,end=ends[i];
  let row;if(start!==null){const at=upperBound(rows,start+.249);if(at>=0&&Math.abs(rows[at].start-start)<.25)row=rows[at]}
  const words=row?.words?.filter(w=>String(w.text||'').trim())||[];
  const valid=words.length>0&&normal(words.map(w=>w.text).join(''))===normal(line.text)&&words.every((w,n)=>finite(w.start)&&finite(w.end)&&w.end>w.start&&w.start>=start-.25&&(!n||w.start>=words[n-1].start));
  if(valid){speech.push({start:words[0].start,end:words.reduce((last,w)=>Math.max(last,w.end),words[0].end)});for(const w of words){units.push({text:String(w.text).trim(),start:w.start,end:w.end,line:i,kind:'word'});wordCount++}}
  else{speech.push({start,end});units.push({text:String(line.text),start,end,line:i,kind:start===null?'plain':'line'});lineCount++}
 }
 // Sorting retains original ordinal as a tie breaker and does not alter text.
 const timed=units.every(x=>x.start!==null);if(timed)units.sort((a,b)=>a.start-b.start);
 return {units,lines,ends,speech,timed,kind:wordCount?(lineCount?'mixed':'word'):(timed?'line':'plain')};
}
const icons=new Map(),cleanWord=text=>String(text).toLowerCase().normalize('NFD').replace(/[\p{M}\p{P}\p{S}\u0640]/gu,'').replace(/[أإآ]/g,'ا').trim();
for(const [symbol,words] of [
 ['♥','love heart lover beloved حب أحب أحبك حبي قلبي قلب'],['☀','sun light bright morning shine شمس نور ضوء صباح'],['☾','moon night midnight قمر ليل ليلة'],['✦','star stars dream hope نجم نجوم حلم أحلام أمل'],['♫','music song sing melody rhythm موسيقى أغنية غناء لحن إيقاع'],['☁','cloud sky heaven غيم غيمة سحاب سماء'],['☂','rain storm thunder مطر عاصفة رعد'],['❀','flower rose garden leaf ورد وردة زهر حديقة ورق'],['≈','ocean sea wave river water بحر موج نهر ماء'],['⌂','home house room city بيت منزل غرفة مدينة'],['◷','time wait moment forever وقت انتظر لحظة دائما للأبد'],['◎','eye eyes see look عين عيون انظر أرى'],['✧','magic wonder سحر عجب'],['⚓','ship boat sail سفينة قارب شراع']
])for(const word of words.split(' '))icons.set(cleanWord(word),symbol);
function iconFor(text){return icons.get(cleanWord(text))||''}
function makeState(){return {measure:new Map(),wrap:new Map(),positions:new Map(),lastStyle:'',lastTime:null,previousIndex:-1,frames:0}}
function measure(ctx,text,font,state){const key=font+'\0'+text;if(state.measure.has(key))return state.measure.get(key);ctx.font=font;const value=ctx.measureText(text).width;if(state.measure.size>=2048)state.measure.clear();state.measure.set(key,value);return value}
const family=value=>'"'+String(value||'Segoe UI').replace(/["\\\r\n]/g,'')+'", "Segoe UI", Tahoma, sans-serif';
function text(ctx,value,x,y,size,color,options,state){
 if(!value||options.alpha===0)return;
 const face=family(options.font),weight=options.weight||700,max=options.max||1e6;
 let font=weight+' '+size+'px '+face,width=measure(ctx,value,font,state);
 if(width>max){size*=max/width;font=weight+' '+size+'px '+face;width=max}
 ctx.save();ctx.translate(x,y);ctx.rotate(options.angle||0);ctx.scale(options.scaleX||1,options.scaleY||1);ctx.globalAlpha=options.alpha??1;ctx.textAlign='center';ctx.textBaseline='middle';ctx.direction=direction(value);ctx.font=font;
 if(options.highlight){const opacity=ctx.globalAlpha;ctx.fillStyle=options.highlight;ctx.globalAlpha=opacity*.2;ctx.beginPath();ctx.roundRect(-width/2-size*.19,-size*.64,width+size*.38,size*1.28,size*.17);ctx.fill();ctx.globalAlpha=opacity*.5;ctx.strokeStyle=options.highlight;ctx.lineWidth=1;ctx.stroke();ctx.globalAlpha=opacity}
 if(options.reveal!==undefined&&options.reveal<1){ctx.beginPath();const span=(width+size*.5)*clamp(options.reveal);ctx.rect(ctx.direction==='rtl'?width/2+size*.25-span:-width/2-size*.25,-size,span,size*2);ctx.clip()}
 ctx.fillStyle=color;ctx.fillText(value,0,0);ctx.restore();
 return {x,y,width,height:size*1.5,size};
}
function symbol(ctx,kind,x,y,size,color,alpha){
 ctx.save();ctx.translate(x,y);ctx.scale(size/24,size/24);ctx.strokeStyle=color;ctx.fillStyle=color;ctx.lineWidth=1.6;ctx.lineCap='round';ctx.lineJoin='round';ctx.globalAlpha=alpha;ctx.beginPath();
 if(kind==='♥'){ctx.moveTo(0,9);ctx.bezierCurveTo(-17,-2,-8,-14,0,-6);ctx.bezierCurveTo(8,-14,17,-2,0,9);ctx.fill()}
 else if(kind==='☾'){ctx.moveTo(6,-10);ctx.bezierCurveTo(-15,-9,-13,13,6,10);ctx.bezierCurveTo(-5,7,-6,-6,6,-10);ctx.fill()}
 else if(kind==='♫'){ctx.moveTo(-3,6);ctx.lineTo(-3,-8);ctx.lineTo(8,-11);ctx.lineTo(8,3);ctx.stroke();ctx.beginPath();ctx.ellipse(-6,7,4,2.5,-.3,0,Math.PI*2);ctx.ellipse(5,4,4,2.5,-.3,0,Math.PI*2);ctx.fill()}
 else if(kind==='⌂'){ctx.moveTo(-11,0);ctx.lineTo(0,-10);ctx.lineTo(11,0);ctx.moveTo(-8,-2);ctx.lineTo(-8,10);ctx.lineTo(8,10);ctx.lineTo(8,-2);ctx.moveTo(-2,10);ctx.lineTo(-2,3);ctx.lineTo(3,3);ctx.lineTo(3,10);ctx.stroke()}
 else if(kind==='≈'){for(let n=-1;n<=1;n++){ctx.moveTo(-11,n*6);ctx.bezierCurveTo(-4,n*6-7,4,n*6+7,11,n*6)}ctx.stroke()}
 else if(kind==='☀'||kind==='◷'||kind==='◎'){ctx.arc(0,0,kind==='◎'?5:7,0,Math.PI*2);ctx.stroke();if(kind==='☀'){ctx.beginPath();for(let n=0;n<8;n++){const a=n*Math.PI/4;ctx.moveTo(Math.cos(a)*10,Math.sin(a)*10);ctx.lineTo(Math.cos(a)*12,Math.sin(a)*12)}ctx.stroke()}else{ctx.beginPath();ctx.moveTo(0,-4);ctx.lineTo(0,0);ctx.lineTo(4,3);ctx.stroke()}}
 else if(kind==='☁'||kind==='☂'){ctx.moveTo(-9,4);ctx.bezierCurveTo(-17,3,-14,-5,-7,-4);ctx.bezierCurveTo(-8,-14,9,-14,9,-4);ctx.bezierCurveTo(17,-4,16,5,9,5);ctx.lineTo(-9,5);ctx.stroke();if(kind==='☂'){ctx.beginPath();for(let n=-1;n<=1;n++){ctx.moveTo(n*7,8);ctx.lineTo(n*7-2,12)}ctx.stroke()}}
 else {const arms=kind==='❀'?6:4;for(let n=0;n<arms*2;n++){const a=n*Math.PI/arms-Math.PI/2,r=n%2?3:11;n?ctx.lineTo(Math.cos(a)*r,Math.sin(a)*r):ctx.moveTo(Math.cos(a)*r,Math.sin(a)*r)}ctx.closePath();ctx.stroke()}
 ctx.restore();
}
function motion(kind,phase,reduced){
 if(reduced)return {};
 const p=clamp(phase/.5),ease=1-Math.pow(1-p,3);
 switch(kind){
 case 'rise':return {dy:(1-ease)*20,alpha:.45+.55*ease};
 case 'fade':return {alpha:.35+.65*ease};
 case 'letters':case 'typewriter':case 'cascade':return {reveal:.3+.7*ease};
 case 'words':return {scale:.95+.05*ease};
 case 'wave':case 'float':return {dy:Math.sin(phase*2)*3};
 case 'focus':return {scale:.97+.03*ease};
 case 'slide':return {dx:(1-ease)*20};
 case 'breathe':return {scale:1+Math.sin(phase*1.8)*.012};
 case 'spring':return {dy:Math.sin(phase*9)*Math.exp(-phase*6)*7};
 default:return {};
 }
}
const hash=n=>{const v=Math.sin(n*78.233+31.717)*43758.5453;return v-Math.floor(v)};
// Render translations with the selected geometry in their own lower viewport.
// The normalized timeline is an ephemeral visual mapping of original speech,
// never a claim of measured translated-word timing or a provider cache write.
function paintTranslation(ctx,frame,owner,line,rect){
 const value=frame.translations?.[line];if(!value)return;
 let cache=owner.translationVisual;
 if(!cache||cache.text!==value||cache.line!==line||cache.source!==frame.data){
  const parts=root.NawaTiming?.tokens(value)||(String(value).match(/\S+/gu)||[]);
  const words=parts.map((text,i)=>({text,start:i/parts.length,end:(i+1)/parts.length})),plain=frame.data.speech[line]?.start===null;
  const projected=plain?prepare([{text:value,time:null}]):prepare([{text:value,time:0}],[{text:value,start:0,end:1,words}]);
  cache=owner.translationVisual={text:value,line,source:frame.data,projected,state:makeState(),units:frame.data.units.filter(u=>u.line===line),kind:'visual-within-original'};
 }
 const source=cache.units,at=upperBound(source,frame.time),word=source[Math.max(0,at)],speech=frame.data.speech[line];let phase;
 if(source.length&&source.every(u=>u.kind==='word')){const gap=at>=0&&at<source.length-1&&frame.time>=word.end&&frame.time<source[at+1].start;phase=at<0?-1e-6:clamp((at+clamp((frame.time-word.start)/Math.max(.02,word.end-word.start)))/source.length-(gap?1e-9:0))}
 else {const end=finite(speech?.end)?speech.end:frame.duration;phase=speech?.start===null?0:frame.time<speech.start?-1e-6:clamp((frame.time-speech.start)/Math.max(.02,end-speech.start))}
 cache.progress=phase;
 ctx.save();ctx.translate(rect.x||0,rect.y||0);ctx.beginPath();ctx.rect(0,0,rect.width,rect.height);ctx.clip();
 const result=paint(ctx,{...frame,width:rect.width,height:rect.height,data:cache.projected,time:phase,duration:1,manualIndex:0,translations:undefined,font:frame.translationFont||frame.font,textScale:(frame.textScale||1)*.88},cache.state);
 ctx.restore();return result;
}
function paint(ctx,frame,state=makeState()){
 if(frame.style?.startsWith("motion-"))return root.MadarMotionLibrary.paint(ctx,frame,state);
 const {width:W,height:H,data,time=0,style='ship'}=frame;
 if(W<=0||H<=0)return {index:-1,hits:[]};
 const {units,lines,ends}=data,colors=frame.colors||{},C={text:colors.text||'#f4f3ff',accent:colors.accent||'#bca5ff',secondary:colors.secondary||'#aeb8ce'};
 const reduced=Boolean(frame.reduced),tempo=reduced?0:time,font=frame.font||'Segoe UI',translationFont=frame.translationFont||font;
 const index=data.timed?upperBound(units,time):Math.max(0,Math.min(units.length-1,frame.manualIndex||0)),at=Math.max(0,index),active=units[at],hits=[];
 const changed=state.lastStyle!==style||state.lastTime===null||Math.abs(time-state.lastTime)>1.25;
 if(changed)state.positions.clear();state.lastStyle=style;
 const dt=state.lastTime===null?.016:Math.min(.05,Math.abs(time-state.lastTime));state.lastTime=time;state.frames++;
 ctx.clearRect(0,0,W,H);if(!active)return {index:-1,hits};
 const line=lines[active.line],translated=frame.translations?.[active.line]||'',footer=translated?Math.min(190,H*.3):28,area=H-footer;
 const unitEnd=active.end??(finite(frame.duration)?frame.duration:null),isActive=index>=0&&(unitEnd===null||time<unitEnd),phase=active.start===null?0:Math.max(0,time-active.start),progress=active.start!==null&&unitEnd!==null?clamp((time-active.start)/Math.max(.02,unitEnd-active.start)):0;
 const animation=index<0?{}:motion(frame.motion||'flow',phase,reduced);
 if(!reduced&&active.start!==null&&['letters','typewriter','words','cascade'].includes(frame.motion)){animation.reveal=progress}
 if(active.start===null)delete animation.reveal;
 const currentSize=Math.min(W*.15,area*.2)*(frame.textScale||1),visual=style.includes('visual')||style==='visual';
 function label(unit,i,x,y,size,opacity,extra={}){
  if(frame.revealMode!=='line'&&unit.start!==null&&unit.start>time)return;const value=unit.text,glyph=visual||style==='branches'?iconFor(value):'';
  const current=i===at&&index>=0,shape=current?{...animation}:{};if(current&&frame.revealMode==='character')shape.reveal=progress;const box=text(ctx,value,x+(shape.dx||0),y+(shape.dy||0),size,C.text,{font,max:W*.86,alpha:opacity*(shape.alpha??1)*(index<0?.35:1),scaleX:shape.scale||1,scaleY:shape.scale||1,reveal:shape.reveal,...extra,highlight:current&&isActive?C.accent:undefined},state);
  if(box){if(glyph)symbol(ctx,glyph,box.x+(direction(value)==='rtl'?-1:1)*(box.width/2+box.size*.65),box.y,box.size*.57,current?C.accent:C.secondary,opacity);hits.push({...box,...(style==='ship'?{x:box.x-.13*box.y+W*.06,y:box.y+.015*box.x}:{}),index:i})}
 }
 if(['branches','web','constellation'].includes(style)){
  const visible=[];const count=style==='branches'?13:15;
  for(let n=-Math.floor(count/2);n<=Math.floor(count/2);n++){
   const i=at+n;if(i<0||i>=units.length)continue;
   const current=i===at,slot=n<0?n+7:n+6,angle=slot*Math.PI*2/14-.8,radius=slot%2?.82:1;
   let x=current?W*.5:W*.5+Math.cos(angle)*W*.4*radius,y=current?area*.5:area*.5+Math.sin(angle)*area*.38*radius;
   if(style==='branches'&&!current){const arm=((i%3)+3)%3,depth=Math.ceil(Math.abs(n)/3);x=W*.5+(arm-1)*W*.27+(n<0?-1:1)*W*.035*depth;y=area*.5+(n<0?-1:1)*area*.19*depth}
   if(!reduced&&!current){x+=Math.sin(tempo*.35+i)*2;y+=Math.cos(tempo*.3+i)*2}
   let point=state.positions.get(i);if(!point||reduced)point={x,y};else{const blend=1-Math.exp(-dt*13);point.x+=(x-point.x)*blend;point.y+=(y-point.y)*blend}state.positions.set(i,point);visible.push({i,current,x:point.x,y:point.y});
  }
  for(const key of state.positions.keys())if(Math.abs(key-at)>25)state.positions.delete(key);
  const center=visible.find(x=>x.current);
  if(center){ctx.save();ctx.strokeStyle=C.secondary;ctx.lineWidth=1;ctx.globalAlpha=style==='constellation'?.4:.24;
   for(let j=0;j<visible.length;j++){const p=visible[j];if(p.current)continue;ctx.beginPath();ctx.moveTo(center.x,center.y);if(style==='branches')ctx.bezierCurveTo(center.x,p.y,p.x,center.y,p.x,p.y);else ctx.lineTo(p.x,p.y);ctx.stroke();if(style==='web'&&j>0){ctx.beginPath();ctx.moveTo(visible[j-1].x,visible[j-1].y);ctx.lineTo(p.x,p.y);ctx.stroke()}}
   ctx.restore();
  }
  for(const p of visible){if(p.current)continue;ctx.save();ctx.fillStyle=C.secondary;ctx.globalAlpha=.65;ctx.beginPath();ctx.arc(p.x,p.y,style==='constellation'?2.8:2,0,Math.PI*2);ctx.fill();ctx.restore();label(units[p.i],p.i,p.x,p.y-17,Math.min(W*.033,area*.065)*(frame.textScale||1),.62,{max:W*.24})}
  if(center)label(active,at,center.x,center.y,currentSize*.78,1,{max:W*.67});
 }else if(style==='code'||style==='code-visual'){
  text(ctx,'// '+(data.kind==='word'?'word timing':'lyrics'),W*.5,area*.12,12,C.secondary,{font:'Cascadia Code',alpha:.7,max:W*.8,weight:400},state);
  for(let n=-2;n<=2;n++){
   const i=at+n;if(i<0||i>=units.length)continue;const y=area*.51+n*area*.15;
   text(ctx,String(i+1).padStart(3,'0'),W*.065,y,12,C.secondary,{font:'Cascadia Code',alpha:.55},state);
   label(units[i],i,W*.52,y,Math.min(W*.09,area*.1)*(frame.textScale||1),i===at?1:.36,{font:frame.font||'Cascadia Code',max:W*.77});
  }
 }else{
  const lens=style.startsWith('fisheye'),ship=style==='ship',intensity=clamp(frame.lens??.75),span=area*.18;
  for(let n=-2;n<=2;n++){
   const i=at+n;if(i<0||i>=units.length)continue;const distance=Math.abs(n);
   let y=area*.51+n*span,x=W*.5,scale=1-distance*.16,angle=0;
   if(lens){y=area*.51+Math.tanh(n*.75)*area*.4;scale=1/(1+distance*distance*.24*intensity);x+=Math.sin(n)*W*.025*intensity}
   if(ship){x+=n*W*.04;angle=-.055+(reduced?0:Math.sin(tempo*.3)*.014);ctx.save();ctx.transform(1,.015,-.13,1,W*.06,0)}
   label(units[i],i,x,y,currentSize*scale,i===at?1:Math.max(.16,.62-distance*.18),{angle,scaleX:lens?1+(.12-distance*.08)*intensity:1});
   if(ship)ctx.restore();
  }
 }
 if(translated){
  paintTranslation(ctx,frame,state,active.line,{x:0,y:area,width:W,height:footer});
 }

 state.previousIndex=at;
 return {index,progress,line:active.line,hits,kind:active.kind};
}
const api={styles,prepare,paint,paintTranslation,makeState,upperBound,direction,iconFor};
if(typeof module==='object'&&module.exports)module.exports=api;else root.MadarLyricArt=api;
})(typeof window==='object'?window:globalThis);
