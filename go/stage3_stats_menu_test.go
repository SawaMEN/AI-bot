package main

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
 "time"
)

func TestAdminStatsOnlyAggregatesNoSecrets(t *testing.T){
 s,err:=openStore(filepath.Join(t.TempDir(),"stat.db"));if err!=nil{t.Fatal(err)}
 defer s.close()
 cfg:=testCfg();c,_:=newCatalog()
 for i:=0;i<3;i++{
  ok,err:=s.reserveQuota(cfg,42,42);if err!=nil||!ok{t.Fatalf("quota: %v %v",ok,err)}
 }
 s.recordModelAttempt("groq:openai/gpt-oss-20b",true,500*time.Millisecond)
 s.recordModelAttempt("groq:openai/gpt-oss-20b",false,2*time.Second)
 result:=s.vkUsageStats(cfg,c)
 if !strings.Contains(result,"Запросов к ИИ: 3")||!strings.Contains(result,"Успешных ответов ИИ: 50%"){
  t.Fatalf("unexpected public stats: %s",result)
 }
 if strings.Contains(result,"api_key")||strings.Contains(result,"http 401")||strings.Contains(result,"secret"){
  t.Fatal("technical details leaked to VK")
 }
 detailed:=s.consoleUsageStats(cfg,c)
 if !strings.Contains(detailed,"GPT-OSS 20B")||!strings.Contains(detailed,"2 запросов"){t.Fatal("no model details in local admin CLI")}
}

func TestVKStatsRejectNonAdminAndAllowAdmin(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 s,err:=openStore(filepath.Join(t.TempDir(),"admin-stat.db"));if err!=nil{t.Fatal(err)}
 defer s.close()
 var isAdmin atomic.Bool
 api:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/method/messages.getConversationMembers"{t.Errorf("unexpected VK API call %s",r.URL.Path)}
  fmt.Fprintf(w,`{"response":{"items":[{"member_id":42,"is_admin":%t}]}}`,isAdmin.Load())
 }))
 defer api.Close()
 prev:=vkAPIBaseURL;vkAPIBaseURL=api.URL+"/method/"
 defer func(){vkAPIBaseURL=prev}()
 v:=newVK(cfg)
 peer,user:=int64(2000000001),int64(42)
 result,handled:=processCommand(context.Background(),v,s,c,cfg,peer,user,"статистика")
 if !handled||!strings.Contains(result,"администратор"){t.Fatalf("public stats leaked: %q",result)}
 isAdmin.Store(true)
 result,handled=processCommand(context.Background(),v,s,c,cfg,peer,user,"статистика")
 if !handled||!strings.Contains(result,"СТАТИСТИКА БОТА"){t.Fatalf("admin stats not shown: %q",result)}
}

func TestVKBotsPreserveLastOpenSectionAfterAIReply(t *testing.T){
 path:=filepath.Join(t.TempDir(),"ui.db")
 s,err:=openStore(path);if err!=nil{t.Fatal(err)}
 cfg:=testCfg();c,_:=newCatalog();bot:=newBot(cfg,s,c)
 s.rememberMenu(42,"выбор поиска")
 if !strings.Contains(bot.lastKeyboard(42),"Искать"){t.Fatal("search selector reset to home")}
 s.rememberMenu(2000000001,"диалог выбор контекста")
 if !strings.Contains(bot.lastKeyboard(2000000001),"сообщений"){t.Fatal("group selector reset to main")}
 s.close()
 again,err:=openStore(path);if err!=nil{t.Fatal(err)}
 defer again.close()
 bot=newBot(cfg,again,c)
 if !strings.Contains(bot.lastKeyboard(42),"Искать"){t.Fatal("menu not durable across restarts")}
 if !strings.Contains(replyGroupMainBoard(),"Статистика"){t.Fatal("admin stats button missing")}
}

func TestVKChoiceIntroShowsCurrentSelection(t *testing.T){
 cfg:=testCfg();c,_:=newCatalog()
 s,err:=openStore(filepath.Join(t.TempDir(),"choices.db"));if err!=nil{t.Fatal(err)}
 defer s.close()
 if err:=s.setPref("user",42,"search","off");err!=nil{t.Fatal(err)}
 output:=choiceIntro(s,c,cfg,42,42,false,"поиска")
 if !strings.Contains(output,"Сейчас:")||!strings.Contains(output,"ВЫКЛ"){t.Fatalf("selected value hidden: %s",output)}
}
