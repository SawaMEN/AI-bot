package main

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "syscall"
 "time"
)

const (
 modelHealthyFor = 24*time.Hour
 modelRecheckAfter = 12*time.Hour
 modelFailureRetryAfter = 30*time.Minute
)

// This file contains only a credential fingerprint (never an API key).
// Results are tied to the current provider credentials and Cloudflare account.
type modelStatus struct {
 CheckedAt time.Time `json:"checked_at"`
 Fingerprint string `json:"fingerprint"`
 Passed bool `json:"passed"`
}
type ModelHealth struct {
 mu sync.RWMutex
 path string
 entries map[string]modelStatus
 lastRead time.Time
}

func modelHealthPath()string{return filepath.Join(filepath.Dir(dataPath()),"model-health.json")}

func modelFingerprint(cfg Config,key string)string {
 provider,_,_:=splitModel(key)
 p:=cfg.Providers[provider]
 h:=sha256.Sum256([]byte(key+"\x00"+p.APIKey+"\x00"+p.CloudflareAccount))
 return hex.EncodeToString(h[:])
}

func newModelHealth(path string)*ModelHealth {
 h:=&ModelHealth{path:path,entries:map[string]modelStatus{}}
 _=h.reload(true)
 return h
}
func (h *ModelHealth) reload(force bool)error{
 h.mu.Lock();defer h.mu.Unlock()
 if !force&&time.Since(h.lastRead)<15*time.Second{return nil}
 raw,err:=os.ReadFile(h.path)
 h.lastRead=time.Now()
 if errors.Is(err,os.ErrNotExist){h.entries=map[string]modelStatus{};return nil}
 if err!=nil{return err}
 var saved map[string]modelStatus
 if err=json.Unmarshal(raw,&saved);err!=nil{return err}
 if saved==nil{saved=map[string]modelStatus{}}
 h.entries=saved
 return nil
}
func (h *ModelHealth) lookup(cfg Config,key string)(modelStatus,bool){
 _=h.reload(false)
 h.mu.RLock();defer h.mu.RUnlock()
 r,ok:=h.entries[key]
 if !ok||r.Fingerprint!=modelFingerprint(cfg,key)||r.CheckedAt.IsZero(){return modelStatus{},false}
 return r,true
}
func(h *ModelHealth) available(cfg Config,key string)bool{
 r,found:=h.lookup(cfg,key)
 return found&&r.Passed&&time.Since(r.CheckedAt)<modelHealthyFor
}
func(h *ModelHealth) due(cfg Config,key string)bool{
 r,found:=h.lookup(cfg,key);if !found{return true}
 age:=time.Since(r.CheckedAt)
 if age<0{return true}
 if r.Passed{return age>=modelRecheckAfter}
 return age>=modelFailureRetryAfter
}
func(h *ModelHealth) record(cfg Config,key string,passed bool)error{
 h.mu.Lock();defer h.mu.Unlock()
 h.entries[key]=modelStatus{
  CheckedAt:time.Now().UTC(),Fingerprint:modelFingerprint(cfg,key),Passed:passed,
 }
 return h.saveLocked()
}
func(h *ModelHealth) saveLocked()error{
 dir:=filepath.Dir(h.path)
 if err:=os.MkdirAll(dir,0750);err!=nil{return err}
 b,err:=json.MarshalIndent(h.entries,"","  ");if err!=nil{return err}
 f,err:=os.CreateTemp(dir,".model-health-*");if err!=nil{return err}
 defer os.Remove(f.Name())
 if err=f.Chmod(0600);err!=nil{f.Close();return err}
 // A root-run CLI must not replace aibot-owned state with a root-only file:
 // the systemd bot runs as user aibot and needs to write future results.
 if os.Geteuid()==0{
  if stat,err:=os.Stat(dir);err==nil{
   if owner,ok:=stat.Sys().(*syscall.Stat_t);ok{
    if err=f.Chown(int(owner.Uid),int(owner.Gid));err!=nil{f.Close();return err}
   }
  }
 }
 if _,err=f.Write(append(b,'\n'));err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 if err=os.Rename(f.Name(),h.path);err!=nil{return err}
 h.lastRead=time.Now()
 return nil
}

// Every configured free model is probed individually, using a real authenticated
// request from the VDS. A provider being reachable does not prove its models work.
// No nonresponsive, rate-limited, or English-only model is shown as available.
func scanAvailableModels(ctx context.Context,cfg Config,c *Catalog,out io.Writer,force bool)error{
 ai:=newAI(c)
 return scanAvailableModelsWith(ctx,cfg,c,out,force,ai.generate)
}
func scanAvailableModelsWith(ctx context.Context,cfg Config,c *Catalog,out io.Writer,force bool,generate modelProbe)error{
 if c.health==nil{return errors.New("не включено хранилище проверки моделей")}
 refreshCtx,cancel:=context.WithTimeout(ctx,35*time.Second)
 problems:=c.refresh(refreshCtx,cfg)
 cancel()
 keys:=c.candidates(cfg)
 if len(keys)==0{
  fmt.Fprintln(out,"Доступных ИИ-моделей нет. Проверьте включённые сервисы, ключи и каталоги.")
  return errors.New("нет моделей для проверки")
 }
 fmt.Fprintf(out,"Проверка всех %d настроенных бесплатных моделей с этого сервера (реальные API-запросы)...\n",len(keys))
 checked,cached:=0,0
 failures:=map[string]int{}
 for _,key:=range keys {
  if err:=ctx.Err();err!=nil{return err}
  if !force&&!c.health.due(cfg,key){cached++;continue}
  checked++
  var response string;var err error
  // One small probe per model. A second request is only used if the provider
  // answered successfully but did not demonstrate Russian understanding.
  for attempt:=0;attempt<2;attempt++{
   requestCtx,stop:=context.WithTimeout(ctx,30*time.Second)
   response,err=generate(requestCtx,cfg,key,[]Message{
    {Role:"system",Content:"Проверка доступности и русского языка. Ответь одним коротким предложением только на русском языке."},
    {Role:"user",Content:"Назови столицу России."},
   })
   stop()
   if err!=nil||russianProbeOK(response){break}
  }
  passed:=err==nil&&russianProbeOK(response)
  if recordErr:=c.health.record(cfg,key,passed);recordErr!=nil{
   return fmt.Errorf("не удалось сохранить результат %s: %w",c.label(key),recordErr)
  }
  if !passed{
   category:="ответ не по-русски или неверный"
   if err!=nil{
    category="ошибка API/сети"
    msg:=err.Error()
    if strings.Contains(msg,"429"){category="лимит частоты (HTTP 429)"}
    if strings.Contains(msg,"401")||strings.Contains(msg,"403"){category="ключ или права доступа"}
    if strings.Contains(msg,"404")||strings.Contains(msg,"410"){category="модель удалена"}
   }
   failures[category]++
  }
 }
 ready:=c.enabled(cfg)
 fmt.Fprintln(out,"\nДОСТУПНЫЕ МОДЕЛИ (только прошедшие проверку):")
 if len(ready)==0{fmt.Fprintln(out,"Ни одной доступной модели не обнаружено.")}
 for i,key:=range ready{fmt.Fprintf(out,"%2d. %s\n",i+1,c.label(key))}
 fmt.Fprintf(out,"\nИтого: найдено %d, проверено сейчас %d, использованы актуальные результаты %d, работают %d, недоступны %d.\n",
  len(keys),checked,cached,len(ready),len(keys)-len(ready))
 if len(problems)>0{
  for provider,reason:=range problems{fmt.Fprintf(out,"Каталог %s: %s\n",provider,reason)}
 }
 for label,count:=range failures{fmt.Fprintf(out,"Скрыто из-за %s: %d\n",label,count)}
 if len(ready)==0{return errors.New("ни одна модель не прошла проверку с сервера")}
 return nil
}
