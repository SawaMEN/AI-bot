package main

import (
 "bytes"
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestAIEndpointRoutes(t *testing.T){
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 if aiProviderEndpoint(c.Specs["gemini"])!="https://generativelanguage.googleapis.com/"{t.Fatal("wrong Gemini hostname")}
 if aiProviderEndpoint(c.Specs["cloudflare"])!="https://api.cloudflare.com/"{t.Fatal("wrong Cloudflare hostname")}
 if !strings.Contains(aiProviderEndpoint(c.Specs["groq"]),"api.groq.com"){t.Fatal("wrong Groq hostname")}
}
func TestDirectProbeRejectsInvalidSchemeAndUserInfo(t *testing.T){
 for _,endpoint:=range []string{"http://127.0.0.1/","https://name:password@example.com/"}{
  if _,err:=probeDirectHTTPS(context.Background(),endpoint);err==nil{t.Fatal("insecure endpoint accepted: ",endpoint)}
 }
}
func TestDirectProbeRejectsInvalidCertificate(t *testing.T){
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){}))
 defer server.Close()
 stages,err:=probeDirectHTTPS(context.Background(),server.URL)
 if err==nil||!strings.Contains(err.Error(),"TLS/сертификат"){t.Fatalf("bad certificate not rejected: %s %v",stages,err)}
 if !strings.Contains(stages,"TCP OK"){t.Fatalf("TCP stage not reached: %s",stages)}
}
func TestServerAIConnectivityUsesRealModelProbe(t *testing.T){
 cfg:=testCfg();cfg.SearchEnabled=false
 c,_:=newCatalog()
 dnsCount,apiCount:=0,0
 var output bytes.Buffer
 err:=checkAIConnectivityWith(context.Background(),cfg,c,&output,false,
  func(_ context.Context,endpoint string)(string,error){
   dnsCount++
   if !strings.HasPrefix(endpoint,"https://"){t.Errorf("bad endpoint %s",endpoint)}
   return "DNS OK, TCP OK, TLS OK",nil
  },
  func(_ context.Context,_ Config,key string,messages []Message)(string,error){
   apiCount++
   if !strings.Contains(key,":")||len(messages)!=2{t.Errorf("invalid model request %s",key)}
   return "Столица России — Москва.",nil
  })
 if err!=nil{t.Fatal(err)}
 if dnsCount!=2||apiCount!=2{t.Fatalf("got %d network checks, %d API calls",dnsCount,apiCount)}
 if !strings.Contains(output.String(),"успешно 2"){t.Fatal(output.String())}
}
func TestServerAINetworkOnlyDoesNotSpendQuota(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 var out bytes.Buffer
 apiCalled:=false
 err:=checkAIConnectivityWith(context.Background(),cfg,c,&out,true,
  func(context.Context,string)(string,error){return "DNS OK, TCP OK, TLS OK",nil},
  func(context.Context,Config,string,[]Message)(string,error){apiCalled=true;return "",nil})
 if err!=nil||apiCalled{t.Fatalf("network-only called AI provider: %v, %v",apiCalled,err)}
 if !strings.Contains(out.String(),"без запросов к ИИ"){t.Fatal(out.String())}
}
func TestServerAIAttemptsHTTPViaProxyWhenDirectNetworkFails(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 calls:=0
 var out bytes.Buffer
 err:=checkAIConnectivityWith(context.Background(),cfg,c,&out,false,
  func(context.Context,string)(string,error){return "DNS OK",errors.New("TCP: connection refused")},
  func(context.Context,Config,string,[]Message)(string,error){calls++;return "Столица России — Москва.",nil})
 if err!=nil||calls!=2||!strings.Contains(out.String(),"TCP: connection refused")||
  !strings.Contains(out.String(),"несмотря на ошибку прямого соединения"){
  t.Fatalf("proxy API not tested: %v %d %s",err,calls,out.String())
 }
}
func TestServerAIReportsAuthenticationAndLanguageSeparately(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 for _,tc:=range []struct{answer string;apiErr error;want string}{
  {"",errors.New("ключ отклонён (HTTP 401)"),"Ошибка API/ключа/квоты"},
  {"The capital city is Moscow.",nil,"проверка русского языка не прошла"},
 }{
  var out bytes.Buffer
  err:=checkAIConnectivityWith(context.Background(),cfg,c,&out,false,
   func(context.Context,string)(string,error){return "DNS OK, TCP OK, TLS OK",nil},
   func(context.Context,Config,string,[]Message)(string,error){return tc.answer,tc.apiErr})
  if err==nil||!strings.Contains(out.String(),tc.want){t.Fatalf("expected %q, got %v %s",tc.want,err,out.String())}
 }
}
func TestServerAINoEnabledProviders(t *testing.T){
 cfg:=defaults();c,_:=newCatalog()
 err:=checkAIConnectivityWith(context.Background(),cfg,c,&bytes.Buffer{},true,
  func(context.Context,string)(string,error){t.Fatal("unexpected network request");return "",nil},
  nil)
 if err==nil{t.Fatal("no providers should be reported")}
}
