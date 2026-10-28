package main

import (
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"
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
 if cfg.ContextMessages<1||cfg.ContextMessages>100{return cfg,errors.New("недопустимый context_messages")}
 if cfg.MaxRequestsHour<1||cfg.MaxUserRequestsHour<1||cfg.MaxChatRequestsHour<1{return cfg,errors.New("неверные лимиты API")}
 if cfg.ContextHours<1||cfg.RetentionDays<1{return cfg,errors.New("неверный срок хранения")}
 // Legacy Python v1/v2 migration: unknown JSON fields ignored, same schema used.
 return cfg,nil
}
func saveConfig(cfg Config) error {
 b,err:=json.MarshalIndent(cfg,"","  ");if err!=nil{return err}
 path:=configPath();if err=os.MkdirAll(filepath.Dir(path),0750);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(path),".config-*");if err!=nil{return err}
 defer os.Remove(f.Name())
 if _,err=f.Write(append(b,'\n'));err!=nil{f.Close();return err}
 if err=f.Chmod(0640);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),path)
}
func validHTTPS(u string) bool {return strings.HasPrefix(u,"https://")}
