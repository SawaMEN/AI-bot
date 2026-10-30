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
 fmt.Fprintf(w.out,"\n════ %s ════\n",p.Name)
 fmt.Fprintf(w.out,"Состояние: %s\n",switchLabel(settings.Enabled))
 fmt.Fprintf(w.out,"Где получить ключ: %s\n",p.KeyURL)
 fmt.Fprintln(w.out,"Используйте только бесплатный тариф. Не включайте платный биллинг и не добавляйте банковскую карту.")
 if id=="aihorde"{
  fmt.Fprintln(w.out,"Внимание: AI Horde использует чужие волонтёрские машины. Не подходит для закрытых и личных переписок.")
  fmt.Fprintln(w.out,"Для пробного доступа возможен анонимный ключ, но работа зависит от наличия волонтёров.")
 }
 fmt.Fprintln(w.out,"\n  1. ✓ ВКЛ — автоматически выбирать доступные бесплатные модели")
 fmt.Fprintln(w.out,"  2. ✓ ВКЛ — выбрать конкретные бесплатные модели")
 fmt.Fprintln(w.out,"  3. ✗ ВЫКЛ — отключить этот сервис")
 fmt.Fprintln(w.out,"  0. Назад (ничего не менять)")
 choice:=w.integer("Ваш выбор",0,0,3)
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
  if p.FreePolicy!="curated"{
   ctx,cancel:=context.WithTimeout(context.Background(),25*time.Second)
   key:=settings.APIKey;if key==""{key=p.AnonymousKey}
   models,err:=c.discover(ctx,p,key);cancel()
   if err!=nil{fmt.Fprintln(w.out,"Каталог временно недоступен:",err)}else{
    c.mu.Lock();c.verified[id]=models;c.mu.Unlock()
   }
  }
  list:=c.allowedFor(id,*cfg)
  if len(list)==0{
   fmt.Fprintln(w.out,"Нет подтверждённых бесплатных моделей. Включите автоподбор позже, когда каталог станет доступен.")
   cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id)
  }else{
   for i,k:=range list{fmt.Fprintf(w.out,"  %d. %s\n",i+1,c.label(k))}
   fmt.Fprintln(w.out,"Введите номера нужных моделей через запятую, например: 1,3")
   raw:=w.ask("Модели","1")
   for _,entry:=range strings.Split(raw,","){
    n,err:=strconv.Atoi(strings.TrimSpace(entry))
    if err==nil&&n>0&&n<=len(list)&&!contains(cfg.EnabledModels,list[n-1]){
     cfg.EnabledModels=append(cfg.EnabledModels,list[n-1])
    }
   }
   if len(c.allowedFor(id,*cfg))==0{cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id)}
  }
  fmt.Fprintln(w.out,"✓ Сервис включён. Выбранные модели можно поменять здесь в любой момент.")
 }
 if cfg.DefaultModel==""{cfg.DefaultModel=c.defaultModel(*cfg)}
 fmt.Fprintln(w.out,"Далее: вернитесь в меню и откройте пункт «Диагностика», затем проверьте упоминание бота в беседе ВК.")
}
func removeExact(list []string,key string)[]string{out:=[]string{};for _,s:=range list{if s!=key{out=append(out,s)}};return out}
func (c *Catalog) allowedFor(provider string,cfg Config)[]string{
 keys:=c.enabled(cfg);result:=[]string{};for _,key:=range keys{if strings.HasPrefix(key,provider+":"){result=append(result,key)}}
 if len(result)>0{return result}
 p:=c.Specs[provider];if p.FreePolicy=="curated"{for _,m:=range p.Models{result=append(result,provider+":"+m[0])};return result}
 c.mu.RLock();defer c.mu.RUnlock();for _,m:=range c.verified[provider]{result=append(result,provider+":"+m.ID)}
 return result
}
func(w Wizard) setupModels(cfg *Config,c *Catalog){
 for{
  fmt.Fprintln(w.out,"\n════════ БЕСПЛАТНЫЕ ИИ-СЕРВИСЫ ════════")
  fmt.Fprintln(w.out,"Для ответа бот обращается к внешнему ИИ. Сервис ИИ — не сообщество ВК.")
  fmt.Fprintln(w.out,"Выберите сервис, откройте ссылку для бесплатного API-ключа и включите нужные модели.")
  for i,id:=range c.Order{
   p:=c.Specs[id]
   fmt.Fprintf(w.out," %2d. %s  %s\n",i+1,switchLabel(cfg.Providers[id].Enabled),p.Name)
  }
  n:=len(c.Order)
  fmt.Fprintf(w.out,"\n %2d. Основная модель: %s\n",n+1,func()string{if cfg.DefaultModel!=""{return c.label(cfg.DefaultModel)};return "не выбрана"}())
  fmt.Fprintf(w.out," %2d. Проверить доступные бесплатные модели\n",n+2)
  fmt.Fprintf(w.out," %2d. Включить автоподбор у уже подключённых сервисов\n",n+3)
  fmt.Fprintf(w.out," %2d. Проверить русский язык и доступность моделей\n",n+4)
  fmt.Fprintln(w.out,"  0. Вернуться в главное меню")
  choice:=w.integer("Номер пункта",0,0,n+4)
  if choice==0{return}
  if choice<=n{w.provider(cfg,c,c.Order[choice-1])}
  if choice==n+1{
   ctx,cancel:=context.WithTimeout(context.Background(),40*time.Second)
   problems:=c.refresh(ctx,*cfg);cancel()
   available:=c.enabled(*cfg)
   if len(available)==0{
    fmt.Fprintln(w.out,"✗ Моделей пока нет. Сначала подключите один из сервисов выше и включите подбор.")
    if len(problems)>0{fmt.Fprintf(w.out,"Ошибка каталога: %v\n",problems)}
    continue
   }
   for i,k:=range available{fmt.Fprintf(w.out," %2d. %s\n",i+1,c.label(k))}
   chosen:=w.integer("Номер основной модели",1,1,len(available))
   cfg.DefaultModel=available[chosen-1]
   fmt.Fprintln(w.out,"✓ Основная модель сохранена. Она используется по умолчанию в новых ответах.")
  }
  if choice==n+2{
   ctx,cancel:=context.WithTimeout(context.Background(),40*time.Second)
   problems:=c.refresh(ctx,*cfg);cancel()
   fmt.Fprintf(w.out,"Доступно моделей: %d\n",len(c.enabled(*cfg)))
   if len(problems)>0{fmt.Fprintf(w.out,"Что не удалось проверить: %v\n",problems)
   }else{fmt.Fprintln(w.out,"✓ Каталоги проверены.")}
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
   ctx,cancel:=context.WithTimeout(context.Background(),15*time.Minute)
   if err:=checkRussianModels(ctx,cfg,c,w.out);err!=nil{fmt.Fprintln(w.out,"Ошибка проверки:",err)}
   cancel()
  }
  if err:=saveConfig(*cfg);err!=nil{fmt.Fprintln(w.out,"✗ Не удалось сохранить настройки:",err)}
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
 if err:=newVK(cfg).setLongPollEvents(ctx);err!=nil{
  fmt.Fprintln(w.out,"✗ ВК отказал в изменении:",err)
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
func menu(w Wizard,cfg Config,c *Catalog){
 for{
  fmt.Fprintln(w.out,"\n════════════════ AI-bot · УПРАВЛЕНИЕ ════════════════")
  fmt.Fprintln(w.out,"Текущая настройка:")
  fmt.Fprint(w.out,setupSummary(cfg,c))
  fmt.Fprintf(w.out,"  %s  Поиск в интернете\n",switchLabel(cfg.SearchEnabled))
  fmt.Fprintf(w.out,"  %s  Резервный бесплатный ИИ\n",switchLabel(cfg.FreeFallback))
  fmt.Fprintln(w.out,"\nЧто хотите сделать?")
  fmt.Fprintln(w.out,"  1. Подключить ВК: страница сообщества, токен и ID")
  fmt.Fprintln(w.out,"  2. Подключить бесплатный ИИ / выбрать модель")
  fmt.Fprintln(w.out,"  3. Контекст сообщений, поиск, лимиты и приватность")
  fmt.Fprintln(w.out,"  4. Проверить, всё ли работает (диагностика)")
  fmt.Fprintln(w.out,"  5. Перезапустить бота")
  fmt.Fprintln(w.out,"  6. Посмотреть ошибки и события (журнал)")
  fmt.Fprintln(w.out,"  7. Удалить сохранённую переписку и очередь")
  fmt.Fprintln(w.out,"  8. Веб-панель по домену / управление без браузера")
  fmt.Fprintln(w.out,"  9. Включить получение сообщений и кнопок ВК")
  fmt.Fprintln(w.out," 10. Где найти ID и токен ВК? (пошаговая инструкция)")
  fmt.Fprintln(w.out," 11. Что делать дальше?")
  fmt.Fprintln(w.out,"  0. Выход")
  fmt.Fprintln(w.out,"\n"+nextSetupStep(cfg,c))
  choice:=w.integer("Введите номер пункта",0,0,11)
  switch choice{
  case 0:return
  case 1:w.setupVK(&cfg);applySettings(w,cfg,c)
  case 2:w.setupModels(&cfg,c);applySettings(w,cfg,c)
  case 3:w.behavior(&cfg);applySettings(w,cfg,c)
  case 4:
   fmt.Fprintln(w.out,"\n════════ ПРОВЕРКА ПОДКЛЮЧЕНИЙ ════════")
   fmt.Fprintln(w.out,doctor(context.Background(),cfg))
   fmt.Fprintln(w.out,"Если обнаружены проблемы, исправьте их в пунктах 1, 2 или 9.")
   fmt.Fprintln(w.out,"Для проверки ответа добавьте сообщество в беседу и задайте ему вопрос.")
  case 5:
   if err:=service("restart");err!=nil{fmt.Fprintln(w.out,"✗ Перезапуск не удался:",err)
   }else{fmt.Fprintln(w.out,"✓ Команда перезапуска отправлена. Статус: sudo systemctl status ai-bot")}
  case 6:
   fmt.Fprintln(w.out,"\nПоследние 60 строк журнала службы:")
   cmd:=exec.Command("journalctl","-u","ai-bot","-n","60","--no-pager")
   cmd.Stdout=w.out;cmd.Stderr=w.out
   if err:=cmd.Run();err!=nil{fmt.Fprintln(w.out,"Не удалось открыть журнал:",err)}
  case 7:
   fmt.Fprintln(w.out,"Удалятся сохранённая переписка и ожидающие запросы. Это действие нельзя отменить.")
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
   fmt.Fprintln(w.out,"\n════════ ЧТО ДЕЛАТЬ ДАЛЬШЕ ════════")
   fmt.Fprint(w.out,setupSummary(cfg,c))
   fmt.Fprintln(w.out,nextSetupStep(cfg,c))
   fmt.Fprintln(w.out,"Полная инструкция: sudo aibot guide")
  }
 }
}
func run()error{
 cfg,err:=loadConfig();if err!=nil{return err};catalog,err:=newCatalog();if err!=nil{return err}
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
 case "version","--version":fmt.Println("AI-bot Go v1.5.2")
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
  ctx,cancel:=context.WithTimeout(context.Background(),15*time.Minute)
  defer cancel()
  return checkRussianModels(ctx,&cfg,catalog,os.Stdout)
 case "vk-events":w.configureVKEvents(cfg)
 case "web":return webServe(cfg,catalog)
 case "panel":return panelCommand(w,os.Args[2:])
 case "logs":cmd:=exec.Command("journalctl","-u","ai-bot","-n","70","--no-pager");cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;return cmd.Run()
 case "restart":return service("restart")
 case "vk":w.setupVK(&cfg);applySettings(w,cfg,catalog)
 case "api":w.setupModels(&cfg,catalog);applySettings(w,cfg,catalog)
 case "behavior":w.behavior(&cfg);applySettings(w,cfg,catalog)
 case "clear":s,e:=openStore(dataPath());if e!=nil{return e};defer s.close();return s.clearMessages()
 default:return errors.New("команды: menu, setup, guide, next, doctor, check-models, api, vk, behavior, logs, restart, panel, version; также: panel setup ДОМЕН / status / off / reset")
 }
 return nil
}
func main(){
 log.SetFlags(log.LstdFlags|log.Lmicroseconds)
 if err:=run();err!=nil{
  if !errors.Is(err,context.Canceled){fmt.Fprintln(os.Stderr,"AI-bot:",err);os.Exit(1)}
 }
}
