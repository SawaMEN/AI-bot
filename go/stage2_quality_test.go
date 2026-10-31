package main

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "time"
 "unicode/utf8"
)

func TestAdaptiveModelRankPrefersReliableFastModel(t *testing.T){
 s,err:=openStore(filepath.Join(t.TempDir(),"metrics.db"));if err!=nil{t.Fatal(err)}
 defer s.close()
 slow,fast:="groq:openai/gpt-oss-120b","groq:openai/gpt-oss-20b"
 for i:=0;i<5;i++{
  s.recordModelAttempt(slow,i<2,20*time.Second)
  s.recordModelAttempt(fast,true,500*time.Millisecond)
 }
 ranked:=s.rankedModels([]string{slow,fast},"Привет")
 if len(ranked)!=2||ranked[0]!=fast{t.Fatalf("unexpected ranked model order: %v",ranked)}
 metrics:=s.modelMetrics()
 if metrics[fast].Attempts!=5||metrics[fast].Successes!=5{t.Fatalf("attempt data lost: %+v",metrics)}
}

func TestModelComplexityAndManualOverrides(t *testing.T){
 if !complexQuestion("Сравни производительность двух алгоритмов")||complexQuestion("Привет"){t.Fatal("wrong query complexity classifier")}
 if modelHeuristic("groq:openai/gpt-oss-120b",true)<=modelHeuristic("groq:openai/gpt-oss-20b",true){t.Fatal("complex question needs large model preference")}
 cfg:=testCfg();c,_:=newCatalog()
 cfg.DefaultModel="groq:openai/gpt-oss-20b"
 s,err:=openStore(filepath.Join(t.TempDir(),"prefs.db"));if err!=nil{t.Fatal(err)}
 defer s.close()
 if s.explicitAIModel(cfg,42,42,c){t.Fatal("server default must not disable automatic choice")}
 if got:=s.chooseAutoModel(cfg,c.enabled(cfg),"Привет");got!=cfg.DefaultModel{t.Fatalf("initial default preference ignored: %s",got)}
 cfg.DefaultModel=""
 if s.explicitAIModel(cfg,42,42,c){t.Fatal("auto mode ignored")}
 if err:=s.setPref("user",42,"model","groq:openai/gpt-oss-20b");err!=nil{t.Fatal(err)}
 if !s.explicitAIModel(cfg,42,42,c){t.Fatal("user's manual model was overridden")}
}

func TestContextIncludesReplyThreadAndImportantEarlierDetails(t *testing.T){
 turns:=[]Turn{{Sender:1,Text:"Нужно сохранить формат отчетов в CSV"}}
 for i:=0;i<22;i++{turns=append(turns,Turn{Sender:3,Text:"Обсуждение деталей"})}
 turns=append(turns,Turn{Sender:42,ReplyTo:3,Text:"Сделаем именно так"})
 output:=buildConversation(turns)
 if !strings.Contains(output,"Ранее упомянутые цели")||!strings.Contains(output,"CSV"){t.Fatalf("lost important context: %q",output)}
 if !strings.Contains(output,"отвечает участнику 3"){t.Fatalf("lost reply threading: %q",output)}
 if !utf8.ValidString(output){t.Fatal("invalid Unicode after context truncation")}
}

func TestReplyThreadStoredAcrossSQLiteRestart(t *testing.T){
 path:=filepath.Join(t.TempDir(),"replies.db")
 s,err:=openStore(path);if err!=nil{t.Fatal(err)}
 msg:=stagedMessage(2000000001,5);msg.ReplyMessage=&VKMessage{FromID:42}
 if _,err:=s.recordAndQueue(msg,true);err!=nil{t.Fatal(err)}
 s.close()
 s,err=openStore(path);if err!=nil{t.Fatal(err)}
 defer s.close()
 turns:=s.recent(msg.PeerID,10,24)
 if len(turns)!=1||turns[0].ReplyTo!=42{t.Fatalf("reply reference missing after restart: %+v",turns)}
}

func TestWebSearchMergesTwoSourcesAndPrioritizesRelevantResults(t *testing.T){
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path{
  case "/duck":
   fmt.Fprint(w,`<a class="result__a" href="https://example.org/go">Памятка по Go</a><a class="result__a" href="https://example.org/other">Другая страница</a>`)
  case "/wiki":
   w.Header().Set("Content-Type","application/json")
   _=json.NewEncoder(w).Encode(map[string]any{"query":map[string]any{"search":[]map[string]string{
    {"title":"Язык Go","snippet":"Статья про компилятор Go"},
   }}})
  default:t.Errorf("unknown path: %s",r.URL.Path)
  }
 }))
 defer server.Close()
 previousDuck,previousWiki:=duckEndpoint,wikiEndpoint
 duckEndpoint=server.URL+"/duck";wikiEndpoint=server.URL+"/wiki"
 defer func(){duckEndpoint=previousDuck;wikiEndpoint=previousWiki}()
 results:=webSearch(context.Background(),"компилятор Go")
 if len(results)<2{t.Fatalf("expected independent sources; got %+v",results)}
 foundWiki,foundDuck:=false,false
 for _,r:=range results{
  if strings.Contains(r.URL,"wikipedia.org"){foundWiki=true}
  if strings.Contains(r.URL,"example.org/go"){foundDuck=true}
 }
 if !foundWiki||!foundDuck{t.Fatalf("one source missing: %+v",results)}
}

func TestVKOptionalNamesAreSanitizedAndCached(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 s,err:=openStore(filepath.Join(t.TempDir(),"names.db"));if err!=nil{t.Fatal(err)}
 defer s.close()
 var calls int
 api:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/method/users.get"{t.Errorf("bad users lookup: %s",r.URL.Path)}
  calls++
  fmt.Fprint(w,`{"response":[{"id":42,"first_name":"Иван","last_name":"Петров"}]}`)
 }))
 defer api.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=api.URL+"/method/"
 defer func(){vkAPIBaseURL=old}()
 bot:=newBot(cfg,s,c);bot.vk.client=api.Client()
 turns:=[]Turn{{Sender:42,Text:"Привет"}}
 names:=bot.chatNames(context.Background(),turns)
 if names[42]!="Иван Петров"{t.Fatalf("VK names not decoded: %+v",names)}
 names=bot.chatNames(context.Background(),turns)
 if calls!=1{t.Fatalf("unnecessary repeated VK lookups: %d",calls)}
 if !strings.Contains(buildConversationWithNames(turns,names),"Иван Петров"){t.Fatal("context ignored participant name")}
}
