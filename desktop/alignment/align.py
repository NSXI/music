"""Madar's local forced-alignment worker. No remote audio requests."""
import sys,os,json,re,statistics,traceback
from quality import audit
if hasattr(sys.stdout,'reconfigure'):sys.stdout.reconfigure(encoding='utf-8')
if hasattr(sys.stderr,'reconfigure'):sys.stderr.reconfigure(encoding='utf-8',errors='backslashreplace')
os.environ.setdefault('NUMBA_DISABLE_JIT','1')
def emit(progress,phase):
 print(json.dumps({'progress':progress,'phase':phase},ensure_ascii=True),flush=True)
def main():
 request=json.load(open(sys.argv[1],encoding='utf-8'))
 os.environ['PATH']=os.path.dirname(request['ffmpeg'])+os.pathsep+os.environ.get('PATH','')
 import numpy as np
 import torch
 import whisper
 import stable_whisper
 torch.set_num_threads(min(6,max(1,int(os.environ.get('OMP_NUM_THREADS') or 2))))
 emit(.04,'تحميل نموذج المزامنة المحلي')
 model=stable_whisper.load_model(request['model'],device='cpu')
 emit(.12,'قراءة الصوت')
 waveform=whisper.load_audio(request['audio'])
 duration=len(waveform)/16000
 language=request.get('language') or 'auto'
 if language=='auto':
  if len(re.findall(r'[\u0600-\u06ff]',request['text']))>len(request['text'])*.18:language='ar'
  else:
   mel=whisper.log_mel_spectrogram(whisper.pad_or_trim(waveform),n_mels=model.dims.n_mels).to(model.device)
   _,probs=model.detect_language(mel);language=max(probs,key=probs.get)
 emit(.2,'مطابقة الكلمات مع الصوت')
 def progress(done,total):emit(.2+.40*min(1,done/max(total,1)),'مطابقة الكلمات مع الصوت')
 result=model.align(waveform,request['text'],language=language,original_split=True,regroup=False,verbose=None,suppress_silence=True,vad=False,remove_instant_words=False,progress_callback=progress)
 if result is None:raise RuntimeError('لم ينجح تحليل التوقيت')
 emit(.61,'تدقيق بداية كل كلمة ونهايتها من الصوت')
 refined=False;warning=''
 try:
  candidate=model.refine(waveform,result,steps='se',precision=.02,word_level=True,
   single_batch=True,inplace=False,rel_dur_change=.35,abs_dur_change=.25,verbose=None,
   progress_callback=lambda done,total:emit(.61+.33*min(1,done/max(total,1)),'تدقيق بداية كل كلمة ونهايتها من الصوت'))
  if candidate is None:raise RuntimeError('empty refinement')
  result=candidate;refined=True
 except Exception:
  warning='تعذّر التدقيق الثاني؛ احتُفظ بالمطابقة الأولية للمراجعة فقط.'
 lines=[];probabilities=[];valid=0;total=0;lrc=[]
 for segment in result.segments:
  words=[]
  for word in segment.words or []:
   start=max(0,float(word.start));end=min(duration,float(word.end));p=getattr(word,'probability',None)
   if p is not None:probabilities.append(float(p))
   total+=1
   if end-start>=.03 and end<=duration+.1:valid+=1
   words.append({'text':word.word,'start':start,'end':max(start,end),'probability':float(p) if p is not None else 0})
  text=segment.text.strip()
  if not text:continue
  start=max(0,min(duration,float(segment.start)));end=max(start,min(duration,float(segment.end)))
  lines.append({'text':text,'start':start,'end':end,'words':words})
  centis=round(start*100);minutes=centis//6000;seconds=(centis%6000)/100
  lrc.append(f'[{minutes:02}:{seconds:05.2f}]{text}')
 report=audit(lines,request['text'],duration,language,refined,warning)
 output={'lrc':'\n'.join(lrc)+'\n','lines':lines,'report':report}
 temp=request['output']+'.tmp'
 with open(temp,'w',encoding='utf-8') as f:json.dump(output,f,ensure_ascii=False,indent=2)
 os.replace(temp,request['output']);emit(1,'اكتمل تحليل التوقيت')
if __name__=='__main__':
 try:main()
 except Exception as e:print(str(e),file=sys.stderr);traceback.print_exc(file=sys.stderr);sys.exit(1)
