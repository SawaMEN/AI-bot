package main

import (
 "context"
 "testing"
 "net/http"
 "net/http/httptest"
 "encoding/json"
)
func TestRegistryFreeOnly(t *testing.T) {
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 if len(c.Specs)<10{t.Fatal("provider registry incomplete")}
 for _,bad:=range []string{"openrouter:gpt-4o","openai:gpt-4o","groq:paid-model"} {
  p,id,e:=splitModel(bad);if e==nil&&c.modelAllowedID(p,id){t.Errorf("paid model allowed %s",bad)}
 }
}
func TestCatalogPrice(t *testing.T) {
 c,_:=newCatalog();openrouter:=c.Specs["openrouter"]
 cases:=[]struct{data string;ok bool}{
  {`{"id":"free/abc:free","pricing":{"prompt":"0","completion":"0"}}`,true},
  {`{"id":"paid/abc:free","pricing":{"prompt":"0","completion":"0.1"}}`,false},
  {`{"id":"unknown/abc:free","pricing":{"prompt":"0"}}`,false},
 }
 for _,tc:=range cases{item:=map[string]any{};_ =json.Unmarshal([]byte(tc.data),&item);if freeItem(openrouter,item)!=tc.ok{t.Error(tc.data)}}
 p:=c.Specs["pollinations"]
 if freeItem(p,map[string]any{"id":"openai/gpt","pricing":map[string]any{"input":0.0,"output":0.1}}){t.Fatal("paid Pollinations allowed")}
}
func TestDynamicDiscoveryFailClosed(t *testing.T){
 c,_:=newCatalog()
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Write([]byte(`{"data":[{"id":"a/model:free","pricing":{"prompt":"0","completion":"0"}},{"id":"b/model:free","pricing":{"prompt":"0.1","completion":"0"}}]}`))
 }));defer srv.Close()
 p:=c.Specs["openrouter"];p.CatalogURL=srv.URL;c.Specs["openrouter"]=p
 // HTTP is used only inside mock TLS-free catalogue test; runtime registry is HTTPS.
 list,err:=c.discover(context.Background(),p,"test");if err!=nil{t.Fatal(err)}
 if len(list)!=1||list[0].ID!="a/model:free"{t.Fatalf("unexpected discovered: %+v",list)}
}
