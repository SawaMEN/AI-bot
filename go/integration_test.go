package main

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "html/template"
 "time"
)

func testCfg() Config{
 c:=defaults()
 c.VKToken="fake";c.GroupID=123
 c.Providers=map[string]ProviderConfig{"groq":{Enabled:true,APIKey:"free"},"zai":{Enabled:true,APIKey:"free"}}
 c.EnabledModels=[]string{"groq:openai/gpt-oss-20b","zai:glm-4.7-flash"}
 c.DefaultModel=c.EnabledModels[0]
 return c
}
func TestPersonalPreferenceAndChatIsolation(t *testing.T){
 store,err:=openStore(filepath.Join(t.TempDir(),"history.db"));if err!=nil{t.Fatal(err)};defer store.close()
 cfg:=testCfg();catalog,_:=newCatalog()
 if got:=cmdApply(store,catalog,cfg,"chat",2000000001,[]string{"модель","2"});!strings.Contains(got,"GLM"){t.Fatal(got)}
 model,_,_,_,_:=store.effective(cfg,2000000001,42,catalog)
 if model!="zai:glm-4.7-flash"{t.Fatal("chat inheritance",model)}
 if got:=cmdApply(store,catalog,cfg,"user",42,[]string{"модель","1"});!strings.Contains(got,"GPT"){t.Fatal(got)}
 model,_,_,_,_ =store.effective(cfg,2000000001,42,catalog)
 if model!="groq:openai/gpt-oss-20b"{t.Fatal("personal must win",model)}
 model,_,_,_,_=store.effective(cfg,2000000002,12,catalog)
 if model!="groq:openai/gpt-oss-20b"{t.Fatal("chat separation",model)}
 _=cmdApply(store,catalog,cfg,"user",42,[]string{"резерв","выкл"})
 _,_,_,_,fallback:=store.effective(cfg,2000000001,42,catalog)
 if fallback{t.Fatal("personal opted out of cross-provider context")}
}
func TestHTTPAIOnlyApprovedFreeModel(t *testing.T){
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("Authorization")!="Bearer free"{t.Error("missing auth")}
  if r.Method!="POST"{t.Error("not post")}
  var body map[string]any;if json.NewDecoder(r.Body).Decode(&body)!=nil{t.Error("bad json")}
  if body["model"]!="openai/gpt-oss-20b"{t.Errorf("unexpected model %v",body["model"])}
  w.Header().Set("Content-Type","application/json")
  w.Write([]byte(`{"choices":[{"message":{"content":"Привет!"}}]}`))
 }));defer server.Close()
 catalog,_:=newCatalog();spec:=catalog.Specs["groq"];spec.Endpoint=server.URL;catalog.Specs["groq"]=spec
 ai:=newAI(catalog);ai.client=server.Client();ai.client.Timeout=5*time.Second
 cfg:=testCfg()
 out,err:=ai.generate(context.Background(),cfg,"groq:openai/gpt-oss-20b",[]Message{{Role:"user",Content:"привет"}})
 if err!=nil||out!="Привет!"{t.Fatalf("%q %v",out,err)}
 if _,err=ai.generate(context.Background(),cfg,"groq:paid-model",[]Message{{Role:"user",Content:"привет"}});err==nil{t.Fatal("disallowed paid model called")}
}
func TestWebTemplateDoesNotPrintSecret(t *testing.T){
 cfg:=testCfg()
 data:=webData{CSRF:"csrf",Allowed:[]string{cfg.DefaultModel},Labels:map[string]string{cfg.DefaultModel:"Groq"},Providers:[]webProvider{{ID:"groq",Name:"Groq",URL:"https://console.groq.com/keys",Enabled:true,Auto:false}}}
 tmpl,err:=template.New("web").Parse(adminHTML);if err!=nil{t.Fatal(err)}
 var out bytes.Buffer;if err=tmpl.Execute(&out,data);err!=nil{t.Fatal(err)}
 if strings.Contains(out.String(),"free"){t.Fatal("template contains token")}
 if !strings.Contains(out.String(),"csrf"){t.Fatal("CSRF missing")}
}
