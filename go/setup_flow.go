package main

import (
 "context"
 "fmt"
 "io"
 "os"
 "os/exec"
 "strings"
 "time"
)

// Only commit a new provider key and model selection when a real Russian
// response has been obtained from this server. Failed trials cannot overwrite
// an existing working setup.
func (w Wizard) configureProviderWith(cfg *Config,c *Catalog,id string,mode int,network networkProbe,generate modelProbe)bool {
 spec,ok:=c.Specs[id]
 if !ok{return false}
 fmt.Fprintf(w.out,"\nПодключение %s\n1/3. Проверка HTTPS с сервера...\n",spec.Name)
 preCtx,stop:=context.WithTimeout(context.Background(),20*time.Second)
 var stages string;var netErr error
 consoleSpinner(w.out,"DNS, TCP и сертификат API",func(){stages,netErr=network(preCtx,aiProviderEndpoint(spec))})
 stop()
 if stages!=""{fmt.Fprintln(w.out,cleanTerminal(stages))}
 if netErr!=nil{
  fmt.Fprintf(w.out,"✗ Прямое подключение недоступно: %v\n",netErr)
  // When the service is reached via a configured proxy, a direct TCP test
  // is not conclusive. The authenticated test below is authoritative.
  if os.Getenv("HTTPS_PROXY")==""&&os.Getenv("https_proxy")==""{
   fmt.Fprintln(w.out,"Ключ не запрашивался и настройки не изменены. Проверьте DNS, TLS и сеть VDS.")
   return false
  }
  fmt.Fprintln(w.out,"Найден HTTPS-прокси. Проверю действительный API-маршрут после ввода ключа.")
 }
 fmt.Fprintf(w.out,"2/3. API-ключ. Получить: %s\n",spec.KeyURL)
 original:=cfg.Providers[id]
 trial:=*cfg
 trial.Providers=make(map[string]ProviderConfig,len(cfg.Providers)+1)
 for k,v:=range cfg.Providers{trial.Providers[k]=v}
 trial.EnabledModels=append([]string{},cfg.EnabledModels...)
 trial.AutoModelsProviders=append([]string{},cfg.AutoModelsProviders...)
 trial.RussianBlockedModels=append([]string{},cfg.RussianBlockedModels...)
 next:=original
 if spec.AnonymousKey==""||original.APIKey!=""{
  next.APIKey=w.secret("Введите API-ключ (Enter — оставить старый)",original.APIKey)
 }
 if strings.TrimSpace(next.APIKey)!=next.APIKey||len(next.APIKey)>4096||strings.ContainsAny(next.APIKey,"\n\r\t "){
  fmt.Fprintln(w.out,"✗ Ключ содержит пробелы/управляющие символы или слишком длинный. Настройки не изменены.")
  return false
 }
 if next.APIKey==""&&spec.AnonymousKey==""{
  fmt.Fprintln(w.out,"✗ Нужен API-ключ. Настройки не изменены.")
  return false
 }
 if id=="cloudflare"{
  next.CloudflareAccount=strings.TrimSpace(w.ask("Cloudflare Account ID",original.CloudflareAccount))
  if next.CloudflareAccount==""{
   fmt.Fprintln(w.out,"✗ Нужен Cloudflare Account ID. Настройки не изменены.")
   return false
  }
 }
 next.Enabled=true
 trial.Providers[id]=next
 trial.EnabledModels=removeProvider(trial.EnabledModels,id)
 trial.AutoModelsProviders=removeExact(trial.AutoModelsProviders,id)
 trial.AutoModelsProviders=append(trial.AutoModelsProviders,id)
 trial.RussianBlockedModels=removeProvider(trial.RussianBlockedModels,id)
 fmt.Fprintln(w.out,"3/3. Проверка ключа и всех бесплатных моделей на русский язык.")
 scanCtx,cancel:=context.WithTimeout(context.Background(),30*time.Minute)
 err:=scanProviderModelsWith(scanCtx,trial,c,w.out,id,generate)
 cancel()
 if err!=nil{
  fmt.Fprintln(w.out,"✗ Подключение не подтверждено:",err)
  fmt.Fprintln(w.out,"Прежний ключ и выбор моделей оставлены без изменений.")
  return false
 }
 valid:=c.allowedFor(id,trial)
 if len(valid)==0{
  fmt.Fprintln(w.out,"✗ Ни одна модель этого сервиса не прошла проверку русского языка.")
  return false
 }
 if mode==2{
  labels:=make([]string,len(valid))
  for i,key:=range valid{labels[i]=c.label(key)}
  selected:=w.chooseMany("ВЫБЕРИТЕ ПРОВЕРЕННЫЕ МОДЕЛИ • "+spec.Name,labels)
  if len(selected)==0{
   fmt.Fprintln(w.out,"Выбор отменён, прежние настройки сохранены.")
   return false
  }
  trial.AutoModelsProviders=removeExact(trial.AutoModelsProviders,id)
  for _,i:=range selected{trial.EnabledModels=append(trial.EnabledModels,valid[i])}
 }
 if c.defaultModel(trial)==""{ // A dynamic catalogue may have changed mid-check.
  fmt.Fprintln(w.out,"✗ Нет пригодных моделей для выбора. Настройки не изменены.")
  return false
 }
 trial.DefaultModel=c.defaultModel(trial)
 // Safe to apply only after all user decisions and authenticated checks.
 *cfg=trial
 fmt.Fprintf(w.out,"✓ %s подключён. Подтверждено русскоязычных моделей: %d.\n",spec.Name,len(valid))
 fmt.Fprintln(w.out,"Ключ сохранится только в защищённом локальном конфиге при выходе из раздела.")
 return true
}

func (w Wizard) setupGuide(cfg *Config,c *Catalog)bool{
 fmt.Fprintln(w.out,"\nМАСТЕР ПОДКЛЮЧЕНИЯ AI-BOT")
 fmt.Fprintln(w.out,"1. ВК: сообщество и его токен; 2. Бесплатный ИИ; 3. Проверка и запуск.")
 old:=*cfg
 originalVKToken:=cfg.VKToken
 originalGroup:=cfg.GroupID
 if cfg.VKToken==""||cfg.GroupID==0{
  w.setupVK(cfg)
 }else{
  fmt.Fprintf(w.out,"✓ ВК уже настроен (ID %d). Можно изменить его позже в «Подключения».\n",cfg.GroupID)
 }
 if cfg.VKToken==""||cfg.GroupID<1{
  cfg.VKToken=originalVKToken;cfg.GroupID=originalGroup
  fmt.Fprintln(w.out,"Завершите подключение ВК, прежде чем запускать бота.")
  return false
 }
 options:=make([]menuOption,0,len(c.Order)+1)
 for i,id:=range c.Order{
  p:=c.Specs[id]
  state:="Не подключён"
  if cfg.Providers[id].Enabled{state="Настроен"}
  options=append(options,menuOption{i+1,p.Name,"Ключ: "+p.KeyURL,state})
 }
 options=append(options,menuOption{0,"Настроить позже","Сохранить текущие настройки ВК",""})
 selected:=w.chooseMenu("ВЫБЕРИТЕ ИИ-СЕРВИС","Сначала проверим его доступность, затем введём ключ и проверим русский язык.",options)
 if selected==0{
  fmt.Fprintln(w.out,"ИИ можно добавить через «Подключения» → «ИИ-провайдеры».")
  return cfg.VKToken!=old.VKToken||cfg.GroupID!=old.GroupID
 }
 id:=c.Order[selected-1]
 if !w.configureProviderWith(cfg,c,id,1,probeDirectHTTPS,newAI(c).generate){
  // Preserve successful VK changes while rolling back failed AI changes.
  return cfg.VKToken!=old.VKToken||cfg.GroupID!=old.GroupID
 }
 return true
}

func(w Wizard) connectionsHub(cfg *Config,c *Catalog){
 for{
  connected:=0
  for _,id:=range c.Order{if cfg.Providers[id].Enabled{connected++}}
  selected:=w.chooseMenu("ПОДКЛЮЧЕНИЯ","Здесь настраиваются только подключения. Поиск, лимиты и обслуживание находятся отдельно.",[]menuOption{
   {1,"ВКонтакте","Токен сообщества, ID и проверка входящих событий",func()string{if cfg.VKToken!=""{return "Настроено"};return "Нет токена"}()},
   {2,"Бесплатные ИИ","Проверка HTTPS → ввод API-ключа → тест русского → выбор моделей",fmt.Sprintf("%d сервисов",connected)},
   {3,"События ВК (Long Poll)","Разрешить получение сообщений и нажатия кнопок",""},
   {0,"Назад","Главное меню",""},
  })
  switch selected{
  case 0:return
  case 1:w.setupVK(cfg);applySettings(w,*cfg,c)
  case 2:w.setupModels(cfg,c);applySettings(w,*cfg,c)
  case 3:w.configureVKEvents(*cfg)
  }
  w.pauseMenu()
 }
}

func(w Wizard) diagnosticsHub(cfg Config,c *Catalog){
 for{
  action:=w.chooseMenu("ДИАГНОСТИКА","Проверки выполняются с этого VDS; ключи не выводятся.",[]menuOption{
   {1,"Состояние бота и ВК","Проверить службу, SQLite, Long Poll и каталоги",""},
   {2,"Все модели + русский язык","Реальный запрос к каждой модели; только прошедшие появятся в списке",""},
   {3,"Проверка сети без запросов к ИИ","DNS, TCP, TLS; без расхода бесплатной квоты",""},
   {4,"Журнал ошибок","Последние 70 строк службы AI-bot",""},
   {5,"Статистика","Очередь, число обращений и успешность моделей",""},
   {0,"Назад","Главное меню",""},
  })
  switch action{
  case 0:return
  case 1:
   var result string
   consoleSpinner(w.out,"Проверка состояния",func(){result=doctor(context.Background(),cfg)})
   fmt.Fprintln(w.out,result)
  case 2:
   ctx,cancel:=context.WithTimeout(context.Background(),time.Hour)
   if err:=scanAvailableModels(ctx,cfg,c,w.out,true);err!=nil{fmt.Fprintln(w.out,"Проверка:",err)}
   cancel()
  case 3:
   ctx,cancel:=context.WithTimeout(context.Background(),10*time.Minute)
   if err:=checkAIConnectivity(ctx,cfg,c,w.out,true);err!=nil{fmt.Fprintln(w.out,"Проверка:",err)}
   cancel()
  case 4:
   fmt.Fprintln(w.out,"Журнал AI-bot: sudo aibot logs")
   showBotLogs(w.out,70)
  case 5:
   store,err:=openStore(dataPath())
   if err!=nil{fmt.Fprintln(w.out,"Ошибка статистики:",err)}else{
    fmt.Fprintln(w.out,store.consoleUsageStats(cfg,c))
    store.close()
   }
  }
  w.pauseMenu()
 }
}

func(w Wizard) maintenanceHub(){
 for{
  action:=w.chooseMenu("ОБСЛУЖИВАНИЕ","Управление службой и хранимыми данными.",[]menuOption{
   {1,"Перезапустить службу","Применить изменения параметров AI-bot",""},
   {2,"Проверить журнал","Ошибки запуска, VK API и моделей",""},
   {3,"Очистить историю и очередь","Удаление без возможности восстановления","Осторожно"},
   {4,"Создать резервную копию","Согласованный снимок SQLite и настроек",""},
   {0,"Назад","Главное меню",""},
  })
  switch action{
  case 0:return
  case 1:
   var err error
   consoleSpinner(w.out,"Перезапуск службы",func(){err=service("restart")})
   if err!=nil{fmt.Fprintln(w.out,"Ошибка:",err)}else{fmt.Fprintln(w.out,"✓ Служба перезапущена")}
  case 2:showBotLogs(w.out,70)
  case 4:
   store,err:=openStore(dataPath())
   if err!=nil{fmt.Fprintln(w.out,"Ошибка резервного копирования:",err)}else{
    folder,err:=backupState(store)
    if err!=nil{fmt.Fprintln(w.out,"Ошибка копирования:",err)}else{fmt.Fprintln(w.out,"Резервная копия:",folder)}
    store.close()
   }
  case 3:
   if w.ask("Для удаления истории введите ДА","нет")=="ДА"{
    store,err:=openStore(dataPath())
    if err!=nil{fmt.Fprintln(w.out,"Ошибка SQLite:",err)
    }else{
     if err=store.clearMessages();err!=nil{fmt.Fprintln(w.out,"Ошибка:",err)
     }else{fmt.Fprintln(w.out,"✓ Сообщения и очередь удалены")}
     store.close()
    }
   }
  }
  w.pauseMenu()
 }
}

func showBotLogs(out io.Writer,lines int){
 // Avoid exposing the user's API keys: model/client code logs only safe errors.
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,"journalctl","-u","ai-bot","-n",fmt.Sprint(lines),"--no-pager")
 cmd.Stdout=out;cmd.Stderr=out
 if err:=cmd.Run();err!=nil{fmt.Fprintln(out,"Журнал недоступен:",err)}
}
