package main

import (
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "time"
 _ "modernc.org/sqlite"
)

type Store struct {DB *sql.DB}
type VKMessage struct {
 PeerID int64 `json:"peer_id"`
 FromID int64 `json:"from_id"`
 ConversationMessageID int64 `json:"conversation_message_id"`
 ID int64 `json:"id"`
 Text string `json:"text"`
 Payload json.RawMessage `json:"payload,omitempty"`
 Out int `json:"out"`
 ReplyMessage *VKMessage `json:"reply_message,omitempty"`
}
func (m VKMessage) EventID()string {id:=m.ConversationMessageID;if id==0{id=m.ID};return fmt.Sprintf("vk:%d:%d",m.PeerID,id)}
func (m VKMessage) Valid()bool{return m.PeerID>0&&m.FromID>0&&(m.ConversationMessageID>0||m.ID>0)&&m.Text!=""&&m.Out==0}
type Turn struct{Sender int64;Text string}
type Pref struct{Model,Search,Style,Fallback string;Context int}
func openStore(path string)(*Store,error){
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return nil,err}
 db,err:=sql.Open("sqlite",path);if err!=nil{return nil,err};db.SetMaxOpenConns(1);db.SetMaxIdleConns(1)
 schema:=[]string{
  "PRAGMA journal_mode=WAL",
  "PRAGMA busy_timeout=15000",
  "CREATE TABLE IF NOT EXISTS messages(id INTEGER PRIMARY KEY AUTOINCREMENT,peer_id INTEGER NOT NULL,sender_id INTEGER NOT NULL,event_id TEXT NOT NULL UNIQUE,text TEXT NOT NULL,created_at INTEGER NOT NULL)",
  "CREATE INDEX IF NOT EXISTS idx_peer ON messages(peer_id,id)",
  "CREATE TABLE IF NOT EXISTS calls(id INTEGER PRIMARY KEY AUTOINCREMENT,created_at INTEGER NOT NULL)",
  "CREATE TABLE IF NOT EXISTS preferences(scope TEXT NOT NULL,scope_id INTEGER NOT NULL,model TEXT,search TEXT,style TEXT,context_messages INTEGER,PRIMARY KEY(scope,scope_id))",
  "CREATE TABLE IF NOT EXISTS pending(event_id TEXT PRIMARY KEY,payload TEXT NOT NULL,created_at INTEGER NOT NULL,attempts INTEGER NOT NULL DEFAULT 0)",
  "CREATE INDEX IF NOT EXISTS idx_pending_created ON pending(created_at,event_id)",
  "CREATE TABLE IF NOT EXISTS meta(name TEXT PRIMARY KEY,value TEXT NOT NULL,updated_at INTEGER NOT NULL)",
 }
 for _,query:=range schema{if _,err=db.Exec(query);err!=nil{db.Close();return nil,fmt.Errorf("SQLite: %w",err)}}
 for _,table:=range []struct{table,column,definition string}{{"calls","peer_id","INTEGER"},{"calls","sender_id","INTEGER"},{"preferences","fallback","TEXT"},{"pending","response_text","TEXT"},{"pending","retry_after","INTEGER NOT NULL DEFAULT 0"}}{
  rows,err:=db.Query("PRAGMA table_info("+table.table+")");if err!=nil{db.Close();return nil,err}
  exists:=false;for rows.Next(){var index,notnull,pk int;var name,kind string;var d sql.NullString;if err=rows.Scan(&index,&name,&kind,&notnull,&d,&pk);err!=nil{break};if name==table.column{exists=true}}
  rows.Close();if err!=nil{db.Close();return nil,err}
  if !exists{if _,err=db.Exec("ALTER TABLE "+table.table+" ADD COLUMN "+table.column+" "+table.definition);err!=nil{db.Close();return nil,err}}
 }
 _,err=db.Exec("CREATE INDEX IF NOT EXISTS idx_calls_at ON calls(created_at)");if err!=nil{db.Close();return nil,err}
 return &Store{db},nil
}
func (s *Store) close(){s.DB.Close()}
func (s *Store) record(m VKMessage)(bool,error){return s.recordAndQueue(m,false)}
// recordAndQueue persists the message and a pending AI job atomically.
// Never advance VK's cursor for an event before both records are committed.
func (s *Store) recordAndQueue(m VKMessage,mentioned bool)(bool,error){
 if !m.Valid(){return false,nil}
 txt:=[]rune(m.Text);if len(txt)>3000{txt=txt[:3000]}
 tx,err:=s.DB.Begin();if err!=nil{return false,err}
 defer tx.Rollback()
 result,err:=tx.Exec("INSERT OR IGNORE INTO messages(peer_id,sender_id,event_id,text,created_at) VALUES(?,?,?,?,?)",
  m.PeerID,m.FromID,m.EventID(),string(txt),time.Now().Unix())
 if err!=nil{return false,err}
 inserted,_:=result.RowsAffected()
 if mentioned&&inserted==1{
  // Do not persist unbounded VK payloads: the durable queue contains the
  // same clipped text as the history. This also bounds downstream AI prompts.
  m.Text=string(txt)
  if m.ReplyMessage!=nil{
   m.ReplyMessage=&VKMessage{FromID:m.ReplyMessage.FromID}
  }
  payload,err:=json.Marshal(m);if err!=nil{return false,err}
  if _,err=tx.Exec("INSERT OR IGNORE INTO pending(event_id,payload,created_at,attempts) VALUES(?,?,?,0)",
   m.EventID(),string(payload),time.Now().Unix());err!=nil{return false,err}
 }
 if err=tx.Commit();err!=nil{return false,err}
 return inserted==1,nil
}
func (s *Store) answer(m VKMessage,text string,groupID int64)error{
 chars:=[]rune(text);if len(chars)>3000{text=string(chars[:3000])}
 _,err:=s.DB.Exec("INSERT OR IGNORE INTO messages(peer_id,sender_id,event_id,text,created_at) VALUES(?,?,?,?,?)",
 m.PeerID,-groupID,"bot:"+m.EventID(),text,time.Now().Unix())
 return err
}
func (s *Store) recent(peer int64,n,hours int)[]Turn{
 rows,err:=s.DB.Query("SELECT sender_id,text FROM messages WHERE peer_id=? AND created_at>=? ORDER BY id DESC LIMIT ?",peer,time.Now().Add(-time.Duration(hours)*time.Hour).Unix(),min(100,max(1,n)))
 if err!=nil{return nil};defer rows.Close()
 turns:=[]Turn{};for rows.Next(){var t Turn;if rows.Scan(&t.Sender,&t.Text)==nil{turns=append(turns,t)}}
 for i,j:=0,len(turns)-1;i<j;i,j=i+1,j-1{turns[i],turns[j]=turns[j],turns[i]}
 return turns
}
// Counts in one indexed scan; on SQLite errors fail closed, never overspend quota.
func (s *Store) counts(peer,sender int64)(int,int,int){
 cutoff:=time.Now().Add(-time.Hour).Unix()
 var total,chat,user int
 err:=s.DB.QueryRow(`SELECT COUNT(*),
  COALESCE(SUM(CASE WHEN peer_id=? THEN 1 ELSE 0 END),0),
  COALESCE(SUM(CASE WHEN sender_id=? THEN 1 ELSE 0 END),0)
  FROM calls WHERE created_at>=?`,peer,sender,cutoff).Scan(&total,&chat,&user)
 if err!=nil{return int(^uint(0)>>1),int(^uint(0)>>1),int(^uint(0)>>1)}
 return total,chat,user
}
func(s *Store) track(peer,sender int64)error{
 _,err:=s.DB.Exec("INSERT INTO calls(created_at,peer_id,sender_id) VALUES(?,?,?)",time.Now().Unix(),peer,sender)
 return err
}
func(s *Store) getPref(scope string,id int64)Pref{
 var a,b,c,d sql.NullString;var n sql.NullInt64
 err:=s.DB.QueryRow("SELECT model,search,style,context_messages,fallback FROM preferences WHERE scope=? AND scope_id=?",scope,id).Scan(&a,&b,&c,&n,&d)
 if err!=nil{return Pref{}};return Pref{a.String,b.String,c.String,d.String,int(n.Int64)}
}
func(s *Store) setPref(scope string,id int64,field string,value any)error{
 if scope!="chat"&&scope!="user"{return errors.New("неверная область")}
 allowed:=map[string]bool{"model":true,"search":true,"style":true,"fallback":true,"context_messages":true}
 if !allowed[field]{return errors.New("неверное поле")}
 _,err:=s.DB.Exec("INSERT OR IGNORE INTO preferences(scope,scope_id) VALUES(?,?)",scope,id);if err!=nil{return err}
 _,err=s.DB.Exec("UPDATE preferences SET "+field+"=? WHERE scope=? AND scope_id=?",value,scope,id);return err
}
func(s *Store) resetPref(scope string,id int64)error{_,err:=s.DB.Exec("DELETE FROM preferences WHERE scope=? AND scope_id=?",scope,id);return err}
func choose(value ...string)string{for _,v:=range value{if v!=""{return v}};return ""}
func(s *Store) effective(cfg Config,peer,sender int64,c *Catalog)(string,string,string,int,bool){
 allowed:=c.enabled(cfg);user:=s.getPref("user",sender);chat:=Pref{};if peer>=2000000000{chat=s.getPref("chat",peer)}
 model:=c.defaultModel(cfg)
 for _,candidate:=range []string{chat.Model,user.Model}{for _,k:=range allowed{if k==candidate{model=k}}}
 search:=choose(user.Search,chat.Search,"auto")
 style:=choose(user.Style,chat.Style,"normal")
 count:=cfg.ContextMessages;if chat.Context>0{count=min(count,chat.Context)};if user.Context>0{count=min(cfg.ContextMessages,user.Context)}
 fallback:=cfg.FreeFallback&&choose(user.Fallback,chat.Fallback,"on")!="off"
 return model,search,style,count,fallback
}
func(s *Store) enqueue(m VKMessage)error{
 blob,_:=json.Marshal(m)
 _,err:=s.DB.Exec("INSERT OR IGNORE INTO pending(event_id,payload,created_at,attempts) VALUES(?,?,?,0)",m.EventID(),string(blob),time.Now().Unix());return err
}
func(s *Store) nextPending()(VKMessage,int,error){
 var msg VKMessage;var id,data string;var attempts int
 err:=s.DB.QueryRow("SELECT event_id,payload,attempts FROM pending WHERE retry_after<=? ORDER BY created_at,event_id LIMIT 1",
  time.Now().Unix()).Scan(&id,&data,&attempts)
 if err!=nil{return msg,0,err}
 if err=json.Unmarshal([]byte(data),&msg);err!=nil||!msg.Valid()||msg.EventID()!=id{
  // Malformed persisted jobs must not block all subsequent VK messages.
  // Do not log their untrusted content (potentially private chat history).
  if _,removeErr:=s.DB.Exec("DELETE FROM pending WHERE event_id=?",id);removeErr!=nil{
   return msg,0,removeErr
  }
  return msg,0,sql.ErrNoRows
 }
 return msg,attempts,nil
}
func(s *Store) finish(m VKMessage)error{
 _,err:=s.DB.Exec("DELETE FROM pending WHERE event_id=?",m.EventID())
 return err
}
func(s *Store) retry(m VKMessage)error{
 // Gradual backoff stops transient network failures from blocking every chat.
 // Pending messages remain retriable until the configured retention limit.
 _,err:=s.DB.Exec("UPDATE pending SET attempts=attempts+1, retry_after=? WHERE event_id=?",
  time.Now().Add(retryBackoff(s.attempts(m)+1)).Unix(),m.EventID())
 return err
}
func(s *Store) attempts(m VKMessage)int{
 var n int;_ =s.DB.QueryRow("SELECT attempts FROM pending WHERE event_id=?",m.EventID()).Scan(&n)
 return n
}
func retryBackoff(attempt int)time.Duration{
 if attempt<1{attempt=1};if attempt>8{attempt=8}
 return time.Duration(1<<uint(attempt))*time.Second
}
func(s *Store) prepared(m VKMessage)(string,bool,error){
 var value sql.NullString
 err:=s.DB.QueryRow("SELECT response_text FROM pending WHERE event_id=?",m.EventID()).Scan(&value)
 if err!=nil{return "",false,err}
 return value.String,value.Valid,nil
}
func(s *Store) prepare(m VKMessage,response string)error{
 result,err:=s.DB.Exec("UPDATE pending SET response_text=? WHERE event_id=?",
  response,m.EventID())
 if err!=nil{return err}
 n,err:=result.RowsAffected();if err!=nil{return err}
 if n!=1{return sql.ErrNoRows}
 return nil
}
func(s *Store) cursor(group int64)string{
 var value string;var updated int64
 err:=s.DB.QueryRow("SELECT value,updated_at FROM meta WHERE name=?",fmt.Sprintf("vk_cursor:%d",group)).Scan(&value,&updated)
 if err!=nil||time.Since(time.Unix(updated,0))>2*time.Hour{return ""};return value
}
func(s *Store) setCursor(group int64,value string){_,_=s.DB.Exec("INSERT INTO meta(name,value,updated_at) VALUES(?,?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at",fmt.Sprintf("vk_cursor:%d",group),value,time.Now().Unix())}
func(s *Store) clearCursor(group int64){_,_=s.DB.Exec("DELETE FROM meta WHERE name=?",fmt.Sprintf("vk_cursor:%d",group))}
func(s *Store) cleanup(days int){
 _,_=s.DB.Exec("DELETE FROM messages WHERE created_at<?",time.Now().Add(-time.Duration(days)*24*time.Hour).Unix())
 _,_=s.DB.Exec("DELETE FROM pending WHERE created_at<?",time.Now().Add(-time.Duration(days)*24*time.Hour).Unix())
 _,_=s.DB.Exec("DELETE FROM calls WHERE created_at<?",time.Now().Add(-24*time.Hour).Unix())
}
// Clearing the history also removes queued copies of private user messages.
func(s *Store) clearMessages()error{
 tx,err:=s.DB.Begin();if err!=nil{return err}
 defer tx.Rollback()
 if _,err=tx.Exec("DELETE FROM pending");err!=nil{return err}
 if _,err=tx.Exec("DELETE FROM messages");err!=nil{return err}
 return tx.Commit()
}
