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
 return "AI-bot — помощник в беседе ВКонтакте.\n\n"+
  "КАК ПОЛЬЗОВАТЬСЯ\n"+
  "• В беседе упомяните сообщество и задайте вопрос.\n"+
  "• Для продолжения отвечайте на сообщение бота.\n"+
  "• Напишите «бот меню», чтобы открыть понятные кнопки.\n\n"+
  "НАСТРОЙКИ\n"+
  "• «Мои настройки» — ваша модель, поиск, стиль и контекст.\n"+
  "• «Настройки беседы» — общие параметры; изменяет только администратор.\n"+
  "• «Выбрать ИИ» — только проверенные бесплатные модели.\n"+
  "• Режим «Авто» выбирает рабочую модель сам и учитывает резерв.\n\n"+
  "В личных сообщениях сообщества меню открывается словом «меню».\n"+
  "Если бот не видит сообщения, нажмите «Права и подключение»."
}
func vkHome(s *Store,c *Catalog,cfg Config,peer,user int64)string{
 key,search,style,count,fallback:=s.effective(cfg,peer,user,c)
 model:="Пока нет работающих ИИ"
 if key!=""{model=c.label(key)}
 chatHint:="Общие параметры доступны в беседе"
 if peer<2000000000{chatHint="Для общей настройки откройте меню внутри беседы"}
 return fmt.Sprintf("AI-bot • ГЛАВНОЕ МЕНЮ\n\n"+
  "ИИ: %s\n"+
  "Работающих бесплатных моделей: %d\n"+
  "Поиск: %s  •  Ответ: %s\n"+
  "Контекст: до %d сообщений  •  Резерв: %s\n\n"+
  "Выберите действие кнопкой ниже. Настройка займёт несколько нажатий, команды запоминать не нужно.\n%s",
  model,len(c.enabled(cfg)),readableSearch(search),readableStyle(style),count,switchLabel(fallback),chatHint)
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
 allowed:=c.enabled(cfg)
 if len(allowed)==0{return "Сейчас нет подтверждённых работающих бесплатных моделей. Бот перепроверяет их автоматически. Администратор сервера может выполнить sudo aibot check-ai."}
 maxPage:=(len(allowed)+2)/3
 if page<1{page=1};if page>maxPage{page=maxPage}
 var b strings.Builder
 fmt.Fprintf(&b,"ВЫБОР ИИ • %d/%d\n\n",page,maxPage)
 for i:=(page-1)*3;i<min(page*3,len(allowed));i++{fmt.Fprintf(&b,"%d. %s\n",i+1,c.label(allowed[i]))}
 b.WriteString("\n«Авто» — рекомендуемый вариант: бот выбирает рабочую модель сам.")
 if chat{b.WriteString("\nДля всей беседы модель может сменить только администратор.")}
 return b.String()
}
func chatInfo(s *Store,c *Catalog,cfg Config,peer int64)string{
 pref:=s.getPref("chat",peer)
 model:="Авто • настройка сервера"
 if pref.Model!=""{
  model="Авто • выбранная ранее модель недоступна"
  for _,key:=range c.enabled(cfg){if key==pref.Model{model=c.label(key);break}}
 }
 contextLabel:=fmt.Sprintf("Авто • до %d сообщений",cfg.ContextMessages)
 if pref.Context>0{contextLabel=fmt.Sprintf("%d сообщений",pref.Context)}
 return fmt.Sprintf("НАСТРОЙКИ БЕСЕДЫ\n\n"+
  "Модель: %s\nПоиск: %s\nОтветы: %s\nКонтекст: %s\nРезервный ИИ: %s\n\n"+
  "Здесь задаются общие параметры. Личные предпочтения участников могут их переопределять. Изменять общие настройки может администратор беседы.",
  model,readableSearch(pref.Search),readableStyle(pref.Style),contextLabel,readableFallback(pref.Fallback))
}
func personalInfo(s *Store,c *Catalog,cfg Config,peer,user int64)string{
 key,search,style,count,fallback:=s.effective(cfg,peer,user,c)
 pref:=s.getPref("user",user)
 model:="Авто • нет доступных моделей"
 if key!=""{
  model="Авто • "+c.label(key)
  if pref.Model!=""&&pref.Model==key{model=c.label(key)}
 }
 return fmt.Sprintf("МОИ НАСТРОЙКИ\n\n"+
  "ИИ-модель: %s\nПоиск: %s\nФормат ответа: %s\nКонтекст: %d сообщений\nРезервный ИИ: %s\n\n"+
  "Нажмите кнопку нужного параметра и выберите значение. Можно вернуть автоматический режим.",
  model,readableSearch(search),readableStyle(style),count,switchLabel(fallback))
}
func choiceIntro(s *Store,c *Catalog,cfg Config,peer,user int64,chat bool,name string)string{
 scope:="вас"
 if chat{scope="всей беседы"}
 captions:=map[string]string{
  "поиска":"РЕЖИМ ПОИСКА\nАвто — искать в интернете только тогда, когда вопрос требует свежей информации.",
  "стиля":"ФОРМАТ ОТВЕТА\nВыберите короткие, обычные или подробные ответы.",
  "контекста":"КОНТЕКСТ\nСколько последних сообщений учитывать при ответе. Максимум ограничивает сервер.",
  "резерва":"РЕЗЕРВНЫЙ ИИ\nЕсли основная модель недоступна, бот попробует другую проверенную бесплатную модель.",
  "сброса":"СБРОС НАСТРОЕК\nВернуть все параметры к исходным. Удаления истории сообщений не будет.",
 }
 headline,ok:=captions[name]
 if !ok{return "Откройте настройки и выберите действие."}
 return fmt.Sprintf("%s\n\nНастройка для %s.\nВыберите один из вариантов кнопкой ниже.",headline,scope)
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
 if len(args)==0{return vkHome(s,c,cfg,peer,user),true}
 first:=strings.ToLower(strings.Trim(args[0],",.:;!?"))
 switch first{
 case "меню","старт":return vkHome(s,c,cfg,peer,user),true
 case "помощь","help":return helpText(),true
 case "права","разрешения","подключение":return permissionHelp(cfg),true
 case "настройки":return personalInfo(s,c,cfg,peer,user),true
 case "модели":
  page:=1;if len(args)>1{page,_=strconv.Atoi(args[1])}
  return listModels(c,cfg,page,false),true
 }
 chat:=first=="диалог"
 if chat {
  if peer<2000000000{return "Общие настройки доступны только в групповой беседе ВК. Откройте «Мои настройки».",true}
  if len(args)==1{return chatInfo(s,c,cfg,peer),true}
  args=args[1:]
  first=strings.ToLower(args[0])
 }
 scope:="user";ident:=user
 if chat{scope="chat";ident=peer}
 if first=="модели"&&chat{
  page:=1;if len(args)>1{page,_=strconv.Atoi(args[1])}
  return listModels(c,cfg,page,true),true
 }
 if first=="выбор"{
  if len(args)<=1{
   if chat{return chatInfo(s,c,cfg,peer),true}
   return personalInfo(s,c,cfg,peer,user),true
  }
  return choiceIntro(s,c,cfg,peer,user,chat,strings.ToLower(args[1])),true
 }
 if first=="подтвердить"&&len(args)==2&&strings.ToLower(args[1])=="сброс"{
  if chat&&!v.admin(ctx,peer,user){return "Изменять настройки беседы может только её администратор.",true}
  result:=cmdApply(s,c,cfg,scope,ident,[]string{"сброс"})
  if chat{return "✓ "+result+"\n\n"+chatInfo(s,c,cfg,peer),true}
  return "✓ "+result+"\n\n"+personalInfo(s,c,cfg,peer,user),true
 }
 for _,cmd:=range []string{"модель","поиск","стиль","контекст","резерв","сброс","сбросить"}{
  if first!=cmd{continue}
  if chat&&!v.admin(ctx,peer,user){
   return "Изменять настройки беседы может только её администратор. Ваши личные настройки доступны всем.",true
  }
  result:=cmdApply(s,c,cfg,scope,ident,args)
  // Avoid displaying a success stamp on rejected input.
  if strings.HasPrefix(result,"Ошибка")||strings.HasPrefix(result,"Нет такой")||strings.HasPrefix(result,"Контекст от")||
   strings.HasPrefix(result,"Напишите")||strings.HasPrefix(result,"Использование")||strings.HasPrefix(result,"Поиск: вкл")||
   strings.HasPrefix(result,"Стиль: кратко")||strings.HasPrefix(result,"Резерв: вкл"){
   return result,true
  }
  if chat{return "✓ "+result+"\n\n"+chatInfo(s,c,cfg,peer),true}
  return "✓ "+result+"\n\n"+personalInfo(s,c,cfg,peer,user),true
 }
 return "",false
}
