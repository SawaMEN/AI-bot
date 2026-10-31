package main

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "testing"
)

type replyKeyboardJSON struct{
 Inline bool `json:"inline"`
 OneTime bool `json:"one_time"`
 Buttons [][]struct{
  Action struct{
   Type string `json:"type"`
   Label string `json:"label"`
   Payload string `json:"payload"`
  } `json:"action"`
  Color string `json:"color"`
 } `json:"buttons"`
}

func TestVKReplyKeyboardIsPermanentAndUsesTextActions(t *testing.T){
 cfg:=testCfg();c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 cases:=map[string]string{
  "главная":replyMainBoard(),
  "личные":replyPersonalBoard(),
  "помощь":replyHelpBoard(),
  "модель":replyModelBoard(c,cfg,1),
  "поиск":replyChoiceBoard("поиска",cfg),
  "стиль":replyChoiceBoard("стиля",cfg),
  "контекст":replyChoiceBoard("контекста",cfg),
  "резерв":replyChoiceBoard("резерва",cfg),
  "сброс":replyChoiceBoard("сброса",cfg),
 }
 for name,raw:=range cases{
  var kb replyKeyboardJSON
  if err:=json.Unmarshal([]byte(raw),&kb);err!=nil{t.Fatalf("%s invalid JSON: %v",name,err)}
  if kb.Inline||kb.OneTime{t.Fatalf("%s must be an always visible reply keyboard",name)}
  if len(kb.Buttons)==0||len(kb.Buttons)>10{t.Fatalf("%s row count %d",name,len(kb.Buttons))}
  for _,row:=range kb.Buttons{
   if len(row)<1||len(row)>4{t.Fatalf("%s row size %d",name,len(row))}
   for _,button:=range row{
    if button.Action.Type!="text"{t.Fatalf("%s uses non-text action %q",name,button.Action.Type)}
    if len([]rune(button.Action.Label))>40{t.Fatalf("%s oversized label %q",name,button.Action.Label)}
    if vkReplyCommand(json.RawMessage(button.Action.Payload))==""{t.Fatalf("%s invalid action payload %q",name,button.Action.Payload)}
    switch button.Color{case "primary","secondary","negative","positive":default:t.Fatalf("%s bad color %s",name,button.Color)}
   }
  }
 }
 if !strings.Contains(replyMainBoard(),"positive"){t.Fatal("main menu lacks distinct highlighted action")}
}

func TestVKReplyPayloadRoutesWithoutSendingToAI(t *testing.T){
 cfg:=testCfg()
 button:=VKMessage{
  PeerID:42,FromID:42,ID:1,Text:"🤖 Выбрать ИИ",
  Payload:json.RawMessage(`"{\\"aibot\\":1,\\"command\\":\\"модели\\"}"`),
 }
 // Simulate VK sending the JSON payload as a JSON-encoded string.
 encoded,_:=json.Marshal(`{"aibot":1,"command":"модели"}`)
 button.Payload=encoded
 command,handled:=questionFor(cfg,button)
 if !handled||command!="модели"{t.Fatalf("private button => %q %v",command,handled)}
 button.Payload=json.RawMessage(`{"aibot":1,"command":"поиск авто"}`)
 command,handled=questionFor(cfg,button)
 if !handled||command!="поиск авто"{t.Fatalf("private setting button => %q %v",command,handled)}
 button.Payload=nil
 command,handled=questionFor(cfg,button)
 if !handled||command!="модели"{t.Fatalf("label fallback => %q %v",command,handled)}
 button.Text="Расскажи о языке Go"
 command,handled=questionFor(cfg,button)
 if !handled||command!="Расскажи о языке Go"{t.Fatalf("free-form private chat => %q %v",command,handled)}
}

func TestVKReplyButtonsDoNotHijackUnmentionedGroupMessages(t *testing.T){
 cfg:=testCfg()
 button:=VKMessage{PeerID:2000000001,FromID:42,ID:1,Text:"🤖 Выбрать ИИ",Payload:json.RawMessage(`{"aibot":1,"command":"модели"}`)}
 if command,ok:=questionFor(cfg,button);!ok||command!="модели"{t.Fatalf("group keyboard payload did not navigate: %q %v",command,ok)}
 button.Payload=nil
 if command,ok:=questionFor(cfg,button);ok{t.Fatalf("unmentioned plain group text must be ignored: %q",command)}
 button.Text="[club123|Бот] меню"
 cfg.GroupID=123
 if command,ok:=questionFor(cfg,button);!ok||command!="меню"{t.Fatalf("group mention broken: %q %v",command,ok)}
}

func TestVKPeerKeyboardDoesNotAffectGroup(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 var personal,group replyKeyboardJSON
 if err:=json.Unmarshal([]byte(keyboardForPeer(42,"настройки",c,cfg)),&personal);err!=nil{t.Fatal(err)}
 if err:=json.Unmarshal([]byte(keyboardForPeer(2000000001,"настройки",c,cfg)),&group);err!=nil{t.Fatal(err)}
 if personal.Inline||group.Inline||personal.OneTime||group.OneTime{t.Fatalf("both keyboards must stay at bottom: %v %v",personal,group)}
 if !strings.Contains(answerKeyboardForPeer(42),`"inline":false`){t.Fatal("private AI answer should keep bottom keyboard")}
 if !strings.Contains(answerKeyboardForPeer(2000000001),`"inline":false`){t.Fatal("group AI answer should keep bottom keyboard")}
 if !strings.Contains(answerKeyboardForPeer(2000000001),"ИИ беседы"){t.Fatal("group keyboard must only contain shared settings")}
}

func TestVKTextKeyboardSentViaMessagesSend(t *testing.T){
 cfg:=testCfg()
 var sent replyKeyboardJSON
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/method/messages.send"{t.Errorf("unexpected VK call %s",r.URL.Path)}
  if err:=r.ParseForm();err!=nil{t.Error(err)}
  if err:=json.Unmarshal([]byte(r.Form.Get("keyboard")),&sent);err!=nil{t.Error(err)}
  if r.Form.Get("message")!="Откройте меню"{t.Error("message was lost")}
  fmt.Fprint(w,`{"response":1}`)
 }))
 defer server.Close()
 previous:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/method/"
 defer func(){vkAPIBaseURL=previous}()
 v:=newVK(cfg)
 if err:=v.send(context.Background(),42,"Откройте меню",replyMainBoard(),"test");err!=nil{t.Fatal(err)}
 if sent.Inline||len(sent.Buttons)!=4{t.Fatalf("reply keyboard not delivered: %+v",sent)}
}

func TestVKReplyMenuNavigationReturnsToContext(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 for _,tc:=range []struct{command,want string}{
  {"меню","💬 Задать вопрос"},
  {"настройки","Сбросить мои настройки"},
  {"модели","Автоматически"},
  {"выбор поиска","Искать"},
  {"поиск выкл","🏠 Главное меню"},
 }{
  raw:=replyButtonsFor(tc.command,c,cfg)
  if !strings.Contains(raw,tc.want){t.Errorf("after %q missing %q: %s",tc.command,tc.want,raw)}
 }
}

func TestVKReplyMenuRejectsInjectedCommands(t *testing.T){
 for _,command:=range []string{"rm -rf /","command xyz","unknown 123"}{
  raw,_:=json.Marshal(map[string]any{"aibot":1,"command":command})
  if got:=vkReplyCommand(raw);got!=""{t.Fatalf("unrecognized payload %q accepted as %q",command,got)}
 }
}

func TestVKTextActionPayloadSurvivesVKForms(t *testing.T){
 button:=replyButton{Label:"✅ Авто",Command:"модель авто",Color:"positive"}
 key:=replyKeyboard([][]replyButton{{button}})
 vals:=url.Values{"keyboard":{key}}
 parsed,err:=url.ParseQuery(vals.Encode());if err!=nil{t.Fatal(err)}
 var kb replyKeyboardJSON
 if err:=json.Unmarshal([]byte(parsed.Get("keyboard")),&kb);err!=nil{t.Fatal(err)}
 if got:=vkReplyCommand(json.RawMessage(kb.Buttons[0][0].Action.Payload));got!="модель авто"{t.Fatalf("escaped payload not decoded: %q",got)}
}
