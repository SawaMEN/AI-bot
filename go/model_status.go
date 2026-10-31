package main

import (
 "fmt"
 "strings"
)

// A configured API provider is not the same as an authenticated, Russian-
// capable model. Explain the difference in VK without leaking credentials.
func modelAvailabilityExplanation(c *Catalog,cfg Config)string{
 active:=0
 for id,settings:=range cfg.Providers{
  spec,ok:=c.Specs[id]
  if ok&&settings.Enabled&&(settings.APIKey!=""||spec.AnonymousKey!=""){active++}
 }
 if active==0{
  return "ИИ-провайдер пока не подключён. Администратор сервера: sudo aibot setup."
 }
 candidates:=c.candidates(cfg)
 if len(candidates)==0{
  return fmt.Sprintf("Подключено ИИ-сервисов: %d, но список бесплатных моделей пуст. Проверьте каталог провайдера и автоподбор: sudo aibot check-ai.",active)
 }
 if c.health==nil{
  return "Для этих моделей ещё не подтверждена доступность. Администратор сервера: sudo aibot check-ai."
 }
 pending,failed,english,expired:=0,0,0,0
 for _,key:=range candidates{
  status,ok:=c.health.lookup(cfg,key)
  if !ok{pending++;continue}
  if status.Passed{
   expired++
   continue
  }
  if status.Reason=="russian_unsupported"{english++}else{failed++}
 }
 var b strings.Builder
 fmt.Fprintf(&b,"Сервисов: %d • моделей на проверке: %d.",active,len(candidates))
 if pending>0{
  fmt.Fprintf(&b," Ещё не проверено: %d. Бот проверяет API и русский язык автоматически.",pending)
 }
 if failed>0{
  fmt.Fprintf(&b," Ошибки API, квоты или ответов: %d.",failed)
 }
 if english>0{
  fmt.Fprintf(&b," Не подтвердили русский язык: %d.",english)
 }
 if expired>0{
  fmt.Fprintf(&b," Требуют повторной проверки: %d.",expired)
 }
 b.WriteString("\n\nПРИЧИНЫ ПО МОДЕЛЯМ:")
 shown:=0
 for _,key:=range candidates{
  status,ok:=c.health.lookup(cfg,key)
  if !ok{
   if shown<6{fmt.Fprintf(&b,"\n• %s: ещё не проверена",c.label(key));shown++}
   continue
  }
  if status.Passed{continue}
  if shown<6{
   fmt.Fprintf(&b,"\n• %s: %s",c.label(key),modelFailureLabel(status.Reason))
   shown++
  }
 }
 if total:=pending+failed+english;total>shown{
  fmt.Fprintf(&b,"\n…и ещё %d моделей.",total-shown)
 }
 b.WriteString("\n\nНа VDS: sudo aibot check-ai — проверить повторно и увидеть причины; sudo aibot logs — журнал.")
 return b.String()
}
