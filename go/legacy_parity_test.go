package main

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func testConfigFile(t *testing.T, data string)string{
 t.Helper()
 path:=filepath.Join(t.TempDir(),"config.json")
 t.Setenv("AIBOT_CONFIG",path)
 if err:=os.WriteFile(path,[]byte(data),0640);err!=nil{t.Fatal(err)}
 return path
}

func TestLegacyPythonV1Migration(t *testing.T){
 original:=`{"vk_token":"vk-old","group_id":987,"api_key":"legacy-secret","provider":"groq","model":"openai/gpt-oss-20b","context_messages":20,"context_hours":24,"retention_days":7,"max_requests_hour":50,"max_user_requests_hour":12,"max_chat_requests_hour":25}`
 path:=testConfigFile(t,original)
 cfg,err:=loadConfig();if err!=nil{t.Fatal(err)}
 if cfg.VKToken!="vk-old"||cfg.GroupID!=987{t.Fatalf("VK lost: %+v",cfg)}
 if !cfg.Providers["groq"].Enabled||cfg.Providers["groq"].APIKey!="legacy-secret"{t.Fatalf("Legacy key lost: %+v",cfg.Providers)}
 if cfg.DefaultModel!="groq:openai/gpt-oss-20b"||len(cfg.EnabledModels)!=1{t.Fatal("Legacy model not selected")}
 if err:=saveConfig(cfg);err!=nil{t.Fatal(err)}
 fresh,err:=loadConfig();if err!=nil||fresh.Providers["groq"].APIKey!="legacy-secret"{t.Fatal("Key lost on save",err)}
 old,err:=os.ReadFile(path+".python-v1.bak");if err!=nil||string(old)!=original{t.Fatalf("backup missing: %v",err)}
 info,err:=os.Stat(path);if err!=nil{t.Fatal(err)}
 if info.Mode().Perm()&0007!=0{t.Fatalf("config world accessible: %v",info.Mode())}
}

func TestLegacyPaidCustomDisabled(t *testing.T){
 testConfigFile(t,`{"vk_token":"old","group_id":25,"provider":"custom","api_key":"secret","model":"premium/paid","context_messages":30,"context_hours":24,"retention_days":7,"max_requests_hour":50,"max_user_requests_hour":12,"max_chat_requests_hour":25}`)
 cfg,err:=loadConfig();if err!=nil{t.Fatal(err)}
 if len(cfg.Providers)!=0||len(cfg.EnabledModels)!=0{t.Fatal("Paid custom provider migrated")}
}
func TestVKAutoSettingsSuccessAndDenial(t *testing.T){
 calls:=0
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  calls++
  if r.URL.Path!="/groups.setLongPollSettings"{t.Errorf("method: %s",r.URL.Path)}
  if err:=r.ParseForm();err!=nil{t.Error(err)}
  for _,key:=range []string{"group_id","enabled","message_new","message_event"}{
   if r.Form.Get(key)==""{t.Errorf("Missing VK field %s",key)}
  }
  fmt.Fprint(w,`{"response":1}`)
 }))
 defer server.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/";defer func(){vkAPIBaseURL=old}()
 cfg:=defaults();cfg.GroupID=123;cfg.VKToken="secret"
 v:=newVK(cfg)
 if err:=v.setLongPollEvents(context.Background());err!=nil{t.Fatal(err)}
 if calls!=1{t.Fatal("Expected one VK settings request")}
}
func TestVKFlagsAcrossJSONVersions(t *testing.T){
 for _,tc:=range []struct{raw string;want bool}{
  {`1`,true},{`true`,true},{`"1"`,true},{`0`,false},{`false`,false},{`null`,false},
 }{
  if actual:=vkEnabled(json.RawMessage(tc.raw));actual!=tc.want{t.Errorf("%s => %t",tc.raw,actual)}
 }
}
func TestVKDeniedPermissionSettings(t *testing.T){
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  fmt.Fprint(w,`{"error":{"error_msg":"Access denied"}}`)
 }))
 defer server.Close()
 old:=vkAPIBaseURL;vkAPIBaseURL=server.URL+"/";defer func(){vkAPIBaseURL=old}()
 v:=newVK(Config{VKToken:"key",GroupID:42})
 if err:=v.setLongPollEvents(context.Background());err==nil||!strings.Contains(err.Error(),"Access denied"){t.Fatal("VK denial ignored")}
}
func TestVKApprovalRequiredForLocalMenu(t *testing.T){
 var out bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("нет\n")),out:&out}
 cfg:=Config{VKToken:"secret",GroupID:42}
 w.configureVKEvents(cfg)
 if !strings.Contains(out.String(),"отменены"){t.Fatal("unexpected acceptance")}
}

func TestDuckResultAttributeOrderAndUnsafeLinks(t *testing.T){
 htmlFixture:=`<div><a href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.org%2Ftest" rel="nofollow" class="result__a">Хороший <b>результат</b></a><span class="result__snippet">Описание &amp; текст</span></div>
 <a href="javascript:alert(1)" class="result__a">Опасный</a>`
 results:=parseDuckHTML([]byte(htmlFixture))
 if len(results)!=1{t.Fatalf("got %d results",len(results))}
 if results[0].URL!="https://example.org/test"||results[0].Body!="Описание & текст"{t.Fatalf("bad result %+v",results[0])}
}
func TestSearchFallsBackToWikipedia(t *testing.T){
 duck:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(429)}));defer duck.Close()
 wiki:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Query().Get("srsearch")!="Проверь Go"{t.Error("Query lost")}
  fmt.Fprint(w,`{"query":{"search":[{"title":"Язык программирования Go","snippet":"Статья <b>Go</b>"}]}}`)
 }));defer wiki.Close()
 oldDuck,oldWiki:=duckEndpoint,wikiEndpoint
 duckEndpoint=duck.URL;wikiEndpoint=wiki.URL
 defer func(){duckEndpoint=oldDuck;wikiEndpoint=oldWiki}()
 res:=webSearch(context.Background(),"Проверь Go")
 if len(res)!=1||!strings.Contains(res[0].URL,"wikipedia.org/wiki/")||res[0].Body!="Статья Go"{t.Fatalf("Fallback failed: %+v",res)}
 if url.PathEscape("Go")!="Go"{t.Fatal("bad test")}
}
