package main

import (
 "bufio"
 "bytes"
 "context"
 "errors"
 "path/filepath"
 "strings"
 "testing"
)

func setupWizardFixture(t *testing.T)(Config,*Catalog){
 t.Helper()
 cfg:=defaults()
 cfg.Providers["groq"]=ProviderConfig{Enabled:true,APIKey:"old-valid-key"}
 cfg.AutoModelsProviders=[]string{"groq"}
 cfg.DefaultModel="groq:openai/gpt-oss-20b"
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 c.health=newModelHealth(filepath.Join(t.TempDir(),"model-health.json"))
 return cfg,c
}
func TestProviderNetworkCheckHappensBeforeAPIKeyInput(t *testing.T){
 cfg,c:=setupWizardFixture(t)
 var out bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("new-key\n")),out:&out}
 called:=false
 valid:=w.configureProviderWith(&cfg,c,"groq",1,
  func(context.Context,string)(string,error){return "DNS: OK",errors.New("TLS blocked")},
  func(context.Context,Config,string,[]Message)(string,error){called=true;return "",nil})
 if valid||called{t.Fatal("API should not be called before network preflight")}
 if cfg.Providers["groq"].APIKey!="old-valid-key"{t.Fatal("old working key overwritten")}
 if !strings.Contains(out.String(),"Прежн")&&!strings.Contains(out.String(),"не изменены"){t.Fatal(out.String())}
}
func TestProviderNeverOverwritesKeyOnModelFailure(t *testing.T){
 cfg,c:=setupWizardFixture(t)
 var out bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("new-invalid-key\n")),out:&out}
 called:=0
 valid:=w.configureProviderWith(&cfg,c,"groq",1,
  func(context.Context,string)(string,error){return "DNS, TCP, TLS OK",nil},
  func(_ context.Context,p Config,key string,_ []Message)(string,error){
   called++
   if p.Providers["groq"].APIKey!="new-invalid-key"{t.Fatal("tested old key, not proposed key")}
   return "",errors.New("HTTP 401")
  })
 if valid||called!=2{t.Fatalf("bad provider accepted, calls=%d",called)}
 if cfg.Providers["groq"].APIKey!="old-valid-key"{t.Fatal("lost previous provider key")}
 if len(cfg.EnabledModels)!=0||cfg.DefaultModel!="groq:openai/gpt-oss-20b"{t.Fatal("previous default or models changed")}
}
func TestProviderCommitsOnlyAfterVerifiedRussian(t *testing.T){
 cfg,c:=setupWizardFixture(t)
 cfg.RussianBlockedModels=[]string{"groq:openai/gpt-oss-120b"}
 var out bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("new-good-key\n")),out:&out}
 calls:=map[string]int{}
 valid:=w.configureProviderWith(&cfg,c,"groq",1,
  func(context.Context,string)(string,error){return "DNS, TCP, TLS OK",nil},
  func(_ context.Context,p Config,key string,_ []Message)(string,error){
   calls[key]++
   if p.Providers["groq"].APIKey!="new-good-key"{t.Fatal("new key not used in probe")}
   return "Столица России — Москва.",nil
  })
 if !valid||cfg.Providers["groq"].APIKey!="new-good-key"{t.Fatal("verified key not applied:",out.String())}
 if len(c.enabled(cfg))!=2||len(cfg.RussianBlockedModels)!=0{t.Fatal("validated models not available",c.enabled(cfg))}
 if len(calls)!=2{t.Fatal("not every free candidate tested",calls)}
}
func TestEnglishOnlyModelsAreExcludedWithoutDeletingProvider(t *testing.T){
 cfg,c:=setupWizardFixture(t)
 var out bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("english-only-key\n")),out:&out}
 called:=map[string]int{}
 applied:=w.configureProviderWith(&cfg,c,"groq",1,
  func(context.Context,string)(string,error){return "",nil},
  func(_ context.Context,_ Config,key string,_ []Message)(string,error){
   called[key]++
   return "Moscow is the capital of Russia.",nil
  })
 if applied||cfg.Providers["groq"].APIKey!="old-valid-key"{t.Fatal("English-only provider was accepted")}
 if called["groq:openai/gpt-oss-20b"]!=2||called["groq:openai/gpt-oss-120b"]!=2{t.Fatal("two independent prompts required",called)}
 staged:=cfg
 staged.Providers=map[string]ProviderConfig{"groq":{Enabled:true,APIKey:"english-only-key"}}
 for _,key:=range []string{"groq:openai/gpt-oss-20b","groq:openai/gpt-oss-120b"}{
  status,ok:=c.health.lookup(staged,key)
  if !ok||status.Reason!="russian_unsupported"||c.health.available(staged,key){t.Fatal("English-only model not excluded",key,status,ok)}
  if c.health.due(staged,key){t.Fatal("known English-only model should not be re-polled immediately")}
 }
}
func TestModelScopeCheckDoesNotHideSuccessfulOtherProviders(t *testing.T){
 cfg,c:=setupWizardFixture(t)
 cfg.Providers["zai"]=ProviderConfig{APIKey:"other-good-key",Enabled:true}
 cfg.AutoModelsProviders=append(cfg.AutoModelsProviders,"zai")
 if err:=c.health.record(cfg,"zai:glm-4.7-flash",true);err!=nil{t.Fatal(err)}
 var out bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("new-key\n")),out:&out}
 ok:=w.configureProviderWith(&cfg,c,"groq",1,
  func(context.Context,string)(string,error){return "",nil},
  func(_ context.Context,_ Config,_ string,_ []Message)(string,error){return "Москва — столица России.",nil})
 if !ok{t.Fatal("provider not connected:",out.String())}
 if !c.health.available(cfg,"zai:glm-4.7-flash"){t.Fatal("other provider availability lost")}
}
