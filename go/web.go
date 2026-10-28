package main

import (
 "context"
 "crypto/rand"
 "crypto/subtle"
 _ "embed"
 "encoding/base64"
 "errors"
 "fmt"
 "html/template"
 "io"
 "net"
 "net/http"
 "net/url"
 "os/exec"
 "strconv"
 "time"
)

//go:embed admin.html
var adminHTML string
type webProvider struct{ID,Name,URL string;Enabled,Auto bool}
type webField struct{Key,Label string;Value int}
type webData struct{CSRF,Notice string;Allowed []string;Labels map[string]string;Providers []webProvider;Fields []webField;Search,Fallback bool}
func secretToken()string{raw:=make([]byte,24);_,_=rand.Read(raw);return base64.RawURLEncoding.EncodeToString(raw)}
func webServe(_ Config,c *Catalog)error{
 cfg,err:=loadConfig();if err!=nil{return err}
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);c.refresh(ctx,cfg);cancel()
 secret:=secretToken();csrf:=secretToken();notice:=""
 tmpl,err:=template.New("admin").Parse(adminHTML);if err!=nil{return err}
 listener,err:=net.Listen("tcp","127.0.0.1:8765");if err!=nil{return err};defer listener.Close()
 fmt.Println("Создайте SSH-туннель на своём компьютере: ssh -L 8765:127.0.0.1:8765 USER@SERVER")
 fmt.Println("Адрес: http://127.0.0.1:8765 | логин: admin | одноразовый пароль:",secret)
 mux:=http.NewServeMux()
 mux.HandleFunc("/",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Frame-Options","DENY");w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
  if r.Host!="127.0.0.1:8765"&&r.Host!="localhost:8765"{http.Error(w,"bad host",400);return}
  user,password,ok:=r.BasicAuth()
  if !ok||subtle.ConstantTimeCompare([]byte(user),[]byte("admin"))!=1||subtle.ConstantTimeCompare([]byte(password),[]byte(secret))!=1{
   w.Header().Set("WWW-Authenticate","Basic realm=\"AI-bot\"");http.Error(w,"Unauthorized",401);return
  }
  if r.URL.Path!="/"{http.NotFound(w,r);return}
  if r.Method=="POST"{
   r.Body=http.MaxBytesReader(w,r.Body,8192);if r.ParseForm()!=nil{http.Error(w,"bad form",400);return}
   if subtle.ConstantTimeCompare([]byte(r.Form.Get("csrf")),[]byte(csrf))!=1{http.Error(w,"CSRF",403);return}
   current,err:=loadConfig();if err!=nil{http.Error(w,"config read error",500);return}
   notice,err=applyWebForm(&current,c,r.Form)
   if err!=nil{notice="Ошибка: "+err.Error()}else if r.Form.Get("action")=="restart"{
    cmd:=exec.Command("systemctl","restart","ai-bot");cmd.Stdout=io.Discard;cmd.Stderr=io.Discard
    if cmd.Run()!=nil{notice="Не удалось перезапустить службу"}
   }else{
    if err=saveConfig(current);err!=nil{notice="Ошибка сохранения: "+err.Error()}
    if r.Form.Get("action")=="provider"{
     ctx,cancel:=context.WithTimeout(r.Context(),20*time.Second);c.refresh(ctx,current);cancel()
    }
   }
   http.Redirect(w,r,"/",http.StatusSeeOther);return
  }
  if r.Method!="GET"{http.Error(w,"method",405);return}
  current,err:=loadConfig();if err!=nil{http.Error(w,"config error",500);return}
  list:=c.enabled(current);data:=webData{CSRF:csrf,Notice:notice,Allowed:list,Labels:map[string]string{},Search:current.SearchEnabled,Fallback:current.FreeFallback}
  for _,id:=range list{data.Labels[id]=c.label(id)}
  for _,id:=range c.Order{p:=c.Specs[id];settings:=current.Providers[id];data.Providers=append(data.Providers,webProvider{id,p.Name,p.KeyURL,settings.Enabled,contains(current.AutoModelsProviders,id)})}
  data.Fields=[]webField{{"context_messages","Макс. сообщений контекста",current.ContextMessages},{"context_hours","Часов контекста",current.ContextHours},{"retention_days","Дней хранения",current.RetentionDays},{"max_requests_hour","Общий лимит в час",current.MaxRequestsHour},{"max_user_requests_hour","Лимит участника",current.MaxUserRequestsHour},{"max_chat_requests_hour","Лимит беседы",current.MaxChatRequestsHour}}
  w.Header().Set("Content-Type","text/html; charset=utf-8");_ =tmpl.Execute(w,data)
 })
 srv:=&http.Server{Handler:mux,ReadHeaderTimeout:5*time.Second,ReadTimeout:15*time.Second,WriteTimeout:30*time.Second}
 return srv.Serve(listener)
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
  if settings.Enabled&&form.Get("auto")=="on"{
   cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,id)
   cfg.EnabledModels=removeProvider(cfg.EnabledModels,id)
  }
  return "Провайдер сохранён",nil
 case "settings":
  fields:=map[string]*int{"context_messages":&cfg.ContextMessages,"context_hours":&cfg.ContextHours,"retention_days":&cfg.RetentionDays,"max_requests_hour":&cfg.MaxRequestsHour,"max_user_requests_hour":&cfg.MaxUserRequestsHour,"max_chat_requests_hour":&cfg.MaxChatRequestsHour}
  for field,target:=range fields{n,err:=strconv.Atoi(form.Get(field));if err!=nil||n<1||n>10000{return "",fmt.Errorf("неверное значение %s",field)};*target=n}
  if cfg.ContextMessages>100||cfg.ContextHours>168||cfg.RetentionDays>30{return "",errors.New("превышен контекст/срок хранения")}
  cfg.SearchEnabled=form.Get("search_enabled")=="on";cfg.FreeFallback=form.Get("free_fallback")=="on"
  return "Ограничения сохранены",nil
 }
 return "",errors.New("неизвестная команда")
}
