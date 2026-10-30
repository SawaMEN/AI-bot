package main

import (
 "context"
 "fmt"
 "strconv"
 "strings"
)

func permissionHelp(cfg Config)string {
 return vkSetupGuide(cfg.GroupID)
}
func helpText()string {
 return "AI-bot — помощник в беседе ВКонтакте.\n"+
 "Бот работает от имени сообщества ВК (группы/паблика), которое добавлено в вашу беседу.\n\n"+
 "Как задать вопрос:\n"+
 "1. Упомяните сообщество в сообщении.\n"+
 "2. Напишите вопрос: «А как ты думаешь?»\n"+
 "3. Дождитесь ответа. Бот учитывает доступные ему сообщения беседы.\n\n"+
 "Команды и кнопки:\n"+
 "• «меню» — открыть кнопки управления\n"+
 "• «модели» — выбрать бесплатную модель\n"+
 "• «настройки» — личные параметры\n"+
 "• «поиск вкл/выкл/авто» — режим поиска\n"+
 "• «стиль кратко/обычно/подробно» — длина ответа\n"+
 "• «контекст 20» — сколько сообщений учитывать\n"+
 "• «резерв вкл/выкл» — другой бесплатный ИИ при ошибке\n\n"+
 "Администратор беседы может изменить общие настройки через «диалог».\n"+
 "Если бот молчит, откройте «права»: там подробная инструкция для ВК."
}
func readableSearch(v string)string{
 switch v{case "on":return "✓ ВКЛ";case "off":return "✗ ВЫКЛ";case "auto","":return "АВТО (по необходимости)";default:return v}
}
func readableFallback(v string)string{
 switch v{case "on":return "✓ ВКЛ";case "off":return "✗ ВЫКЛ";default:return "По настройке сервера"}
}
func readableStyle(v string)string{
 switch v{case "short":return "Кратко";case "detailed":return "Подробно";case "normal","":return "Обычно";default:return v}
}
func listModels(c *Catalog,cfg Config,page int,chat bool)string{
 allowed:=c.enabled(cfg);if len(allowed)==0{return "Пока нет доступных бесплатных моделей. Администратор сервера: sudo aibot"}
 maxPage:=(len(allowed)+3)/4;if page<1{page=1};if page>maxPage{page=maxPage}
 var b strings.Builder;fmt.Fprintf(&b,"Бесплатные модели (страница %d/%d):\n",page,maxPage)
 for i:=(page-1)*4;i<min(page*4,len(allowed));i++{fmt.Fprintf(&b,"%d. %s\n",i+1,c.label(allowed[i]))}
 if chat{b.WriteString("Выбор модели всей беседы: кнопка или «диалог модель 2».")
 }else{b.WriteString("Выберите кнопкой или напишите «модель 2».")}
 return b.String()
}
func chatInfo(s *Store,c *Catalog,cfg Config,peer int64)string{
 pref:=s.getPref("chat",peer)
 model:="основная модель сервера"
 if pref.Model!=""{model=c.label(pref.Model)}
 contextLabel:=fmt.Sprintf("по настройке сервера (до %d сообщений)",cfg.ContextMessages)
 if pref.Context>0{contextLabel=fmt.Sprintf("%d сообщений",pref.Context)}
 return fmt.Sprintf("НАСТРОЙКИ БЕСЕДЫ\nМодель: %s\nПоиск: %s\nСтиль: %s\nКонтекст: %s\nРезервный ИИ: %s\n\nИзменять общие настройки может только администратор беседы.",
  model,readableSearch(pref.Search),readableStyle(pref.Style),contextLabel,readableFallback(pref.Fallback))
}
func personalInfo(s *Store,c *Catalog,cfg Config,peer,user int64)string{
 key,search,style,count,fallback:=s.effective(cfg,peer,user,c)
 model:="не настроена";if key!=""{model=c.label(key)}
 return fmt.Sprintf("ВАШИ НАСТРОЙКИ\nМодель: %s\nПоиск: %s\nСтиль: %s\nКонтекст: %d сообщений\nРезервный ИИ: %s\n\nИзменить: нажмите кнопку под сообщением или напишите «меню».",
  model,readableSearch(search),readableStyle(style),count,switchLabel(fallback))
}
func cmdApply(s *Store,c *Catalog,cfg Config,scope string,ident int64,parts []string)string{
 if len(parts)==0{return "Откройте меню кнопками."}
 field:=strings.ToLower(parts[0])
 if field=="сброс"||field=="сбросить" {
  if len(parts)!=1{return "Использование: сброс"}
  if err:=s.resetPref(scope,ident);err!=nil{return "Ошибка SQLite"}
  return "Настройки сброшены."
 }
 if len(parts)!=2{return "Напишите «меню» или выберите нужное значение кнопкой."}
 value:=strings.ToLower(parts[1])
 var save any
 var result string
 switch field{
 case "модель":
  if value=="авто"||value=="сброс"{save=nil;result="Модель: по умолчанию";break}
  n,err:=strconv.Atoi(value);allowed:=c.enabled(cfg)
  if err!=nil||n<1||n>len(allowed){return "Нет такой разрешённой модели. Откройте «модели»."}
  save=allowed[n-1];result="Выбрана модель: "+c.label(allowed[n-1])
 case "поиск":
  switch value{case "авто":save="auto";case "вкл":save="on";case "выкл":save="off";default:return "Поиск: вкл / выкл / авто"}
  result="Настройки поиска сохранены."
 case "стиль":
  switch value{case "кратко":save="short";case "обычно":save="normal";case "подробно":save="detailed";default:return "Стиль: кратко / обычно / подробно"}
  result="Стиль ответа сохранён."
 case "контекст":
  n,err:=strconv.Atoi(value);if err!=nil||n<1||n>cfg.ContextMessages{return fmt.Sprintf("Контекст от 1 до %d сообщений.",cfg.ContextMessages)}
  save=n;result=fmt.Sprintf("Контекст: %d сообщений.",n)
 case "резерв":
  switch value{case "вкл":save="on";case "выкл":save="off";case "авто":save=nil;default:return "Резерв: вкл / выкл / авто"}
  result="Резервная бесплатная модель: "+value
 default:return "Неизвестная настройка. Откройте «меню»."
 }
 columns:=map[string]string{"модель":"model","поиск":"search","стиль":"style","контекст":"context_messages","резерв":"fallback"}
 if err:=s.setPref(scope,ident,columns[field],save);err!=nil{return "Ошибка сохранения настройки: "+err.Error()}
 return result
}
func processCommand(ctx context.Context,v *VK,s *Store,c *Catalog,cfg Config,peer,user int64,question string)(string,bool){
 args:=strings.Fields(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(question),"?")))
 if len(args)==0{return "Напишите вопрос или откройте меню.",true}
 first:=strings.ToLower(strings.Trim(args[0],",.:;!?"))
 if first=="меню"||first=="помощь"||first=="help"||first=="старт"{return helpText(),true}
 if first=="права"||first=="разрешения"||first=="подключение"{return permissionHelp(cfg),true}
 if first=="настройки"{return personalInfo(s,c,cfg,peer,user),true}
 if first=="модели"{
  page:=1;if len(args)>1{page,_=strconv.Atoi(args[1])};return listModels(c,cfg,page,false),true
 }
 if first=="диалог"{
  if peer<2000000000{return "Настройки беседы доступны только в групповых чатах ВК.",true}
  if len(args)==1{return chatInfo(s,c,cfg,peer),true}
  if args[1]=="модели"{page:=1;if len(args)>2{page,_=strconv.Atoi(args[2])};return listModels(c,cfg,page,true),true}
  if args[1]=="выбор"{return "Выберите значение кнопкой.",true}
  // Read-only menu doesn't require admin, mutations do.
  if !v.admin(ctx,peer,user){return "Настройки беседы может менять только администратор. Убедитесь, что ВК дал боту права проверять участников.",true}
  return cmdApply(s,c,cfg,"chat",peer,args[1:]),true
 }
 if first=="выбор"{return "Выберите значение кнопкой.",true}
 for _,cmd:=range []string{"модель","поиск","стиль","контекст","резерв","сброс","сбросить"}{
  if first==cmd{return cmdApply(s,c,cfg,"user",user,append([]string{first},args[1:]...)),true}
 }
 return "",false
}
