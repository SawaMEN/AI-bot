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
 defaultVal:="н";if current{defaultVal="д"}
 val:=strings.ToLower(w.ask(label+" (д/н)",defaultVal))
 return val=="д"||val=="да"||val=="y"||val=="yes"
}
func(w Wizard) setupVK(cfg *Config){
 fmt.Fprintln(w.out,"=== Настройка ВК ===")
 fmt.Fprintln(w.out,"Сообщество → Управление → Сообщения → включите сообщения, Bots Long Poll и события message_new/message_event.")
 fmt.Fprintln(w.out,"Для контекста всех участников администратор беседы должен дать сообществу доступ ко всей переписке.")
 cfg.VKToken=w.secret("Токен сообщества ВК",cfg.VKToken)
 cfg.GroupID=int64(w.integer("Числовой ID сообщества",int(cfg.GroupID),1,999999999))
}
func contains(list []string,key string)bool{for _,s:=range list{if s==key{return true}};return false}
func removeProvider(list []string,provider string)[]string{
 out:=[]string{};for _,s:=range list{if !strings.HasPrefix(s,provider+":"){out=append(out,s)}};return out
}
func(w Wizard) provider(cfg *Config,c *Catalog,id string){
 p:=c.Specs[id];settings:=cfg.Providers[id]
 fmt.Fprintf(w.out,"\n%s\nКлюч: %s\n",p.Name,p.KeyURL)
 fmt.Fprintln(w.out,"Используйте бесплатный тариф без банковской карты и без платного fallback.")
 if id=="aihorde"{fmt.Fprintln(w.out,"AI Horde — волонтёрская сеть. НЕ включайте для личных/закрытых бесед. Можно без ключа (0000000000).")}
 settings.APIKey=w.secret("API-ключ",settings.APIKey)
 if id=="cloudflare"{settings.CloudflareAccount=w.ask("Cloudflare Account ID",settings.CloudflareAccount)}
 if settings.APIKey==""&&p.AnonymousKey==""{settings.Enabled=false;cfg.Providers[id]=settings;fmt.Fprintln(w.out,"Нет ключа. Провайдер отключён.");return}
 choice:=w.integer("1 — все бесплатные автоматически, 2 — выбрать номера, 3 — отключить",1,1,3)
 cfg.AutoModelsProviders=removeExact(cfg.AutoModelsProviders,id)
 cfg.EnabledModels=removeProvider(cfg.EnabledModels,id)
 if choice==3{settings.Enabled=false;cfg.Providers[id]=settings;return}
 settings.Enabled=true;cfg.Providers[id]=settings
 if choice==1{cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id);fmt.Fprintln(w.out,"Включён безопасный автоматический выбор.")}
 if choice==2{
  ctx,cancel:=context.WithTimeout(context.Background(),25*time.Second);defer cancel()
  if p.FreePolicy!="curated"{
   key:=settings.APIKey;if key==""{key=p.AnonymousKey}
   discovered,err:=c.discover(ctx,p,key)
   if err!=nil{fmt.Fprintln(w.out,"Каталог временно недоступен:",err)}else{
    c.mu.Lock();c.verified[id]=discovered;c.mu.Unlock()
   }
  }
  list:=c.allowedFor(id,*cfg)
  if len(list)==0{fmt.Fprintln(w.out,"Пока нет доступных подтверждённо бесплатных моделей. Используйте автоматический режим позже.");return}
  for i,k:=range list{fmt.Fprintf(w.out,"%d. %s\n",i+1,c.label(k))}
  raw:=w.ask("Номера через запятую, например 1,3","1")
  for _,entry:=range strings.Split(raw,","){n,err:=strconv.Atoi(strings.TrimSpace(entry));if err==nil&&n>0&&n<=len(list)&&!contains(cfg.EnabledModels,list[n-1]){cfg.EnabledModels=append(cfg.EnabledModels,list[n-1])}}
 }
 if cfg.DefaultModel==""{cfg.DefaultModel=c.defaultModel(*cfg)}
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
  fmt.Fprintln(w.out,"\n=== Бесплатные ИИ (без платных API) ===")
  for i,id:=range c.Order{p:=c.Specs[id];status:="выкл";if cfg.Providers[id].Enabled{status="ВКЛ"}
   fmt.Fprintf(w.out,"%d. %s [%s]\n",i+1,p.Name,status)}
  n:=len(c.Order);fmt.Fprintf(w.out,"%d. Основная модель\n%d. Проверить каталоги\n%d. Включить автоподбор у подключённых\n0. Назад\n",n+1,n+2,n+3)
  choice:=w.integer("Пункт",0,0,n+3)
  if choice==0{return}
  if choice<=n{w.provider(cfg,c,c.Order[choice-1])}
  if choice==n+1{
   c.refresh(context.Background(),*cfg);available:=c.enabled(*cfg)
   if len(available)==0{fmt.Fprintln(w.out,"Нет доступных моделей.");continue}
   for i,k:=range available{fmt.Fprintf(w.out,"%d. %s\n",i+1,c.label(k))}
   selected:=w.integer("Номер основной модели",1,1,len(available));cfg.DefaultModel=available[selected-1]
  }
  if choice==n+2{errorsBy:=c.refresh(context.Background(),*cfg);fmt.Fprintf(w.out,"Всего сейчас подтверждено %d моделей. Ошибки: %v\n",len(c.enabled(*cfg)),errorsBy)}
  if choice==n+3{for _,id:=range c.Order{if cfg.Providers[id].Enabled&&!contains(cfg.AutoModelsProviders,id){cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id)}};fmt.Fprintln(w.out,"Автоподбор включён у всех подключённых.")}
  if err:=saveConfig(*cfg);err!=nil{fmt.Fprintln(w.out,"Ошибка сохранения:",err)}
 }
}
func(w Wizard) behavior(cfg *Config){
 fmt.Fprintln(w.out,"\n=== Настройки и бесплатные лимиты ===")
 cfg.ContextMessages=w.integer("Сообщений контекста (максимум)",cfg.ContextMessages,1,100)
 cfg.ContextHours=w.integer("Часов контекста",cfg.ContextHours,1,168)
 cfg.RetentionDays=w.integer("Дней хранения",cfg.RetentionDays,1,30)
 cfg.MaxRequestsHour=w.integer("Вызовов API за час на сервер",cfg.MaxRequestsHour,1,10000)
 cfg.MaxUserRequestsHour=w.integer("Вызовов за час на участника",cfg.MaxUserRequestsHour,1,1000)
 cfg.MaxChatRequestsHour=w.integer("Вызовов за час на беседу",cfg.MaxChatRequestsHour,1,1000)
 cfg.CooldownSeconds=w.integer("Пауза между ответами в беседе, сек",cfg.CooldownSeconds,0,3600)
 cfg.SearchEnabled=w.yes("Разрешить поиск в интернете",cfg.SearchEnabled)
 cfg.FreeFallback=w.yes("Разрешить резервный бесплатный ИИ (передаст контекст другому провайдеру)",cfg.FreeFallback)
}
func (w Wizard) configureVKEvents(cfg Config){
 fmt.Fprintln(w.out, "Это изменит настройки событий ВК для сообщества. Другие боты тоже могут использовать Long Poll.")
 fmt.Fprintln(w.out, "Будут включены только Long Poll, message_new и message_event; остальные события специально не отключаются.")
 if w.ask("Для подтверждения введите ДА", "нет")!="ДА"{
  fmt.Fprintln(w.out,"Изменения отменены.");return
 }
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 if err:=newVK(cfg).setLongPollEvents(ctx);err!=nil{
  fmt.Fprintln(w.out,"VK API не разрешил изменение:",err)
  fmt.Fprintln(w.out,"Включите Long Poll, message_new и message_event в настройках сообщества вручную.")
  return
 }
 fmt.Fprintln(w.out,"События включены. Проверка:",vkDiagnostics(ctx,cfg))
}

func service(action string){cmd:=exec.Command("systemctl",action,"ai-bot");cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;_ =cmd.Run()}
func menu(w Wizard,cfg Config,c *Catalog){
 for {
  fmt.Fprintln(w.out,"\n===== AI-bot — Go =====\n1. Настроить ВК\n2. Бесплатные провайдеры и модели\n3. Контекст/поиск/лимиты\n4. Диагностика\n5. Перезапустить\n6. Журнал\n7. Очистить историю\n8. Веб-панель через SSH\n9. Включить события ВК (с подтверждением)\n0. Выход")
  choice:=w.integer("Пункт",0,0,9)
  if choice==0{return}
  switch choice{
  case 1:w.setupVK(&cfg);_ =saveConfig(cfg);service("restart")
  case 2:w.setupModels(&cfg,c);service("restart")
  case 3:w.behavior(&cfg);_ =saveConfig(cfg);service("restart")
  case 4:fmt.Fprintln(w.out,doctor(context.Background(),cfg))
  case 5:service("restart")
  case 6:cmd:=exec.Command("journalctl","-u","ai-bot","-n","60","--no-pager");cmd.Stdout=w.out;_ =cmd.Run()
  case 7:if w.ask("Для удаления истории напишите ДА","нет")=="ДА"{s,err:=openStore(dataPath());if err!=nil{fmt.Fprintln(w.out,err)}else{if err=s.clearMessages();err!=nil{fmt.Fprintln(w.out,"Ошибка очистки:",err)};s.close()}}
  case 8:if err:=webServe(cfg,c);err!=nil{fmt.Fprintln(w.out,"Веб-панель:",err)}
  case 9:w.configureVKEvents(cfg)
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
 w:=Wizard{bufio.NewReader(os.Stdin),os.Stdout}
 if command=="is-configured"{
  connected:=false;for id,p:=range cfg.Providers{spec:=catalog.Specs[id];if p.Enabled&&(p.APIKey!=""||spec.AnonymousKey!=""){connected=true}}
  if !connected||cfg.VKToken==""||cfg.GroupID<1{return errors.New("настройка не завершена")};return nil
 }
 if os.Geteuid()!=0{return errors.New("для управления выполните sudo aibot")}
 switch command{
 case "version","--version":fmt.Println("AI-bot Go v1.2.0")
 case "setup":
  w.setupVK(&cfg);w.setupModels(&cfg,catalog);w.behavior(&cfg)
  if err=saveConfig(cfg);err!=nil{return err};fmt.Fprintln(os.Stdout,"Настройки сохранены. Проверка: sudo aibot doctor")
  service("restart")
 case "menu":menu(w,cfg,catalog)
 case "doctor","status":fmt.Println(doctor(context.Background(),cfg))
 case "vk-events":w.configureVKEvents(cfg)
 case "web":return webServe(cfg,catalog)
 case "logs":cmd:=exec.Command("journalctl","-u","ai-bot","-n","70","--no-pager");cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;return cmd.Run()
 case "restart":service("restart")
 case "vk":w.setupVK(&cfg);return saveConfig(cfg)
 case "api":w.setupModels(&cfg,catalog);return saveConfig(cfg)
 case "behavior":w.behavior(&cfg);return saveConfig(cfg)
 case "clear":s,e:=openStore(dataPath());if e!=nil{return e};defer s.close();return s.clearMessages()
 default:return errors.New("команды: run, setup, menu, doctor, web, logs, restart, status, vk, vk-events, api, behavior, clear, version")
 }
 return nil
}
func main(){
 log.SetFlags(log.LstdFlags|log.Lmicroseconds)
 if err:=run();err!=nil{
  if !errors.Is(err,context.Canceled){fmt.Fprintln(os.Stderr,"AI-bot:",err);os.Exit(1)}
 }
}
