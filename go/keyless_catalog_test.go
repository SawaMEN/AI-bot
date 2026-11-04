package main
import (
 "context"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
)
func TestProductionRegistryOnlyApprovedModels(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 if len(c.Order)!=11{t.Fatalf("unexpected provider count: %v",c.Order)}
 for _,id:=range []string{"g4fgroq","g4fgemini","g4follama","g4fpollinations","llmfaucet","gemini","openrouter","ptero","cloudflare","llm7","rok"}{if _,ok:=c.Specs[id];!ok{t.Fatalf("provider %s missing",id)}}
 for _,id:=range []string{"groq","mistral","zai","siliconflow","modelscope","pollinations"}{
  if _,ok:=c.Specs[id];ok{t.Errorf("retired provider %s still registered",id)}
 }
 for _,bad:=range []string{"llm7:paid-model","g4fgroq:openai/gpt-oss-20b","duckai:claude-haiku-4-5"}{
  p,id,err:=splitModel(bad);if err==nil&&c.modelAllowedID(p,id){t.Errorf("unsafe model %s allowed",bad)}
 }
}
func TestKeylessAPIGeneratesWithoutAuthorization(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if got:=r.Header.Get("Authorization");got!=""{t.Errorf("sent authorization to anonymous service: %s",got)}
  if r.Method!="POST"{t.Errorf("method %s",r.Method)}
  w.Header().Set("Content-Type","application/json")
  _,_=w.Write([]byte(`{"choices":[{"message":{"content":"Столица России — Москва."}}]}`))
 }));defer srv.Close()
 spec:=c.Specs["g4fgroq"];spec.Endpoint=srv.URL;c.Specs["g4fgroq"]=spec
 cfg:=defaults();cfg.Providers["g4fgroq"]=ProviderConfig{Enabled:true}
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 key:="g4fgroq:openai/gpt-oss-120b"
 ai:=newAI(c);ai.client=srv.Client()
 reply,err:=ai.generate(context.Background(),cfg,key,[]Message{{Role:"user",Content:"Столица?"}})
 if err!=nil||!strings.Contains(reply,"Москва"){t.Fatalf("anonymous request failed: %q %v",reply,err)}
 if len(c.enabled(cfg))!=0{t.Fatal("model shown without real Russian check")}
 if err:=c.health.record(cfg,key,true);err!=nil{t.Fatal(err)}
 if got:=c.enabled(cfg);len(got)!=1||got[0]!=key{t.Fatal("verified keyless model hidden",got)}
}
func TestMigrationKeepsGeminiAndRemovesRetiredProviders(t *testing.T){
 cfg:=defaults()
 cfg.Providers=map[string]ProviderConfig{"gemini":{Enabled:true,APIKey:"gemini-secret"},"groq":{Enabled:true,APIKey:"old-secret"},"llm7":{Enabled:false}}
 cfg.EnabledModels=[]string{"groq:openai/gpt-oss-20b","gemini:gemini-2.5-flash"}
 cfg.AutoModelsProviders=[]string{"groq","gemini"}
 cfg.DefaultModel="groq:openai/gpt-oss-20b"
 enableKeylessCatalog(&cfg)
 if len(cfg.Providers)!=6||cfg.Providers["gemini"].APIKey!="gemini-secret"{t.Fatal("Gemini credentials lost or old provider retained",cfg.Providers)}
 if cfg.Providers["llm7"].Enabled{t.Fatal("explicit disable ignored")}
 if !cfg.Providers["g4fgroq"].Enabled{t.Fatal("anonymous default not enabled")}
 if len(cfg.EnabledModels)!=1||cfg.EnabledModels[0]!="gemini:gemini-2.5-flash"{t.Fatal("retired model remains",cfg.EnabledModels)}
 if cfg.DefaultModel!=""{t.Fatal("retired default remains")}
}

func TestOpenRouterFreeOnlyWithPriceValidation(t *testing.T) {
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 p:=c.Specs["openrouter"]
 if p.FreePolicy!="priced_openrouter"{t.Fatal("OpenRouter must use dynamically priced models")}
 for _,id:=range []string{"google/gemini-2.5-pro","openai/gpt-4o","meta-llama/llama-3.3-70b"} {
  if c.modelAllowedID("openrouter",id){t.Fatalf("paid or unqualified model allowed: %s",id)}
 }
 for _,tc:=range []struct{id,price string;allowed bool}{
  {"deepseek/deepseek-r1:free","0",true},
  {"deepseek/deepseek-r1:free","0.0001",false},
  {"deepseek/deepseek-r1","0",false},
 }{
  item:=map[string]any{"id":tc.id,"pricing":map[string]any{"prompt":tc.price,"completion":"0"}}
  if got:=freeItem(p,item)&&c.modelAllowedID("openrouter",tc.id);got!=tc.allowed{t.Errorf("free policy %s %s = %v",tc.id,tc.price,got)}
 }
}

func TestCloudflareCuratedFreeModelsNeedCredentials(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 cfg:=defaults()
 cfg.Providers["cloudflare"]=ProviderConfig{Enabled:true}
 if len(c.candidates(cfg))!=0{t.Fatal("Cloudflare allowed without key")}
 cfg.Providers["cloudflare"]=ProviderConfig{Enabled:true,APIKey:"mock",CloudflareAccount:"account-123"}
 models:=c.candidates(cfg)
 if len(models)!=3{t.Fatalf("expected 3 Cloudflare free models, got %v",models)}
 for _,id:=range models{if !strings.HasPrefix(id,"cloudflare:@cf/"){t.Error("unexpected Cloudflare model",id)}}
 if c.modelAllowedID("cloudflare","@cf/deepseek-ai/deepseek-v4-flash-0731"){t.Fatal("paid-only Cloudflare model allowed")}
}

func TestOpenRouterAndCloudflareSettingsSurviveMigration(t *testing.T){
 cfg:=defaults()
 cfg.Providers=map[string]ProviderConfig{
  "openrouter":{Enabled:true,APIKey:"or-key"},
  "cloudflare":{Enabled:true,APIKey:"cf-key",CloudflareAccount:"cf-account"},
  "gemini":{Enabled:true,APIKey:"gemini-key"},
 }
 cfg.AutoModelsProviders=[]string{"openrouter","cloudflare"}
 cfg.EnabledModels=[]string{"cloudflare:@cf/google/gemma-4-26b-a4b-it","groq:openai/gpt-oss-20b"}
 cfg.DefaultModel="cloudflare:@cf/google/gemma-4-26b-a4b-it"
 enableKeylessCatalog(&cfg)
 if cfg.Providers["openrouter"].APIKey!="or-key"||cfg.Providers["cloudflare"].CloudflareAccount!="cf-account"||cfg.Providers["gemini"].APIKey!="gemini-key"{t.Fatal("existing provider credentials lost")}
 if len(cfg.EnabledModels)!=1||cfg.EnabledModels[0]!=cfg.DefaultModel{t.Fatal("Cloudflare selection lost",cfg.EnabledModels)}
 if !contains(cfg.AutoModelsProviders,"openrouter"){t.Fatal("OpenRouter automatic catalog lost")}
}

func TestRegistryRanksKeylessFirstAndHasTenFreeCandidates(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 seenRegistered:=false
 models:=0
 for _,id:=range c.Order{
  p:=c.Specs[id]
  if p.requiresKey(){
   seenRegistered=true
   if !strings.Contains(p.Name,"(с регистрацией)"){t.Errorf("missing registration label: %s",p.Name)}
  }else{
   if seenRegistered{t.Errorf("anonymous provider after registered: %s",id)}
   if !strings.Contains(p.Name,"без регистрации"){t.Errorf("missing anonymous label: %s",p.Name)}
  }
  for _,m:=range p.Models{
   if !c.modelAllowedID(id,m[0]){t.Errorf("curated model rejected: %s:%s",id,m[0])}
   models++
  }
 }
 if models<10{t.Fatalf("expected >=10 free model candidates, got %d",models)}
 if c.modelAllowedID("llm7","DeepSeek-V4-Flash-0731"){t.Fatal("paid LLM7 model allowed")}
 cfg:=defaults();cfg.Providers["llm7"]=ProviderConfig{Enabled:true}
 if got:=c.candidates(cfg);len(got)!=0{t.Fatalf("LLM7 accepted without key: %v",got)}
 cfg.Providers["llm7"]=ProviderConfig{Enabled:true,APIKey:"free-token"}
 if len(c.candidates(cfg))!=2{t.Fatal("free LLM7 router variants absent")}
}
func TestOptionalKeyIsForwardedOnlyWhenProvided(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 got:=[]string{}
 srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  got=append(got,r.Header.Get("Authorization"))
  w.Header().Set("Content-Type","application/json")
  _,_=w.Write([]byte(`{"choices":[{"message":{"content":"Столица России — Москва."}}]}`))
 }));defer srv.Close()
 p:=c.Specs["llmfaucet"];p.Endpoint=srv.URL;c.Specs["llmfaucet"]=p
 cfg:=defaults();cfg.Providers["llmfaucet"]=ProviderConfig{Enabled:true}
 ai:=newAI(c);ai.client=srv.Client()
 key:="llmfaucet:auto"
 if _,err:=ai.generate(context.Background(),cfg,key,[]Message{{Role:"user",Content:"Столица?"}});err!=nil{t.Fatal(err)}
 cfg.Providers["llmfaucet"]=ProviderConfig{Enabled:true,APIKey:"my-free-key"}
 if _,err:=ai.generate(context.Background(),cfg,key,[]Message{{Role:"user",Content:"Столица?"}});err!=nil{t.Fatal(err)}
 if len(got)!=2||got[0]!=""||got[1]!="Bearer my-free-key"{t.Fatalf("wrong auth: %v",got)}
}
func TestKeylessNotRegisteredAndRegisteredKeyRequiredInWebForm(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 cfg:=defaults()
 for _,id:=range []string{"g4fgroq","llmfaucet"}{
  form:=map[string][]string{"action":{"provider"},"provider":{id},"enabled":{"on"}}
  if _,err:=applyWebForm(&cfg,c,form);err!=nil{t.Errorf("no-key %s failed: %v",id,err)}
 }
 form:=map[string][]string{"action":{"provider"},"provider":{"ptero"},"enabled":{"on"}}
 if _,err:=applyWebForm(&cfg,c,form);err==nil{t.Fatal("Ptero accepted without registered key")}
}
