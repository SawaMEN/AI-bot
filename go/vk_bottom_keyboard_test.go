package main

import (
 "bufio"
 "bytes"
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

func TestConsoleMainMenuKeepsAIAndToolsSeparate(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 got:=consoleMainOptions(cfg,c)
 titles:=map[int]string{}
 for _,o:=range got{
  if _,ok:=titles[o.Number];ok{t.Fatalf("duplicate console menu number %d",o.Number)}
  titles[o.Number]=o.Title
 }
 for id,expected:=range map[int]string{1:"ИИ и модели",2:"Функции бота",3:"ВКонтакте",4:"Диагностика",5:"Веб-панель",6:"Обслуживание",7:"Помощь",8:"Мастер настройки",0:"Выход"}{
  if titles[id]!=expected{t.Fatalf("menu %d is %q instead of %q",id,titles[id],expected)}
 }
 if strings.Contains(got[0].Detail,"Поиск")||strings.Contains(got[1].Detail,"провайдеры"){
  t.Fatalf("AI and functions mixed on main console screen: %+v",got)
 }
}

func TestConsoleFunctionsAndVKHubsHaveSafeBackNavigation(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 for _,section:=range []struct{name string;run func(Wizard,*Config,*Catalog)}{
  {"функции",func(w Wizard,cfg *Config,c *Catalog){w.consoleFunctionsHub(cfg,c)}},
  {"вк",func(w Wizard,cfg *Config,c *Catalog){w.consoleVKHub(cfg,c)}},
 }{
  var output bytes.Buffer
  w:=Wizard{in:bufio.NewReader(strings.NewReader("0\n")),out:&output}
  section.run(w,&cfg,c)
  if !strings.Contains(output.String(),"Назад"){t.Errorf("%s missing return navigation: %s",section.name,output.String())}
 }
}

func TestVKGroupBottomKeyboardCoversAllScreens(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 menus:=map[string]string{
  "home":replyGroupMainBoard(),
  "chat":replyGroupChatBoard(),
  "models":replyGroupModelBoard(c,cfg,1),
  "search":replyGroupChoiceBoard("поиска",cfg),
  "style":replyGroupChoiceBoard("стиля",cfg),
  "context":replyGroupChoiceBoard("контекста",cfg),
  "fallback":replyGroupChoiceBoard("резерва",cfg),
  "reset":replyGroupChoiceBoard("сброса",cfg),
 }
 for name,raw:=range menus{
  var kb replyKeyboardJSON
  if err:=json.Unmarshal([]byte(raw),&kb);err!=nil{t.Fatalf("%s: %v",name,err)}
  if kb.Inline||kb.OneTime||len(kb.Buttons)==0||len(kb.Buttons)>10{
   t.Fatalf("%s not bottom VK keyboard: %+v",name,kb)
  }
  for _,row:=range kb.Buttons{
   if len(row)>4{t.Errorf("%s has %d buttons per row",name,len(row))}
   for _,button:=range row{
    if button.Action.Type!="text"{t.Errorf("%s not text action: %s",name,button.Action.Type)}
    if cmd:=vkGroupReplyCommand(json.RawMessage(button.Action.Payload));cmd==""{
     t.Errorf("%s button %s has invalid group command",name,button.Action.Label)
    }
   }
  }
 }
 if !strings.Contains(replyGroupChatBoard(),"диалог выбор поиска")||!strings.Contains(replyGroupChatBoard(),"диалог выбор контекста"){
  t.Fatal("shared settings do not use group scope")
 }
 if strings.Contains(replyGroupMainBoard(),"Мои настройки"){t.Fatal("group menu should not expose personal preferences")}
}

func TestVKGroupKeyboardTextTapDoesNotRequireMention(t *testing.T){
 cfg:=testCfg()
 var payload json.RawMessage
 for _,command:=range []string{"диалог","диалог модели 1","диалог модель авто","диалог выбор поиска","диалог поиск выкл","меню"}{
  payload,_=json.Marshal(map[string]any{"aibot":1,"command":command})
  msg:=VKMessage{PeerID:2000000001,FromID:42,ID:1,Text:"Кнопка ВК",Payload:payload}
  got,handled:=questionFor(cfg,msg)
  if !handled||got!=command{t.Errorf("group keyboard %q => %q, handled=%v",command,got,handled)}
 }
 for _,command:=range []string{"модель 2","поиск выкл","rm -rf /"}{
  payload,_=json.Marshal(map[string]any{"aibot":1,"command":command})
  msg:=VKMessage{PeerID:2000000001,FromID:42,ID:2,Text:"Кнопка ВК",Payload:payload}
  if got,handled:=questionFor(cfg,msg);handled{t.Errorf("unscoped group command was accepted: %q",got)}
 }
 if got,handled:=questionFor(cfg,VKMessage{PeerID:2000000001,FromID:42,ID:3,Text:"🤖 ИИ беседы"});handled{
  t.Fatalf("ordinary group text without payload must be ignored: %q",got)
 }
}

func TestVKGroupKeyboardCannotEscalateChatPermissions(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 store,err:=openStore(filepath.Join(t.TempDir(),"chat.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 var admin atomic.Bool
 api:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/method/messages.getConversationMembers"{t.Fatalf("unexpected VK call: %s",r.URL.Path)}
  fmt.Fprintf(w,`{"response":{"items":[{"member_id":42,"is_admin":%t}]}}`,admin.Load())
 }))
 defer api.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=api.URL+"/method/"
 defer func(){vkAPIBaseURL=old}()
 v:=newVK(cfg)
 peer,user:=int64(2000000001),int64(42)
 cmdPayload,_:=json.Marshal(map[string]any{"aibot":1,"command":"диалог поиск выкл"})
 command,ok:=questionFor(cfg,VKMessage{PeerID:peer,FromID:user,ID:1,Text:"Кнопка",Payload:cmdPayload})
 if !ok{t.Fatal("group payload ignored")}
 response,ok:=processCommand(context.Background(),v,store,c,cfg,peer,user,command)
 if !ok||!strings.Contains(response,"администратор")||store.getPref("chat",peer).Search!=""{
  t.Fatalf("unauthorized group action: %s",response)
 }
 admin.Store(true)
 response,ok=processCommand(context.Background(),v,store,c,cfg,peer,user,command)
 if !ok||store.getPref("chat",peer).Search!="off"{t.Fatalf("authorized admin could not update chat: %s",response)}
}

func TestVKSendDeliversGroupKeyboardBelowInput(t *testing.T){
 cfg:=testCfg()
 var got replyKeyboardJSON
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/method/messages.send"{t.Errorf("wrong method: %s",r.URL.Path)}
  if err:=r.ParseForm();err!=nil{t.Error(err)}
  if err:=json.Unmarshal([]byte(r.Form.Get("keyboard")),&got);err!=nil{t.Error(err)}
  if r.Form.Get("peer_id")!="2000000001"{t.Error("did not target group conversation")}
  fmt.Fprint(w,`{"response":1}`)
 }))
 defer server.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/method/"
 defer func(){vkAPIBaseURL=old}()
 vk:=newVK(cfg)
 if err:=vk.send(context.Background(),2000000001,"Меню",replyGroupMainBoard(),"test-group-bottom");err!=nil{t.Fatal(err)}
 if got.Inline||got.OneTime||len(got.Buttons)==0{
  t.Fatalf("VK group keyboard not persistent below input: %+v",got)
 }
}
