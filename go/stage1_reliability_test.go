package main

import (
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
)

func stagedMessage(peer,id int64)VKMessage{return VKMessage{PeerID:peer,FromID:peer,ID:id,Text:"Привет"}}

func TestConcurrentQueueClaimsDifferentPeersAndPreservesOrder(t *testing.T){
 store,err:=openStore(filepath.Join(t.TempDir(),"queue.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 a1,a2,b1:=stagedMessage(42,1),stagedMessage(42,2),stagedMessage(99,3)
 for _,msg:=range []VKMessage{a1,a2,b1}{
  if _,err:=store.recordAndQueue(msg,true);err!=nil{t.Fatal(err)}
  time.Sleep(3*time.Millisecond)
 }
 first,_,err:=store.claimPending();if err!=nil{t.Fatal(err)}
 if first.EventID()!=a1.EventID(){t.Fatalf("expected a1 first, got %s",first.EventID())}
 second,_,err:=store.claimPending();if err!=nil{t.Fatal(err)}
 if second.EventID()!=b1.EventID(){t.Fatalf("second must be from another peer, got %s",second.EventID())}
 if _,_,err:=store.claimPending();err==nil{t.Fatal("same peer received a concurrent claim")}
 if err:=store.finish(first);err!=nil{t.Fatal(err)}
 third,_,err:=store.claimPending();if err!=nil{t.Fatal(err)}
 if third.EventID()!=a2.EventID(){t.Fatalf("same-peer FIFO broken: %s",third.EventID())}
}
func TestAtomicFreeTierQuota(t *testing.T){
 store,err:=openStore(filepath.Join(t.TempDir(),"quota.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 cfg:=defaults();cfg.MaxRequestsHour=5;cfg.MaxChatRequestsHour=5;cfg.MaxUserRequestsHour=5
 var accepted int
 var mu sync.Mutex
 var wg sync.WaitGroup
 for n:=0;n<24;n++{
  wg.Add(1);go func(){
   defer wg.Done()
   ok,err:=store.reserveQuota(cfg,42,42)
   if err!=nil{t.Errorf("quota: %v",err);return}
   if ok{mu.Lock();accepted++;mu.Unlock()}
  }()
 }
 wg.Wait()
 if accepted!=5{t.Fatalf("quota overspent or underused: %d/5",accepted)}
}
func TestFailedQueueClaimRetryReleasesLease(t *testing.T){
 store,err:=openStore(filepath.Join(t.TempDir(),"retry.db"));if err!=nil{t.Fatal(err)}
 defer store.close()
 m:=stagedMessage(41,10)
 if err:=store.enqueue(m);err!=nil{t.Fatal(err)}
 got,_,err:=store.claimPending();if err!=nil||got.EventID()!=m.EventID(){t.Fatalf("claim: %v",err)}
 if err:=store.retry(m);err!=nil{t.Fatal(err)}
 var lease int64
 if err:=store.DB.QueryRow("SELECT lease_until FROM pending WHERE event_id=?",m.EventID()).Scan(&lease);err!=nil||lease!=0{t.Fatalf("retry left lease %d: %v",lease,err)}
}
func TestPrivateSQLiteAndConfigBackup(t *testing.T){
 root:=t.TempDir();path:=filepath.Join(root,"history.db")
 t.Setenv("AIBOT_DATA",path)
 t.Setenv("AIBOT_CONFIG",filepath.Join(root,"config.json"))
 if err:=os.WriteFile(configPath(),[]byte(`{"vk_token":"secret-key"}`),0600);err!=nil{t.Fatal(err)}
 store,err:=openStore(path);if err!=nil{t.Fatal(err)}
 defer store.close()
 if _,err:=store.recordAndQueue(stagedMessage(1,1),true);err!=nil{t.Fatal(err)}
 folder,err:=backupState(store);if err!=nil{t.Fatal(err)}
 backup,err:=os.ReadFile(filepath.Join(folder,"config.json"));if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(backup),"secret-key"){t.Fatal("config backup lost settings")}
 st,err:=os.Stat(filepath.Join(folder,"config.json"));if err!=nil||st.Mode().Perm()!=0600{t.Fatalf("config backup permission: %v %v",st,err)}
 database,err:=openStore(filepath.Join(folder,"history.db"));if err!=nil{t.Fatal(err)}
 defer database.close()
 var count int
 if err:=database.DB.QueryRow("SELECT count(*) FROM messages").Scan(&count);err!=nil||count!=1{t.Fatalf("backup invalid: %d %v",count,err)}
 entries,err:=os.ReadDir(filepath.Join(root,"backups"));if err!=nil||len(entries)!=1{t.Fatal(err)}
}
func TestBackgroundQuotaFailureCooldown(t *testing.T){
 cfg:=testCfg()
 health:=newModelHealth(filepath.Join(t.TempDir(),"health.json"))
 key:="groq:openai/gpt-oss-20b"
 if err:=health.recordReason(cfg,key,false,"rate_limit");err!=nil{t.Fatal(err)}
 if health.due(cfg,key){t.Fatal("rate limited provider retried immediately")}
 blob,err:=os.ReadFile(health.path);if err!=nil{t.Fatal(err)}
 statuses:=map[string]modelStatus{}
 if err:=json.Unmarshal(blob,&statuses);err!=nil{t.Fatal(err)}
 state:=statuses[key];state.CheckedAt=time.Now().Add(-5*time.Hour);statuses[key]=state
 updated,err:=json.Marshal(statuses);if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(health.path,updated,0600);err!=nil{t.Fatal(err)}
 if err:=health.reload(true);err!=nil{t.Fatal(err)}
 if !health.due(cfg,key){t.Fatal("rate-limited provider should eventually be rechecked")}
}
