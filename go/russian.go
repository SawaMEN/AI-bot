package main

import (
 "context"
 "fmt"
 "io"
 "strings"
 "time"
 "unicode"
)

// A normal answer may legitimately consist of a number, code or a URL.
// Only quarantine clearly non-Russian prose when a Russian-language answer
// was requested. Do not reject translation requests to another language.
func wantsRussian(question string) bool {
 q:=strings.ToLower(question)
 if strings.Contains(q,"по-английски")||strings.Contains(q,"на английском")||
  strings.Contains(q,"на английский")||strings.Contains(q,"in english")||
  strings.Contains(q,"по-немецки")||strings.Contains(q,"на немецком"){return false}
 for _,r:=range q{if unicode.In(r,unicode.Cyrillic){return true}}
 return false
}
func nonRussianProse(answer string)bool {
 // Ignore fenced source code, inline code and URLs when checking prose.
 for {
  start:=strings.Index(answer,"```");if start<0{break}
  end:=strings.Index(answer[start+3:],"```")
  if end<0 {answer=answer[:start];break}
  answer=answer[:start]+answer[start+3+end+3:]
 }
 words:=strings.Fields(answer)
 var latin,cyrillic,other int
 for _,word:=range words {
  if strings.HasPrefix(word,"https://")||strings.HasPrefix(word,"http://"){continue}
  if strings.HasPrefix(word,"`"){continue}
  for _,r:=range word {
   if unicode.In(r,unicode.Cyrillic){cyrillic++
   }else if unicode.In(r,unicode.Latin){latin++
   }else if unicode.IsLetter(r){other++}
  }
 }
 return (cyrillic==0&&latin+other>=12) || (latin+other>40&&latin+other>6*cyrillic)
}
func russianProbeOK(answer string)bool {
 if !strings.Contains(strings.ToLower(answer),"москв"){return false}
 var cyr,latin int
 for _,r:=range answer{
  if unicode.In(r,unicode.Cyrillic){cyr++}
  if unicode.In(r,unicode.Latin){latin++}
 }
 return cyr>=5&&cyr*2>=latin
}
// Check connected models with real authenticated Russian prompts. An HTTP
// timeout, quota error or expired key is NOT evidence of a language failure.
// Only two successful but off-target language responses quarantine a model.
func checkRussianModels(ctx context.Context,cfg *Config,c *Catalog,out io.Writer)error{
 fmt.Fprintln(out,"Проверка моделей на понимание русского языка через их API...")
 problems:=c.refresh(ctx,*cfg)
 for provider,problem:=range problems{fmt.Fprintf(out,"Каталог %s: %s\n",provider,problem)}
 // Re-test previously quarantined models as well, to allow recovery
 // without asking the administrator to hand-edit the config.
 probeConfig:=*cfg
 probeConfig.RussianBlockedModels=nil
 keys:=c.candidates(probeConfig)
 if len(keys)==0{return fmt.Errorf("нет доступных моделей: проверьте ключи и включённые модели")}
 ai:=newAI(c)
 blocked:=0;okCount:=0;unavailable:=0;restored:=0;changed:=false
 for i,key:=range keys {
  if ctx.Err()!=nil{return ctx.Err()}
  fmt.Fprintf(out,"[%d/%d] %s ... ",i+1,len(keys),c.label(key))
  valid:=false;bad:=0;var failed error
  for _,prompt:=range []string{
   "Ответь одним коротким предложением по-русски: какой город является столицей России?",
   "Назови на русском языке столицу России. Не используй английский.",
  } {
   requestCtx,cancel:=context.WithTimeout(ctx,28*time.Second)
   result,err:=ai.generate(requestCtx,probeConfig,key,[]Message{
    {Role:"system",Content:"Тест понимания русского языка. Отвечай исключительно по-русски."},
    {Role:"user",Content:prompt},
   })
   cancel()
   if err!=nil{failed=err;break}
   if russianProbeOK(result){valid=true;break}
   bad++
  }
  switch {
  case valid:
   fmt.Fprintln(out,"✓ русский язык подтверждён")
   okCount++
   if contains(cfg.RussianBlockedModels,key){
    cfg.RussianBlockedModels=removeExact(cfg.RussianBlockedModels,key)
    restored++;changed=true
   }
  case failed!=nil:
   fmt.Fprintf(out,"? API недоступен (%v), модель не удалена\n",failed)
   unavailable++
  case bad==2:
   fmt.Fprintln(out,"✗ не прошла две проверки русского языка — исключена из выбора")
   if !contains(cfg.RussianBlockedModels,key){
    cfg.RussianBlockedModels=append(cfg.RussianBlockedModels,key)
    blocked++;changed=true
   }
  }
 }
 if changed{
  cfg.DefaultModel=c.defaultModel(*cfg)
  if err:=saveConfig(*cfg);err!=nil{return fmt.Errorf("не удалось сохранить исключения: %w",err)}
 }
 fmt.Fprintf(out,"Результат: подтвердили русский — %d, исключены — %d, восстановлены — %d, временно недоступны — %d.\n",okCount,blocked,restored,unavailable)
 if changed{fmt.Fprintln(out,"Чтобы применить исключения к работающему боту: sudo aibot restart")}
 return nil
}
