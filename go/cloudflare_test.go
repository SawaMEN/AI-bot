package main

import (
 "context"
 "io"
 "net/http"
 "strings"
 "testing"
)

type cloudflareRoundTripper func(*http.Request)(*http.Response,error)
func (fn cloudflareRoundTripper)RoundTrip(r *http.Request)(*http.Response,error){return fn(r)}

func TestCuratedRegistryModelIDsAreSelectable(t *testing.T){
 catalog,err:=newCatalog();if err!=nil{t.Fatal(err)}
 for _,name:=range catalog.Order{
  spec:=catalog.Specs[name]
  if spec.FreePolicy!="curated"{continue}
  cfg:=defaults()
  cfg.Providers[name]=ProviderConfig{Enabled:true,APIKey:"fake",CloudflareAccount:"test-account"}
  cfg.AutoModelsProviders=[]string{name}
  allowed:=catalog.enabled(cfg)
  if len(allowed)!=len(spec.Models){t.Errorf("%s: %d curated models, but %d selectable: %v",name,len(spec.Models),len(allowed),allowed)}
  for _,model:=range spec.Models{
   if !catalog.modelAllowedID(name,model[0]){t.Errorf("%s model ID rejected: %s",name,model[0])}
  }
 }
}
func TestCloudflareModelPrefixIsValidAndUnsafeIDsAreRejected(t *testing.T){
 for _,valid:=range []string{"@cf/zai-org/glm-4.7-flash","@cf/google/gemma-4-26b-a4b-it","Qwen/Qwen3-8B"}{
  if !safeModelID.MatchString(valid){t.Errorf("valid model rejected: %q",valid)}
 }
 for _,invalid:=range []string{"@", "@/bad","model?key=secret","a model","https://host/model#fragment","<script>",""}{
  if safeModelID.MatchString(invalid){t.Errorf("unsafe model accepted: %q",invalid)}
 }
}
func TestCloudflareFreeModelGeneratesThroughCorrectEndpoint(t *testing.T){
 catalog,err:=newCatalog();if err!=nil{t.Fatal(err)}
 cfg:=defaults()
 cfg.Providers["cloudflare"]=ProviderConfig{Enabled:true,APIKey:"mock-secret",CloudflareAccount:"account-123"}
 cfg.EnabledModels=[]string{"cloudflare:@cf/zai-org/glm-4.7-flash"}
 if len(catalog.enabled(cfg))!=1{t.Fatal("Cloudflare model cannot be selected")}
 ai:=newAI(catalog)
 calls:=0
 ai.client=&http.Client{Transport:cloudflareRoundTripper(func(r *http.Request)(*http.Response,error){
  calls++
  if r.URL.Host!="api.cloudflare.com"||!strings.Contains(r.URL.EscapedPath(),"/ai/run/@cf/zai-org/glm-4.7-flash") {
   t.Errorf("wrong endpoint: %s",r.URL.String())
  }
  if r.Header.Get("Authorization")!="Bearer mock-secret"{t.Errorf("missing API auth")}
  return &http.Response{StatusCode:200,Body:io.NopCloser(strings.NewReader(`{"result":{"response":"Готово"}}`)),Header:http.Header{},Request:r},nil
 })}
 out,err:=ai.generate(context.Background(),cfg,"cloudflare:@cf/zai-org/glm-4.7-flash",[]Message{{Role:"user",Content:"Тест"}})
 if err!=nil||out!="Готово"||calls!=1{t.Fatalf("Cloudflare generated %q: %v (calls %d)",out,err,calls)}
}
