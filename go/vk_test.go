package main

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "net/url"
 "sync/atomic"
 "testing"
 "time"
)
func TestMentionWithoutBotPrefix(t *testing.T){
 cfg:=defaults();cfg.GroupID=123
 cases:=[]struct{msg,want string}{
  {"[club123|Лис] что думаешь?","что думаешь?"},
  {"а твоё мнение, [club123|Лис]?","а твоё мнение"},
  {"@club123 расскажи","расскажи"},
  {"привет",""},
 }
 for _,tc:=range cases{
  q,ok:=questionFor(cfg,VKMessage{PeerID:2000000001,FromID:13,ID:1,Text:tc.msg})
  if q!=tc.want||(tc.want!=""&&!ok){t.Fatalf("mention %q -> %q %v want %q",tc.msg,q,ok,tc.want)}
 }
}
func TestVKCallbackButtons(t *testing.T){
 raw:=mainBoard()
 var board map[string]any
 if json.Unmarshal([]byte(raw),&board)!=nil{t.Fatal("not JSON")}
 rows,ok:=board["buttons"].([]any);if !ok||len(rows)>6{t.Fatal("invalid buttons")}
 if callbackCommand(json.RawMessage(`{"aibot":1,"command":"модель 2"}`))!="модель 2"{t.Fatal("callback")}
}
func TestGoAdminWebLimits(t *testing.T) {
 cfg:=defaults();c,_:=newCatalog()
 _,err:=applyWebForm(&cfg,c,url.Values{"action":{"provider"},"provider":{"openai"},"api_key":{"key"}})
 if err==nil{t.Fatal("unknown paid provider allowed")}
 _,err=applyWebForm(&cfg,c,url.Values{"action":{"settings"},"context_messages":{"1000"}})
 if err==nil{t.Fatal("limit bypass")}
}

func TestVKTypingActivityRefreshesAndStops(t *testing.T) {
 var calls atomic.Int32
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/method/messages.setActivity"{t.Errorf("unexpected VK method %s",r.URL.Path);return}
  if err:=r.ParseForm();err!=nil{t.Error(err);return}
  if r.Form.Get("peer_id")!="2000000001"||r.Form.Get("type")!="typing"{t.Errorf("bad activity: %v",r.Form)}
  calls.Add(1)
  fmt.Fprint(w,`{"response":1}`)
 }))
 defer server.Close()
 original:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/method/"
 defer func(){vkAPIBaseURL=original}()
 v:=newVK(testCfg());v.client=server.Client()
 stop:=v.startThinkingWithInterval(context.Background(),2000000001,20*time.Millisecond)
 deadline:=time.After(time.Second)
 for calls.Load()<2{
  select {case <-deadline:stop();t.Fatalf("no refresh: calls=%d",calls.Load());default:time.Sleep(5*time.Millisecond)}
 }
 stop()
 after:=calls.Load()
 time.Sleep(60*time.Millisecond)
 if calls.Load()!=after{t.Fatal("typing indicator continued after stop")}
 stop() // idempotent, including after a successful reply.
}
