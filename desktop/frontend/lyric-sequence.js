'use strict';
// Display-only timing plan. Estimated words never enter the provider/alignment cache.
(()=>{
const clean=s=>String(s||'').normalize('NFC').replace(/[\s\u200b\u200e\u200f\u2066-\u2069]/g,'');
const finite=Number.isFinite,segmenter=typeof Intl.Segmenter==='function'?new Intl.Segmenter(undefined,{granularity:'word'}):null;
function tokens(text){if(!segmenter)return String(text).match(/\S+/gu)||[];const out=[];for(const part of segmenter.segment(String(text))){if(!part.segment.trim())continue;if(!part.isWordLike&&out.length)out[out.length-1]+=part.segment;else out.push(part.segment)}return out}
function plan(lines,aligned=[],duration=0){
 const index=new Map();for(const row of aligned){if(!finite(row.start))continue;const key=clean(row.text||row.words?.map(w=>w.text).join(''));if(!index.has(key))index.set(key,[]);index.get(key).push(row)}for(const a of index.values())a.sort((x,y)=>x.start-y.start);
 const ends=[],rows=[];let next=null,group=null,greater=null,actual=0,estimated=0;
 for(let i=lines.length-1;i>=0;i--){const t=lines[i].time;if(finite(t)){if(group!==null&&t<group)greater=group;group=t;ends[i]=greater??(duration>t?duration:null)}else ends[i]=next;next=finite(t)?t:next}
 for(let i=0;i<lines.length;i++){
  const line=lines[i],start=line.time,end=ends[i];if(!finite(start))continue;
  const candidates=index.get(clean(line.text))||[];let lo=0,hi=candidates.length;while(lo<hi){const mid=(lo+hi)>>>1;if(candidates[mid].start<start)lo=mid+1;else hi=mid}
  const chosen=[candidates[lo],candidates[lo-1],candidates[lo+1]].filter(Boolean).sort((a,b)=>Math.abs(a.start-start)-Math.abs(b.start-start)).find(r=>{
   const w=r.words||[];return w.length&&clean(w.map(w=>w.text).join(''))===clean(line.text)&&w.every((x,j)=>finite(x.start)&&finite(x.end)&&x.end>x.start&&x.start>=start-.25&&(!finite(end)||x.start<end)&&(!j||x.start>=w[j-1].start));
  });
  if(chosen){rows.push({...chosen,start,origin:'source',words:chosen.words.map(w=>({...w,origin:'source'}))});actual++;continue}
  if(!finite(end)||end<=start)continue;const parts=tokens(line.text);if(!parts.length)continue;
  const weights=parts.map(w=>Math.sqrt(Math.min(16,[...w].length))+.65),sum=weights.reduce((a,b)=>a+b,0);let cursor=start;
  const words=parts.map((text,j)=>{const a=cursor;cursor=j===parts.length-1?end:cursor+(end-start)*weights[j]/sum;return {text,start:a,end:cursor,origin:'estimated'}});
  rows.push({text:line.text,start,end,words,origin:'estimated'});estimated++;
 }
 return {rows,actual,estimated,kind:estimated?(actual?'mixed-estimated':'estimated'):actual?'word':lines.some(l=>finite(l.time))?'line':'plain'};
}
window.NawaTiming={plan,tokens,clean};
})();
