"""Madar's offline, line-preserving NLLB worker. No network or model code loading."""
import json, os, sys
from pathlib import Path

LANGS={'ar':'arb_Arab','en':'eng_Latn','fr':'fra_Latn','es':'spa_Latn','de':'deu_Latn','ru':'rus_Cyrl','ja':'jpn_Jpan','ko':'kor_Hang','zh':'zho_Hans','hi':'hin_Deva','tr':'tur_Latn','pt':'por_Latn','it':'ita_Latn','fa':'pes_Arab','ur':'urd_Arab','id':'ind_Latn','nl':'nld_Latn','uk':'ukr_Cyrl'}
def emit(progress,phase):
 print(json.dumps({'progress':progress,'phase':phase},ensure_ascii=True),flush=True)
def source_language(text,requested):
 if requested!='auto':
  if requested not in LANGS:raise ValueError('Unsupported source language')
  return requested
 from langdetect import detect_langs, DetectorFactory
 DetectorFactory.seed=0
 candidates=detect_langs(text)
 language=candidates[0].lang.split('-')[0]
 if language not in LANGS:raise ValueError('اختر لغة الكلمات الأصلية يدويًا؛ التعرف التلقائي غير واثق أو اللغة غير مدعومة')
 return language
def main():
 for stream in [sys.stdout,sys.stderr]:
  if hasattr(stream,'reconfigure'):stream.reconfigure(encoding='utf-8',errors='replace')
 request=json.loads(Path(sys.argv[1]).read_text(encoding='utf-8-sig'))
 lines=request['lines'];target=request['target']
 if target not in LANGS or not lines or len(lines)>3000:raise ValueError('Invalid translation request')
 source=source_language('\n'.join(lines),request.get('source','auto'))
 emit(.05,'تجهيز الترجمة المحلية')
 import ctranslate2
 from tokenizers import Tokenizer
 model=request['model'];tokenizer=Tokenizer.from_file(str(Path(model)/'tokenizer.json'))
 translator=None
 if source!=target:
  translator=ctranslate2.Translator(model,device='cpu',compute_type='int8',inter_threads=1,intra_threads=min(6,os.cpu_count() or 2))
 # Use the legacy NLLB sequence expected by this converted checkpoint, replacing
 # its unusable <unk> language placeholder explicitly. Never truncate a line.
 unique=list(dict.fromkeys(lines));translated={}
 for index,text in enumerate(unique):
  if source==target:result=text
  else:
   tokens=tokenizer.encode(text,add_special_tokens=False).tokens
   if len(tokens)>480:raise ValueError('سطر طويل جدًا؛ قسّمه إلى سطور أقصر قبل الترجمة')
   batch=translator.translate_batch([tokens+['</s>',LANGS[source]]],target_prefix=[[LANGS[target]]],beam_size=4,max_decoding_length=512,repetition_penalty=1.05,no_repeat_ngram_size=4,return_end_token=True)
   output=batch[0].hypotheses[0][1:]
   if '</s>' not in output:raise ValueError('ترجمة غير مكتملة؛ اختصر السطر وأعد المحاولة')
   ids=[tokenizer.token_to_id(t) for t in output if t!='</s>' and t!=LANGS[target]]
   result=tokenizer.decode(ids,skip_special_tokens=True).strip()
   if not result:raise ValueError('المحرك أعاد سطرًا فارغًا؛ احتُفظ بالأصل')
  translated[text]=result
  emit(.1+.88*(index+1)/len(unique),'ترجمة السطور وحفظ تطابقها')
 result={'source':source,'target':target,'lines':[translated[x] for x in lines],'engine':'NLLB-200 distilled 600M · int8','sameLanguage':source==target}
 output=Path(request['output']);temporary=output.with_suffix('.tmp')
 temporary.write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf8');temporary.replace(output)
 emit(1,'اكتملت الترجمة المحلية')
if __name__=='__main__':main()
