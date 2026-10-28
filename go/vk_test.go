package main

import (
 "encoding/json"
 "testing"
 "net/url"
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
