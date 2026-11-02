'use strict';
// Audio-driven adapter: source demos supply drawing geometry, never timestamps.
(()=>{
const presets=new Map(window.MadarMotionPresets.map(p=>[p.id,p]));
function paint(c,f,owner){
 const preset=presets.get(f.style);if(!preset)return {index:-1,line:-1,hits:[]};
 const {data,width:W,height:H}=f,art=window.MadarLyricArt;
 let index=data.timed?art.upperBound(data.units,f.time):Math.max(0,Math.min(data.units.length-1,f.manualIndex||0));
 const unit=data.units[Math.max(0,index)];if(!unit)return {index:-1,line:-1,hits:[]};
 const time=Number(f.time)||0,end=Number.isFinite(unit.end)?unit.end:f.duration;
 const progress=unit.start===null?1:Math.max(0,Math.min(1,(time-unit.start)/Math.max(.02,end-unit.start)));
 const hot=unit.start!==null&&time>=unit.start&&Number.isFinite(end)&&time<end;
 const colors=f.colors||{};
 const palette={foreground:colors.text||'#f4f3ff',accent:colors.accent||'#bca5ff',second:colors.secondary||'#aeb8ce'};
 const translated=f.translations?.[unit.line]||'',footer=translated?Math.min(H*.3,190):20,area=Math.max(1,H-footer);
 const family=JSON.stringify(f.font||'Segoe UI')+',Tahoma,sans-serif';
 const key=f.style+':'+unit.line+':'+W+':'+area+':'+family+':'+f.textScale+':'+f.duration;
 let s=owner.library;
 if(!s||s.key!==key){
  // At most 25 surrounding words; cached until the selected word leaves the window.
  const all=data.units.filter(u=>u.line===unit.line),at=all.indexOf(unit),lo=Math.max(0,at-12),selected=all.slice(lo,lo+25);
  s=owner.library={key,c,ctx:c,w:W,h:area,mode:preset.mode,items:new Map(),smooth:{x:0,y:0},pointer:{x:0,y:0},drawCache:{},words:selected.map((u,j)=>({text:u.text,l:1,j,duration:Math.max(.02,((Number.isFinite(u.end)?u.end:f.duration)-u.start)||1)*1000,unit:u})),starts:selected.map(u=>(u.start||0)*1000),fontFamily:family};
 }
 let at=s.words.findIndex(w=>w.unit===unit);
 if(at<0){owner.library=null;return paint(c,f,owner)}
 const last=s.lastTime,delta=Number.isFinite(last)?Math.abs(time-last):0;
 if(delta>.3||last>time){s.items.clear();s.smooth={x:0,y:0}}
 s.dt=f.reduced||f.paused||last===undefined||delta>.3||last>time?10000:Math.max(0,Math.min(50,(time-last)*1000));s.lastTime=time;
 s.revealMode=f.revealMode||'word';s.index=at;s.progress=progress;s.hot=hot;s.reduced=Boolean(f.reduced);s.time=f.reduced?0:time*1000;s.phase=s.time;s.textScale=f.textScale||1;s.lang=art.direction(data.lines[unit.line].text)==='rtl'?'ar':'en';s.pointer=f.pointer||{x:0,y:0};s.palette={...palette,accent:hot?palette.accent:palette.foreground};
 s.colors={ink:palette.foreground,accent:hot?palette.accent:palette.foreground,second:palette.second,soft:'#252433'};
 const lines=[-1,0,1].map(d=>({ar:data.lines[unit.line+d]?.text||'',en:data.lines[unit.line+d]?.text||''}));
 c.clearRect(0,0,W,H);c.save();c.beginPath();c.rect(0,0,W,area);c.clip();
 if(f.reduced){c.font='500 '+Math.min(38,W*.06)+'px '+family;c.fillStyle=hot?palette.accent:palette.foreground;c.textAlign='center';c.textBaseline='middle';c.direction=art.direction(data.lines[unit.line].text);c.fillText(s.revealMode==='line'?data.lines[unit.line].text:s.words.filter(w=>w.unit.start===null||w.unit.start<=time).map(w=>w.text.trim()).join(' '),W/2,area/2,W*.9)}
 else window.MadarMotionDraw[preset.group](s,{lines});
 c.restore();
 if(translated){
  art.paintTranslation(c,f,owner,unit.line,{x:0,y:area,width:W,height:footer});
 }
 owner.frames++;return {index,progress,line:unit.line,kind:unit.kind,hits:[]};
}
window.MadarMotionLibrary={paint,presets:window.MadarMotionPresets};
})();
