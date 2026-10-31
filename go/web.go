package main

import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 _ "embed"
 "encoding/base64"
 "encoding/hex"
 "errors"
 "fmt"
 "html/template"
 "io"
 "net"
 "net/http"
 "net/url"
 "os/exec"
 "strconv"
 "sync/atomic"
 "time"
)

//go:embed admin.html
var adminHTML string
type webProvider struct{ID,Name,URL string;Enabled,Auto bool}
type webField struct{Key,Label string;Value int}
type webData struct{CSRF,Notice string;Allowed []string;Labels map[string]string;Providers []webProvider;Fields []webField;Search,Fallback bool;Domain string;VKReady bool;GroupID int64;NextStep string}
func secretToken()string{
 raw:=make([]byte,24)
 if _,err:=rand.Read(raw);err!=nil{panic("не удалось сгенерировать пароль")}
 return base64.RawURLEncoding.EncodeToString(raw)
}
// The ephemeral local password is printed once. The domain panel stores
// only a SHA-256 hash of a random 256-bit password in a 0600 root-owned file.
func webServe(_ Config,c *Catalog)error{
 password:=secretToken()
 fmt.Println("Создайте SSH-туннель: ssh -L 8765:127.0.0.1:8765 USER@SERVER")
 fmt.Println("Адрес: http://127.0.0.1:8765 | логин: admin | одноразовый пароль:",password)
 return servePanelHTTP(c,panelDigest(password),"")
}
func webServeDomain(c *Catalog)error{
 panel,err:=loadPanelConfig();if err!=nil{return err}
 return servePanelHTTP(c,panel.PasswordHash,panel.Domain)
}
func servePanelHTTP(c *Catalog,hash,domain string)error{
 cfg,err:=loadConfig();if err!=nil{return err}
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second)
 c.refresh(ctx,cfg);cancel()
 listener,err:=net.Listen("tcp","127.0.0.1:8765");if err!=nil{return err}
 defer listener.Close()
 srv:=&http.Server{Handler:panelHTTPHandler(c,hash,domain),
  ReadHeaderTimeout:5*time.Second,ReadTimeout:15*time.Second,
  WriteTimeout:30*time.Second,IdleTimeout:60*time.Second}
 return srv.Serve(listener)
}
func panelHTTPHandler(c *Catalog,hash,domain string)http.Handler{
 csrf:=secretToken()
 var notice atomic.Value
 notice.Store("")
 tmpl:=template.Must(template.New("admin").Parse(adminHTML))
 expectedHash,err:=hex.DecodeString(hash)
 if err!=nil||len(expectedHash)!=sha256.Size{panic("неверный хеш пароля панели")}
 mux:=http.NewServeMux()
 mux.HandleFunc("/",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Frame-Options","DENY")
  w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Referrer-Policy","no-referrer")
  w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
  // Only the local loopback or the exact, configured TLS domain is trusted.
  local:=r.Host=="127.0.0.1:8765"||r.Host=="localhost:8765"
  public:=domain!=""&&r.Host==domain&&r.Header.Get("X-Forwarded-Proto")=="https"
  if !local&&!public{http.Error(w,"bad host or insecure proxy",400);return}
  user,password,ok:=r.BasicAuth()
  digest:=sha256.Sum256([]byte(password))
  if !ok||subtle.ConstantTimeCompare([]byte(user),[]byte("admin"))!=1||
   subtle.ConstantTimeCompare(digest[:],expectedHash)!=1{
   w.Header().Set("WWW-Authenticate","Basic realm=\"AI-bot\"")
   http.Error(w,"Unauthorized",401)
   return
  }
  if r.URL.Path!="/"{http.NotFound(w,r);return}
  if r.Method=="POST"{
   r.Body=http.MaxBytesReader(w,r.Body,8192)
   if r.ParseForm()!=nil{http.Error(w,"bad form",400);return}
   if subtle.ConstantTimeCompare([]byte(r.Form.Get("csrf")),[]byte(csrf))!=1{
    http.Error(w,"CSRF",403);return
   }
   current,err:=loadConfig();if err!=nil{http.Error(w,"config read error",500);return}
   status,err:=applyWebForm(&current,c,r.Form)
   if err!=nil{status="Ошибка: "+err.Error()
   }else if r.Form.Get("action")=="restart"{
    restartCtx,restartCancel:=context.WithTimeout(r.Context(),12*time.Second)
    cmd:=exec.CommandContext(restartCtx,"systemctl","restart","ai-bot")
    cmd.Stdout=io.Discard;cmd.Stderr=io.Discard
    if cmd.Run()!=nil{status="Не удалось перезапустить службу"}
    restartCancel()
   }else{
    if err=saveConfig(current);err!=nil{status="Ошибка сохранения: "+err.Error()}
    if err==nil{
     if r.Form.Get("action")=="provider"{
      refreshCtx,refreshCancel:=context.WithTimeout(r.Context(),20*time.Second)
      c.refresh(refreshCtx,current);refreshCancel()
     }
     // The bot reads config only on startup. Without an automatic restart,
     // the panel reports success but VK continues using the old API keys and
     // its old model list indefinitely.
     restartCtx,restartCancel:=context.WithTimeout(context.Background(),20*time.Second)
     cmd:=exec.CommandContext(restartCtx,"systemctl","restart","ai-bot.service")
     if e:=cmd.Run();e!=nil{
      status+=". Настройки сохранены, но перезапуск бота не удался: sudo aibot restart"
     }else{
      status+=". Служба перезапущена; проверка моделей выполняется автоматически"
     }
     restartCancel()
    }
   }
   notice.Store(status)
   http.Redirect(w,r,"/",http.StatusSeeOther)
   return
  }
  if r.Method!="GET"{http.Error(w,"method",405);return}
  current,err:=loadConfig();if err!=nil{http.Error(w,"config error",500);return}
  list:=c.enabled(current)
  data:=webData{CSRF:csrf,Notice:notice.Load().(string),Allowed:list,
   Labels:map[string]string{},Search:current.SearchEnabled,
   Fallback:current.FreeFallback,Domain:domain,VKReady:current.VKToken!=""&&current.GroupID>0,
   GroupID:current.GroupID,NextStep:nextSetupStep(current,c)}
  for _,id:=range list{data.Labels[id]=c.label(id)}
  for _,id:=range c.Order{
   p:=c.Specs[id];settings:=current.Providers[id]
   data.Providers=append(data.Providers,webProvider{id,p.Name,p.KeyURL,settings.Enabled,contains(current.AutoModelsProviders,id)})
  }
  data.Fields=[]webField{
   {"context_messages","Макс. сообщений контекста",current.ContextMessages},
   {"context_hours","Часов контекста",current.ContextHours},
   {"retention_days","Дней хранения",current.RetentionDays},
   {"max_requests_hour","Общий лимит в час",current.MaxRequestsHour},
   {"max_user_requests_hour","Лимит участника",current.MaxUserRequestsHour},
   {"max_chat_requests_hour","Лимит беседы",current.MaxChatRequestsHour},
  }
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  _=tmpl.Execute(w,data)
 })
 return mux
}
func applyWebForm(cfg *Config,c *Catalog,form url.Values)(string,error){
 switch form.Get("action"){
 case "restart":return "Запрошен перезапуск",nil
 case "default":
  key:=form.Get("model");for _,allowed:=range c.enabled(*cfg){if key==allowed{cfg.DefaultModel=key;return "Основная модель сохранена",nil}}
  return "",errors.New("модель недоступна")
 case "provider":
  id:=form.Get("provider");spec,ok:=c.Specs[id];if !ok{return "",errors.New("нет такого провайдера")}
  settings:=cfg.Providers[id]
  if len(form.Get("api_key"))>500{return "",errors.New("ключ слишком длинный")}
  if form.Get("api_key")!=""{settings.APIKey=form.Get("api_key")}
  if form.Get("remove_key")=="on"{settings.APIKey=""}
  if id=="cloudflare"&&form.Get("account_id")!=""{settings.CloudflareAccount=form.Get("account_id")}
  settings.Enabled=form.Get("enabled")=="on"
  if settings.Enabled&&settings.APIKey==""&&spec.AnonymousKey==""{return "",errors.New("нужен API-ключ")}
  cfg.Providers[id]=settings
  cfg.AutoModelsProviders=removeExact(cfg.AutoModelsProviders,id)
  hasManual:=false
  for _,key:=range cfg.EnabledModels{
   p,model,err:=splitModel(key)
   if err==nil&&p==id&&c.modelAllowedID(p,model){hasManual=true;break}
  }
  // When the web form connects a provider but selects no individual model,
  // opt into curated/free discovery instead of saving an unusable setup.
  if settings.Enabled&&(form.Get("auto")=="on"||!hasManual){
   cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id)
   cfg.EnabledModels=removeProvider(cfg.EnabledModels,id)
  }
  return "Провайдер подключён. Проверка доступности бесплатных моделей запущена",nil
 case "settings":
  fields:=map[string]*int{"context_messages":&cfg.ContextMessages,"context_hours":&cfg.ContextHours,"retention_days":&cfg.RetentionDays,"max_requests_hour":&cfg.MaxRequestsHour,"max_user_requests_hour":&cfg.MaxUserRequestsHour,"max_chat_requests_hour":&cfg.MaxChatRequestsHour}
  for field,target:=range fields{n,err:=strconv.Atoi(form.Get(field));if err!=nil||n<1||n>10000{return "",fmt.Errorf("неверное значение %s",field)};*target=n}
  if cfg.ContextMessages>100||cfg.ContextHours>168||cfg.RetentionDays>30{return "",errors.New("превышен контекст/срок хранения")}
  cfg.SearchEnabled=form.Get("search_enabled")=="on";cfg.FreeFallback=form.Get("free_fallback")=="on"
  return "Ограничения сохранены",nil
 }
 return "",errors.New("неизвестная команда")
}
