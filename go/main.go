package main

import (
 "bufio"
 "context"
 "errors"
 "fmt"
 "io"
 "log"
 "os"
 "os/exec"
 "os/signal"
 "strconv"
 "strings"
 "syscall"
 "time"
)

type Wizard struct{in *bufio.Reader;out io.Writer}
func (w Wizard) ask(label,current string)string{
 if current!=""{fmt.Fprintf(w.out,"%s [%s]: ",label,current)}else{fmt.Fprintf(w.out,"%s: ",label)}
 line,err:=w.in.ReadString('\n');if err!=nil&&len(line)==0{return current}
 line=strings.TrimSpace(line);if line==""{return current};return line
}
func(w Wizard) secret(label,current string)string {
 fmt.Fprintf(w.out,"%s (%s, Enter — оставить прежний, '-' — удалить): ",label,func()string{if current!=""{return "ключ сохранён"};return "ключ пока отсутствует"}())
 // stty hides input only for TTY. Never echo existing secret back to screen.
 if _,err:=os.Stdin.Stat();err==nil {
  command:=exec.Command("stty","-echo");command.Stdin=os.Stdin;_ = command.Run()
  defer func(){restore:=exec.Command("stty","echo");restore.Stdin=os.Stdin;_ = restore.Run();fmt.Fprintln(w.out)}()
 }
 value,err:=w.in.ReadString('\n');if err!=nil&&len(value)==0{return current}
 value=strings.TrimSpace(value);if value=="-"{return ""};if value==""{return current};return value
}
func(w Wizard) integer(label string,current,minValue,maxValue int)int{
 for {value:=w.ask(label,strconv.Itoa(current));v,err:=strconv.Atoi(value);if err==nil&&v>=minValue&&v<=maxValue{return v};fmt.Fprintf(w.out,"Введите число от %d до %d.\n",minValue,maxValue)}
}
func(w Wizard) yes(label string,current bool)bool{
 if terminalEnabled(w.out){
  options:=[]menuOption{
   {1,"Включить","Да — разрешить эту функцию",""},
   {2,"Выключить","Нет — не использовать эту функцию",""},
  }
  if !current{options[0],options[1]=options[1],options[0]}
  options=append(options,menuOption{0,"Оставить как есть",switchLabel(current),""})
  choice:=w.chooseMenu("НАСТРОЙКА • "+label,"Текущее значение: "+switchLabel(current),options)
  if choice==0{return current}
  return choice==1
 }
 fmt.Fprintf(w.out,"  Сейчас: %s. Нажмите Enter, чтобы оставить как есть.\n",switchLabel(current))
 defaultVal:="н";if current{defaultVal="д"}
 val:=strings.ToLower(w.ask(label+" (д/н)",defaultVal))
 return val=="д"||val=="да"||val=="y"||val=="yes"
}
func(w Wizard) setupVK(cfg *Config){
 fmt.Fprintln(w.out,"\n════════ ПОДКЛЮЧЕНИЕ ВКОНТАКТЕ ════════")
 fmt.Fprintln(w.out,"Бот отвечает от имени сообщества ВК, а не от вашего личного профиля.")
 fmt.Fprintln(w.out,vkSetupGuide(cfg.GroupID))
 fmt.Fprintln(w.out,"Сначала создайте сообщество и токен в ВК; если уже сделали это, заполните поля ниже.")
 cfg.VKToken=w.secret("Токен сообщества (создаётся в «Работа с API» → «Ключи доступа»)",cfg.VKToken)
 cfg.GroupID=int64(w.integer("Числовой ID сообщества (только цифры)",int(cfg.GroupID),1,999999999))
 fmt.Fprintf(w.out,"\n%s  Токен сообщества\n%s  ID сообщества: %d\n",readyLabel(cfg.VKToken!=""),readyLabel(cfg.GroupID>0),cfg.GroupID)
 fmt.Fprintln(w.out,"Следующий шаг: включите сообщения и Long Poll в настройках этого сообщества, затем выберите бесплатный ИИ в пункте 2.")
}
func contains(list []string,key string)bool{for _,s:=range list{if s==key{return true}};return false}
func removeProvider(list []string,provider string)[]string{
 out:=[]string{};for _,s:=range list{if !strings.HasPrefix(s,provider+":"){out=append(out,s)}};return out
}
func(w Wizard) provider(cfg *Config,c *Catalog,id string){
 p:=c.Specs[id]
 settings:=cfg.Providers[id]
 oldSettings:=settings
 oldModels:=append([]string(nil),cfg.EnabledModels...)
 oldAuto:=append([]string(nil),cfg.AutoModelsProviders...)
 oldDefault:=cfg.DefaultModel
 detail:=fmt.Sprintf("Сервис: %s   •   %s\nКлюч API: %s   •   %s\nБесплатные модели: %d подтверждено",p.Name,p.KeyURL,
  func()string{if settings.APIKey!=""||p.AnonymousKey!=""{return "сохранён"};return "не задан"}(),switchLabel(settings.Enabled),len(c.allowedFor(id,*cfg)))
 choice:=w.chooseMenu("НАСТРОЙКА ИИ • "+p.Name,detail,[]menuOption{
  {1,"Автоматический выбор работающих моделей","Проверенные бесплатные модели; новые добавятся автоматически","Рекомендуется"},
  {2,"Выбрать модели вручную","Проверка доступности перед выбором, можно отметить несколько",""},
  {3,"Отключить провайдера","Ключ остаётся сохранённым; бот перестаёт использовать сервис",""},
  {0,"Назад","Без изменения настроек",""},
 })
 if choice==0{return}
 if choice==3{
  settings.Enabled=false
  cfg.Providers[id]=settings
  cfg.AutoModelsProviders=removeExact(cfg.AutoModelsProviders,id)
  cfg.EnabledModels=removeProvider(cfg.EnabledModels,id)
  fmt.Fprintln(w.out,"✗ ВЫКЛ. Этот сервис больше не используется ботом.")
  return
 }
 fmt.Fprintln(w.out,"Скопируйте API-ключ на указанном сайте. Он останется только на сервере.")
 if p.AnonymousKey==""||settings.APIKey!=""{
  settings.APIKey=w.secret("API-ключ",settings.APIKey)
 }
 if id=="cloudflare"{
  fmt.Fprintln(w.out,"Cloudflare Account ID — идентификатор аккаунта из панели Cloudflare (не домен и не API-ключ).")
  settings.CloudflareAccount=w.ask("Cloudflare Account ID",settings.CloudflareAccount)
  if settings.CloudflareAccount==""{
   fmt.Fprintln(w.out,"✗ Не указан Account ID. Cloudflare пока не включён.")
   return
  }
 }
 if settings.APIKey==""&&p.AnonymousKey==""{
  fmt.Fprintln(w.out,"✗ Нет API-ключа: сервис не включён. Получите бесплатный ключ по ссылке выше.")
  return
 }
 settings.Enabled=true
 cfg.Providers[id]=settings
 cfg.AutoModelsProviders=removeExact(cfg.AutoModelsProviders,id)
 cfg.EnabledModels=removeProvider(cfg.EnabledModels,id)
 if choice==1{
  cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id)
  fmt.Fprintln(w.out,"✓ ВКЛ. Автоподбор включён: бот использует только модели из разрешённого бесплатного каталога.")
 }else{
  // Probe all models before showing a manual selection. The health filter
  // intentionally hides models that have not responded from this VDS.
  cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id)
  if p.FreePolicy!="curated"{
   ctx,cancel:=context.WithTimeout(context.Background(),25*time.Second)
   key:=settings.APIKey;if key==""{key=p.AnonymousKey}
   models,err:=c.discover(ctx,p,key);cancel()
   if err!=nil{fmt.Fprintln(w.out,"Каталог временно недоступен:",err)}else{
    c.mu.Lock();c.verified[id]=models;c.mu.Unlock()
   }
  }
  ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
  if err:=scanAvailableModels(ctx,*cfg,c,w.out,false);err!=nil{fmt.Fprintln(w.out,"Проверка:",err)}
  cancel()
  list:=c.allowedFor(id,*cfg)
  if len(list)==0{
   fmt.Fprintln(w.out,"Нет подтверждённых бесплатных моделей. Включите автоподбор позже, когда каталог станет доступен.")
   cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id)
  }else{
   labels:=make([]string,len(list))
   for i,key:=range list{labels[i]=c.label(key)}
   selected:=w.chooseMany("ВЫБОР БЕСПЛАТНЫХ МОДЕЛЕЙ • "+p.Name,labels)
   if len(selected)>0{
    cfg.AutoModelsProviders=removeExact(cfg.AutoModelsProviders,id)
    for _,i:=range selected{cfg.EnabledModels=append(cfg.EnabledModels,list[i])}
   }else{
    cfg.Providers[id]=oldSettings
    cfg.EnabledModels=oldModels
    cfg.AutoModelsProviders=oldAuto
    cfg.DefaultModel=oldDefault
    fmt.Fprintln(w.out,"Выбор отменён. Прежние настройки сохранены.")
    return
   }
  }
  fmt.Fprintln(w.out,"✓ Сервис включён. Выбранные модели можно поменять здесь в любой момент.")
 }
 if cfg.DefaultModel==""{cfg.DefaultModel=c.defaultModel(*cfg)}
 fmt.Fprintln(w.out,"Далее: вернитесь в меню и откройте пункт «Диагностика», затем проверьте упоминание бота в беседе ВК.")
}
func removeExact(list []string,key string)[]string{out:=[]string{};for _,s:=range list{if s!=key{out=append(out,s)}};return out}
func (c *Catalog) allowedFor(provider string,cfg Config)[]string{
 keys:=c.enabled(cfg);result:=[]string{};for _,key:=range keys{if strings.HasPrefix(key,provider+":"){result=append(result,key)}}
 if len(result)>0||c.health!=nil{return result}
 p:=c.Specs[provider];if p.FreePolicy=="curated"{for _,m:=range p.Models{result=append(result,provider+":"+m[0])};return result}
 c.mu.RLock();defer c.mu.RUnlock();for _,m:=range c.verified[provider]{result=append(result,provider+":"+m.ID)}
 return result
}
func(w Wizard) setupModels(cfg *Config,c *Catalog){
 for{
  opts:=make([]menuOption,0,len(c.Order)+5)
  for i,id:=range c.Order{
   p:=c.Specs[id];setting:=cfg.Providers[id]
   state:="Выкл";if setting.Enabled{state="Вкл"}
   count:=len(c.allowedFor(id,*cfg))
   opts=append(opts,menuOption{i+1,p.Name,fmt.Sprintf("Подтверждено рабочих моделей: %d • %s",count,p.KeyURL),state})
  }
  n:=len(c.Order)
  selectedName:="не выбрана"
  if key:=c.defaultModel(*cfg);key!=""{selectedName=c.label(key)}
  opts=append(opts,
   menuOption{n+1,"Основная модель","Сейчас: "+selectedName,""},
   menuOption{n+2,"Проверить все модели","Показать прогресс и список только реально работающих",""},
   menuOption{n+3,"Автоподбор для всех провайдеров","Включить периодическую проверку без ограничения по одной модели",""},
   menuOption{n+4,"Проверка русского языка","Запрос на русском к каждой настроенной модели",""},
   menuOption{0,"Назад","Вернуться в управление AI-bot",""},
  )
  choice:=w.chooseMenu("БЕСПЛАТНЫЕ МОДЕЛИ","Проверенные модели: "+strconv.Itoa(len(c.enabled(*cfg)))+"  •  Проверка выполняется с VDS\nВыберите сервис для настройки ключа и режима работы.",opts)
  if choice==0{return}
  if choice<=n{w.provider(cfg,c,c.Order[choice-1])}
  if choice==n+1{
   ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
   if err:=scanAvailableModels(ctx,*cfg,c,w.out,false);err!=nil{fmt.Fprintln(w.out,"Проверка:",err)}
   cancel()
   available:=c.enabled(*cfg)
   if len(available)==0{
    fmt.Fprintln(w.out,"✗ Моделей пока нет. Сначала подключите один из сервисов выше и включите подбор.")
    fmt.Fprintln(w.out,"Для повторной проверки: sudo aibot check-ai")
    w.pauseMenu()
    continue
   }
   options:=make([]menuOption,0,len(available)+1)
   for i,k:=range available{options=append(options,menuOption{i+1,c.label(k),k,func()string{if k==cfg.DefaultModel{return "Выбрана"};return "Доступна"}()})}
   options=append(options,menuOption{0,"Назад","Не менять основной ИИ",""})
   chosen:=w.chooseMenu("ВЫБОР ОСНОВНОЙ МОДЕЛИ","Только модели, подтвердившие работу и русский язык.",options)
   if chosen==0{continue}
   cfg.DefaultModel=available[chosen-1]
   fmt.Fprintln(w.out,"✓ Основная модель сохранена. Она используется по умолчанию в новых ответах.")
  }
  if choice==n+2{
   ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
   if err:=scanAvailableModels(ctx,*cfg,c,w.out,true);err!=nil{fmt.Fprintln(w.out,"Ошибка проверки:",err)}
   cancel()
  }
  if choice==n+3{
   changed:=0
   for _,id:=range c.Order{
    if cfg.Providers[id].Enabled&&!contains(cfg.AutoModelsProviders,id){
     cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id);changed++
    }
   }
   fmt.Fprintf(w.out,"✓ Автоподбор включён для %d дополнительных сервисов.\n",changed)
  }
  if choice==n+4{
   ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
   if err:=scanAvailableModels(ctx,*cfg,c,w.out,true);err!=nil{fmt.Fprintln(w.out,"Ошибка проверки:",err)}
   cancel()
  }
  if err:=saveConfig(*cfg);err!=nil{fmt.Fprintln(w.out,"✗ Не удалось сохранить настройки:",err)}
  w.pauseMenu()
 }
}
func(w Wizard) behavior(cfg *Config){
 fmt.Fprintln(w.out,"\n════════ ПОВЕДЕНИЕ БОТА ════════")
 fmt.Fprintln(w.out,"Нажмите Enter, чтобы оставить значение в квадратных скобках без изменений.")
 fmt.Fprintln(w.out,"«Контекст» — сколько предыдущих сообщений беседы ИИ учитывает при ответе.")
 cfg.ContextMessages=w.integer("Сколько последних сообщений учитывать (1–100)",cfg.ContextMessages,1,100)
 cfg.ContextHours=w.integer("Не брать сообщения старше, часов (1–168)",cfg.ContextHours,1,168)
 fmt.Fprintln(w.out,"История удаляется автоматически после указанного срока.")
 cfg.RetentionDays=w.integer("Хранить историю, дней (1–30)",cfg.RetentionDays,1,30)
 fmt.Fprintln(w.out,"\nЛимиты защищают бесплатные квоты API от слишком частых обращений.")
 cfg.MaxRequestsHour=w.integer("Максимум запросов к ИИ со всего сервера в час",cfg.MaxRequestsHour,1,10000)
 cfg.MaxUserRequestsHour=w.integer("Максимум запросов одного участника в час",cfg.MaxUserRequestsHour,1,1000)
 cfg.MaxChatRequestsHour=w.integer("Максимум запросов одной беседы в час",cfg.MaxChatRequestsHour,1,1000)
 cfg.CooldownSeconds=w.integer("Пауза между ответами в беседе, секунд (0 — без паузы)",cfg.CooldownSeconds,0,3600)
 fmt.Fprintf(w.out,"\n%s  Поиск в интернете (DuckDuckGo/Википедия)\n",switchLabel(cfg.SearchEnabled))
 cfg.SearchEnabled=w.yes("Разрешить боту искать информацию в интернете",cfg.SearchEnabled)
 fmt.Fprintf(w.out,"%s  Резервный бесплатный ИИ\n",switchLabel(cfg.FreeFallback))
 fmt.Fprintln(w.out,"Резерв: если основной сервис недоступен, вопрос и контекст могут уйти другому разрешённому ИИ.")
 cfg.FreeFallback=w.yes("Разрешить резервную модель",cfg.FreeFallback)
 fmt.Fprintf(w.out,"\nНастройки: поиск %s, резерв %s.\n",switchLabel(cfg.SearchEnabled),switchLabel(cfg.FreeFallback))
 fmt.Fprintln(w.out,"Далее: сохраните настройки и проверьте ответ бота в беседе ВК.")
}
func (w Wizard) configureVKEvents(cfg Config){
 fmt.Fprintln(w.out,"\n════════ СОБЫТИЯ ВК (LONG POLL) ════════")
 fmt.Fprintln(w.out,"Long Poll — способ получать новые сообщения и нажатия кнопок из вашего сообщества ВК.")
 fmt.Fprintln(w.out,"Включаем только: получение новых сообщений и нажатий кнопок. Остальные события не отключаем.")
 fmt.Fprintln(w.out,"Где найти: сообщество ВК → «Управление» → «Работа с API» → «Long Poll API».")
 if cfg.VKToken==""||cfg.GroupID<1{
  fmt.Fprintln(w.out,"✗ Сначала настройте токен и ID сообщества в пункте 1.")
  return
 }
 if w.ask("Для включения через VK API введите ДА","нет")!="ДА"{
  fmt.Fprintln(w.out,"Изменения отменены. Настройки ВК не изменены.");return
 }
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 var vkErr error
 consoleSpinner(w.out,"Настройка событий Long Poll ВК",func(){vkErr=newVK(cfg).setLongPollEvents(ctx)})
 if vkErr!=nil{
  fmt.Fprintln(w.out,"✗ ВК отказал в изменении:",vkErr)
  fmt.Fprintln(w.out,"Откройте «Работа с API» → «Long Poll API» и включите message_new и message_event вручную.")
  return
 }
 fmt.Fprintln(w.out,"✓ События включены. Проверка:",vkDiagnostics(ctx,cfg))
 fmt.Fprintln(w.out,"Далее: добавьте сообщество в беседу ВК и проверьте ответ на упоминание.")
}
func service(action string)error{
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"systemctl",action,"ai-bot")
 output,err:=cmd.CombinedOutput()
 if err!=nil{return fmt.Errorf("служба ai-bot: %w (%s)",err,strings.TrimSpace(string(output)))}
 return nil
}
func applySettings(w Wizard,cfg Config,c *Catalog){
 if err:=saveConfig(cfg);err!=nil{
  fmt.Fprintln(w.out,"✗ Не удалось сохранить настройки:",err)
  return
 }
 fmt.Fprintln(w.out,"✓ Настройки сохранены.")
 if cfg.VKToken==""||cfg.GroupID<1{
  if err:=service("stop");err!=nil{fmt.Fprintln(w.out,"Внимание: не удалось остановить прежнюю службу:",err)}
  fmt.Fprintln(w.out,"Бот остановлен до настройки ВК. Далее: пункт 1 — токен и ID сообщества.")
  return
 }
 connected:=false
 for id,p:=range cfg.Providers{
  spec,ok:=c.Specs[id]
  if ok&&p.Enabled&&(p.APIKey!=""||spec.AnonymousKey!=""){connected=true;break}
 }
 if !connected{
  if err:=service("stop");err!=nil{fmt.Fprintln(w.out,"Внимание: не удалось остановить прежнюю службу:",err)}
  fmt.Fprintln(w.out,"Бот остановлен до подключения ИИ. Далее: пункт 2 — включите бесплатный сервис.")
  return
 }
 if err:=service("restart");err!=nil{
  fmt.Fprintln(w.out,"✗ Не удалось перезапустить бота:",err)
  fmt.Fprintln(w.out,"Посмотрите подробности: sudo aibot logs")
 }else{fmt.Fprintln(w.out,"✓ Служба бота перезапущена с новыми настройками.")}
}
func activeBotService()string{
 ctx,stop:=context.WithTimeout(context.Background(),2*time.Second)
 defer stop()
 if exec.CommandContext(ctx,"systemctl","is-active","--quiet","ai-bot").Run()==nil{return "Работает"}
 return "Остановлен"
}
func menu(w Wizard,cfg Config,c *Catalog){
 for{
  connected:=0
  for _,id:=range c.Order{if cfg.Providers[id].Enabled{connected++}}
  usable:=len(c.enabled(cfg))
  serviceState:="Не настроено"
  if cfg.VKToken!=""&&cfg.GroupID>0{serviceState=fmt.Sprintf("ID %d",cfg.GroupID)}
  modelState:="Проверка не завершена"
  if usable>0{modelState=fmt.Sprintf("%d работают",usable)}
  description:=fmt.Sprintf("%s\nСлужба AI-bot: %s   •   ВК: %s\nПровайдеры: %d подключено   •   Рабочие модели: %d\nПоиск: %s   •   Резерв: %s\n%s",
   "Обзор настроек и состояния VDS",activeBotService(),serviceState,connected,usable,switchLabel(cfg.SearchEnabled),switchLabel(cfg.FreeFallback),nextSetupStep(cfg,c))
  options:=[]menuOption{
   {1,"ВКонтакте • подключение","Токен, ID сообщества и разрешения",serviceState},
   {2,"ИИ-модели и провайдеры","Список только проверенных моделей; выбор бесплатного API",modelState},
   {3,"Поведение и лимиты","Контекст беседы, поиск, квоты и приватность",""},
   {4,"Диагностика сервера","ВК, SQLite, каталоги и состояние подключения",""},
   {5,"Перезапустить AI-bot","Перезапуск системной службы после изменений",""},
   {6,"Журнал событий","Последние 60 строк системного журнала",""},
   {7,"Очистить историю бесед","Безвозвратное удаление сообщений и очереди","Осторожно"},
   {8,"Веб-панель","Настроить HTTPS на домене или временный SSH-доступ",""},
   {9,"События ВК","Включить Long Poll и callback-кнопки",""},
   {10,"Инструкция по ВК","Пошаговая настройка сообщества",""},
   {11,"Следующий шаг","Подсказки по незавершённой настройке",""},
   {12,"Проверить доступность всех ИИ","Тестовые запросы с этого VDS с прогрессом и результатами",""},
   {0,"Выйти","Завершить работу с консольным меню",""},
  }
  choice:=w.chooseMenu("ГЛАВНОЕ МЕНЮ  •  AI-bot v1.5.5",description,options)
  switch choice{
  case 0:return
  case 1:w.setupVK(&cfg);applySettings(w,cfg,c)
  case 2:w.setupModels(&cfg,c);applySettings(w,cfg,c)
  case 3:w.behavior(&cfg);applySettings(w,cfg,c)
  case 4:
   var result string
   consoleSpinner(w.out,"Проверка ВК, SQLite и каталогов",func(){result=doctor(context.Background(),cfg)})
   fmt.Fprintln(w.out,result)
  case 5:
   var err error
   consoleSpinner(w.out,"Перезапуск службы AI-bot",func(){err=service("restart")})
   if err!=nil{fmt.Fprintln(w.out,"✗ Перезапуск не удался:",err)
   }else{fmt.Fprintln(w.out,"✓ AI-bot перезапущен. Статус: sudo systemctl status ai-bot")}
  case 6:
   fmt.Fprintln(w.out,"\nЖурнал AI-bot • 60 последних событий")
   cmd:=exec.Command("journalctl","-u","ai-bot","-n","60","--no-pager")
   cmd.Stdout=w.out;cmd.Stderr=w.out
   if err:=cmd.Run();err!=nil{fmt.Fprintln(w.out,"Журнал недоступен:",err)}
  case 7:
   fmt.Fprintln(w.out,"Удалятся сохранённая переписка и ожидающие запросы. Восстановить их нельзя.")
   if w.ask("Введите ДА, чтобы удалить","нет")=="ДА"{
    store,err:=openStore(dataPath())
    if err!=nil{fmt.Fprintln(w.out,"✗ SQLite:",err)
    }else{
     if err=store.clearMessages();err!=nil{fmt.Fprintln(w.out,"✗ Ошибка удаления:",err)
     }else{fmt.Fprintln(w.out,"✓ История и очередь сообщений удалены.")}
     store.close()
    }
   }else{fmt.Fprintln(w.out,"Отменено.")}
  case 8:w.panelMenu()
  case 9:w.configureVKEvents(cfg)
  case 10:fmt.Fprintln(w.out,vkSetupGuide(cfg.GroupID))
  case 11:
   fmt.Fprint(w.out,setupSummary(cfg,c))
   fmt.Fprintln(w.out,nextSetupStep(cfg,c))
  case 12:
   ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
   if err:=checkAIConnectivity(ctx,cfg,c,w.out,false);err!=nil{fmt.Fprintln(w.out,"✗ Проверка ИИ:",err)}
   cancel()
  }
  w.pauseMenu()
 }
}

func run()error{
 cfg,err:=loadConfig();if err!=nil{return err};catalog,err:=newCatalog();if err!=nil{return err}
 catalog.health=newModelHealth(modelHealthPath())
 command:="menu";if len(os.Args)>1{command=os.Args[1]}
 if command=="run"{
  ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop()
  return runBot(ctx,cfg)
 }
 if command=="panel"&&len(os.Args)>2&&os.Args[2]=="serve" {
  if os.Geteuid()!=0{return errors.New("доменная панель запускается только системной службой")}
  return webServeDomain(catalog)
 }
 w:=Wizard{bufio.NewReader(os.Stdin),os.Stdout}
 if command=="is-configured"{
  connected:=false;for id,p:=range cfg.Providers{spec:=catalog.Specs[id];if p.Enabled&&(p.APIKey!=""||spec.AnonymousKey!=""){connected=true}}
  if !connected||cfg.VKToken==""||cfg.GroupID<1{return errors.New("настройка не завершена")};return nil
 }
 if os.Geteuid()!=0{return errors.New("для управления выполните sudo aibot")}
 switch command{
 case "version","--version":fmt.Println("AI-bot Go v1.5.5")
 case "setup":
  fmt.Fprintln(w.out,"=== ПЕРВАЯ НАСТРОЙКА AI-bot ===")
  fmt.Fprintln(w.out,"Нужно сделать всего две вещи: (1) подключить сообщество ВК, (2) выбрать бесплатный ИИ.")
  w.setupVK(&cfg)
  w.setupModels(&cfg,catalog)
  if w.yes("Открыть дополнительные настройки (необязательно)",false){w.behavior(&cfg)}
  applySettings(w,cfg,catalog)
  fmt.Fprintln(w.out,"\n"+nextSetupStep(cfg,catalog))
  fmt.Fprintln(w.out,"Для подробной инструкции: sudo aibot guide")
 case "menu":menu(w,cfg,catalog)
 case "guide":fmt.Fprintln(w.out,vkSetupGuide(cfg.GroupID))
 case "next":fmt.Fprint(w.out,setupSummary(cfg,catalog));fmt.Fprintln(w.out,nextSetupStep(cfg,catalog))
 case "doctor","status":fmt.Println(doctor(context.Background(),cfg))
 case "check-models":
  ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
  defer cancel()
  return scanAvailableModels(ctx,cfg,catalog,os.Stdout,true)
 case "check-ai":
  networkOnly:=len(os.Args)==3&&os.Args[2]=="network"
  if len(os.Args)>3||len(os.Args)==3&&!networkOnly{return errors.New("использование: sudo aibot check-ai [network]")}
  ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
  defer cancel()
  return checkAIConnectivity(ctx,cfg,catalog,os.Stdout,networkOnly)
 case "vk-events":w.configureVKEvents(cfg)
 case "web":return webServe(cfg,catalog)
 case "panel":return panelCommand(w,os.Args[2:])
 case "logs":cmd:=exec.Command("journalctl","-u","ai-bot","-n","70","--no-pager");cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;return cmd.Run()
 case "restart":return service("restart")
 case "vk":w.setupVK(&cfg);applySettings(w,cfg,catalog)
 case "api":w.setupModels(&cfg,catalog);applySettings(w,cfg,catalog)
 case "behavior":w.behavior(&cfg);applySettings(w,cfg,catalog)
 case "clear":s,e:=openStore(dataPath());if e!=nil{return e};defer s.close();return s.clearMessages()
 default:return errors.New("команды: menu, setup, guide, next, doctor, check-models, check-ai, api, vk, behavior, logs, restart, panel, version; также: panel setup ДОМЕН / status / off / reset")
 }
 return nil
}
func main(){
 log.SetFlags(log.LstdFlags|log.Lmicroseconds)
 if err:=run();err!=nil{
  if !errors.Is(err,context.Canceled){fmt.Fprintln(os.Stderr,"AI-bot:",err);os.Exit(1)}
 }
}
