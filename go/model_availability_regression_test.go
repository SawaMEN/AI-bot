package main

import (
 "path/filepath"
 "strings"
 "testing"
 "net/url"
)

func TestParallelModelHealthWritersMergeResults(t *testing.T){
 cfg:=testCfg()
 path:=filepath.Join(t.TempDir(),"model-health.json")
 // Separate instances represent a root CLI and the aibot systemd process.
 a:=newModelHealth(path)
 b:=newModelHealth(path)
 keys:=[]string{"groq:openai/gpt-oss-20b","zai:glm-4.7-flash"}
 if err:=a.record(cfg,keys[0],true);err!=nil{t.Fatal(err)}
 if err:=b.record(cfg,keys[1],true);err!=nil{t.Fatal(err)}
 fresh:=newModelHealth(path)
 for _,key:=range keys{
  if !fresh.available(cfg,key){t.Fatalf("concurrent processes erased successful check for %s",key)}
 }
}

func TestModelHealthConcurrentFailuresDoNotEraseSuccesses(t *testing.T){
 cfg:=testCfg()
 path:=filepath.Join(t.TempDir(),"model-health.json")
 cli:=newModelHealth(path);bot:=newModelHealth(path)
 pass:="groq:openai/gpt-oss-20b"
 fail:="zai:glm-4.7-flash"
 if err:=cli.record(cfg,pass,true);err!=nil{t.Fatal(err)}
 if err:=bot.recordReason(cfg,fail,false,"api_error");err!=nil{t.Fatal(err)}
 fresh:=newModelHealth(path)
 if !fresh.available(cfg,pass){t.Fatal("failed provider erased working provider status")}
 value,ok:=fresh.lookup(cfg,fail)
 if !ok||value.Passed||value.Reason!="api_error"{t.Fatalf("failed entry lost: %+v %v",value,ok)}
}

func TestEnabledProviderWithoutManualSelectionUsesAutomaticCatalog(t *testing.T){
 cfg:=testCfg()
 cfg.Providers=map[string]ProviderConfig{"groq":cfg.Providers["groq"]}
 cfg.EnabledModels=nil
 cfg.AutoModelsProviders=nil
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 candidates:=c.candidates(cfg)
 if len(candidates)!=2{t.Fatalf("enabled free providers with no explicit models should work automatically: %v",candidates)}
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 if err:=c.health.record(cfg,"groq:openai/gpt-oss-20b",true);err!=nil{t.Fatal(err)}
 if allowed:=c.enabled(cfg);len(allowed)!=1{t.Fatalf("verified free model hidden: %v",allowed)}
}

func TestPanelAddingProviderAutomaticallyEnablesModelSelection(t *testing.T){
 cfg:=defaults()
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 _,err=applyWebForm(&cfg,c,url.Values{
  "action":{"provider"},"provider":{"groq"},"api_key":{"test-only-key"},"enabled":{"on"},
 })
 if err!=nil{t.Fatal(err)}
 if !cfg.Providers["groq"].Enabled||!contains(cfg.AutoModelsProviders,"groq"){
  t.Fatalf("connected provider lacks automatic model selection: %+v %+v",cfg.Providers["groq"],cfg.AutoModelsProviders)
 }
 if len(c.candidates(cfg))!=2{t.Fatal("new provider has no candidates")}
}

func TestVKMissingModelStatusExplainsPendingVersusFailed(t *testing.T){
 cfg:=testCfg()
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 pending:=modelAvailabilityExplanation(c,cfg)
 if !strings.Contains(pending,"Ещё не проверено")||!strings.Contains(pending,"sudo aibot check-ai"){
  t.Fatalf("waiting reason not explained: %s",pending)
 }
 key:="groq:openai/gpt-oss-20b"
 if err:=c.health.recordReason(cfg,key,false,"api_error");err!=nil{t.Fatal(err)}
 failed:=modelAvailabilityExplanation(c,cfg)
 if !strings.Contains(failed,"Ошибки API")||!strings.Contains(failed,"Ещё не проверено"){
  t.Fatalf("failed API not distinguished: %s",failed)
 }
 if got:=listModels(c,cfg,1,false);!strings.Contains(got,"Ещё не проверено"){
  t.Fatalf("VK model list hides reason: %s",got)
 }
 if got:=replyModelBoard(c,cfg,1);!strings.Contains(got,"Обновить список"){
  t.Fatal("no one-tap refresh when models are pending")
 }
}

func TestVKAvailableModelsReappearAfterSuccessfulProbe(t *testing.T){
 cfg:=testCfg()
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 if err:=c.health.record(cfg,"groq:openai/gpt-oss-20b",true);err!=nil{t.Fatal(err)}
 if got:=listModels(c,cfg,1,false);!strings.Contains(got,"GPT-OSS 20B")||strings.Contains(got,"Ещё не проверено"){
  t.Fatalf("successful verified model not available in VK: %s",got)
 }
}
