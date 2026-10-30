package main

import (
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "syscall"
)

type ProviderConfig struct {
 APIKey string `json:"api_key"`
 Enabled bool `json:"enabled"`
 CloudflareAccount string `json:"cloudflare_account"`
}
type Config struct {
 VKToken string `json:"vk_token"`
 GroupID int64 `json:"group_id"`
 Trigger string `json:"trigger"`
 ContextMessages int `json:"context_messages"`
 ContextHours int `json:"context_hours"`
 RetentionDays int `json:"retention_days"`
 SearchEnabled bool `json:"search_enabled"`
 CooldownSeconds int `json:"cooldown_seconds"`
 MaxRequestsHour int `json:"max_requests_hour"`
 MaxUserRequestsHour int `json:"max_user_requests_hour"`
 MaxChatRequestsHour int `json:"max_chat_requests_hour"`
 FreeFallback bool `json:"free_fallback"`
 Providers map[string]ProviderConfig `json:"providers"`
 EnabledModels []string `json:"enabled_models"`
 RussianBlockedModels []string `json:"russian_blocked_models,omitempty"`
 AutoModelsProviders []string `json:"auto_models_providers"`
 DefaultModel string `json:"default_model"`
}
func defaults() Config {return Config{Trigger:"бот",ContextMessages:30,ContextHours:24,RetentionDays:7,SearchEnabled:true,CooldownSeconds:15,MaxRequestsHour:50,MaxUserRequestsHour:12,MaxChatRequestsHour:25,FreeFallback:true,Providers:map[string]ProviderConfig{},EnabledModels:[]string{},AutoModelsProviders:[]string{}}}
func configPath() string {if x:=os.Getenv("AIBOT_CONFIG");x!="" {return x};return "/etc/ai-bot/config.json"}
func dataPath() string {if x:=os.Getenv("AIBOT_DATA");x!="" {return x};return "/var/lib/ai-bot/history.db"}
func loadConfig() (Config,error) {
 cfg:=defaults();b,err:=os.ReadFile(configPath());if errors.Is(err,os.ErrNotExist){return cfg,nil};if err!=nil{return cfg,err}
 if err:=json.Unmarshal(b,&cfg);err!=nil{return cfg,err}
 if cfg.Trigger==""{cfg.Trigger="бот"};if cfg.Providers==nil{cfg.Providers=map[string]ProviderConfig{}}
 // Legacy Python v1 used a single provider/api_key/model. Migrate in memory,
 // without admitting custom/paid providers. Do not replace the original file
 // until the administrator explicitly saves the new configuration.
 if len(cfg.Providers)==0 {
  var legacy struct {
   Provider string `json:"provider"`
   APIKey string `json:"api_key"`
   Model string `json:"model"`
   CloudflareAccount string `json:"cloudflare_account"`
  }
  if err:=json.Unmarshal(b,&legacy);err!=nil{return cfg,err}
  if legacy.APIKey!="" {
   catalog,err:=newCatalog();if err!=nil{return cfg,err}
   if spec,ok:=catalog.Specs[legacy.Provider];ok {
    name:=legacy.Model
    if name=="" {
     if len(spec.Models)>0{name=spec.Models[0][0]}
    }
    if catalog.modelAllowedID(legacy.Provider,name) &&
      (spec.FreePolicy=="curated" || legacy.Provider=="openrouter"&&name=="openrouter/free") {
     cfg.Providers[legacy.Provider]=ProviderConfig{
      APIKey:legacy.APIKey,Enabled:true,CloudflareAccount:legacy.CloudflareAccount,
     }
     selected:=legacy.Provider+":"+name
     cfg.EnabledModels=[]string{selected}
     cfg.DefaultModel=selected
    }
   }
  }
 }
 if cfg.ContextMessages<1||cfg.ContextMessages>100{return cfg,errors.New("недопустимый context_messages")}
 if cfg.MaxRequestsHour<1||cfg.MaxUserRequestsHour<1||cfg.MaxChatRequestsHour<1{return cfg,errors.New("неверные лимиты API")}
 if cfg.ContextHours<1||cfg.RetentionDays<1{return cfg,errors.New("неверный срок хранения")}
 return cfg,nil
}
func saveConfig(cfg Config) error {
 b,err:=json.MarshalIndent(cfg,"","  ");if err!=nil{return err}
 path:=configPath();if err=os.MkdirAll(filepath.Dir(path),0750);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(path),".config-*");if err!=nil{return err}
 defer os.Remove(f.Name())
 if _,err=f.Write(append(b,'\n'));err!=nil{f.Close();return err}
 if err=f.Chmod(0640);err!=nil{f.Close();return err}
 // Root-owned config must remain readable by the aibot systemd group.
 // Renaming root:root (0640) over root:aibot would break every restart.
 var owner os.FileInfo
 if stat,err:=os.Stat(path);err==nil{owner=stat
 }else if !errors.Is(err,os.ErrNotExist){f.Close();return err}
 if os.Geteuid()==0{
  var gid int
  if owner!=nil{if st,ok:=owner.Sys().(*syscall.Stat_t);ok{gid=int(st.Gid)}
  }else if dir,err:=os.Stat(filepath.Dir(path));err==nil{
   if st,ok:=dir.Sys().(*syscall.Stat_t);ok{gid=int(st.Gid)}
  }
  if err:=f.Chown(0,gid);err!=nil{f.Close();return err}
 }
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 // On the first conversion, preserve a locked-down, local v1 backup.
 if owner!=nil{
  if prev,err:=os.ReadFile(path);err==nil{
   var data map[string]json.RawMessage
   if json.Unmarshal(prev,&data)==nil&&len(data["api_key"])>0&&len(data["providers"])==0 {
    backup:=path+".python-v1.bak"
    if _,err:=os.Stat(backup);errors.Is(err,os.ErrNotExist){
     if err:=os.WriteFile(backup,prev,0600);err!=nil{return err}
     if os.Geteuid()==0{
      if err:=os.Chown(backup,0,0);err!=nil{return err}
     }
    }
   }
  }
 }
 return os.Rename(f.Name(),path)
}
func validHTTPS(u string) bool {return strings.HasPrefix(u,"https://")}
