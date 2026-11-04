package main

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestVKAdminIDInputFormats(t *testing.T){
 good:=map[string]string{
  "12345":"12345","id12345":"12345","@id12345":"12345",
  "https://vk.com/id12345":"12345","vk.com/id12345?from=search":"12345",
  "https://m.vk.com/id12345":"12345","https://vk.ru/id12345":"12345",
  "https://vk.com/profile.php?id=12345":"12345",
  "[id12345|Имя Фамилия]":"12345","@some_name":"some_name",
  "https://vk.com/some_name":"some_name","https://vk.me/some_name":"some_name",
 }
 for input,want:=range good{
  got,err:=parseVKUserReference(input)
  if err!=nil||got!=want{t.Errorf("%q => %q, %v, want %q",input,got,err,want)}
 }
 for _,input:=range []string{
  "","0","id0","-123","https://evil.com/id123","https://vk.com.evil.com/id123",
  "https://vk.com/away.php?to=https://evil.com","https://vk.com/club123",
  "public123","https://vk.com/id123/extra","https://vk.com:1234/id123",
  "javascript:alert(1)","https://vk.com/id2147483648",
 }{
  if name,err:=parseVKUserReference(input);err==nil{t.Errorf("unsafe VK ref accepted %q -> %q",input,name)}
 }
}
func TestVKAdminScreenNameResolvesOnlyUsers(t *testing.T){
 previous:=vkAPIBaseURL;defer func(){vkAPIBaseURL=previous}()
 kind:="user"
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/utils.resolveScreenName"{t.Errorf("unexpected VK method %s",r.URL.Path)}
  if err:=r.ParseForm();err!=nil{t.Error(err)}
  if r.Form.Get("screen_name")!="some_name"{t.Errorf("lost screen name %q",r.Form.Get("screen_name"))}
  fmt.Fprintf(w,`{"response":{"type":%q,"object_id":42}}`,kind)
 }))
 defer server.Close()
 vkAPIBaseURL=server.URL+"/"
 v:=newVK(Config{VKToken:"fake-vk-token",GroupID:1})
 id,err:=resolveVKAdminID(context.Background(),v,"https://vk.com/some_name")
 if err!=nil||id!=42{t.Fatalf("VK user lookup: %d %v",id,err)}
 kind="group"
 if _,err:=resolveVKAdminID(context.Background(),v,"some_name");err==nil{t.Fatal("VK group granted administrator role")}
}
func TestKeylessProvidersAreEnabledUnlessExplicitlyDisabled(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 cfg:=defaults();enableKeylessCatalog(&cfg)
 keyless:=0
 for _,id:=range c.Order{
  spec:=c.Specs[id]
  if !spec.requiresKey(){
   keyless++
   if !cfg.Providers[id].Enabled{t.Errorf("%s not enabled by default",id)}
  }
 }
 if keyless<5{t.Fatalf("expected all free anonymous gateways, got %d",keyless)}
 cfg.Providers["g4fgroq"]=ProviderConfig{Enabled:false}
 enableKeylessCatalog(&cfg)
 if cfg.Providers["g4fgroq"].Enabled{t.Fatal("an explicit disable was ignored")}
}
func TestVKAdminOverlayPersistsInUnprivilegedDataFolder(t *testing.T){
 dir:=t.TempDir()
 t.Setenv("AIBOT_CONFIG",filepath.Join(dir,"config.json"))
 t.Setenv("AIBOT_DATA",filepath.Join(dir,"store","history.db"))
 original:=defaults()
 original.VKToken="old-vk-token";original.GroupID=42;original.AdminVKID=101
 original.Providers["gemini"]=ProviderConfig{APIKey:"GEMINI_PRIVATE",Enabled:true}
 if err:=saveConfig(original);err!=nil{t.Fatal(err)}
 changed,err:=saveVKAdminField("max_user_requests_hour",33);if err!=nil{t.Fatal(err)}
 if changed.MaxUserRequestsHour!=33||changed.VKToken!="old-vk-token"{t.Fatal("settings overlay not applied")}
 settings:=changed.Providers["gemini"];settings.APIKey="NEW_PRIVATE";settings.Enabled=true
 changed,err=saveVKAdminProvider("gemini",settings);if err!=nil{t.Fatal(err)}
 if changed.Providers["gemini"].APIKey!="NEW_PRIVATE"{t.Fatal("provider update lost")}
 persisted,err:=loadConfig();if err!=nil{t.Fatal(err)}
 if persisted.Providers["gemini"].APIKey!="NEW_PRIVATE"||persisted.AdminVKID!=101{t.Fatal("overlay not reloaded")}
 raw,err:=os.ReadFile(configPath());if err!=nil{t.Fatal(err)}
 if strings.Contains(string(raw),"NEW_PRIVATE"){t.Fatal("root-owned bootstrap file was overwritten by VK")}
 stat,err:=os.Stat(vkAdminOverlayPath());if err!=nil{t.Fatal(err)}
 if stat.Mode().Perm()!=0600{t.Fatalf("admin overlay file mode %o, want 0600",stat.Mode().Perm())}
 if err:=json.Unmarshal(raw,&original);err!=nil{t.Fatal(err)}
 if original.Providers["gemini"].APIKey!="GEMINI_PRIVATE"{t.Fatal("bootstrapping credentials modified")}
}
func TestVKAdminKeyboardPrivateAndNotSpoofable(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 cfg:=defaults();cfg.AdminVKID=101;cfg.GroupID=500;enableKeylessCatalog(&cfg)
 for _,tc:=range []struct{peer,user int64;allowed bool}{
  {101,101,true},{102,102,false},{2000000001,101,false},{101,102,false},
 }{
  screen:=keyboardForViewer(tc.peer,tc.user,"меню",c,cfg)
  hasAdmin:=strings.Contains(screen,"Меню админа")
  if hasAdmin!=tc.allowed{t.Errorf("VK keyboard peer %d user %d: admin=%v want %v",tc.peer,tc.user,hasAdmin,tc.allowed)}
  if len(screen)==0{t.Fatal("empty reply keyboard")}
 }
 b:=newBot(cfg,nil,c)
 if _,ok:=b.adminCommand(context.Background(),cfg,102,102,"админ поиск выкл");!ok{t.Fatal("admin command not handled")}
 if _,ok:=b.adminCommand(context.Background(),cfg,2000000001,101,"админ поиск выкл");!ok{t.Fatal("group admin command not handled")}
 text,handled:=b.adminCommand(context.Background(),cfg,102,102,"админ поиск выкл")
 if !handled||!strings.Contains(text,"нет доступа"){t.Fatal("unauthorized administrator changed settings")}
}


func TestVKAdminCanChangeLiveSettingsFromPrivateMenu(t *testing.T){
 dir:=t.TempDir()
 t.Setenv("AIBOT_CONFIG",filepath.Join(dir,"config.json"))
 t.Setenv("AIBOT_DATA",filepath.Join(dir,"state","history.db"))
 cfg:=defaults();cfg.AdminVKID=42;cfg.GroupID=777;cfg.VKToken="dummy-community-token"
 if err:=saveConfig(cfg);err!=nil{t.Fatal(err)}
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 b:=newBot(cfg,nil,c)
 apply:=func(cmd string){
  t.Helper()
  response,handled:=b.adminCommand(context.Background(),b.currentConfig(),42,42,cmd)
  if !handled||strings.Contains(response,"Ошибка")||strings.Contains(response,"Не удалось"){
   t.Fatalf("%s -> %q handled=%v",cmd,response,handled)
  }
 }
 apply("админ поиск выкл")
 apply("админ резерв выкл")
 apply("админ контекст 20")
 apply("админ часы 48")
 apply("админ история 14")
 apply("админ всего 200")
 apply("админ пользователь 18")
 apply("админ беседа 45")
 apply("админ пауза 5")
 apply("админ триггер помощник")
 apply("админ переключить g4fgroq")
 updated,err:=loadConfig();if err!=nil{t.Fatal(err)}
 if updated.SearchEnabled||updated.FreeFallback||updated.ContextMessages!=20||updated.ContextHours!=48||updated.RetentionDays!=14||
  updated.MaxRequestsHour!=200||updated.MaxUserRequestsHour!=18||updated.MaxChatRequestsHour!=45||
  updated.CooldownSeconds!=5||updated.Trigger!="помощник"||updated.Providers["g4fgroq"].Enabled{
  t.Fatalf("admin changes did not persist: %+v",updated)
 }
 denied,_:=b.adminCommand(context.Background(),b.currentConfig(),43,43,"админ контекст 10")
 if !strings.Contains(denied,"нет доступа"){t.Fatalf("unauthorized response: %s",denied)}
 updatedAgain,err:=loadConfig();if err!=nil{t.Fatal(err)}
 if updatedAgain.ContextMessages!=20{t.Fatal("unauthorized user changed global settings")}
}
func TestVKAdminSecretKeySkipsSQLiteAndModelContext(t *testing.T){
 dir:=t.TempDir()
 t.Setenv("AIBOT_CONFIG",filepath.Join(dir,"config.json"))
 t.Setenv("AIBOT_DATA",filepath.Join(dir,"data","history.db"))
 cfg:=defaults();cfg.AdminVKID=42;cfg.GroupID=77;cfg.VKToken="dummy-community-token"
 if err:=saveConfig(cfg);err!=nil{t.Fatal(err)}
 saved,err:=loadConfig();if err!=nil{t.Fatal(err)}
 catalog,err:=newCatalog();if err!=nil{t.Fatal(err)}
 store,err:=openStore(dataPath());if err!=nil{t.Fatal(err)};defer store.close()
 b:=newBot(saved,store,catalog)
 original:=vkAPIBaseURL;defer func(){vkAPIBaseURL=original}()
 sent:=""
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if err:=r.ParseForm();err!=nil{t.Error(err)}
  if r.URL.Path=="/messages.send"{
   sent=r.Form.Get("message")
   if strings.Contains(sent,"SUPER-PRIVATE-KEY"){t.Error("VK reply exposed saved secret")}
  }
  fmt.Fprint(w,`{"response":1}`)
 }))
 defer srv.Close();vkAPIBaseURL=srv.URL+"/"
 msg:=VKMessage{PeerID:42,FromID:42,ID:1,Text:"админ ключ ptero SUPER-PRIVATE-KEY"}
 if err:=b.event(context.Background(),msg);err!=nil{t.Fatal(err)}
 updated,err:=loadConfig();if err!=nil{t.Fatal(err)}
 if updated.Providers["ptero"].APIKey!="SUPER-PRIVATE-KEY"||!updated.Providers["ptero"].Enabled{
  t.Fatal("new key was not applied")
 }
 if !strings.Contains(sent,"сохранён"){t.Fatalf("missing confirmation: %s",sent)}
 var messages,pending int
 if err:=store.DB.QueryRow("SELECT COUNT(*) FROM messages").Scan(&messages);err!=nil{t.Fatal(err)}
 if err:=store.DB.QueryRow("SELECT COUNT(*) FROM pending").Scan(&pending);err!=nil{t.Fatal(err)}
 if messages!=0||pending!=0{t.Fatalf("secret entered SQLite: %d messages %d pending",messages,pending)}
}
