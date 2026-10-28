package main

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "sync/atomic"
 "testing"
 "time"
)

func TestFreeCatalogRetryPolicy(t *testing.T) {
 if catalogRetryDelay(nil,5)!=12*time.Hour{t.Fatal("normal interval changed")}
 if catalogRetryDelay(map[string]string{"pollinations":"network"},5)!=10*time.Minute{t.Fatal("errors should retry promptly")}
 if catalogRetryDelay(nil,0)!=10*time.Minute{t.Fatal("missing model should retry promptly")}
}

func TestFreeCatalogDiscoveryConcurrent(t *testing.T){
 var active atomic.Int32
 var peak atomic.Int32
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  n:=active.Add(1)
  for {
   top:=peak.Load()
   if n<=top||peak.CompareAndSwap(top,n){break}
  }
  time.Sleep(65*time.Millisecond)
  active.Add(-1)
  fmt.Fprint(w,`{"data":[{"id":"example/free:free","pricing":{"prompt":"0","completion":"0","input":"0","output":"0"}}]}`)
 }))
 defer server.Close()
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 for _,key:=range []string{"openrouter","pollinations"}{
  spec:=c.Specs[key];spec.CatalogURL=server.URL;c.Specs[key]=spec
 }
 cfg:=defaults()
 cfg.Providers=map[string]ProviderConfig{
  "openrouter":{Enabled:true,APIKey:"free"},
  "pollinations":{Enabled:true,APIKey:"free"},
 }
 cfg.AutoModelsProviders=[]string{"openrouter","pollinations"}
 if failures:=c.refresh(context.Background(),cfg);len(failures)>0{t.Fatalf("catalog errors: %+v",failures)}
 if peak.Load()<2{t.Fatalf("discovery ran serially; peak %d",peak.Load())}
 if len(c.enabled(cfg))<2{t.Fatal("free models missing after refresh")}
}
