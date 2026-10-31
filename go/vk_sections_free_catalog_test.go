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
)

func buttonLabels(t *testing.T,raw string)[]string{
 t.Helper()
 var v struct{Buttons [][]struct{Action struct{Label string `json:"label"`;Payload string `json:"payload"`} `json:"action"`} `json:"buttons"`}
 if err:=json.Unmarshal([]byte(raw),&v);err!=nil{t.Fatal(err)}
 labels:=[]string{}
 for _,row:=range v.Buttons{
  for _,b:=range row{labels=append(labels,b.Action.Label)}
 }
 return labels
}
func TestMenuSeparatesAIFromFeatures(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 checks:=[]struct{name,raw string;present,absent string}{
  {"main private",replyMainBoard(),"🤖 ИИ","Выбрать модель"},
  {"AI private",replyButtonsFor("ии",c,cfg),"Выбрать модель","🔎 Поиск"},
  {"features private",replyButtonsFor("функции",c,cfg),"🔎 Поиск","Выбрать модель"},
  {"settings private",replyPersonalBoard(),"Сбросить мои настройки","🔎 Поиск"},
  {"main group",mainBoard(),"🤖 ИИ","🔎 Поиск"},
  {"AI group",buttonsFor("ии",c,cfg),"Выбрать модель","🔎 Поиск"},
  {"features group",buttonsFor("функции",c,cfg),"🔎 Поиск","Выбрать модель"},
 }
 for _,tc:=range checks{
  if !strings.Contains(tc.raw,tc.present)||strings.Contains(tc.raw,tc.absent){
   t.Errorf("%s menu mixed unrelated features: %v",tc.name,buttonLabels(t,tc.raw))
  }
 }
}
func TestVKShowsNoAPITechnicalInformation(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 store,err:=openStore(filepath.Join(t.TempDir(),"vk.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 v:=newVK(cfg)
 for _,command:=range []string{"меню","ии","модели","функции","настройки","помощь"}{
  result,handled:=processCommand(context.Background(),v,store,c,cfg,42,42,command)
  if !handled{t.Fatalf("did not handle %q",command)}
  lower:=strings.ToLower(result)
  for _,s:=range []string{"api-ключ","http 40","sudo aibot","провайдер","квота","vds","model-health"}{
   if strings.Contains(lower,s){t.Errorf("%q leaked technical detail %q: %s",command,s,result)}
  }
 }
}

func TestVKShowsOnlyPreverifiedRussianModels(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 if len(c.enabled(cfg))!=0{t.Fatal("untested model should be invisible")}
 if strings.Contains(replyButtonsFor("ии",c,cfg),"Выбрать модель"){t.Fatal("unverified AI button should be hidden")}
 if err:=c.health.record(cfg,"groq:openai/gpt-oss-20b",true);err!=nil{t.Fatal(err)}
 if !strings.Contains(replyButtonsFor("ии",c,cfg),"Выбрать модель"){t.Fatal("verified AI not selectable")}
 if !strings.Contains(replyButtonsFor("модели",c,cfg),"GPT-OSS 20B"){t.Fatal("verified AI not in picker")}
 if strings.Contains(replyButtonsFor("модели",c,cfg),"GLM 4.7"){t.Fatal("unverified AI leaked into picker")}
}

func TestExpandedRussianFreeModelDirectorySupportsThirty(t *testing.T){
 cfg:=defaults();cfg.Providers=map[string]ProviderConfig{"openrouter":{Enabled:true,APIKey:"dummy"}}
 cfg.AutoModelsProviders=[]string{"openrouter"}
 c,_:=newCatalog()
 // The catalog only admits individual free variants with zero *input AND*
 // output pricing, plus a whitelisted multilingual family. Paid/safety/chat-
 // incompatible variants must not appear even if the slug includes :free.
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  data:=[]map[string]any{}
  for i:=0;i<34;i++{
   id:=fmt.Sprintf("qwen/example-russian-%02d:free",i)
   data=append(data,map[string]any{"id":id,"pricing":map[string]any{"prompt":"0","completion":"0"},"architecture":map[string]any{"output_modalities":[]string{"text"}}})
  }
  data=append(data,map[string]any{"id":"qwen/expensive:free","pricing":map[string]any{"prompt":"0.1","completion":"0"}})
  data=append(data,map[string]any{"id":"qwen/safety-guard:free","pricing":map[string]any{"prompt":"0","completion":"0"}})
  data=append(data,map[string]any{"id":"unknown/model:free","pricing":map[string]any{"prompt":"0","completion":"0"}})
  w.Header().Set("Content-Type","application/json")
  _=json.NewEncoder(w).Encode(map[string]any{"data":data})
 }))
 defer server.Close()
 p:=c.Specs["openrouter"];p.CatalogURL=server.URL;c.Specs["openrouter"]=p
 models,err:=c.discover(context.Background(),p,"dummy");if err!=nil{t.Fatal(err)}
 if len(models)!=34{t.Fatalf("free multilingual filter must reject paid/safety and admit 34: %d",len(models))}
 c.verified["openrouter"]=models
 candidate:=c.candidates(cfg)
 if len(candidate)!=30{t.Fatalf("expected 30 zero-price Russian-capable candidates; got %d",len(candidate))}
 for _,key:=range candidate{
  if !strings.Contains(key,"example-russian")||!strings.HasSuffix(key,":free"){
   t.Fatalf("non-free candidate escaped whitelist: %s",key)
  }
 }
 // Candidate ≠ available. Until the API passes the real Russian-language
 // test, not even one of these thirty candidates is visible in VK.
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 if len(c.enabled(cfg))!=0{t.Fatal("discovered models bypassed authenticated Russian verification")}
}

func TestRussianFamilyScreenExcludesSpecialists(t *testing.T){
 for _,bad:=range []string{"nvidia/nemotron-content-safety:free","qwen/qwen3-embedding:free","meta/llama-guard:free","gemini/flash-tts:free","x/unknown-specialist:free"}{
  if russianCandidate(bad){t.Errorf("unsafe/non-Russian-text family in catalogue: %s",bad)}
 }
 for _,good:=range []string{"nvidia/nemotron-3-ultra:free","qwen/qwen3:free","google/gemma-4:free","deepseek/deepseek-r1:free","openai/gpt-oss:free"}{
  if !russianCandidate(good){t.Errorf("multilingual family improperly excluded: %s",good)}
 }
}
