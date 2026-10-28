package main

import (
 "context"
 "fmt"
 "strconv"
 "strings"
)

func permissionHelp(cfg Config)string {
 return fmt.Sprintf("НАСТРОЙКА ВК:\n1. Сообщество → Управление → Сообщения → включить сообщения.\n2. Настройки для бота → возможности ботов и добавление в беседы.\n3. Работа с API → ключ сообщества с правами «Сообщения» (ввести через sudo aibot, никому не отправлять).\n4. Bots Long Poll API → включить message_new И message_event.\n5. Администратор каждой беседы выдаёт сообществу «Доступ ко всей переписке» либо права администратора.\n6. В группе упомяните [club%d|AI-bot] и спросите «А как ты думаешь?».\nБот не видит историю до вступления и то, к чему ВК не дал доступ.",cfg.GroupID)
}
func helpText()string {
 return "AI-bot — выбор через кнопки под сообщением.\n"+
 "• меню, модели, модель 2, настройки\n• поиск авто/вкл/выкл\n"+
 "• стиль кратко/обычно/подробно\n• контекст 20\n• резерв выкл/вкл/авто\n"+
 "• сброс (личные настройки)\n\nАдминистратор беседы: "+
 "диалог, диалог модель 1, диалог поиск авто, диалог стиль кратко, диалог контекст 20, диалог резерв выкл, диалог сброс.\n"+
 "Чтобы обратиться без слова «бот», упомяните сообщество ВК."
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
 label:=pref.Model;if label==""{label="основная модель сервера"}else{label=c.label(label)}
 return fmt.Sprintf("Настройки беседы:\nМодель: %s\nПоиск: %s\nСтиль: %s\nКонтекст: %d\nРезерв: %s\nИзменять может только администратор беседы.",
 label,choose(pref.Search,"авто"),choose(pref.Style,"обычно"),pref.Context,choose(pref.Fallback,"наследование"))
}
func personalInfo(s *Store,c *Catalog,cfg Config,peer,user int64)string{
 key,search,style,count,fallback:=s.effective(cfg,peer,user,c);model:="не настроена";if key!=""{model=c.label(key)}
 res:="выключен";if fallback{res="разрешён"}
 return fmt.Sprintf("Ваши настройки:\nМодель: %s\nПоиск: %s\nСтиль: %s\nКонтекст: %d сообщений\nРезерв: %s",model,search,style,count,res)
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
