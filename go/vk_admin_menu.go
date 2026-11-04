package main

import (
 "context"
 "fmt"
 "strconv"
 "strings"
 "time"
)

// This role is distinct from a VK group-chat administrator. Only the specific
// VK account configured from the trusted server console can administer the bot,
// and only in a one-to-one DM with the bot community.
func isBotAdministrator(cfg Config,peer,user int64)bool{
 return cfg.AdminVKID>0&&peer==user&&user==cfg.AdminVKID&&peer<2000000000
}
func adminMenuButtons(c *Catalog,cfg Config,command string)string{
 parts:=strings.Fields(strings.ToLower(command))
 if len(parts)<2{return replyKeyboard([][]replyButton{
  {{"🤖 ИИ-провайдеры","админ провайдеры 1","primary"}},
  {{"🎛 Основная модель","админ модели 1","primary"}},
  {{"🧰 Функции бота","админ функции","secondary"},{"📏 Лимиты","админ лимиты","secondary"}},
  {{"🔗 Подключение VK","админ вк","secondary"},{"📊 Состояние","админ статус","secondary"}},
  {{"🏠 Обычное меню","меню","positive"}},
 })}
 switch parts[1]{
 case "переключить":
  if len(parts)>2{return adminMenuButtons(c,cfg,"админ провайдер "+parts[2])}
 case "ключ":
  if len(parts)>2{return adminMenuButtons(c,cfg,"админ провайдер "+parts[2])}
 case "аккаунт":return adminMenuButtons(c,cfg,"админ провайдер cloudflare")
 case "модель":return adminMenuButtons(c,cfg,"админ модели 1")
 case "поиск","резерв","контекст","часы","история","триггер":
  return adminMenuButtons(c,cfg,"админ функции")
 case "всего","пользователь","беседа","пауза":
  return adminMenuButtons(c,cfg,"админ лимиты")
 case "группа","токен","администратор","события":
  return adminMenuButtons(c,cfg,"админ вк")
 case "провайдеры":
  page:=1;if len(parts)>2{page,_=strconv.Atoi(parts[2])};if page<1{page=1}
  pages:=(len(c.Order)+3)/4;if page>pages{page=pages}
  rows:=[][]replyButton{}
  for i:=(page-1)*4;i<min(page*4,len(c.Order));i++{
   id:=c.Order[i];spec:=c.Specs[id];status:="выкл";if cfg.Providers[id].Enabled{status="вкл"}
   label:=spec.Name
   if len([]rune(label))>26{label=string([]rune(label)[:25])+"…"}
   rows=append(rows,[]replyButton{{label+" • "+status,"админ провайдер "+id,"primary"}})
  }
  nav:=[]replyButton{}
  if page>1{nav=append(nav,replyButton{"◀ Назад","админ провайдеры "+strconv.Itoa(page-1),"secondary"})}
  if page<pages{nav=append(nav,replyButton{"Далее ▶","админ провайдеры "+strconv.Itoa(page+1),"secondary"})}
  if len(nav)>0{rows=append(rows,nav)}
  rows=append(rows,[]replyButton{{"← Меню админа","админ","positive"}})
  return replyKeyboard(rows)
 case "провайдер":
  if len(parts)<3{return adminMenuButtons(c,cfg,"админ провайдеры 1")}
  id:=parts[2];p,ok:=c.Specs[id];if !ok{return adminMenuButtons(c,cfg,"админ провайдеры 1")}
  on:="✓ Включить";if cfg.Providers[id].Enabled{on="✗ Отключить"}
  rows:=[][]replyButton{{{on,"админ переключить "+id,"primary"}}}
  if p.KeyMode!="none"{rows=append(rows,[]replyButton{{"🔑 Задать API-ключ","админ ключ "+id,"secondary"}})}
  if id=="cloudflare"{rows=append(rows,[]replyButton{{"Cloudflare Account ID","админ аккаунт","secondary"}})}
  rows=append(rows,[]replyButton{{"← Провайдеры","админ провайдеры 1","secondary"},{"🏠 Админ","админ","positive"}})
  return replyKeyboard(rows)
 case "модели":
  available:=c.enabled(cfg)
  page:=1;if len(parts)>2{page,_=strconv.Atoi(parts[2])};if page<1{page=1}
  pages:=(len(available)+3)/4;if pages<1{pages=1};if page>pages{page=pages}
  rows:=[][]replyButton{}
  for i:=(page-1)*4;i<min(page*4,len(available));i++{
   label:=c.label(available[i])
   if len([]rune(label))>30{label=string([]rune(label)[:29])+"…"}
   rows=append(rows,[]replyButton{{fmt.Sprintf("%d. %s",i+1,label),"админ модель "+strconv.Itoa(i+1),"primary"}})
  }
  nav:=[]replyButton{}
  if page>1{nav=append(nav,replyButton{"◀ Назад","админ модели "+strconv.Itoa(page-1),"secondary"})}
  if page<pages{nav=append(nav,replyButton{"Далее ▶","админ модели "+strconv.Itoa(page+1),"secondary"})}
  if len(nav)>0{rows=append(rows,nav)}
  rows=append(rows,[]replyButton{{"✓ Автовыбор","админ модель авто","positive"}})
  rows=append(rows,[]replyButton{{"← Меню админа","админ","secondary"}})
  return replyKeyboard(rows)
 case "функции":
  search:="Включить поиск";if cfg.SearchEnabled{search="Выключить поиск"}
  fallback:="Включить резерв";if cfg.FreeFallback{fallback="Выключить резерв"}
  return replyKeyboard([][]replyButton{
   {{search,"админ поиск переключить","primary"}},
   {{fallback,"админ резерв переключить","primary"}},
   {{"🧠 Контекст","админ выбор контекст","secondary"},{"🕒 Часы","админ выбор часы","secondary"}},
   {{"📁 Хранение истории","админ выбор история","secondary"}},
   {{"⌨ Обращение к боту","админ выбор триггер","secondary"}},
   {{"← Меню админа","админ","positive"}},
  })
 case "лимиты":
  return replyKeyboard([][]replyButton{
   {{"🌍 Всего в час","админ выбор всего","primary"}},
   {{"👤 Пользователь в час","админ выбор пользователь","primary"}},
   {{"👥 Беседа в час","админ выбор беседа","primary"}},
   {{"⏳ Пауза между ответами","админ выбор пауза","secondary"}},
   {{"← Меню админа","админ","positive"}},
  })
 case "вк":
  return replyKeyboard([][]replyButton{
   {{"🆔 ID сообщества","админ выбор группа","primary"}},
   {{"🔐 Токен сообщества","админ выбор токен","secondary"}},
   {{"⚡ Включить события VK","админ события","secondary"}},
   {{"👤 Администратор бота","админ выбор администратор","secondary"}},
   {{"⌨ Обращение к боту","админ выбор триггер","secondary"}},
   {{"← Меню админа","админ","positive"}},
  })
 case "выбор":
  if len(parts)<3{return adminMenuButtons(c,cfg,"админ")}
  field:=parts[2]
  limits:=map[string][]int{
   "контекст":{5,10,20,30,50,100},"часы":{1,6,12,24,72,168},
   "история":{1,3,7,14,30},"всего":{20,50,100,300,1000},
   "пользователь":{5,12,25,50,100},"беседа":{10,25,50,100,200},
   "пауза":{0,5,10,15,30,60},
  }
  if options,ok:=limits[field];ok{
   rows:=[][]replyButton{}
   for i:=0;i<len(options);i+=2{
    row:=[]replyButton{}
    for _,value:=range options[i:min(i+2,len(options))]{
     row=append(row,replyButton{strconv.Itoa(value),"админ "+field+" "+strconv.Itoa(value),"primary"})
    }
    rows=append(rows,row)
   }
   rows=append(rows,[]replyButton{{"← Меню админа","админ","secondary"}})
   return replyKeyboard(rows)
  }
  if field=="токен"||field=="администратор"||field=="группа"||field=="триггер"{
   return replyKeyboard([][]replyButton{{{"← VK и триггер","админ вк","secondary"},{"🏠 Админ","админ","positive"}}})
  }
 }
 return adminMenuButtons(c,cfg,"админ")
}
func adminSettingsSummary(c *Catalog,cfg Config)string{
 enabled:=0
 for _,id:=range c.Order{if cfg.Providers[id].Enabled{enabled++}}
 main:="Автоматический выбор";if cfg.DefaultModel!=""{main=c.label(cfg.DefaultModel)}
 return fmt.Sprintf("МЕНЮ АДМИНИСТРАТОРА • AI-bot v1.6.0\n\n"+
  "ИИ-провайдеров включено: %d из %d\nПроверенных моделей: %d\nОсновная: %s\n"+
  "Поиск: %s • Резерв: %s\nКонтекст: %d сообщений / %d ч\n"+
  "История: %d дней\nЛимиты в час: всего %d, пользователь %d, беседа %d\n"+
  "Пауза: %d сек\nОбращение: %s\nСообщество VK: %d\nАдминистратор VK: %d\n\n"+
  "Изменения действуют без консоли. Непроверенные ИИ не показываются пользователям.",
  enabled,len(c.Order),len(c.enabled(cfg)),main,switchLabel(cfg.SearchEnabled),switchLabel(cfg.FreeFallback),
  cfg.ContextMessages,cfg.ContextHours,cfg.RetentionDays,cfg.MaxRequestsHour,cfg.MaxUserRequestsHour,
  cfg.MaxChatRequestsHour,cfg.CooldownSeconds,cfg.Trigger,cfg.GroupID,cfg.AdminVKID)
}
func (b *Bot) signalModelCheck(){
 if b.scanNow==nil{return}
 select{case b.scanNow<-struct{}{}:default:}
}
func (b *Bot) adminCommand(ctx context.Context,cfg Config,peer,user int64,raw string)(string,bool){
 parts:=strings.Fields(strings.TrimSpace(raw))
 if len(parts)==0||strings.ToLower(parts[0])!="админ"{return "",false}
 if !isBotAdministrator(cfg,peer,user){
  return "У вас нет доступа к настройкам администратора бота.",true
 }
 if len(parts)==1{return adminSettingsSummary(b.catalog,cfg),true}
 action:=strings.ToLower(parts[1])
 if action=="статус"{return adminSettingsSummary(b.catalog,cfg),true}
 if action=="провайдеры"{
  page:=1;if len(parts)>2{page,_=strconv.Atoi(parts[2])}
  return fmt.Sprintf("ИИ-ПРОВАЙДЕРЫ • страница %d\n\nБесплатные без регистрации идут первыми; ниже — с бесплатным ключом. Выберите сервис, чтобы включить или отключить его.",max(1,page)),true
 }
 if action=="провайдер"&&len(parts)>2{
  id:=strings.ToLower(parts[2]);spec,ok:=b.catalog.Specs[id]
  if !ok{return "Такого ИИ-провайдера нет.",true}
  settings:=cfg.Providers[id]
  status:="выключен";if settings.Enabled{status="включён"}
  key:="не нужен";if spec.requiresKey(){key="не задан";if settings.APIKey!=""{key="сохранён"}}
  if spec.KeyMode=="optional"{key="необязателен";if settings.APIKey!=""{key="сохранён"}}
  return fmt.Sprintf("%s\n\nСостояние: %s\nКлюч: %s\nМоделей в каталоге: %d\n\nИзменить состояние или ключ можно кнопками ниже.",spec.Name,status,key,len(spec.Models)),true
 }
 if action=="переключить"&&len(parts)==3{
  id:=strings.ToLower(parts[2]);spec,ok:=b.catalog.Specs[id];if !ok{return "Неизвестный провайдер.",true}
  settings:=cfg.Providers[id];settings.Enabled=!settings.Enabled
  if settings.Enabled&&spec.requiresKey()&&settings.APIKey==""{return "Для этого сервиса нужен бесплатный API-ключ с регистрацией. Нажмите «Задать API-ключ».",true}
  if _,err:=saveVKAdminProvider(id,settings);err!=nil{return "Ошибка изменения провайдера: "+err.Error(),true}
  b.signalModelCheck()
  if settings.Enabled{return "✓ Бесплатный сервис включён. Модели станут доступны после проверки.",true}
  return "✓ Сервис выключен.",true
 }
 if action=="модели"{
  count:=len(b.catalog.enabled(cfg))
  if count==0{return "Сейчас нет проверенных бесплатных моделей. Включите API-провайдеры — проверка запускается автоматически.",true}
  return fmt.Sprintf("ОСНОВНАЯ МОДЕЛЬ\n\nПроверенных бесплатных моделей: %d. Выберите модель кнопкой или включите автоматический выбор.",count),true
 }
 if action=="модель"&&len(parts)==3{
  key:=""
  if strings.ToLower(parts[2])!="авто"{
   index,err:=strconv.Atoi(parts[2]);allowed:=b.catalog.enabled(cfg)
   if err!=nil||index<1||index>len(allowed){return "Нет такой проверенной модели.",true}
   key=allowed[index-1]
  }
  if _,err:=saveVKAdminField("default_model",key);err!=nil{return "Не удалось сохранить модель: "+err.Error(),true}
  if key==""{return "✓ Для всех включён автоматический выбор бесплатной модели.",true}
  return "✓ Основная модель: "+b.catalog.label(key),true
 }
 if action=="функции"||action=="лимиты"||action=="вк"{return adminSettingsSummary(b.catalog,cfg),true}
 if action=="события"{
  vk:=newVK(cfg)
  checkCtx,cancel:=context.WithTimeout(ctx,12*time.Second);defer cancel()
  if err:=vk.setLongPollEvents(checkCtx);err!=nil{return "ВК не разрешил включить Long Poll: "+err.Error(),true}
  return "✓ Включены события сообщений и нажатий кнопок ВК.",true
 }
 if action=="выбор"&&len(parts)==3{
  label:=map[string]string{
   "контекст":"максимум сообщений контекста (1–100)","часы":"максимальный возраст контекста в часах (1–168)",
   "история":"хранение истории в днях (1–30)","всего":"лимит запросов в час (1–10000)",
   "пользователь":"лимит пользователя в час (1–1000)","беседа":"лимит беседы в час (1–1000)",
   "пауза":"паузу между ответами (0–3600 секунд)","триггер":"слово обращения (до 40 символов)",
   "группа":"числовой ID сообщества VK","токен":"токен сообщества VK",
   "администратор":"новый VK ID или ссылку на профиль",
  }
  field:=strings.ToLower(parts[2]);if label[field]==""{return "Неизвестный параметр.",true}
  hint:="Выберите значение кнопкой или отправьте: «админ "+field+" <значение>»."
  if field=="токен"{hint="Отправьте в ЛИЧНЫЕ сообщения: «админ токен <токен>». Сообщение с токеном останется в истории ВК; для большей приватности используйте защищённую веб-панель."}
  if field=="администратор"{hint="Отправьте «админ администратор <ID или ссылка на профиль ВК>». После смены доступ перейдёт этому аккаунту."}
  return "НАСТРОЙКА\n\nВведите "+label[field]+".\n\n"+hint,true
 }
 if action=="ключ"&&len(parts)==3{
  id:=strings.ToLower(parts[2]);spec,ok:=b.catalog.Specs[id];if !ok{return "Неизвестный провайдер.",true}
  if spec.KeyMode=="none"{return "Для этого бесплатного API ключ не требуется.",true}
  return "БЕСПЛАТНЫЙ API-КЛЮЧ • "+spec.Name+"\n\n"+
   "Получить: "+spec.KeyURL+"\nОтправьте в личные сообщения: «админ ключ "+id+" <ваш_ключ>».\n"+
   "Ключ не будет сохранён в базе бесед или отправлен нейросети как сообщение. Однако ВК сохраняет вашу отправку: для большей приватности пользуйтесь защищённой веб-панелью.",true
 }
 if action=="аккаунт"&&len(parts)==2{return "Cloudflare Account ID: отправьте «админ аккаунт <ID>».",true}
 if action=="аккаунт"&&len(parts)==3{
  value:=parts[2]
  if len(value)>100||strings.ContainsAny(value,"/\\?&\"'<>"){return "Неверный Cloudflare Account ID.",true}
  settings:=cfg.Providers["cloudflare"];settings.CloudflareAccount=value
  if _,err:=saveVKAdminProvider("cloudflare",settings);err!=nil{return "Ошибка сохранения: "+err.Error(),true}
  b.signalModelCheck();return "✓ Cloudflare Account ID сохранён.",true
 }
 if action=="поиск"||action=="резерв"{
  if len(parts)<3{return "Выберите включение или отключение.",true}
  val:=strings.ToLower(parts[2]);var result bool
  switch val{case "переключить":result=cfg.SearchEnabled;if action=="резерв"{result=cfg.FreeFallback};result=!result
   case "вкл":result=true;case "выкл":result=false;default:return "Допустимые значения: вкл / выкл.",true}
  field:="search_enabled";if action=="резерв"{field="free_fallback"}
  if _,err:=saveVKAdminField(field,result);err!=nil{return "Ошибка сохранения: "+err.Error(),true}
  return "✓ Настройка изменена.",true
 }
 fields:=map[string]struct{Name string;Min,Max int}{
  "контекст":{"context_messages",1,100},"часы":{"context_hours",1,168},
  "история":{"retention_days",1,30},"всего":{"max_requests_hour",1,10000},
  "пользователь":{"max_user_requests_hour",1,1000},"беседа":{"max_chat_requests_hour",1,1000},
  "пауза":{"cooldown_seconds",0,3600},"группа":{"group_id",1,999999999},
 }
 if field,ok:=fields[action];ok&&len(parts)==3{
  number,err:=strconv.Atoi(parts[2])
  if err!=nil||number<field.Min||number>field.Max{return fmt.Sprintf("Введите число от %d до %d.",field.Min,field.Max),true}
  if _,err:=saveVKAdminField(field.Name,number);err!=nil{return "Ошибка сохранения: "+err.Error(),true}
  if action=="группа"{b.signalReconfigure();return "✓ ID сообщества сохранён. Служба переподключится к ВК.",true}
  return "✓ Значение сохранено и действует для следующих запросов.",true
 }
 if action=="триггер"&&len(parts)==3{
  trigger:=parts[2]
  if len([]rune(trigger))<1||len([]rune(trigger))>40||strings.ContainsAny(trigger,"<>{}\r\n"){return "Неверное слово обращения.",true}
  if _,err:=saveVKAdminField("trigger",trigger);err!=nil{return "Ошибка сохранения: "+err.Error(),true}
  return "✓ Обращение к боту изменено: "+trigger,true
 }
 if action=="администратор"&&len(parts)==3{
  checkCtx,cancel:=context.WithTimeout(ctx,15*time.Second);defer cancel()
  id,err:=resolveVKAdminID(checkCtx,b.vk,parts[2])
  if err!=nil{return "Ошибка VK ID: "+err.Error(),true}
  if _,err=saveVKAdminField("admin_vk_id",id);err!=nil{return "Ошибка сохранения: "+err.Error(),true}
  return fmt.Sprintf("✓ Администратор бота изменён: vk.com/id%d. Теперь доступ имеет только новый аккаунт.",id),true
 }
 return "Неизвестная команда администратора. Нажмите «Меню админа».",true
}

// Secret values must never enter the SQLite history, AI prompt, queue,
// diagnostics, or message preview. VK itself still retains the user's DM.
func (b *Bot) adminSecretMessage(ctx context.Context,cfg Config,m VKMessage)(bool,bool,error){
 words:=strings.Fields(m.Text)
 if len(words)<3||strings.ToLower(words[0])!="админ"{return false,false,nil}
 if m.PeerID!=m.FromID{return false,false,nil}
 action:=strings.ToLower(words[1])
 if action!="ключ"&&action!="токен"{return false,false,nil}
 // "админ ключ <provider>" is a harmless menu command.
 if action=="ключ"&&len(words)<4{return false,false,nil}
 if action=="токен"&&len(words)<3{return false,false,nil}
 if !isBotAdministrator(cfg,m.PeerID,m.FromID){
  err:=b.send(ctx,m,"Доступ к настройкам администратора запрещён.",homeKeyboardForPeer(m.PeerID))
  return true,false,err
 }
 if action=="токен"{
  token:=strings.TrimSpace(strings.TrimPrefix(m.Text,words[0]+" "+words[1]+" "))
  if len(token)<20||len(token)>500||strings.ContainsAny(token," \t\r\n"){
   return true,false,b.send(ctx,m,"Некорректный токен ВК.",adminMenuButtons(b.catalog,cfg,"админ вк"))
  }
  if _,err:=saveVKAdminField("vk_token",token);err!=nil{
   return true,false,b.send(ctx,m,"Не удалось сохранить токен ВК.",adminMenuButtons(b.catalog,cfg,"админ вк"))
  }
  err:=b.send(ctx,m,"✓ Токен сообщества сохранён. Бот переподключится к ВК.",adminMenuButtons(b.catalog,cfg,"админ вк"))
  return true,true,err
 }
 id:=strings.ToLower(words[2]);spec,ok:=b.catalog.Specs[id]
 if !ok{return true,false,b.send(ctx,m,"Неизвестный провайдер.",adminMenuButtons(b.catalog,cfg,"админ провайдеры 1"))}
 if spec.KeyMode=="none"{return true,false,b.send(ctx,m,"Этот бесплатный провайдер работает без API-ключа.",adminMenuButtons(b.catalog,cfg,"админ провайдер "+id))}
 key:=strings.TrimSpace(strings.TrimPrefix(m.Text,words[0]+" "+words[1]+" "+words[2]+" "))
 if key==""||len(key)>4096||strings.ContainsAny(key," \n\r\t"){
  return true,false,b.send(ctx,m,"Ключ не принят: запрещены пробелы и переносы строк.",adminMenuButtons(b.catalog,cfg,"админ провайдер "+id))
 }
 settings:=cfg.Providers[id]
 if key=="-"{settings.APIKey=""}else{settings.APIKey=key;settings.Enabled=true}
 if _,err:=saveVKAdminProvider(id,settings);err!=nil{return true,false,b.send(ctx,m,"Не удалось сохранить ключ: проверьте, что сервис отключён перед удалением обязательного ключа.",adminMenuButtons(b.catalog,cfg,"админ провайдер "+id))}
 b.signalModelCheck()
 return true,false,b.send(ctx,m,"✓ Ключ сохранён локально. Бесплатные модели будут проверены автоматически.",adminMenuButtons(b.catalog,cfg,"админ провайдер "+id))
}
