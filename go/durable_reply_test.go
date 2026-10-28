package main

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
 "time"
)

func TestVKReplayDoesNotRegenerateAIOrChargeTwice(t *testing.T){
 var aiCalls atomic.Int32
 var vkCalls atomic.Int32
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path{
  case "/v1/chat/completions":
   aiCalls.Add(1)
   fmt.Fprint(w, `{"choices":[{"message":{"content":"Ответ ИИ"}}]}`)
  case "/method/messages.send":
   if vkCalls.Add(1)==1{w.WriteHeader(http.StatusServiceUnavailable);return}
   fmt.Fprint(w, `{"response":100}`)
  default:w.WriteHeader(http.StatusNotFound)
  }
 }))
 defer server.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/method/"
 defer func(){vkAPIBaseURL=old}()
 cfg:=testCfg()
 cfg.FreeFallback=false
 catalog,err:=newCatalog();if err!=nil{t.Fatal(err)}
 spec:=catalog.Specs["groq"];spec.Endpoint=server.URL+"/v1/chat/completions";catalog.Specs["groq"]=spec
 store,err:=openStore(filepath.Join(t.TempDir(),"history.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 b:=newBot(cfg,store,catalog);b.ai.client=server.Client();b.vk.client=server.Client()
 message:=VKMessage{PeerID:2000000001,FromID:42,ConversationMessageID:300,Text:"бот, привет"}
 if err=b.event(context.Background(),message);err!=nil{t.Fatal(err)}
 if err=b.execute(context.Background(),message);err==nil{t.Fatal("first VK send must fail")}
 cached,ready,err:=store.prepared(message)
 if err!=nil||!ready||!strings.Contains(cached,"Ответ ИИ"){t.Fatalf("missing outbox text %q %v",cached,err)}
 if err=b.execute(context.Background(),message);err!=nil{t.Fatal(err)}
 if aiCalls.Load()!=1||vkCalls.Load()!=2{t.Fatalf("LLM called %d times, VK called %d",aiCalls.Load(),vkCalls.Load())}
 total,_,_:=store.counts(message.PeerID,message.FromID)
 if total!=1{t.Fatalf("expected 1 charged AI attempt, got %d",total)}
 recent:=store.recent(message.PeerID,10,24)
 if len(recent)!=2||recent[1].Sender!=-cfg.GroupID{t.Fatalf("replayed bot answer not stored correctly: %+v",recent)}
}

func TestQueuedMessageRetryBackoffDoesNotBlockOtherChats(t *testing.T){
 store,err:=openStore(filepath.Join(t.TempDir(),"history.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 first:=VKMessage{PeerID:2000000001,FromID:1,ConversationMessageID:1,Text:"бот первый"}
 second:=VKMessage{PeerID:2000000002,FromID:2,ConversationMessageID:2,Text:"бот второй"}
 if _,err=store.recordAndQueue(first,true);err!=nil{t.Fatal(err)}
 if _,err=store.recordAndQueue(second,true);err!=nil{t.Fatal(err)}
 if err=store.retry(first);err!=nil{t.Fatal(err)}
 next,_,err:=store.nextPending()
 if err!=nil||next.EventID()!=second.EventID(){t.Fatalf("retry blocked other chat: %+v %v",next,err)}
 if _,err=store.DB.Exec("UPDATE pending SET retry_after=0 WHERE event_id=?",first.EventID());err!=nil{t.Fatal(err)}
 next,attempts,err:=store.nextPending()
 if err!=nil||next.EventID()!=first.EventID()||attempts!=1{t.Fatalf("retry lost job: %+v %d %v",next,attempts,err)}
 if retryBackoff(1)!=2*time.Second||retryBackoff(100)>5*time.Minute{t.Fatal("unexpected retry schedule")}
}

func TestPendingPayloadHasBoundedQuestionAndNoFullReply(t *testing.T){
 store,err:=openStore(filepath.Join(t.TempDir(),"history.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 msg:=VKMessage{PeerID:2000000001,FromID:42,ConversationMessageID:4,
  Text:"бот "+strings.Repeat("а",6000),
  ReplyMessage:&VKMessage{FromID:-123,Text:"private earlier reply "+strings.Repeat("z",6000)}}
 if _,err=store.recordAndQueue(msg,true);err!=nil{t.Fatal(err)}
 next,_,err:=store.nextPending();if err!=nil{t.Fatal(err)}
 if len([]rune(next.Text))>3000{t.Fatal("unbounded VK queue input")}
 if next.ReplyMessage==nil||next.ReplyMessage.FromID!=-123{t.Fatal("reply trigger lost")}
 if next.ReplyMessage.Text!=""{t.Fatal("nested private reply leaked to queue")}
}

func TestReplyPreparationSurvivesSQLiteRestart(t *testing.T){
 path:=filepath.Join(t.TempDir(),"history.db")
 store,err:=openStore(path);if err!=nil{t.Fatal(err)}
 msg:=VKMessage{PeerID:2000000001,FromID:42,ID:77,Text:"бот привет"}
 if _,err=store.recordAndQueue(msg,true);err!=nil{t.Fatal(err)}
 if err=store.prepare(msg,"Уже сгенерирован");err!=nil{t.Fatal(err)}
 store.close()
 again,err:=openStore(path);if err!=nil{t.Fatal(err)};defer again.close()
 cached,ok,err:=again.prepared(msg)
 if err!=nil||!ok||cached!="Уже сгенерирован"{t.Fatalf("outbox lost: %q %t %v",cached,ok,err)}
}
