"""Deterministic checks; does not manufacture or distribute word timestamps."""
import math, statistics, unicodedata

def identity(text):
 return ''.join(unicodedata.normalize('NFC',text).split())

def audit(lines, original, duration, language, refined, warning=''):
 total=valid=overlaps=instant=low=long=0;previous=-1;probabilities=[];texts=[]
 for line in lines:
  texts.append(line['text'])
  for word in line['words']:
   total+=1;start,end=word['start'],word['end'];p=word.get('probability',0)
   sane=all(math.isfinite(v) for v in [start,end,p]) and 0<=start<end<=duration+.02
   if sane and end-start>=.02:valid+=1
   if not sane or end-start<.02:instant+=1
   if start<previous-.02:overlaps+=1
   previous=max(previous,end)
   if p<.15:low+=1
   if end-start>4:long+=1
   probabilities.append(p)
 matched=identity(''.join(texts))==identity(original) and all(identity(''.join(w['text'] for w in line['words']))==identity(line['text']) for line in lines)
 coverage=valid/max(total,1);mean=statistics.mean(probabilities) if probabilities else 0
 return dict(coverage=coverage,meanProbability=mean,words=total,validWords=valid,
  needsReview=not refined or not matched or coverage<.95 or mean<.35 or overlaps>0 or (low+long)>max(1,total*.15),
  language=language,model='Whisper base / stable-ts 2.19.1 · word refinement v2',
  refined=refined,resolutionSeconds=.02 if refined else .02,textMatched=matched,
  overlapWords=overlaps,instantWords=instant,lowConfidenceWords=low,longWords=long,
  characterTiming='visual-within-word',warning=warning,
  note='توقيت كلمات محلل من الصوت مع تدقيق الحدود. خطوة البحث 20 مللي ثانية وليست ضمانًا للدقة. الحروف حركة بصرية داخل الكلمة، وليست توقيت نطق مستقلًا.')
