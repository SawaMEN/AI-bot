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
)

func TestRussianLanguageGuards(t *testing.T){
 for _,tc:=range []struct{text string;wrong bool}{
  {"Это полноценный русский ответ.",false},
  {"Москва",false},
  {"The provider cannot answer in Russian at this time.",true},
  {"```go\nfmt.Println(\"hello world\")\n```\nНа русском объясняю код.",false},
  {"42",false},
  {"https://example.com/long-url-in-english",false},
 }{
  if actual:=nonRussianProse(tc.text);actual!=tc.wrong{t.Errorf("nonRussianProse(%q)=%v want %v",tc.text,actual,tc.wrong)}
 }
 if !wantsRussian("бот, привет")||wantsRussian("переведи на английский текст")||wantsRussian("answer in English"){t.Fatal("language preference detection")}
 if !russianProbeOK("Столица России — Москва.")||russianProbeOK("Moscow"){t.Fatal("Russian probe accepted the wrong language")}
}

func TestRussianExclusionsSurviveAutoModeAndPreference(t *testing.T){
 cfg:=testCfg()
 cfg.AutoModelsProviders=[]string{"groq","zai"}
 cfg.RussianBlockedModels=[]string{"groq:openai/gpt-oss-20b"}
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 list:=c.enabled(cfg)
 for _,key:=range list{if key==cfg.RussianBlockedModels[0]{t.Fatal("blocked language model re-enabled automatically")}}
 if c.defaultModel(cfg)!="zai:glm-4.7-flash"{t.Fatalf("default did not migrate: %s",c.defaultModel(cfg))}
}

func TestEnglishOnlyModelFallsBackToRussianAndQuarantines(t *testing.T){
 var primary,secondary atomic.Int32
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path{
  case "/groq":
   primary.Add(1)
   fmt.Fprint(w,`{"choices":[{"message":{"content":"I can only provide an answer in English."}}]}`)
  case "/zai":
   secondary.Add(1)
   fmt.Fprint(w,`{"choices":[{"message":{"content":"Конечно, я отвечу по-русски."}}]}`)
  case "/method/messages.setActivity":fmt.Fprint(w,`{"response":1}`)
  case "/method/messages.send":fmt.Fprint(w,`{"response":333}`)
  default: http.NotFound(w,r)
  }
 }))
 defer server.Close()
 oldVK:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/method/"
 defer func(){vkAPIBaseURL=oldVK}()
 cfg:=testCfg()
 cfg.SearchEnabled=false
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 spec:=c.Specs["groq"];spec.Endpoint=server.URL+"/groq";c.Specs["groq"]=spec
 spec=c.Specs["zai"];spec.Endpoint=server.URL+"/zai";c.Specs["zai"]=spec
 store,err:=openStore(filepath.Join(t.TempDir(),"history.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 b:=newBot(cfg,store,c);b.ai.client=server.Client();b.vk.client=server.Client()
 message:=VKMessage{PeerID:2000000001,FromID:44,ConversationMessageID:1,Text:"бот, привет"}
 if err=b.event(context.Background(),message);err!=nil{t.Fatal(err)}
 if err=b.execute(context.Background(),message);err!=nil{t.Fatal(err)}
 prepared,_,err:=store.prepared(message);if err!=nil{t.Fatal(err)}
 if !strings.Contains(prepared,"Конечно, я отвечу по-русски."){t.Fatalf("unexpected fallback: %q",prepared)}
 if primary.Load()!=1||secondary.Load()!=1{t.Fatalf("calls primary=%d secondary=%d",primary.Load(),secondary.Load())}
 if _,ok:=b.unavailable["groq:openai/gpt-oss-20b"];!ok{t.Fatal("English-only model not quarantined")}
}
