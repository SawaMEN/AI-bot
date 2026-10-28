package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
)

func TestAtomicVKInboxAndBotAuthor(t *testing.T) {
 store,err:=openStore(filepath.Join(t.TempDir(),"history.db"))
 if err!=nil{t.Fatal(err)};defer store.close()
 m:=VKMessage{PeerID:2000000001,FromID:42,ConversationMessageID:7,Text:"[club123|Бот], привет"}
 inserted,err:=store.recordAndQueue(m,true)
 if err!=nil||!inserted{t.Fatalf("record failed: %v %v",inserted,err)}
 again,err:=store.recordAndQueue(m,true)
 if err!=nil||again{t.Fatalf("duplicate inserted %v: %v",again,err)}
 got,attempts,err:=store.nextPending()
 if err!=nil||got.EventID()!=m.EventID()||attempts!=0{t.Fatalf("pending lost: %v",err)}
 store.finish(m)
 if _,_,err=store.nextPending();err==nil{t.Fatal("unexpected second pending job")}
 store.answer(m,"Ответ",123)
 rows:=store.recent(m.PeerID,5,24)
 if len(rows)!=2||rows[1].Sender!=-123{t.Fatalf("bot should have community sender -123, got %+v",rows)}
}

func TestQuotaCountsOneScanAndFailClosed(t *testing.T){
 store,err:=openStore(filepath.Join(t.TempDir(),"history.db"))
 if err!=nil{t.Fatal(err)}
 if err=store.track(2000000001,1);err!=nil{t.Fatal(err)}
 if err=store.track(2000000001,2);err!=nil{t.Fatal(err)}
 if err=store.track(2000000002,1);err!=nil{t.Fatal(err)}
 total,chat,user:=store.counts(2000000001,1)
 if total!=3||chat!=2||user!=2{t.Fatalf("bad quotas: %d/%d/%d",total,chat,user)}
 store.close()
 total,chat,user=store.counts(2000000001,1)
 if total<1000000||chat<1000000||user<1000000{t.Fatal("sqlite failure incorrectly resets limits")}
 if store.track(1,1)==nil{t.Fatal("closed sqlite accepted new calls")}
}

func TestVKTimestampFormats(t *testing.T){
 good:=[]string{`"123456789"`,`123456789`}
 for _,item:=range good{got,err:=vkTimestamp(json.RawMessage(item));if err!=nil||got!="123456789"{t.Fatalf("timestamp %s: %s, %v",item,got,err)}}
 for _,item:=range []string{`null`,`""`,`"bad ts"`,`{"ts":1}`}{
  if _,err:=vkTimestamp(json.RawMessage(item));err==nil{t.Fatalf("accepted invalid ts %s",item)}
 }
}

func TestCallbackJSONAndEncodedString(t *testing.T) {
 payload:=`{"aibot":1,"command":"диалог модель 2"}`
 if got:=callbackCommand(json.RawMessage(payload));got!="диалог модель 2"{t.Fatal(got)}
 quoted,_:=json.Marshal(payload)
 if got:=callbackCommand(quoted);got!="диалог модель 2"{t.Fatalf("encoded callback not parsed: %q",got)}
}

func TestVKCanKickIsNotChatAdmin(t *testing.T){
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if strings.HasSuffix(r.URL.Path,"getConversationMembers"){
   fmt.Fprint(w,`{"response":{"items":[{"member_id":42,"can_kick":true}]}}`)
  }else{w.WriteHeader(404)}
 }))
 defer srv.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=srv.URL+"/";defer func(){vkAPIBaseURL=old}()
 v:=newVK(Config{VKToken:"test",GroupID:123})
 if v.admin(context.Background(),2000000001,42){t.Fatal("user with can_kick must not be treated as chat admin")}
}

func TestVKPollDoesNotAdvanceCursorOnHandlerError(t *testing.T){
 var server *httptest.Server
 server=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path {
  case "/method/groups.getLongPollServer":
   fmt.Fprintf(w,`{"response":{"server":%q,"key":"secret","ts":101}}`,server.URL+"/poll")
  case "/poll":
   fmt.Fprint(w,`{"ts":102,"updates":[{"type":"message_new","object":{"message":{"peer_id":2000000001,"from_id":42,"conversation_message_id":1,"text":"бот привет"}}}]}`)
  default:
   w.WriteHeader(404)
  }
 }))
 defer server.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/method/";defer func(){vkAPIBaseURL=old}()
 store,err:=openStore(filepath.Join(t.TempDir(),"history.db"));if err!=nil{t.Fatal(err)};defer store.close()
 v:=newVK(Config{VKToken:"fake",GroupID:123});v.client=server.Client()
 called:=0
 err=vkPoll(context.Background(),v,store,func(VKMessage)error{called++;return errors.New("disk full")},func(int64,int64,string,json.RawMessage){})
 if err==nil||called!=1{t.Fatalf("handler error not propagated: %v, calls %d",err,called)}
 if store.cursor(123)!=""{t.Fatal("cursor advanced after lost VK event")}
}
