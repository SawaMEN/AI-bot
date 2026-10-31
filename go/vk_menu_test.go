package main

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
)

func TestVKGuidedBoardsStayInsideLimits(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 for name,raw:=range map[string]string{
  "главная":mainBoard(),
  "под ответом":answerBoard(),
  "личные":personalBoard(),
  "беседа":chatBoard(),
  "помощь":helpBoard(),
  "модели":modelBoard(c,cfg,1,false),
  "модели беседы":modelBoard(c,cfg,1,true),
  "поиск":choiceBoard("","поиска",cfg),
  "стиль":choiceBoard("","стиля",cfg),
  "контекст":choiceBoard("","контекста",cfg),
  "резерв":choiceBoard("диалог ","резерва",cfg),
  "подтверждение":choiceBoard("диалог ","сброса",cfg),
 }{
  var p struct{Inline bool `json:"inline"`;Buttons [][]struct{Action struct{Label string `json:"label"`;Payload string `json:"payload"`} `json:"action"`} `json:"buttons"`}
  if err:=json.Unmarshal([]byte(raw),&p);err!=nil{t.Fatalf("%s bad VK JSON: %v",name,err)}
  if !p.Inline||len(p.Buttons)==0||len(p.Buttons)>6{t.Fatalf("%s bad rows %d",name,len(p.Buttons))}
  for _,row:=range p.Buttons{
   if len(row)<1||len(row)>4{t.Fatalf("%s invalid buttons per row: %d",name,len(row))}
   for _,btn:=range row{
    if btn.Action.Label==""||len([]rune(btn.Action.Label))>40{t.Fatalf("%s bad label %q",name,btn.Action.Label)}
    if callbackCommand(json.RawMessage(btn.Action.Payload))==""{t.Fatalf("%s invalid callback: %s",name,btn.Action.Payload)}
   }
  }
 }
}
func TestVKHomeAndSettingsNavigation(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 store,err:=openStore(filepath.Join(t.TempDir(),"vk-menu.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 v:=newVK(cfg)
 peer,user:=int64(100),int64(100)
 for _,tc:=range []struct{cmd,want string}{
  {"меню","ГЛАВНОЕ МЕНЮ"},
  {"настройки","МОИ НАСТРОЙКИ"},
  {"выбор поиска","РЕЖИМ ПОИСКА"},
  {"выбор контекста","КОНТЕКСТ"},
  {"модели","ВЫБОР ИИ"},
  {"поиск выкл","Поиск: ✗ ВЫКЛ"},
  {"модель авто","Модель: по умолчанию"},
 }{
  txt,ok:=processCommand(context.Background(),v,store,c,cfg,peer,user,tc.cmd)
  if !ok||!strings.Contains(txt,tc.want){t.Fatalf("%q => %q (ok=%v), missing %q",tc.cmd,txt,ok,tc.want)}
 }
 if pref:=store.getPref("user",user);pref.Search!="off"{t.Fatalf("personal preference wasn't saved: %+v",pref)}
 if raw:=buttonsFor("поиск выкл",c,cfg);!strings.Contains(raw,"Стиль ответа"){t.Fatal("post-selection keyboard did not return to the functions section")}
 if raw:=buttonsFor("выбор поиска",c,cfg);!strings.Contains(raw,"Авто"){t.Fatal("search choices missing")}
}
func TestVKPrivateMenuDoesNotRequireMention(t *testing.T){
 cfg:=testCfg()
 for _,input:=range []string{"меню","настройки","модель 1","помощь"}{
  q,ok:=questionFor(cfg,VKMessage{PeerID:46,FromID:46,ID:1,Text:input})
  if !ok||q!=input{t.Fatalf("private %q => %q (%v)",input,q,ok)}
 }
 _,ok:=questionFor(cfg,VKMessage{PeerID:2000000001,FromID:46,ID:1,Text:"меню"})
 if ok{t.Fatal("unmentioned group menu would spam the group")}
}
func TestVKGroupConfigMutationsRequireAdmin(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 store,err:=openStore(filepath.Join(t.TempDir(),"vk-admin.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 var allowed atomic.Bool
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/method/messages.getConversationMembers"{t.Errorf("unexpected method: %s",r.URL.Path)}
  fmt.Fprintf(w,`{"response":{"items":[{"member_id":42,"is_admin":%t}]}}`,allowed.Load())
 }))
 defer server.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/method/"
 defer func(){vkAPIBaseURL=old}()
 v:=newVK(cfg)
 peer,user:=int64(2000000001),int64(42)
 msg,ok:=processCommand(context.Background(),v,store,c,cfg,peer,user,"диалог поиск выкл")
 if !ok||!strings.Contains(msg,"администратор"){t.Fatalf("not refused: %q",msg)}
 if store.getPref("chat",peer).Search!=""{t.Fatal("unauthorized change persisted")}
 allowed.Store(true)
 msg,ok=processCommand(context.Background(),v,store,c,cfg,peer,user,"диалог поиск выкл")
 if !ok||!strings.Contains(msg,"НАСТРОЙКИ БЕСЕДЫ")||store.getPref("chat",peer).Search!="off"{t.Fatalf("admin change not applied: %q",msg)}
 processCommand(context.Background(),v,store,c,cfg,peer,user,"диалог подтвердить сброс")
 if store.getPref("chat",peer).Search!=""{t.Fatal("reset confirmation failed")}
}
func TestVKLegacyCallbacksInstallPermanentPrivateMenu(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 store,err:=openStore(filepath.Join(t.TempDir(),"vk-edit.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 var edits,sends,snacks atomic.Int32
 var privateInline,groupInline bool
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path{
  case "/method/messages.edit":edits.Add(1)
  case "/method/messages.send":
   sends.Add(1)
   if err:=r.ParseForm();err!=nil{t.Error(err)}
   var keyboard struct{Inline bool `json:"inline"`;OneTime bool `json:"one_time"`}
   if err:=json.Unmarshal([]byte(r.Form.Get("keyboard")),&keyboard);err!=nil{t.Error(err)}
   if r.Form.Get("peer_id")=="42"{privateInline=keyboard.Inline}
   if r.Form.Get("peer_id")=="2000000001"{groupInline=keyboard.Inline}
  case "/method/messages.sendMessageEventAnswer":snacks.Add(1)
  default:t.Errorf("unexpected API: %s",r.URL.Path)
  }
  fmt.Fprint(w,`{"response":1}`)
 }))
 defer server.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/method/"
 defer func(){vkAPIBaseURL=old}()
 bot:=newBot(cfg,store,c);bot.vk.client=server.Client()
 payload:=[]byte(`{"aibot":1,"command":"настройки"}`)
 bot.button(context.Background(),42,42,"event-1",payload,37)
 bot.button(context.Background(),2000000001,42,"event-2",payload,37)
 if edits.Load()!=0||sends.Load()!=2||snacks.Load()!=2{t.Fatalf("edit=%d send=%d snack=%d",edits.Load(),sends.Load(),snacks.Load())}
 if privateInline||!groupInline{t.Fatalf("private should be persistent, group inline (private=%v group=%v)",privateInline,groupInline)}
}
func TestVKMenuModelPagesRetainAutoAndBack(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 cfg.AutoModelsProviders=[]string{"groq","zai"}
 var all [][]struct{Action struct{Label string `json:"label"`;Payload string `json:"payload"`} `json:"action"`}
 raw:=modelBoard(c,cfg,1,false)
 if err:=json.Unmarshal([]byte(raw),&struct{Buttons *[][]struct{Action struct{Label string `json:"label"`;Payload string `json:"payload"`} `json:"action"`} `json:"buttons"`}{Buttons:&all});err!=nil{t.Fatal(err)}
 joined:=raw
 if !strings.Contains(joined,"модель авто")||!strings.Contains(joined,"ии"){t.Fatal("must allow automatic choice and navigation back")}
}
