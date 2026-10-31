package main

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
)

func TestModelFailureClassificationAndVKLabels(t *testing.T){
 tests:=[]struct{err error;reason string;fragment string}{
  {errors.New("HTTP 401/403 key rejected"),"auth","ключ"},
  {errors.New("HTTP 429"),"rate_limit","квота"},
  {errors.New("HTTP 402"),"billing","баланса"},
  {errors.New("HTTP 404/410"),"not_found","не существует"},
  {errors.New("HTTP 400"),"request_rejected","формат"},
  {errors.New("HTTP 422"),"request_rejected","формат"},
  {errors.New("API не вернул текст"),"empty_response","текст"},
  {context.DeadlineExceeded,"network","сети"},
  {errors.New("dial tcp no such host"),"network","сети"},
  {errors.New("HTTP 503"),"server_error","провайдера"},
  {errors.New("unknown provider response"),"api_error","Ошибка API"},
 }
 for _,test:=range tests{
  got:=classifyModelFailure(test.err,0)
  if got!=test.reason{t.Errorf("%v => %q, want %q",test.err,got,test.reason)}
  if !strings.Contains(modelFailureLabel(got),test.fragment){
   t.Errorf("bad user-facing status for %s: %s",got,modelFailureLabel(got))
  }
 }
 if classifyModelFailure(nil,2)!="russian_unsupported"{t.Fatal("language rejection not distinguished")}
 if classifyModelFailure(nil,0)!="invalid_response"{t.Fatal("invalid response not distinguished")}
}

func TestVKShowsThreeConcreteFailureReasons(t *testing.T){
 cfg:=testCfg()
 cfg.Providers=map[string]ProviderConfig{"gemini":{APIKey:"private-gemini-secret",Enabled:true}}
 cfg.EnabledModels=nil
 cfg.AutoModelsProviders=[]string{"gemini"}
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 keys:=c.candidates(cfg)
 if len(keys)!=3{t.Fatalf("expected three candidate models, got %v",keys)}
 reasons:=[]string{"auth","rate_limit","empty_response"}
 for i,key:=range keys{
  if err:=c.health.recordReason(cfg,key,false,reasons[i]);err!=nil{t.Fatal(err)}
 }
 result:=modelAvailabilityExplanation(c,cfg)
 if !strings.Contains(result,"моделей на проверке: 3")||!strings.Contains(result,"ПРИЧИНЫ ПО МОДЕЛЯМ"){
  t.Fatal(result)
 }
 for i,key:=range keys{
  if !strings.Contains(result,c.label(key))||!strings.Contains(result,modelFailureLabel(reasons[i])){
   t.Errorf("VK hides reason for %s: %s",key,result)
  }
 }
 if strings.Contains(result,"private-gemini-secret"){t.Fatal("API key leaked into VK")}
}

func TestModelScannerPrintsIndividualFailureStatuses(t *testing.T){
 cfg:=testCfg()
 c,_:=newCatalog()
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 var out bytes.Buffer
 err:=scanAvailableModelsWith(context.Background(),cfg,c,&out,true,
  func(_ context.Context,_ Config,key string,_ []Message)(string,error){
   if strings.HasPrefix(key,"groq:"){return "",errors.New("HTTP 401")}
   return "",errors.New("HTTP 429")
  })
 if err==nil{t.Fatal("failed providers must fail scanner")}
 for _,frag:=range []string{"GroqCloud","Z.ai","HTTP 401/403","HTTP 429"}{
  if !strings.Contains(out.String(),frag){t.Fatalf("missing %q in output: %s",frag,out.String())}
 }
 if strings.Contains(out.String(),"S3CRET_FAKE_KEY_2026"){t.Fatal("scanner printed private key")}
}

func TestOpenAICompatibleTextContentArray(t *testing.T){
 cfg:=testCfg()
 c,_:=newCatalog()
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/openai/v1/chat/completions"{t.Errorf("wrong endpoint %s",r.URL.Path)}
  w.Header().Set("Content-Type","application/json")
  fmt.Fprint(w,`{"choices":[{"message":{"content":[{"type":"text","text":"Столица России — "},{"type":"text","text":"Москва."}]}}]}`)
 }))
 defer server.Close()
 spec:=c.Specs["groq"];spec.Endpoint=server.URL+"/openai/v1/chat/completions";c.Specs["groq"]=spec
 ai:=newAI(c);ai.client=server.Client()
 output,err:=ai.generate(context.Background(),cfg,"groq:openai/gpt-oss-20b",[]Message{{Role:"user",Content:"Столица?"}})
 if err!=nil||output!="Столица России — Москва."{t.Fatalf("array output lost: %q %v",output,err)}
}

func TestOpenAILengthFinishReasonIsActionable(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","application/json")
  fmt.Fprint(w,`{"choices":[{"finish_reason":"length","message":{"content":""}}]}`)
 }))
 defer server.Close()
 p:=c.Specs["groq"];p.Endpoint=server.URL;c.Specs["groq"]=p
 ai:=newAI(c);ai.client=server.Client()
 _,err:=ai.generate(context.Background(),cfg,"groq:openai/gpt-oss-20b",[]Message{{Role:"user",Content:"Привет"}})
 if err==nil||!strings.Contains(err.Error(),"лимит выходных токенов"){t.Fatalf("silent missing text: %v",err)}
 if classifyModelFailure(err,0)!="empty_response"{t.Fatalf("misclassified empty token-limited reply: %v",err)}
}
