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
 state:="отсутствует"
 if current!=""{state="сохранён"}
 fmt.Fprintf(w.out,"%s (%s; Enter — оставить, - — удалить): ",label,state)
 // Hide input using termios rather than shelling out to stty. Restore echo
 // even on read errors; piping input continues to work without a terminal.
 if w.in!=nil&&terminalInputAvailable(){
  old,err:=termGet(int(os.Stdin.Fd()))
  if err==nil{
   masked:=old;masked.Lflag &^=syscall.ECHO
   if termSet(int(os.Stdin.Fd()),&masked)==nil{
    defer func(){_ =termSet(int(os.Stdin.Fd()),&old);fmt.Fprintln(w.out)}()
   }
  }
 }
 value,err:=w.in.ReadString('\n')
 if err!=nil&&len(value)==0{return current}
 value=strings.TrimSpace(value)
 if value=="-"{return ""}
 if value==""{return current}
 return value
}
func terminalInputAvailable()bool{
 info,err:=os.Stdin.Stat()
 return err==nil&&info.Mode()&os.ModeCharDevice!=0
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
 fmt.Fprintln(w.out,"\nВКОНТАКТЕ • ПОДКЛЮЧЕНИЕ СООБЩЕСТВА")
 fmt.Fprintln(w.out,"Нужны только числовой ID и токен сообщества с правом на сообщения.")
 fmt.Fprintln(w.out,"ВК → Управление сообществом → Работа с API → Ключи доступа.")
 fmt.Fprintln(w.out,"Пошаговая инструкция доступна отдельно в разделе «Помощь».")
 cfg.VKToken=w.secret("Токен ВК",cfg.VKToken)
 cfg.GroupID=int64(w.integer("Числовой ID сообщества",int(cfg.GroupID),1,999999999))
 fmt.Fprintf(w.out,"Токен: %s; сообщество: %d\n",readyLabel(cfg.VKToken!=""),cfg.GroupID)
 fmt.Fprintln(w.out,"Далее: включите Long Poll (получение сообщений и нажатия кнопок).")
}

func contains(list []string,key string)bool{for _,s:=range list{if s==key{return true}};return false}
func removeProvider(list []string,provider string)[]string{
 out:=[]string{};for _,s:=range list{if !strings.HasPrefix(s,provider+":"){out=append(out,s)}};return out
}
func(w Wizard) provider(cfg *Config,c *Catalog,id string){
 p:=c.Specs[id]
 settings:=cfg.Providers[id]
 status:="Отключён"
 if settings.Enabled{status="Включён"}
 keyState:="нет ключа"
 if settings.APIKey!=""||p.AnonymousKey!=""{keyState="ключ сохранён"}
 choice:=w.chooseMenu("ИИ-СЕРВИС • "+p.Name,
  fmt.Sprintf("Доступен ключ: %s\nСостояние: %s; %s; моделей проверено: %d",
   p.KeyURL,status,keyState,len(c.allowedFor(id,*cfg))),[]menuOption{
   {1,"Подключить с автоподбором","HTTPS → ключ → проверка всех моделей на русский → сохранение","Рекомендуется"},
   {2,"Подключить с ручным выбором","После проверки отметить несколько работающих моделей",""},
   {3,"Отключить этот сервис","Оставить ключ сохранённым для будущего подключения",""},
   {0,"Назад","Без изменений",""},
  })
 switch choice{
 case 0:return
 case 3:
  settings.Enabled=false;cfg.Providers[id]=settings
  cfg.AutoModelsProviders=removeExact(cfg.AutoModelsProviders,id)
  cfg.EnabledModels=removeProvider(cfg.EnabledModels,id)
  fmt.Fprintln(w.out,"✗ ВЫКЛ. Сервис отключён, его ключ сохранён.")
 default:
  w.configureProviderWith(cfg,c,id,choice,probeDirectHTTPS,newAI(c).generate)
 }
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
  opts:=make([]menuOption,0,len(c.Order)+4)
  for i,id:=range c.Order{
   p:=c.Specs[id];setting:=cfg.Providers[id]
   state:="Не подключён"
   if setting.Enabled{state=fmt.Sprintf("Работают: %d",len(c.allowedFor(id,*cfg)))}
   opts=append(opts,menuOption{i+1,p.Name,
    "API-ключ, проверка с VDS и русский язык • "+p.KeyURL,state})
  }
  n:=len(c.Order)
  model:="Не выбрана"
  if selected:=c.defaultModel(*cfg);selected!=""{model=c.label(selected)}
  opts=append(opts,
   menuOption{n+1,"Основная модель","Только из подтверждённых работающих • "+model,""},
   menuOption{n+2,"Перепроверить все модели","Повторно проверить ключи, русский язык и показать список",""},
   menuOption{0,"Назад","К подключению",""},
  )
  choice:=w.chooseMenu("ИИ-ПРОВАЙДЕРЫ И МОДЕЛИ",
   fmt.Sprintf("Подтверждённых моделей: %d\nВыберите провайдера для автоматизированного подключения.",len(c.enabled(*cfg))),opts)
  switch {
  case choice==0:return
  case choice>=1&&choice<=n:
   w.provider(cfg,c,c.Order[choice-1])
   if err:=saveConfig(*cfg);err!=nil{fmt.Fprintln(w.out,"Ошибка сохранения:",err)}
   w.pauseMenu()
  case choice==n+1:
   allowed:=c.enabled(*cfg)
   if len(allowed)==0{
    fmt.Fprintln(w.out,"Нет подтверждённых моделей. Сначала подключите сервис и проверьте русский язык.")
    w.pauseMenu();continue
   }
   options:=make([]menuOption,0,len(allowed)+1)
   for i,key:=range allowed{
    state:="Доступна";if key==cfg.DefaultModel{state="Основная"}
    options=append(options,menuOption{i+1,c.label(key),key,state})
   }
   options=append(options,menuOption{0,"Назад","Не менять выбор",""})
   selected:=w.chooseMenu("ОСНОВНАЯ МОДЕЛЬ","Выбор доступен только среди успешно прошедших проверку.",options)
   if selected>0{
    cfg.DefaultModel=allowed[selected-1]
    if err:=saveConfig(*cfg);err!=nil{fmt.Fprintln(w.out,"Ошибка сохранения:",err)}
   }
  case choice==n+2:
   ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
   if err:=scanAvailableModels(ctx,*cfg,c,w.out,true);err!=nil{fmt.Fprintln(w.out,"Проверка:",err)}
   cancel()
   w.pauseMenu()
  }
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
  serviceState:="Не настроен"
  if cfg.VKToken!=""&&cfg.GroupID>0{serviceState=fmt.Sprintf("Сообщество %d",cfg.GroupID)}
  selectedModel:="Не выбрана"
  if key:=c.defaultModel(cfg);key!=""{selectedModel=c.label(key)}
  summary:=fmt.Sprintf("Служба: %s • ВК: %s\nИИ-провайдеров: %d • Работающих моделей: %d\nОсновная модель: %s\n%s",
   activeBotService(),serviceState,connected,len(c.enabled(cfg)),selectedModel,nextSetupStep(cfg,c))
  choice:=w.chooseMenu("AI-BOT • УПРАВЛЕНИЕ",summary,[]menuOption{
   {1,"Автоматическая настройка","ВК → доступность API → ввод ключа → русский язык → запуск","Начать здесь"},
   {2,"Подключения","Сообщество ВК, ИИ-провайдеры и события Long Poll",""},
   {3,"Поведение бота","Контекст, поиск, лимиты и резервные модели",""},
   {4,"Диагностика","Состояние системы, русский язык всех ИИ, сеть и журнал",""},
   {5,"Веб-панель","HTTPS-домен или временный доступ через SSH",""},
   {6,"Обслуживание","Перезапуск, журнал и очистка данных",""},
   {7,"Помощь","Как подключить сообщество ВКонтакте",""},
   {0,"Выход","Завершить управление",""},
  })
  switch choice{
  case 0:return
  case 1:
   if w.setupGuide(&cfg,c){applySettings(w,cfg,c)}
  case 2:w.connectionsHub(&cfg,c)
  case 3:w.behavior(&cfg);applySettings(w,cfg,c)
  case 4:w.diagnosticsHub(cfg,c)
  case 5:w.panelMenu()
  case 6:w.maintenanceHub()
  case 7:fmt.Fprintln(w.out,vkSetupGuide(cfg.GroupID))
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
 case "version","--version":fmt.Println("AI-bot Go v1.5.8")
 case "setup":
  if w.setupGuide(&cfg,catalog){applySettings(w,cfg,catalog)}
  fmt.Fprintln(w.out,"\n"+nextSetupStep(cfg,catalog))
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
