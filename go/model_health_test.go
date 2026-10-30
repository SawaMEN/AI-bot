package main

import (
 "bytes"
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestHealthShowsOnlyRealWorkingModels(t *testing.T){
 cfg:=testCfg()
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 c.health=newModelHealth(filepath.Join(t.TempDir(),"model-health.json"))
 if len(c.enabled(cfg))!=0{t.Fatal("untested models shown as available")}
 var out bytes.Buffer
 calls:=map[string]int{}
 err=scanAvailableModelsWith(context.Background(),cfg,c,&out,true,
  func(_ context.Context,_ Config,key string,_ []Message)(string,error){
   calls[key]++
   if strings.HasPrefix(key,"zai:"){return "",errors.New("HTTP 429")}
   return "Столица России — Москва.",nil
  })
 if err!=nil{t.Fatal(err)}
 if calls["groq:openai/gpt-oss-20b"]!=1||calls["zai:glm-4.7-flash"]!=1{
  t.Fatalf("did not scan every model: %+v",calls)
 }
 allowed:=c.enabled(cfg)
 if len(allowed)!=1||allowed[0]!="groq:openai/gpt-oss-20b"{t.Fatal("unverified model shown",allowed)}
 if strings.Contains(out.String(),"Z.ai •")||!strings.Contains(out.String(),"GPT-OSS 20B"){t.Fatal("non-working models leaked into list: ",out.String())}
 if selectable:=c.allowedFor("zai",cfg);len(selectable)!=0{t.Fatal("unavailable model listed for selection",selectable)}
 if got:=listModels(c,cfg,1,true);strings.Contains(got,"Z.ai"){t.Fatal("VK listed unavailable model",got)}
}

func TestHealthRetriesAndRecoversModels(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 c.health=newModelHealth(filepath.Join(t.TempDir(),"model-health.json"))
 var out bytes.Buffer
 calls:=0
 generate:=func(_ context.Context,_ Config,key string,_ []Message)(string,error){
  calls++
  if strings.HasPrefix(key,"groq:"){return "Москва — столица России.",nil}
  return "",errors.New("server unavailable")
 }
 if err:=scanAvailableModelsWith(context.Background(),cfg,c,&out,true,generate);err!=nil{t.Fatal(err)}
 if err:=scanAvailableModelsWith(context.Background(),cfg,c,&out,false,generate);err!=nil{t.Fatal(err)}
 if calls!=2{t.Fatalf("fresh results not reused: %d calls",calls)}
 // Failed models are not hidden permanently. A forced recheck restores them.
 generate=func(_ context.Context,_ Config,_ string,_ []Message)(string,error){return "Столица России — Москва.",nil}
 out.Reset()
 if err:=scanAvailableModelsWith(context.Background(),cfg,c,&out,true,generate);err!=nil{t.Fatal(err)}
 if len(c.enabled(cfg))!=2{t.Fatalf("model never restored: %v",c.enabled(cfg))}
}

func TestHealthInvalidatedOnKeyAndAccountChange(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog();c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 key:=cfg.EnabledModels[0]
 if err:=c.health.record(cfg,key,true);err!=nil{t.Fatal(err)}
 if !c.health.available(cfg,key){t.Fatal("saved health not recognized")}
 changed:=cfg
 changed.Providers=map[string]ProviderConfig{}
 for k,v:=range cfg.Providers{changed.Providers[k]=v}
 p:=changed.Providers["groq"];p.APIKey="NEW_KEY";changed.Providers["groq"]=p
 if c.health.available(changed,key){t.Fatal("health status survived credential change")}
}

func TestHealthPersistsWithoutLeakingSecrets(t *testing.T){
 cfg:=testCfg();path:=filepath.Join(t.TempDir(),"health.json")
 c,_:=newCatalog();c.health=newModelHealth(path)
 if err:=c.health.record(cfg,cfg.EnabledModels[0],true);err!=nil{t.Fatal(err)}
 data,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 if strings.Contains(string(data),"S3CRET_FAKE_KEY_2026"){t.Fatal("plaintext API key leaked into health cache")}
 c2,_:=newCatalog();c2.health=newModelHealth(path)
 if got:=c2.enabled(cfg);len(got)!=1{t.Fatal("successful result not recovered from disk",got)}
}

func TestHealthStaleSuccessIsHidden(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 key:=cfg.EnabledModels[0]
 if err:=c.health.record(cfg,key,true);err!=nil{t.Fatal(err)}
 c.health.mu.Lock()
 value:=c.health.entries[key]
 value.CheckedAt=time.Now().Add(-25*time.Hour)
 c.health.entries[key]=value
 c.health.mu.Unlock()
 if c.health.available(cfg,key){t.Fatal("expired success stays visible")}
 if !c.health.due(cfg,key){t.Fatal("expired success not scheduled for retry")}
}

func TestHealthNoWorkingModels(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog();c.health=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 var out bytes.Buffer
 err:=scanAvailableModelsWith(context.Background(),cfg,c,&out,true,
  func(context.Context,Config,string,[]Message)(string,error){return "",errors.New("authentication denied")})
 if err==nil{t.Fatal("should report zero available models")}
 if len(c.enabled(cfg))!=0{t.Fatal("non-working models visible")}
 if !strings.Contains(out.String(),"Ни одной доступной модели"){t.Fatal(out.String())}
}
