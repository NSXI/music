'use strict';
// Adapted from the two user-supplied lyric-motion galleries; no demo scheduler or network assets.
(()=>{
const original=(()=>{const clamp=(v,a,b)=>Math.max(a,Math.min(b,v)),mix=(a,b,t)=>a+(b-a)*t,ease=t=>{t=clamp(t,0,1);return t*t*(3-2*t)};let reduced={matches:false},pref={strength:.7,textScale:1},prefs={intensity:1,font:1},words=[],starts=[],durations=[],lines=[];const current=s=>s.index;function draw(s,dt){
 const {ctx:c,w,h,mode}=s;
 const {foreground,accent,second}=s.palette;
 c.clearRect(0,0,w,h);
 const damp=1-Math.exp(-dt/90),amount=reduced.matches?0:prefs.intensity;
 s.smooth.x=mix(s.smooth.x,s.pointer.x,damp);s.smooth.y=mix(s.smooth.y,s.pointer.y,damp);
 const mx=s.smooth.x*amount,my=s.smooth.y*amount;
 const index=s.index;
 const q=s.progress,beat=Math.sin(Math.max(0,q)*Math.PI),t=s.phase*.001;
 const cx=w/2,cy=h/2,base=Math.min(42,w*.095)*prefs.font;
 c.font=`500 ${base}px ${s.fontFamily}`;c.textAlign='center';c.textBaseline='middle';c.direction=s.lang==='ar'?'rtl':'ltr';
 const links=[];const rendered=[];
 const lo=mode===7?Math.floor(index/6)*6:Math.max(0,index-4);
 const hi=mode===7?Math.min(words.length,lo+6):Math.min(words.length,index+6);
 if(mode===0||mode===1||mode===5){
  c.save();c.translate(cx+mx*16,cy+my*10);c.strokeStyle=accent;c.globalAlpha=.12;c.lineWidth=1;
  for(let r=0;r<3;r++){c.beginPath();c.ellipse(0,0,w*(.19+r*.075),h*(.18+r*.09),-.35+r*.2,0,Math.PI*2);c.stroke();}c.restore();
 }
 if(mode===4){
  c.save();const gx=cx+mx*w*.25,gy=cy+my*h*.25;const g=c.createRadialGradient(gx,gy,1,gx,gy,w*.35);g.addColorStop(0,accent);g.addColorStop(1,'transparent');c.fillStyle=g;c.globalAlpha=.12;c.fillRect(0,0,w,h);c.restore();
 }
 for(let j=lo;j<hi;j++){
  const d=j-index,active=d===0;let x=cx,y=cy,scale=1,alpha=1,rot=0,stretch=1;
  switch(mode){
   case 0:{const a=d*1.25+t*.6;x=active?cx+mx*24:cx+Math.cos(a)*w*.32+mx*10;y=active?cy+my*20:cy+Math.sin(a)*h*.34;scale=active?1+beat*.08:.35+Math.sin(a)*.06;alpha=active?1:.32;rot=active?mx*.05:Math.sin(a)*.12;break;}
   case 1:{const r=active?(1-q)*36:(Math.abs(d)+1)*29;const a=d*1.7-t*.8+q*.6;x=cx+Math.cos(a)*r+mx*(active?25:12);y=cy+Math.sin(a)*r*.7+my*18;scale=active?1-q*.35:.25+Math.abs(d)*.04;rot=active?q*.14:Math.sin(a)*.35;alpha=active?1-q*.45:.3;break;}
   case 2:{let z=d-q*.5;scale=active?.8+q*.45:Math.max(.16,1-z*.19);x=cx+(active?mx*22:d*19+mx*(35-d*4));y=cy+(active?my*18:d*30);alpha=active?1:Math.max(.06,.25-Math.abs(d)*.035);if(d<0){x+=d*40;y-=d*26;scale=1.1-d*.22;alpha=.1;}break;}
   case 3:{x=cx-d*w*.21+mx*35;y=cy+Math.sin(d*.95+t*2.1)*22*amount+my*32*Math.cos(d*.8);scale=active?1+beat*.12:.48;stretch=active?1+Math.sin(q*Math.PI*2)*.2*amount:1;rot=Math.sin(d+t)*.09*amount;alpha=active?1:.4;if(Math.abs(d)>2)alpha=0;break;}
   case 4:{x=cx-d*w*.23;y=cy+Math.sin(d*.9)*32;const distance=Math.abs(d+mx*1.7);scale=Math.max(.34,1.15-distance*.45);y+=my*(active?35:14);alpha=active?1:.32;stretch=1+Math.sin(t*2+d)*.15*amount;rot=Math.sin(t+d)*.04*amount;if(Math.abs(d)>2)alpha=0;break;}
   case 5:{const a=d*.85+t*.65;x=cx+Math.sin(a)*w*.27+mx*20;y=cy+d*40+my*16;scale=active?1:.36+(Math.cos(a)+1)*.14;stretch=.55+(Math.cos(a)+1)*.23;alpha=active?1:.25+(Math.cos(a)+1)*.12;rot=Math.sin(a)*.13;break;}
   case 6:{const positions=[[.5,.5],[.77,.27],[.27,.23],[.19,.65],[.72,.76],[.5,.12],[.88,.55],[.4,.86],[.13,.39],[.63,.35]];const pos=positions[d===0?0:((Math.abs(d)*2+(d<0?0:1))%9)+1];x=w*pos[0]+mx*22*Math.cos(j);y=h*pos[1]+my*20*Math.sin(j)+Math.sin(t+j)*5*amount;scale=active?1+beat*.08:.4;alpha=active?1:.5;links.push([x,y]);break;}
   case 7:{const local=j-lo;const pos=[[.7,.25],[.3,.28],[.55,.48],[.22,.6],[.76,.7],[.43,.8]][local];x=w*pos[0]+mx*12;y=h*pos[1]+my*10;rot=[-.17,.12,-.08,.18,-.11,.09][local];scale=active?1+beat*.08:.62;alpha=d>0?.08:active?1:.68;if(d===0)y+=(1-q)*20;break;}
   case 8:{x=cx+mx*30;y=cy+my*20;scale=active?1+beat*.08:.5;alpha=active?1:0;stretch=1+Math.sin(q*6.28)*.08*amount;break;}
   case 9:{if(d>0){x=cx+d*45;y=cy+70+d*15;scale=.3;alpha=.18;}else if(active){x=cx+mx*22;y=cy+my*18-Math.sin(q*Math.PI)*12;scale=1;alpha=1;}else{const age=-d+q;x=cx+Math.sin(j*2.1)*w*.24+mx*20;y=cy-age*38;rot=Math.sin(j+q)*age*.12;scale=.5;alpha=Math.max(0,.6-age*.12);}break;}
  }
  if(reduced.matches){x=active?cx:cx-d*w*.18;y=active?cy:cy+60;scale=active?1:.35;rot=0;stretch=1;alpha=active?1:Math.abs(d)<3?.3:0;}
  c.font=`500 ${base}px ${s.fontFamily}`;
  const tw=c.measureText(words[j]).width;
  const half=Math.min(w*.46,tw*scale*stretch/2+12);
  x=Math.max(half,Math.min(w-half,x));y=Math.max(32,Math.min(h-30,y));
  if(s.revealMode!=='line'&&s.time<s.starts[j])alpha=0;const target={x,y,scale,alpha,rot,stretch};let item=s.items.get(j);
  if(!item){item={...target,scale:target.scale*.85,alpha:0};s.items.set(j,item);}
  for(const key of Object.keys(target))item[key]=mix(item[key],target[key],1-Math.exp(-dt/(active?95:160)));
  item.alpha=target.alpha;rendered.push({j,active,item,tw});
 }
 // Render the network and the elastic thread beneath the words.
 if((mode===6||mode===3)&&!reduced.matches){c.save();c.strokeStyle=accent;c.globalAlpha=.24;c.lineWidth=1;c.beginPath();if(mode===6){for(const pt of links.slice(1)){c.moveTo(links[0][0],links[0][1]);c.lineTo(pt[0],pt[1]);}}else{rendered.filter(z=>Math.abs(z.j-index)<=2).forEach((z,k)=>{const a=z.item;k?c.lineTo(a.x,a.y+25):c.moveTo(a.x,a.y+25);});}c.stroke();c.restore();}
 rendered.sort((a,b)=>Number(a.active)-Number(b.active));
 for(const {j,active,item:a,tw}of rendered){
  if(a.alpha<.015)continue;c.save();c.translate(a.x,a.y);c.rotate(a.rot);c.scale(a.scale*a.stretch,a.scale);c.font=`500 ${base}px ${s.fontFamily}`;c.globalAlpha=a.alpha;if(s.revealMode==='character'&&active){c.beginPath();const span=tw*s.progress;c.rect(s.lang==='ar'?tw/2-span:-tw/2,-base,span,base*2);c.clip()}
  if(mode===8&&active&&!reduced.matches){for(let e=3;e>0;e--){const offset=(1-q)*e*16*amount;c.save();c.globalAlpha=.14/e;c.fillStyle=e%2?second:accent;c.fillText(words[j],offset,-offset*.3);c.restore();}for(let k=0;k<3;k++){c.save();const offset=Math.sin(q*Math.PI)*(k-1)*9*amount;c.beginPath();c.rect(-tw/2-8,-base/2+k*base/3,tw+16,base/3);c.clip();c.fillStyle=k===1?accent:foreground;c.fillText(words[j],offset,0);c.restore();}}
  else{c.fillStyle=active&&s.hot?accent:foreground;c.fillText(words[j],0,0,Math.max(20,(w-32)/Math.max(1,a.scale*Math.abs(a.stretch))));}
  if(active&&mode===0){c.strokeStyle=accent;c.lineWidth=1.5;c.globalAlpha=.4;c.beginPath();c.ellipse(0,base*.7,tw*.35,3,0,0,Math.PI*2);c.stroke();}
  c.restore();
 }
 for(const j of [...s.items.keys()])if(j<lo||j>=hi)s.items.delete(j);
 if(s.held&&!reduced.matches){c.save();c.strokeStyle=accent;c.globalAlpha=.25;c.lineWidth=1;c.beginPath();c.arc(cx+s.pointer.x*cx,cy+s.pointer.y*cy,24+beat*12,0,Math.PI*2);c.stroke();c.restore();}
}
return (s,env)=>{reduced.matches=s.reduced;words=s.words.map(w=>w.text);starts=s.starts;durations=s.words.map(w=>w.duration);lines=env.lines;pref.textScale=s.textScale;prefs.font=s.textScale;draw(s,s.dt)};})();
const flow=(()=>{const clamp=(v,a,b)=>Math.max(a,Math.min(b,v)),mix=(a,b,t)=>a+(b-a)*t,ease=t=>{t=clamp(t,0,1);return t*t*(3-2*t)};let reduced={matches:false},pref={strength:.7,textScale:1},prefs={intensity:1,font:1},words=[],starts=[],durations=[],lines=[];const current=s=>s.index;function layout(s,words,font){
 if(s.boxes)return s.boxes;const c=s.c;c.font=`500 ${font}px ${s.fontFamily}`;
 const rows=[[]];let used=0;const gap=22;
 words.forEach((word,j)=>{const width=c.measureText(word.text).width;if(used+width>s.w-48&&rows.at(-1).length){rows.push([]);used=0;}rows.at(-1).push({j,width});used+=width+gap;});
 const boxes=[];rows.forEach((row,r)=>{const width=row.reduce((v,b)=>v+b.width,0)+(row.length-1)*gap;let x=(s.w-width)/2;row.forEach(b=>{boxes[b.j]={x:s.lang==='ar'?s.w-(x+b.width/2):x+b.width/2,y:s.h/2+(r-(rows.length-1)/2)*48,width:b.width};x+=b.width+gap;});});return s.boxes=boxes;
}
function paint(s,dt){
 const {c,w,h,mode}=s,{ink,accent,second,soft}=s.colors;
 c.clearRect(0,0,w,h);
 const index=current(s),word=s.words[index],line=word.l,q=s.progress,u=word.j+ease(q),t=s.time*.001;
 const amp=reduced.matches?0:pref.strength;
 const smooth=1-Math.exp(-dt/110);s.smooth.x=mix(s.smooth.x,s.pointer.x,smooth);s.smooth.y=mix(s.smooth.y,s.pointer.y,smooth);
 const mx=s.smooth.x*amp,my=s.smooth.y*amp,cx=w/2,cy=h/2;
 const font=Math.min(44,w*.071)*pref.textScale;
 const group=s.words,boxes=layout(s,group,font);
 const targets=[];const rtl=s.lang==='ar'?-1:1;
 group.forEach((v,j)=>{
  const d=j-word.j,active=d===0,b=boxes[j],n=group.length;
  let x=b.x,y=b.y,scale=active?1.12:1,alpha=active?1:.42,rot=0,sx=1,clip=1;
  switch(mode){
   case 0:y+=Math.sin(j*.7-t*1.5)*16*amp;rot=Math.cos(j*.7-t*1.5)*.025*amp;break;
   case 1:{const a=(j-u)*.9;x=cx+Math.sin(a)*w*.32;y=cy+Math.sin(a*2)*h*.23;scale=active?1.6:.65;alpha=active?1:.24;rot=Math.cos(a)*.06;break;}
   case 2:y=cy-45+Math.pow((b.x-cx)/(w/2),2)*80;rot=(b.x-cx)/w*.16*amp;break;
   case 3:y+=Math.sin(t*1.1-j*.6)*22*amp;rot=Math.sin(t*1.1-j*.6)*.13*amp;sx=1-Math.abs(Math.sin(t-j))*.06*amp;break;
   case 4:x=cx+d*w*.2*rtl;y=cy+d*37;scale=active?1.4:.65;alpha=Math.abs(d)>2?0:active?1:.3;break;
   case 5:x=cx+(active?Math.sin((1-q)*Math.PI/2)*w*.12:(d<0?-1:1)*w*.31);y=cy+(active?0:(d<0?-60:60));scale=active?1.65:.58;alpha=active?1:Math.abs(d)===1?.23:0;break;
   case 6:y=b.y+(d>0?22:d<0?-8:(1-ease(q))*18)*amp;clip=d>0?.17:active?.25+.75*ease(q):1;alpha=d>0?.15:active?1:.5;break;
   case 7:{const tide=Math.sin(t*.8)*amp;x=cx+(b.x-cx)*(1+tide*.06);y=b.y+tide*20;scale*=1+tide*.04;break;}
   case 8:rot=Math.sin(clamp(u-j+1,0,2)*Math.PI)*.22*amp;y=b.y-Math.sin(clamp(u-j+1,0,2)*Math.PI)*12*amp;break;
   case 9:x=b.x+mx*8;y=b.y+my*10;scale=active?1.12+Math.sin(q*Math.PI)*.08*amp:1;break;
   case 10:x=cx+(b.x-cx)*(.88+Math.sin(t*.85)*.08*amp)+mx*8;y=b.y+Math.sin(j*.5+t)*6*amp;sx=active?1.04:1;break;
   case 11:{const angle=Math.sin(t*1.2+j*.18)*.15*amp+mx*.05;x=b.x+Math.sin(angle)*55;y=b.y+(1-Math.cos(angle))*55;rot=angle;break;}
   case 12:{const r=clamp(d-q*.2,-3,3)/3;x=cx+r*w*.36;y=cy+(r*r*r-r)*95;rot=(r*r*.1-.06)*amp;scale=active?1.55:.6;alpha=Math.abs(d)>2?0:active?1:.28;break;}
   case 13:{const side=j%2?1:-1;x=cx+side*w*.23;y=cy+d*52;scale=active?1.3:.66;alpha=Math.abs(d)>2?0:active?1:.3;break;}
   case 14:{const a=clamp(d,-3,3)*.38;x=cx+Math.sin(a)*w*.36;y=cy+48-Math.cos(a)*48;rot=a*.36;scale=active?1.5:.62;alpha=Math.abs(d)>2?0:active?1:.3;break;}
   case 15:scale=active?1.12:1;y=b.y+Math.sin(t*.65)*7*amp;break;
   case 16:y=b.y+Math.sin((b.x/w)*4+t*.9+mx)*23*amp;rot=Math.cos((b.x/w)*4+t*.9+mx)*.04*amp;break;
   case 17:x=cx+d*32;y=cy+d*30;scale=active?1.65:Math.max(.4,.8-Math.abs(d)*.1);alpha=active?1:Math.abs(d)>2?0:.2;rot=d*.025;break;
   case 18:scale=1;alpha=d<0?.85:d===0?1:.3;y=b.y+Math.sin(t*.7)*4*amp;break;
   case 19:{const breathe=1+Math.sin(t*.85)*.045*amp;x=cx+(b.x-cx)*breathe;y=cy+(b.y-cy)*breathe;scale*=breathe;break;}
  }
  x+=mx*(active?14:7);y+=my*(active?10:5);
  if(reduced.matches){x=b.x;y=b.y;rot=0;scale=1;sx=1;clip=1;}
  const half=b.width*scale*sx/2+8;
  x=clamp(x,Math.min(half,w/2),Math.max(w-half,w/2));y=clamp(y,35,h-35);
  if(s.revealMode!=='line'&&s.time<s.starts[j])alpha=0;targets.push({key:line+':'+j,text:v.text,active,x,y,scale,alpha,rot,sx,clip,width:b.width});
 });
 // All visual elements share the same easing and restrained motion amplitude.
 const live=new Set(targets.map(a=>a.key)),items=[];
 targets.forEach(a=>{let b=s.items.get(a.key);if(!b){b={...a,alpha:.05,y:a.y+12*amp};s.items.set(a.key,b);}for(const k of ['x','y','scale','alpha','rot','sx','clip'])b[k]=mix(b[k],a[k],1-Math.exp(-dt/125));b.alpha=a.alpha;b.active=a.active;items.push(b);});
 for(const [key,b]of s.items){if(!live.has(key)){b.alpha*=Math.exp(-dt/100);b.y-=dt*.035*amp;if(b.alpha<.015)s.items.delete(key);else items.push({...b,active:false});}}
 if(mode===15&&s.revealMode==='line'){c.save();c.fillStyle=ink;c.textAlign='center';c.textBaseline='middle';c.direction=s.lang==='ar'?'rtl':'ltr';c.font=`400 ${Math.min(16,w*.047)}px ${s.fontFamily}`;for(const offset of [-1,1]){const other=(line+offset+3)%3;c.globalAlpha=offset<0?.16:.11;c.fillText((lines[other]?.[s.lang]||''),cx+mx*6,cy+offset*88, w-32);}c.restore();}
 if([2,10,11,16,18].includes(mode)&&!reduced.matches){c.save();c.strokeStyle=mode===16?second:accent;c.lineWidth=mode===16?2:1;c.globalAlpha=.18;c.beginPath();targets.forEach((b,j)=>{const yy=b.y+(mode===11?-54:font*.9);if(j===0)c.moveTo(b.x,yy);else{const a=targets[j-1];c.bezierCurveTo(a.x+(b.x-a.x)*.5,a.y+(mode===11?-54:font*.9),a.x+(b.x-a.x)*.5,yy,b.x,yy);}});c.stroke();if(mode===11){targets.forEach(b=>{c.beginPath();c.moveTo(b.x,b.y-54);c.lineTo(b.x,b.y-font*.7);c.stroke();});}c.restore();}
 if(mode===5){c.save();c.strokeStyle=accent;c.lineWidth=1;c.globalAlpha=.16;for(const side of [-1,1]){c.beginPath();c.ellipse(cx+side*w*.35,cy,12,68,0,0,Math.PI*2);c.stroke();}c.restore();}
 if(mode===9){const a=items.find(a=>a.active);if(a){c.save();c.fillStyle=accent;c.globalAlpha=.09;c.beginPath();c.ellipse(a.x,a.y,a.width*.65+10,font*1.1,0,0,Math.PI*2);c.fill();c.restore();}}
 items.sort((a,b)=>Number(a.active)-Number(b.active));
 for(const a of items){if(a.alpha<.02)continue;c.save();c.translate(a.x,a.y);c.rotate(a.rot);c.scale(a.scale*a.sx,a.scale);c.textAlign='center';c.textBaseline='middle';c.direction=s.lang==='ar'?'rtl':'ltr';c.font=`500 ${font}px ${s.fontFamily}`;c.globalAlpha=a.alpha;if(s.revealMode==='character'&&a.active){c.beginPath();const span=a.width*s.progress;c.rect(s.lang==='ar'?a.width/2-span:-a.width/2,-font,span,font*2);c.clip()}
  if(mode===19&&!reduced.matches){c.save();c.fillStyle=accent;c.globalAlpha=a.alpha*.1;c.fillText(a.text,mx*9,9);c.restore();}
  if(mode===6){c.beginPath();c.rect(-a.width/2-5,-font*.8,a.width+10,font*1.6*a.clip);c.clip();}
  c.fillStyle=a.active&&s.hot?accent:ink;c.fillText(a.text,0,0,Math.max(20,(w-32)/Math.max(1,a.scale*Math.abs(a.sx))));
  if(mode===18&&a.active){c.fillStyle=ink;c.fillText(a.text,0,0,Math.max(20,(w-32)/Math.max(1,a.scale*Math.abs(a.sx))));c.beginPath();const reveal=a.width*ease(q);c.rect(s.lang==='ar'?a.width/2-reveal:-a.width/2,-font,reveal,font*2);c.clip();c.fillStyle=accent;c.fillText(a.text,0,0,Math.max(20,(w-32)/Math.max(1,a.scale*Math.abs(a.sx))));}
  c.restore();
 }
}
return (s,env)=>{reduced.matches=s.reduced;words=s.words.map(w=>w.text);starts=s.starts;durations=s.words.map(w=>w.duration);lines=env.lines;pref.textScale=s.textScale;prefs.font=s.textScale;paint(s,s.dt)};})();
const games=(()=>{const clamp=(v,a,b)=>Math.max(a,Math.min(b,v)),mix=(a,b,t)=>a+(b-a)*t,ease=t=>{t=clamp(t,0,1);return t*t*(3-2*t)};let reduced={matches:false},pref={strength:.7,textScale:1},prefs={intensity:1,font:1},words=[],starts=[],durations=[],lines=[];const current=s=>s.index;function layout(s,words,font){
 if(s.boxes)return s.boxes;const c=s.c;c.font=`500 ${font}px ${s.fontFamily}`;
 const rows=[[]];let used=0;const gap=22;
 words.forEach((word,j)=>{const width=c.measureText(word.text).width;if(used+width>s.w-48&&rows.at(-1).length){rows.push([]);used=0;}rows.at(-1).push({j,width});used+=width+gap;});
 const boxes=[];rows.forEach((row,r)=>{const width=row.reduce((v,b)=>v+b.width,0)+(row.length-1)*gap;let x=(s.w-width)/2;row.forEach(b=>{boxes[b.j]={x:s.lang==='ar'?s.w-(x+b.width/2):x+b.width/2,y:s.h/2+(r-(rows.length-1)/2)*48,width:b.width};x+=b.width+gap;});});return s.boxes=boxes;
}
function polygon(c,points,fill,stroke){c.beginPath();points.forEach((p,i)=>i?c.lineTo(...p):c.moveTo(...p));c.closePath();if(fill)c.fill();if(stroke)c.stroke();}
function star(c,x,y,r,rotation=0){const pts=[];for(let j=0;j<8;j++){const a=j*Math.PI/4+rotation,rr=j%2?r*.3:r;pts.push([x+Math.cos(a)*rr,y+Math.sin(a)*rr]);}polygon(c,pts,true,false);}
function backdrop(s,t,q,mx,my,amp){
 const {c,w,h,mode}=s,{ink,accent,second,soft}=s.colors,cx=w/2,cy=h/2;
 const move=t*amp;c.save();c.translate(mx*9,my*7);c.lineWidth=1;c.strokeStyle=accent;c.fillStyle=accent;c.globalAlpha=.14;
 switch(mode){
 case 0:for(const side of [-1,1]){c.save();c.translate(cx+side*w*.2,cy);c.rotate(side*.08+Math.sin(move*.4)*.02);c.strokeRect(-w*.19,-94,w*.38,188);c.strokeRect(-w*.19+6,-88,w*.38-12,176);c.restore();}break;
 case 1:for(let y=30;y<h;y+=48)for(let x=24;x<w;x+=48){const pulse=Math.sin(move+x*.01+y*.01);c.globalAlpha=.06+(pulse+1)*.035;polygon(c,[[x,y-18],[x+18,y],[x,y+18],[x-18,y]],true,true);}break;
 case 2:for(const side of [-1,1]){c.save();c.translate(cx,cy);c.scale(side,1);c.rotate(Math.sin(move*.8)*.05);c.beginPath();c.moveTo(0,0);c.bezierCurveTo(18,-118,135,-106,100,-20);c.bezierCurveTo(158,69,52,90,0,0);c.fill();c.stroke();c.restore();}break;
 case 3:c.save();c.translate(cx,cy);for(const r of [78,104]){c.beginPath();c.arc(0,0,r,0,Math.PI*2);c.stroke();}for(let i=0;i<12;i++){const a=i*Math.PI/6;c.beginPath();c.moveTo(Math.cos(a)*90,Math.sin(a)*90);c.lineTo(Math.cos(a)*99,Math.sin(a)*99);c.stroke();}c.rotate(move*.13);c.beginPath();c.moveTo(0,65);c.lineTo(0,-85);c.stroke();c.restore();break;
 case 4:for(let i=0;i<4;i++){c.globalAlpha=.06+i*.025;const x=(i*.27*w+move*12)%(w+150)-120;polygon(c,[[x,0],[x+w*.2,0],[x+w*.5,h],[x+w*.3,h]],true,false);}for(let r=0;r<3;r++){c.beginPath();c.ellipse(cx,cy+54,70+r*30+Math.sin(move)*5,7+r*5,0,0,Math.PI*2);c.stroke();}break;
 case 5:c.save();c.translate(cx,cy);c.rotate(-move*.08);for(let i=0;i<24;i++){const a=i*Math.PI/12;c.beginPath();c.moveTo(Math.cos(a)*91,Math.sin(a)*91);c.lineTo(Math.cos(a)*(i%2?98:108),Math.sin(a)*(i%2?98:108));c.stroke();}c.beginPath();c.arc(0,0,83,0,Math.PI*1.65);c.stroke();c.restore();break;
 case 6:for(let i=-2;i<=2;i++){c.save();c.translate(cx+i*27,cy+Math.abs(i)*7);c.rotate(i*.12);c.strokeRect(-65,-88,130,176);c.beginPath();c.ellipse(0,0,35,56,0,0,Math.PI*2);c.stroke();c.restore();}break;
 case 7:c.strokeRect(18,26,w-48,h-52);c.strokeRect(25,33,w-62,h-66);c.globalAlpha=.05;for(let y=40;y<h-35;y+=5){c.beginPath();c.moveTo(29,y);c.lineTo(w-41,y);c.stroke();}c.globalAlpha=.12;const sy=42+(move*18)%(h-84);c.fillRect(29,sy,w-70,5);for(let i=0;i<3;i++){c.beginPath();c.arc(w-22,cy+(i-1)*24,4,0,Math.PI*2);c.stroke();}break;
 case 8:c.save();c.translate(cx,cy);c.rotate(move*.07);for(let i=0;i<12;i++){c.rotate(Math.PI/6);polygon(c,[[40,-7],[145,-22],[145,14],[40,7]],true,false);}c.beginPath();c.arc(0,0,52,0,Math.PI*2);c.stroke();c.restore();break;
 case 9:c.fillStyle=accent;polygon(c,[[0,35],[cx+20,55],[cx-32,h-25],[0,h-10]],true,false);c.fillStyle=second;polygon(c,[[w,20],[cx+32,30],[cx-5,h-55],[w,h-20]],true,false);c.globalAlpha=.24;c.beginPath();c.moveTo(cx-32,h-25);c.lineTo(cx+32,30);c.stroke();break;
 case 10:for(let k=0;k<3;k++){const y=48+k*80+Math.sin(move+k)*4;polygon(c,[[8,y+15],[w-18,y-28],[w-35,y+25],[22,y+52]],true,false);}break;
 case 11:c.save();c.translate(cx,cy);for(let i=-2;i<=2;i++){c.save();c.rotate(i*.19);c.strokeRect(-60,-89,120,178);c.restore();}c.fillStyle=second;star(c,0,-71,13,Math.PI/4);c.restore();break;
 case 12:for(let i=0;i<12;i++){const y=22+i*23,x=(move*100+i*61)%(w+160)-160;c.globalAlpha=.05+(i%3)*.025;c.beginPath();c.moveTo(x,y+18);c.lineTo(x+100+i%3*20,y);c.stroke();}break;
 case 13:for(let row=0;row<3;row++)for(let col=0;col<4;col++){const x=cx+(col-1.5)*w*.2+(row%2?12:0),y=65+row*72;polygon(c,[[x,y-20],[x+w*.085,y],[x,y+20],[x-w*.085,y]],(row+col)%3===0,true);}break;
 case 14:for(const side of [18,w-18])for(let y=16;y<h;y+=25){c.fillRect(side-4,y,8,12);}c.beginPath();c.moveTo(45,45);c.lineTo(w*.7,45);c.lineTo(w*.7,85);c.lineTo(w*.3,85);c.lineTo(w*.3,h-45);c.lineTo(w-45,h-45);c.stroke();break;
 case 15:for(let row=0;row<4;row++)for(let col=0;col<5;col++){const size=22+row*4,x=cx+(col-2)*size*1.55,y=cy+15+row*25;c.globalAlpha=.035+(.5+.5*Math.sin(move*2+row+col))*.09;polygon(c,[[x-size/2,y],[x,y-7],[x+size/2,y],[x,y+7]],true,false);}c.globalAlpha=.15;c.beginPath();c.ellipse(cx,cy-10,w*.29,62,0,0,Math.PI*2);c.stroke();break;
 case 16:{const glow=c.createRadialGradient(cx,cy-30,1,cx,cy-30,125);glow.addColorStop(0,accent);glow.addColorStop(1,'transparent');c.globalAlpha=.13+Math.sin(move*.9)*.025;c.fillStyle=glow;c.fillRect(0,0,w,h);c.globalAlpha=.3;c.strokeRect(cx-14,cy-67,28,39);polygon(c,[[cx-20,cy-70],[cx,cy-90],[cx+20,cy-70]],false,true);c.beginPath();c.moveTo(cx,cy-90);c.lineTo(cx,14);c.stroke();break;}
 case 17:for(let i=0;i<4;i++){const r=26+i*18;c.beginPath();c.moveTo(cx-r,cy+110);c.lineTo(cx-r,cy-10);c.quadraticCurveTo(cx-r,cy-65,cx,cy-105-i*4);c.quadraticCurveTo(cx+r,cy-65,cx+r,cy-10);c.lineTo(cx+r,cy+110);c.stroke();}break;
 case 18:c.globalAlpha=.12;c.beginPath();c.arc(cx+Math.sin(move*.3)*10,cy-20,91,0,Math.PI*2);c.fill();c.globalAlpha=.2;for(let k=0;k<4;k++){c.beginPath();c.ellipse(cx,cy-12,112+k*4,29+k*5,-.25,0,Math.PI*2);c.stroke();}break;
 case 19:c.save();c.translate(cx,cy);c.rotate(Math.sin(move*.3)*.04);for(let i=0;i<6;i++){c.rotate(Math.PI/3);c.beginPath();c.moveTo(0,-94);c.lineTo(0,-65);c.lineTo(16,-48);c.moveTo(0,-65);c.lineTo(-16,-48);c.stroke();}c.beginPath();c.arc(0,0,80,0,Math.PI*2);c.stroke();c.restore();break;
 case 20:for(let k=0;k<14;k++){const x=30+k*(w-60)/13,len=20+(.5+.5*Math.sin(k*7))*70;c.globalAlpha=.025+(k%3)*.02;c.fillRect(x,cy-35,2,len+Math.sin(move+k)*4);}break;
 case 21:for(let k=0;k<5;k++){c.globalAlpha=.05;c.lineWidth=12+k*5;c.beginPath();c.moveTo(-20,40+k*50);c.bezierCurveTo(w*.3,20+k*50+Math.sin(move*.5+k)*15,w*.7,75+k*40,w+20,50+k*40);c.stroke();}break;
 case 22:for(let i=0;i<18;i++){const a=i*2.4+move*.1,x=cx+Math.cos(a)*w*.33,y=cy+Math.sin(a*1.3)*h*.36;c.save();c.translate(x,y);c.rotate(a);c.globalAlpha=.08+i%3*.025;c.beginPath();c.ellipse(0,0,3,9,0,0,Math.PI*2);c.fill();c.restore();}break;
 case 23:c.save();c.translate(cx,cy);c.rotate(move*.035);for(let i=0;i<12;i++){const a=i*Math.PI/6,b=(i+1)*Math.PI/6;c.fillStyle=i%2?accent:second;c.globalAlpha=.07+i%3*.025;c.beginPath();c.moveTo(0,0);c.arc(0,0,106,a,b);c.closePath();c.fill();c.stroke();}c.globalAlpha=.2;for(const r of [50,99,108]){c.beginPath();c.arc(0,0,r,0,Math.PI*2);c.stroke();}c.restore();break;
 case 24:c.save();c.translate(cx,cy);for(let i=-2;i<=2;i++){const a=i*.34;c.save();c.rotate(a);c.globalAlpha=.08;c.fillRect(-w*.3,-20+i*2,w*.6,40);c.globalAlpha=.25;c.strokeRect(-w*.3,-20+i*2,w*.6,40);c.restore();}c.restore();break;
 case 25:for(let k=0;k<5;k++){c.globalAlpha=.1;c.strokeStyle=k%2?accent:second;c.beginPath();c.moveTo(10,cy+70+k*8);c.bezierCurveTo(w*.3,cy-95-k*5,w*.6,cy+90,w-10,cy-90+k*10);c.stroke();}c.fillStyle=second;for(let i=0;i<8;i++)star(c,25+(i*.137*w)%(w-40),30+(i*57)%(h-60),3+Math.sin(move+i),move*.1);break;
 case 26:c.save();c.translate(cx,cy);c.rotate(move*.08);for(let i=0;i<3;i++){c.rotate(Math.PI*2/3);c.beginPath();c.moveTo(0,0);c.bezierCurveTo(-50,-45,-20,-107,0,-112);c.bezierCurveTo(20,-107,50,-45,0,0);c.fill();c.stroke();}c.restore();break;
 case 27:for(let k=-2;k<=2;k++){c.save();c.translate(cx+k*30,cy+Math.abs(k)*6);c.rotate(k*.1);c.globalAlpha=.1;c.fillStyle=soft;c.fillRect(-61,-88,122,176);c.globalAlpha=.25;c.strokeRect(-61,-88,122,176);c.restore();}break;
 case 28:for(let i=0;i<16;i++){const x=20+(i*79)%(w-40),y=h-((i*39+move*16)%(h+30));c.globalAlpha=.06+i%3*.025;c.beginPath();c.arc(x,y,4+i%4*3,0,Math.PI*2);c.stroke();}break;
 case 29:for(let i=0;i<12;i++){const x=25+(i*91)%(w-50),y=25+(i*43)%(h-50),shift=Math.sin(move*.6+i)*5;c.save();c.translate(x+shift,y);c.rotate(i*.7+move*.025);c.globalAlpha=.08;c.fillStyle=i%2?accent:second;polygon(c,[[0,-19],[10,0],[0,25],[-8,0]],true,true);c.restore();}break;
 }
 c.restore();
}
function paint(s,dt){
 const {c,w,h,mode}=s,{ink,accent,second,soft}=s.colors;c.clearRect(0,0,w,h);
 const index=current(s),word=s.words[index],line=word.l,q=s.progress,t=s.time*.001,amp=reduced.matches?0:pref.strength;
 const blend=1-Math.exp(-dt/110);s.smooth.x=mix(s.smooth.x,s.pointer.x,blend);s.smooth.y=mix(s.smooth.y,s.pointer.y,blend);
 const mx=s.smooth.x*amp,my=s.smooth.y*amp,cx=w/2,cy=h/2;
 backdrop(s,t,q,mx,my,amp);
 const font=Math.min(44,w*.076)*pref.textScale,serif=mode>=16&&mode<=22&&s.lang==='en';
 const family=s.fontFamily;
 const group=s.words,boxes=layout(s,group,font),targets=[];
 group.forEach((v,j)=>{
  const d=j-word.j,active=d===0,b=boxes[j];let x=cx,y=cy+d*47,scale=active?1.65:.65,rot=0,sx=1,alpha=active?1:Math.abs(d)<=2?.22:0;
  switch(mode){
   case 0:x=cx+d*w*.25;y=cy+Math.sin(t*.5+d)*8*amp;rot=active?0:d*.035;scale=active?1.65:.55;alpha=Math.abs(d)<=1?active?1:.23:0;break;
   case 1:x=b.x;y=b.y+Math.sin(j-t)*10*amp;scale=active?1.12:.9;alpha=active?1:.36;rot=Math.sin(t*.4+j)*.035*amp;break;
   case 2:x=cx+d*w*.2;y=cy+Math.sin(d*.8+t*.7)*25*amp;rot=Math.sin(t+d)*.08*amp;scale=active?1.65:.55;alpha=Math.abs(d)<=1?active?1:.26:0;break;
   case 3:{const a=d*.75+Math.sin(t*.2)*.07;x=cx+Math.sin(a)*w*.31;y=cy+55-Math.cos(a)*55;rot=d*.09;scale=active?1.65:.55;break;}
   case 4:x=b.x+(1-ease(q))*amp*4;y=b.y+(active?-5:5)+Math.sin(t+j*.5)*6*amp;rot=-.065;scale=active?1.15:.9;alpha=active?1:.38;break;
   case 5:{const a=d*.66;x=cx+Math.sin(a)*w*.26;y=cy+Math.abs(d)*24-10;rot=active?0:d*.06;break;}
   case 6:x=cx+d*w*.25;y=cy+Math.abs(d)*17;sx=active?.82+.18*Math.sin(q*Math.PI):.65;rot=d*.09;alpha=Math.abs(d)<=1?active?1:.28:0;break;
   case 7:x=b.x;y=b.y;scale=active?1.1:.92;alpha=active?1:.38;break;
   case 8:x=b.x;y=b.y+Math.sin(t*.6)*7*amp;rot=(b.x-cx)/w*.07;scale=active?1.1:.93;alpha=active?1:.4;break;
   case 9:x=cx+(active?(1-ease(q))*32*amp:(d<0?-1:1)*w*.27);y=cy+(d<0?-62:d>0?62:0);rot=active?-.05:d*.03;alpha=Math.abs(d)<=1?active?1:.25:0;break;
   case 10:x=b.x;y=b.y+(j%2?10:-10);rot=(j%2?-.08:-.04);scale=active?1.13:.91;alpha=active?1:.5;break;
   case 11:x=cx+d*w*.22;y=cy+Math.abs(d)*21;rot=d*.13;sx=active?1:.7;alpha=Math.abs(d)<=1?active?1:.25:0;break;
   case 12:x=cx+d*w*.25-(active?(1-ease(q))*25*amp:0);y=cy-d*25;rot=-.08;scale=active?1.7:.55;break;
   case 13:x=w*(.25+(j%3)*.25);y=cy+(Math.floor(j/3)-.5)*77-(active?8:0);scale=active?1.06:.77;alpha=active?1:.44;rot=-.04;break;
   case 14:x=b.x;y=b.y+(j%2?-18:18);rot=(j%2?1:-1)*.045;scale=active?1.12:.88;alpha=active?1:.35;break;
   case 15:x=b.x;y=b.y-Math.sin(q*Math.PI)*(active?18:6)*amp;rot=Math.sin(t+j)*.035*amp;scale=active?1.16:.9;alpha=active?1:.43;break;
   case 16:x=cx+d*w*.23+Math.sin(t*.8)*8*amp;y=cy+25+Math.abs(d)*16;rot=Math.sin(t*.8)*.025*amp;scale=active?1.65:.54;alpha=Math.abs(d)<=1?active?1:.2:0;break;
   case 17:x=cx+d*w*.15;y=cy+d*46;scale=active?1.65:Math.max(.35,.72-Math.abs(d)*.12);alpha=active?1:Math.abs(d)<=2?.18:0;break;
   case 18:{const a=d*.63;x=cx+Math.sin(a)*w*.3;y=cy+Math.sin(t*.4)*8*amp+d*25;rot=d*.03;scale=active?1.65:.55;break;}
   case 19:x=b.x;y=b.y+Math.sin(t*.6+j)*5*amp;scale=active?1.14:.9;alpha=active?1:.27;break;
   case 20:x=b.x;y=b.y+(1-ease(q))*(active?5:0)*amp;scale=active?1.08:.93;alpha=d<0?.75:active?1:.16;break;
   case 21:x=cx+d*34;y=cy+d*32+Math.sin(t*.4)*8*amp;scale=active?1.6:Math.max(.25,.8-Math.abs(d)*.15);alpha=active?1:Math.abs(d)<=2?.14:0;break;
   case 22:x=b.x;y=b.y+Math.sin(t*.7+j*.45)*12*amp;rot=Math.sin(t*.4+j)*.025*amp;scale=active?1.13:.92;alpha=active?1:.4;break;
   case 23:{const a=d*.7;x=cx+Math.sin(a)*w*.29;y=cy+Math.abs(d)*29;rot=d*.06;scale=active?1.65:.6;break;}
   case 24:x=cx+d*32;y=cy+d*43;rot=-.06;scale=active?1.55:.66;alpha=Math.abs(d)<=2?active?1:.28:0;break;
   case 25:x=b.x;y=b.y+Math.sin(j*.75+t*.8)*17*amp;rot=Math.cos(j*.75+t*.8)*.035*amp;scale=active?1.14:.95;alpha=active?1:.44;break;
   case 26:{const a=d*Math.PI*.6;x=cx+Math.sin(a)*w*.27;y=cy+(active?0:Math.cos(a)*65);rot=active?0:d*.045;scale=active?1.65:.58;alpha=Math.abs(d)<=1?active?1:.28:0;break;}
   case 27:x=cx+d*w*.2;y=cy+Math.abs(d)*19;rot=d*.12;sx=active?.85+Math.sin(q*Math.PI)*.15:.65;alpha=Math.abs(d)<=1?active?1:.27:0;break;
   case 28:x=cx+Math.sin(d*.8+t*.35)*w*(active?.025:.21);y=cy+d*44;rot=active?Math.sin(t*.3)*.025:d*.06;scale=active?1.65:.58;break;
   case 29:x=b.x;y=b.y+(j%2?1:-1)*Math.sin(q*Math.PI)*12*amp;rot=(j%2?1:-1)*.055*amp;scale=active?1.12:.92;alpha=active?1:.42;break;
  }
  x+=mx*(active?15:7);y+=my*(active?9:4);
  if(reduced.matches){x=b.x;y=b.y;scale=active?1.08:1;rot=0;sx=1;alpha=active?1:.4;}
  const text=mode>=9&&mode<=13&&s.lang==='en'?v.text.toUpperCase():v.text;
  c.font=`500 ${font}px ${family}`;const width=c.measureText(text).width;
  scale=Math.min(scale,(w-36)/(width*sx));const half=width*scale*sx/2+12;
  x=clamp(x,Math.min(half,w/2),Math.max(w-half,w/2));y=clamp(y,40,h-36);
  if(s.revealMode!=='line'&&s.time<s.starts[j])alpha=0;targets.push({key:line+':'+j,text,active,x,y,scale,rot,sx,alpha,width});
 });
 const live=new Set(targets.map(v=>v.key)),items=[];
 targets.forEach(a=>{let b=s.items.get(a.key);if(!b){b={...a,alpha:.06,y:a.y+8*amp};s.items.set(a.key,b);}for(const k of ['x','y','scale','rot','sx','alpha'])b[k]=mix(b[k],a[k],1-Math.exp(-dt/115));b.alpha=a.alpha;b.active=a.active;items.push(b);});
 for(const [key,b]of s.items)if(!live.has(key)){b.alpha*=Math.exp(-dt/100);if(b.alpha<.015)s.items.delete(key);else items.push({...b,active:false});}
 items.sort((a,b)=>Number(a.active)-Number(b.active));
 for(const a of items){if(a.alpha<.018)continue;c.save();c.translate(a.x,a.y);c.rotate(a.rot);c.scale(a.scale*a.sx,a.scale);c.font=`500 ${font}px ${family}`;c.textAlign='center';c.textBaseline='middle';c.direction=s.lang==='ar'?'rtl':'ltr';c.globalAlpha=a.alpha;if(s.revealMode==='character'&&a.active){c.beginPath();const span=a.width*s.progress;c.rect(s.lang==='ar'?a.width/2-span:-a.width/2,-font,span,font*2);c.clip()}
  if([6,11,27].includes(mode)){c.fillStyle=soft;c.globalAlpha=a.alpha*.82;c.fillRect(-a.width/2-10,-font*.9,a.width+20,font*1.8);c.strokeStyle=accent;c.lineWidth=.6;c.strokeRect(-a.width/2-10,-font*.9,a.width+20,font*1.8);c.globalAlpha=a.alpha;}
  if([9,10,12].includes(mode)){c.fillStyle=a.active?accent:soft;c.globalAlpha=a.alpha*.15;polygon(c,[[-a.width/2-15,-font*.75],[a.width/2+17,-font*.95],[a.width/2+8,font*.7],[-a.width/2-22,font*.9]],true,false);c.globalAlpha=a.alpha;}
  if(mode===13){c.strokeStyle=accent;c.globalAlpha=a.alpha*.4;polygon(c,[[-a.width/2-10,0],[0,-font],[a.width/2+10,0],[0,font]],false,true);c.globalAlpha=a.alpha;}
  c.fillStyle=a.active&&s.hot?accent:ink;c.fillText(a.text,0,0,Math.max(20,(w-32)/Math.max(1,a.scale*Math.abs(a.sx))));
  if(mode===0&&!reduced.matches){c.save();c.translate(0,font*1.15);c.scale(1,-.5);c.globalAlpha=a.alpha*.14;c.fillStyle=second;c.fillText(a.text,0,0,Math.max(20,(w-32)/Math.max(1,a.scale*Math.abs(a.sx))));c.restore();}
  if(mode===20&&a.active){c.fillStyle=ink;c.fillText(a.text,0,0,Math.max(20,(w-32)/Math.max(1,a.scale*Math.abs(a.sx))));c.beginPath();const reveal=a.width*ease(q);c.rect(s.lang==='ar'?a.width/2-reveal:-a.width/2,-font,reveal,font*2);c.clip();c.fillStyle=accent;c.fillText(a.text,0,0,Math.max(20,(w-32)/Math.max(1,a.scale*Math.abs(a.sx))));}
  if(mode===25&&a.active){c.strokeStyle=second;c.globalAlpha=.3;c.lineWidth=1;c.beginPath();c.moveTo(-a.width/2-18,font*.65);c.quadraticCurveTo(0,font*.9,a.width/2+12,font*.6);c.stroke();}
  c.restore();
 }
}

return (s,env)=>{reduced.matches=s.reduced;words=s.words.map(w=>w.text);starts=s.starts;durations=s.words.map(w=>w.duration);lines=env.lines;pref.textScale=s.textScale;prefs.font=s.textScale;paint(s,s.dt)};})();
const studio=(()=>{let viewState,drawIndex=0,ctx,lines=[],cache={},mode=1,dur=3,tm=0,pulse=1,fontFamily="Segoe UI",progress=()=>0,hot=()=>0,heat=()=>0;const W=720,H=1280;const cl=(x,a=0,b=1)=>Math.min(b,Math.max(a,x));
const E={out:t=>1-Math.pow(1-t,3),io:t=>t<.5?4*t*t*t:1-Math.pow(-2*t+2,3)/2,expo:t=>t>=1?1:1-Math.pow(2,-10*t),back:t=>1+2.70158*Math.pow(t-1,3)+1.70158*Math.pow(t-1,2)};
const rng=s=>()=>(s=(s*9301+49297)%233280)/233280;
const G=[246,211,58],C=[244,241,230],mix=h=>`rgb(${C[0]+(G[0]-C[0])*h|0},${C[1]+(G[1]-C[1])*h|0},${C[2]+(G[2]-C[2])*h|0})`;
const F=s=>`700 ${s}px ${fontFamily}`;
function drawText(value,x,y,max){ctx.save();if(viewState.revealMode==='character'&&drawIndex===viewState.index){let width=ctx.measureText(value).width;if(Number.isFinite(max))width=Math.min(width,max);const left=ctx.textAlign==='center'?x-width/2:ctx.textAlign==='right'?x-width:x;const p=viewState.progress,h=Number(/([\d.]+)px/.exec(ctx.font)?.[1]||80);ctx.beginPath();ctx.rect(viewState.lang==='ar'?left+width*(1-p):left,y-h*2,width*p,h*4);ctx.clip()}if(max===undefined)ctx.fillText(value,x,y);else ctx.fillText(value,x,y,max);ctx.restore()}
function lay(i,size){const k=i+'_'+size;if(cache[k])return cache[k];const L=lines[i];ctx.font=F(size);const gap=size*.3,mx=560;
 const ws=L.words.map((w,j)=>({w,k:j,wd:ctx.measureText(w).width}));let rows=[],row=[],rw=0;
 for(const o of ws){const need=row.length?rw+gap+o.wd:o.wd;if(need>mx&&row.length){rows.push({row,rw});row=[o];rw=o.wd}else{row.push(o);rw=need}}
 rows.push({row,rw});const lh=size*1.25,y0=H*.5-(rows.length-1)*lh/2,out=[];
 rows.forEach((r,ri)=>{let x=(W-r.rw)/2;(L.rtl?r.row.slice().reverse():r.row).forEach(o=>{out.push({...o,x:x+o.wd/2,y:y0+ri*lh,rtl:L.rtl});x+=o.wd+gap})});
 return cache[k]=out}
function letters(o,s){return [{c:o.w,x:0,i:0}]}
function star(x,y,r,rot){ctx.save();ctx.translate(x,y);ctx.rotate(rot);ctx.beginPath();for(let i=0;i<8;i++){const a=i*Math.PI/4,q=i%2?r*.28:r;ctx.lineTo(Math.cos(a)*q,Math.sin(a)*q)}ctx.closePath();ctx.fill();ctx.restore()}
// constellation (line-level)
function cons(i,u,st){const L=lines[i],k0='c'+i;if(!cache[k0]){const r=rng(i*77+5),p=[];L.words.forEach((_,k)=>{let q,t=0;do{q=[110+r()*500,430+r()*440]}while(k&&Math.hypot(q[0]-p[k-1][0],q[1]-p[k-1][1])<150&&++t<8);p.push(q)});cache[k0]=p}
 const P=cache[k0];ctx.textAlign='center';ctx.lineWidth=1.6;
 L.words.forEach((w,k)=>{drawIndex=k;const u0=.1+k*st,p=progress(k);if(p<=0)return;const hot=heat(k),[x,y]=P[k];
  if(k){const a=P[k-1],e=E.out(p);ctx.strokeStyle='rgba(246,211,58,.5)';ctx.beginPath();ctx.moveTo(a[0],a[1]);ctx.lineTo(a[0]+(x-a[0])*e,a[1]+(y-a[1])*e);ctx.stroke()}
  ctx.save();ctx.fillStyle='rgb(246,211,58)';ctx.shadowColor='#f6d33a';ctx.shadowBlur=14;star(x,y,(8+10*hot)*E.back(p),u*1.5+k);
  ctx.shadowBlur=hot*26;const s=40+30*hot*E.back(p);ctx.font=F(s);ctx.fillStyle=mix(hot);ctx.globalAlpha=E.out(p)*(.55+.45*hot+.2);ctx.direction=L.rtl?'rtl':'ltr';drawText(w,x,y-s*.7);ctx.restore()})}
// 10 constellation variants
const rp=(k,n,r,P)=>{let q,t=0;do{q=[110+r()*500,430+r()*440]}while(k&&Math.hypot(q[0]-P[k-1][0],q[1]-P[k-1][1])<150&&++t<8);return q};
const CV=[
{pos:(k,n)=>{const a=-1.57+k*6.283/Math.max(n,5);return[360+Math.cos(a)*200,640+Math.sin(a)*200]},mk:'dt',ln:'c',lab:'r',orb:1},
{pos:rp,mk:'st',ln:'s',lab:'a',web:1},
{pos:k=>{const a=k*.95,d=40+k*30;return[360+Math.cos(a)*d,640+Math.sin(a)*d*.9]},mk:'dt',ln:'c',lab:'r'},
{pos:rp,mk:'rg',ln:'p',lab:'b',rip:1},
{pos:(k,n,r)=>{const t=n>1?k/(n-1):.5;return[120+t*480+(r()-.5)*40,430+t*420+(r()-.5)*60]},mk:'dm',ln:'s',lab:'b',glow:1},
{pos:k=>{const c=(k*5)%12;return[150+c%3*210,430+(c/3|0)*130]},mk:'pl',ln:'o',lab:'a'},
{pos:(k,n)=>[k%2?520:200,430+k*(430/Math.max(n-1,3))],mk:'dm',ln:'s',lab:'s',glow:1},
{pos:(k,n,r)=>{const a=k*6.283/n+r()*.4,d=140+r()*130;return[360+Math.cos(a)*d,640+Math.sin(a)*d*1.05]},mk:'st',ln:'s',lab:'r',burst:1},
{pos:(k,n)=>[100+k*(520/Math.max(n-1,4)),640+Math.sin(k*1.1)*160],mk:'dt',ln:'c',lab:'x'},
{pos:(k,n,r)=>{const c=k%3;return[[200,500,350][c]+(r()-.5)*150,[520,560,800][c]+(r()-.5)*130]},mk:'pl',ln:'d',lab:'a',rip:1}];
const at=(a,b,ty,t)=>{if(ty=='c'){const c=[(a[0]+b[0])/2-(b[1]-a[1])*.22,(a[1]+b[1])/2+(b[0]-a[0])*.22],m=1-t;return[m*m*a[0]+2*m*t*c[0]+t*t*b[0],m*m*a[1]+2*m*t*c[1]+t*t*b[1]]}
 if(ty=='o'){const c=[b[0],a[1]];return t<.5?[a[0]+(c[0]-a[0])*t*2,a[1]]:[c[0],c[1]+(b[1]-c[1])*(t-.5)*2]}
 return[a[0]+(b[0]-a[0])*t,a[1]+(b[1]-a[1])*t]};
function mk(t,x,y,r,rot){if(t=='st')return star(x,y,r,rot);ctx.save();ctx.translate(x,y);ctx.rotate(rot);ctx.lineWidth=2;ctx.strokeStyle='#f6d33a';
 if(t=='dt'){ctx.beginPath();ctx.arc(0,0,r*.6,0,7);ctx.fill()}
 else if(t=='rg'){ctx.beginPath();ctx.arc(0,0,r*.8,0,7);ctx.stroke()}
 else if(t=='dm'){ctx.rotate(.785);ctx.fillRect(-r*.6,-r*.6,r*1.2,r*1.2)}
 else{ctx.fillRect(-r,-1.5,r*2,3);ctx.fillRect(-1.5,-r,3,r*2)}
 ctx.restore()}
function consV(i,u,st,vi){const V=CV[vi],L=lines[i],n=L.words.length,kk='v'+i+'_'+vi;
 if(!cache[kk]){const r=rng(i*77+5),P=[];for(let k=0;k<n;k++)P.push(V.pos(k,n,r,P));cache[kk]=P}
 const Q=cache[kk].map((q,k)=>{let x=q[0]+Math.sin(u*1.3+k)*6,y=q[1]+Math.cos(u*1.1+k*2)*6;if(V.orb){const a=u*.1,dx=x-360,dy=y-640;x=360+dx*Math.cos(a)-dy*Math.sin(a);y=640+dx*Math.sin(a)+dy*Math.cos(a)}return[x,y]});
 ctx.save();ctx.lineWidth=1.5;ctx.lineCap='round';
 if(V.orb){ctx.strokeStyle='rgba(246,211,58,.16)';ctx.beginPath();ctx.arc(360,640,200,0,7);ctx.stroke()}
 for(let k=0;k<n;k++){drawIndex=k;const u0=.1+k*st,p=progress(k);if(p<=0)continue;const hot=heat(k),[x,y]=Q[k],e=E.out(p);
  const src=V.burst?[[360,640]]:V.web?[k-1,k-2,k-3].filter(j=>j>=0).map(j=>Q[j]):k?[Q[k-1]]:[];
  src.forEach((a,j)=>{ctx.save();ctx.strokeStyle=`rgba(246,211,58,${(j?.22:.5)+hot*.3})`;if(V.glow){ctx.shadowColor='#f6d33a';ctx.shadowBlur=14*hot;ctx.lineWidth=1.5+2*hot}
   ctx.setLineDash(V.ln=='d'?[8,8]:V.ln=='p'?[1,8]:[]);ctx.beginPath();for(let s=0;s<=20;s++){if(s/20>e)break;const q=at(a,[x,y],V.ln,s/20);s?ctx.lineTo(q[0],q[1]):ctx.moveTo(q[0],q[1])}ctx.stroke();ctx.restore()});
  ctx.save();ctx.fillStyle='#f6d33a';ctx.shadowColor='#f6d33a';ctx.shadowBlur=14;
  if(V.burst&&!k)star(360,640,16*e,u);
  mk(V.mk,x,y,(8+10*hot)*E.back(p),u*1.2+k);
  if(V.rip){const q=progress(k);ctx.shadowBlur=0;ctx.globalAlpha=1-q;ctx.strokeStyle='#f6d33a';ctx.lineWidth=2;ctx.beginPath();ctx.arc(x,y,10+q*70,0,7);ctx.stroke();ctx.globalAlpha=1}
  const s=38+28*hot*E.back(p),wd0=L.words[k];ctx.font=F(s);ctx.shadowBlur=hot*26;ctx.fillStyle=mix(hot);ctx.globalAlpha=E.out(p)*(.75+.25*hot);ctx.direction=L.rtl?'rtl':'ltr';
  const w=ctx.measureText(wd0).width,dx=x-360,dy=y-640,dl=Math.hypot(dx,dy)||1;let lx=x,ly=y,al='center';
  if(V.lab=='a')ly=y-s*.7;else if(V.lab=='b')ly=y+s*.85;else if(V.lab=='x')ly=y+(k%2?s*.85:-s*.7);
  else if(V.lab=='r'){lx=x+dx/dl*(w/2+14);ly=y+dy/dl*(s*.5+10)}
  else{al=x>360?'right':'left';lx=x+(x>360?-24:24)}
  if(al=='center')lx=cl(lx,50+w/2,670-w/2);
  ctx.textAlign=al;drawText(wd0,lx,ly);ctx.restore()}
 ctx.restore()}
// word-level modes
function wd(m,o,p,hot,u,s){drawIndex=o.k;const e=E.out(p),{x,y,wd:w}=o;ctx.save();ctx.font=F(s);ctx.textAlign='center';ctx.fillStyle=mix(hot);ctx.shadowColor='#f6d33a';ctx.shadowBlur=hot*22;
 if(m==1){ctx.globalAlpha=e;ctx.shadowBlur=(1-p)*40+hot*22;drawText(o.w,x,y+(1-e)*70)}
 else if(m==2){ctx.save();ctx.beginPath();ctx.rect(o.rtl?x+w/2-w*p:x-w/2,y-s,w*p,s*2);ctx.clip();drawText(o.w,x,y);ctx.restore()}
 else if(m==3){ctx.textAlign='left';ctx.globalAlpha=e;letters(o,s).forEach(c=>drawText(c.c,x-w/2+c.x,y+Math.sin(u*6+c.i*.7+o.k)*14))}
 else if(m==4){const k=1+(1-e)*2.6;ctx.translate(x,y);ctx.scale(k,k);ctx.globalAlpha=Math.min(1,p*3);drawText(o.w,Math.sin(p*38)*(1-p)*6,0)}
 else if(m==5){const q=E.io(p);ctx.beginPath();ctx.rect(x-w/2-6,y-s,(w+12)*q,s*2);ctx.clip();drawText(o.w,x,y);if(p<1){ctx.fillStyle='#f6d33a';ctx.fillRect(x-w/2-6+(w+12)*q-3,y-s*.55,4,s*1.1)}}
 else if(m==6){ctx.translate(x,y+(1-p)*30);ctx.scale(1,Math.max(.001,E.back(p)));ctx.globalAlpha=Math.min(1,p*2.5);drawText(o.w,0,0)}
 else if(m==7){ctx.textAlign='left';const q=E.expo(p);letters(o,s).forEach(c=>{const r=rng(c.i*7+o.k*13+3),sx=(r()-.5)*520,sy=(r()-.5)*520,rt=(r()-.5)*3;ctx.save();ctx.translate(x-w/2+c.x+(1-q)*sx,y+(1-q)*sy);ctx.rotate((1-q)*rt);ctx.globalAlpha=q;drawText(c.c,0,0);ctx.restore()})}
 else if(m==8){const q=E.io(p);ctx.shadowBlur=0;ctx.fillStyle='#5b574a';drawText(o.w,x,y);ctx.beginPath();o.rtl?ctx.rect(x+w/2-w*q,y-s,w*q,s*2):ctx.rect(x-w/2,y-s,w*q,s*2);ctx.clip();ctx.fillStyle='#f6d33a';ctx.shadowBlur=hot*22;ctx.shadowColor='#f6d33a';drawText(o.w,x,y)}
 else if(m==9){const j=(1-p)*22,r=rng(Math.floor(u*30)+o.k);if(p<1){ctx.globalCompositeOperation='lighter';ctx.globalAlpha=.75;ctx.fillStyle='rgb(255,50,90)';drawText(o.w,x-j*r(),y+(r()-.5)*j);ctx.fillStyle='rgb(40,220,255)';drawText(o.w,x+j*r(),y+(r()-.5)*j);ctx.globalCompositeOperation='source-over'}ctx.globalAlpha=Math.min(1,p*4);ctx.fillStyle=mix(hot);drawText(o.w,x,y)}
 ctx.restore()}
function drawLine(i,u,outP){const L=lines[i];if(!L)return;const m=mode?mode-1:i%20,n=L.words.length,st=Math.min(.3,dur*.5/n);
 ctx.save();if(outP>0){ctx.globalAlpha=1-E.io(outP);ctx.translate(0,-outP*60)}
 if(m==0)cons(i,u,st);else if(m>9)consV(i,u,st,m-10);else{const s=n>4?64:80;for(const o of lay(i,s)){const u0=.1+o.k*st,p=progress(o.k);if(p>0)wd(m,o,p,hot(o.k),u,s)}}
 ctx.restore()}
return(s)=>{viewState=s;ctx=s.c;cache=s.drawCache;mode=s.mode+1;tm=s.time/1000;fontFamily=s.fontFamily;lines=[{t:s.words.map(w=>w.text).join(' '),words:s.words.map(w=>w.text),rtl:s.lang==='ar'}];progress=k=>s.revealMode==='line'?1:s.words[k].unit.start===null?1:s.time<s.starts[k]?0:cl((s.time-s.starts[k])/s.words[k].duration);heat=hot=k=>s.hot&&k===s.index?Math.sin(s.progress*Math.PI):0;ctx.save();const scale=Math.min(s.w/720,s.h/620);ctx.translate((s.w-720*scale)/2,(s.h-620*scale)/2);ctx.scale(scale,scale);ctx.translate(0,-330);const st=Math.min(.3,dur*.5/Math.max(1,s.words.length));drawLine(0,.1+(s.index+s.progress)*st,0);ctx.restore()};})();
window.MadarMotionDraw={original,flow,games,studio};
window.MadarMotionPresets=[{"id": "motion-original-0", "name": "مدار مغناطيسي", "group": "original", "mode": 0}, {"id": "motion-original-1", "name": "دوّامة الكلمات", "group": "original", "mode": 1}, {"id": "motion-original-2", "name": "نفق زمني", "group": "original", "mode": 2}, {"id": "motion-original-3", "name": "وتر مطاطي", "group": "original", "mode": 3}, {"id": "motion-original-4", "name": "عدسة سائلة", "group": "original", "mode": 4}, {"id": "motion-original-5", "name": "شريط حلزوني", "group": "original", "mode": 5}, {"id": "motion-original-6", "name": "كوكبة كلمات", "group": "original", "mode": 6}, {"id": "motion-original-7", "name": "قصاصات إيقاعية", "group": "original", "mode": 7}, {"id": "motion-original-8", "name": "صدى متشظي", "group": "original", "mode": 8}, {"id": "motion-original-9", "name": "جاذبية معكوسة", "group": "original", "mode": 9}, {"id": "motion-flow-0", "name": "موجة حرير", "group": "flow", "mode": 0}, {"id": "motion-flow-1", "name": "مسار لانهائي", "group": "flow", "mode": 1}, {"id": "motion-flow-2", "name": "قوس ضوئي", "group": "flow", "mode": 2}, {"id": "motion-flow-3", "name": "رفرفة ورق", "group": "flow", "mode": 3}, {"id": "motion-flow-4", "name": "سُلّم انسيابي", "group": "flow", "mode": 4}, {"id": "motion-flow-5", "name": "بوابة مزدوجة", "group": "flow", "mode": 5}, {"id": "motion-flow-6", "name": "ستارة مخملية", "group": "flow", "mode": 6}, {"id": "motion-flow-7", "name": "مدّ وجزر", "group": "flow", "mode": 7}, {"id": "motion-flow-8", "name": "دومينو هوائي", "group": "flow", "mode": 8}, {"id": "motion-flow-9", "name": "نَفَس زجاجي", "group": "flow", "mode": 9}, {"id": "motion-flow-10", "name": "أكورديون مرن", "group": "flow", "mode": 10}, {"id": "motion-flow-11", "name": "بندول متزن", "group": "flow", "mode": 11}, {"id": "motion-flow-12", "name": "رحلة بيزيه", "group": "flow", "mode": 12}, {"id": "motion-flow-13", "name": "شلال مزدوج", "group": "flow", "mode": 13}, {"id": "motion-flow-14", "name": "مروحة كلمات", "group": "flow", "mode": 14}, {"id": "motion-flow-15", "name": "رفوف عائمة", "group": "flow", "mode": 15}, {"id": "motion-flow-16", "name": "وشاح ضوئي", "group": "flow", "mode": 16}, {"id": "motion-flow-17", "name": "طبقات زجاج", "group": "flow", "mode": 17}, {"id": "motion-flow-18", "name": "حبر متدفق", "group": "flow", "mode": 18}, {"id": "motion-flow-19", "name": "زوم تنفّسي", "group": "flow", "mode": 19}, {"id": "motion-games-0", "name": "P1 · مرآة النفس", "group": "games", "mode": 0}, {"id": "motion-games-1", "name": "P1 · شبكة الإدراك", "group": "games", "mode": 1}, {"id": "motion-games-2", "name": "P2 IS · جناح الفراشة", "group": "games", "mode": 2}, {"id": "motion-games-3", "name": "P2 EP · عقارب المصير", "group": "games", "mode": 3}, {"id": "motion-games-4", "name": "P3 Reload · قطع الماء", "group": "games", "mode": 4}, {"id": "motion-games-5", "name": "P3 FES · ساعة الظل", "group": "games", "mode": 5}, {"id": "motion-games-6", "name": "P3 Portable · ورق المخمل", "group": "games", "mode": 6}, {"id": "motion-games-7", "name": "P4 · قناة منتصف الليل", "group": "games", "mode": 7}, {"id": "motion-games-8", "name": "P4 Golden · شمس الكلمات", "group": "games", "mode": 8}, {"id": "motion-games-9", "name": "P4 Arena · سلسلة المواجهة", "group": "games", "mode": 9}, {"id": "motion-games-10", "name": "P5 · قصاصات اللص", "group": "games", "mode": 10}, {"id": "motion-games-11", "name": "P5 Royal · رزمة التاج", "group": "games", "mode": 11}, {"id": "motion-games-12", "name": "P5 Strikers · اندفاع متسلسل", "group": "games", "mode": 12}, {"id": "motion-games-13", "name": "P5 Tactica · مربعات التمرد", "group": "games", "mode": 13}, {"id": "motion-games-14", "name": "PQ وQ2 · متاهة السينما", "group": "games", "mode": 14}, {"id": "motion-games-15", "name": "Persona Dancing · أرضية الإيقاع", "group": "games", "mode": 15}, {"id": "motion-games-16", "name": "Bloodborne · فانوس الصياد", "group": "games", "mode": 16}, {"id": "motion-games-17", "name": "Bloodborne · أقواس يارنام", "group": "games", "mode": 17}, {"id": "motion-games-18", "name": "Bloodborne · قمر قرمزي", "group": "games", "mode": 18}, {"id": "motion-games-19", "name": "Bloodborne · نقش البصيرة", "group": "games", "mode": 19}, {"id": "motion-games-20", "name": "Bloodborne · حبر العهد", "group": "games", "mode": 20}, {"id": "motion-games-21", "name": "Bloodborne · بوابة الضباب", "group": "games", "mode": 21}, {"id": "motion-games-22", "name": "Bloodborne · بتلات الحلم", "group": "games", "mode": 22}, {"id": "motion-games-23", "name": "KH I · زجاج البدايات", "group": "games", "mode": 23}, {"id": "motion-games-24", "name": "KH II · دوّار الأوامر", "group": "games", "mode": 24}, {"id": "motion-games-25", "name": "KH III · مسارات الضوء", "group": "games", "mode": 25}, {"id": "motion-games-26", "name": "Birth by Sleep · بوصلة الوصل", "group": "games", "mode": 26}, {"id": "motion-games-27", "name": "Chain of Memories · رزمة الذكريات", "group": "games", "mode": 27}, {"id": "motion-games-28", "name": "Dream Drop Distance · غوص الحلم", "group": "games", "mode": 28}, {"id": "motion-games-29", "name": "358/2 Days · شظايا الأيام", "group": "games", "mode": 29}, {"id": "motion-studio-0", "name": "Constellation", "group": "studio", "mode": 0}, {"id": "motion-studio-1", "name": "Blur Rise", "group": "studio", "mode": 1}, {"id": "motion-studio-2", "name": "Typewriter", "group": "studio", "mode": 2}, {"id": "motion-studio-3", "name": "Wave", "group": "studio", "mode": 3}, {"id": "motion-studio-4", "name": "Punch", "group": "studio", "mode": 4}, {"id": "motion-studio-5", "name": "Wipe", "group": "studio", "mode": 5}, {"id": "motion-studio-6", "name": "Flip", "group": "studio", "mode": 6}, {"id": "motion-studio-7", "name": "Scatter", "group": "studio", "mode": 7}, {"id": "motion-studio-8", "name": "Karaoke", "group": "studio", "mode": 8}, {"id": "motion-studio-9", "name": "Glitch", "group": "studio", "mode": 9}, {"id": "motion-studio-10", "name": "Orbit", "group": "studio", "mode": 10}, {"id": "motion-studio-11", "name": "Web", "group": "studio", "mode": 11}, {"id": "motion-studio-12", "name": "Spiral", "group": "studio", "mode": 12}, {"id": "motion-studio-13", "name": "Rings", "group": "studio", "mode": 13}, {"id": "motion-studio-14", "name": "Comet", "group": "studio", "mode": 14}, {"id": "motion-studio-15", "name": "Grid", "group": "studio", "mode": 15}, {"id": "motion-studio-16", "name": "Zigzag", "group": "studio", "mode": 16}, {"id": "motion-studio-17", "name": "Burst", "group": "studio", "mode": 17}, {"id": "motion-studio-18", "name": "Wave Line", "group": "studio", "mode": 18}, {"id": "motion-studio-19", "name": "Cluster", "group": "studio", "mode": 19}];
})();
