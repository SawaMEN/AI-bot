package main

import (
 "database/sql"
 "path/filepath"
 "testing"
 "os"
)
func TestSQLitePythonMigration(t *testing.T){
 path:=filepath.Join(t.TempDir(),"history.db")
 old,err:=sql.Open("sqlite",path);if err!=nil{t.Fatal(err)}
 queries:=[]string{
 "CREATE TABLE calls(id INTEGER PRIMARY KEY AUTOINCREMENT,created_at INTEGER NOT NULL)",
 "CREATE TABLE preferences(scope TEXT NOT NULL,scope_id INTEGER NOT NULL,model TEXT,search TEXT,style TEXT,context_messages INTEGER,PRIMARY KEY(scope,scope_id))",
 "INSERT INTO preferences(scope,scope_id,model) VALUES('user',123,'groq:openai/gpt-oss-20b')",
 }
 for _,q:=range queries{if _,err=old.Exec(q);err!=nil{t.Fatal(err)}}
 old.Close()
 store,err:=openStore(path);if err!=nil{t.Fatal(err)};defer store.close()
 pref:=store.getPref("user",123);if pref.Model!="groq:openai/gpt-oss-20b"{t.Fatalf("pref lost %+v",pref)}
 if err=store.setPref("user",123,"fallback","off");err!=nil{t.Fatal(err)}
 if store.getPref("user",123).Fallback!="off"{t.Fatal("fallback migration failed")}
 msg:=VKMessage{PeerID:2000000001,FromID:123,ConversationMessageID:1,Text:"Здравствуйте"}
 ok,err:=store.record(msg);if err!=nil||!ok{t.Fatal(err)}
 ok,err=store.record(msg);if err!=nil||ok{t.Fatal("duplicate")}
 if err=store.enqueue(msg);err!=nil{t.Fatal(err)}
 pending,_,err:=store.nextPending();if err!=nil||pending.Text!=msg.Text{t.Fatal(err)}
 store.finish(msg);store.setCursor(15,"ts-foo")
 if store.cursor(15)!="ts-foo"{t.Fatal("cursor not saved")}
}
func TestConfigMigration(t *testing.T){
 path:=filepath.Join(t.TempDir(),"config.json")
 os.Setenv("AIBOT_CONFIG",path);defer os.Unsetenv("AIBOT_CONFIG")
 raw:=[]byte(`{"vk_token":"test","group_id":1,"context_messages":30,"context_hours":24,"retention_days":7,"max_requests_hour":50,"max_user_requests_hour":12,"max_chat_requests_hour":25,"providers":{"gemini":{"api_key":"abc","enabled":true},"groq":{"api_key":"retired","enabled":true}},"enabled_models":["gemini:gemini-2.5-flash","groq:openai/gpt-oss-20b"]}`)
 if err:=os.WriteFile(path,raw,0640);err!=nil{t.Fatal(err)}
 cfg,err:=loadConfig();if err!=nil{t.Fatal(err)}
 if cfg.Providers["gemini"].APIKey!="abc"{t.Fatal("Gemini secret lost")}
 if _,old:=cfg.Providers["groq"];old{t.Fatal("retired Groq credentials still active")}
 if len(cfg.EnabledModels)!=1||cfg.EnabledModels[0]!="gemini:gemini-2.5-flash"{t.Fatal("Gemini model selection lost",cfg.EnabledModels)}
 if err=saveConfig(cfg);err!=nil{t.Fatal(err)}
}
