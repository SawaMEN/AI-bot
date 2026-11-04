package main

import (
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sync"
)

// ai-bot.service runs as the unprivileged "aibot" user. The bootstrapping
// /etc/ai-bot/config.json remains root-owned and read-only to the service.
// Changes made from VK are stored separately in the existing, service-writable
// /var/lib/ai-bot directory and layered on top of the original configuration.
type vkAdminOverlay struct {
 Settings map[string]json.RawMessage `json:"settings,omitempty"`
 Providers map[string]ProviderConfig `json:"providers,omitempty"`
}
var vkAdminWriteMu sync.Mutex

var vkAdminWritableFields=map[string]bool{
 "trigger":true,"search_enabled":true,"free_fallback":true,
 "context_messages":true,"context_hours":true,"retention_days":true,
 "cooldown_seconds":true,"max_requests_hour":true,
 "max_user_requests_hour":true,"max_chat_requests_hour":true,
 "default_model":true,"enabled_models":true,"auto_models_providers":true,
 "group_id":true,"vk_token":true,"admin_vk_id":true,
}
func vkAdminOverlayPath()string{return filepath.Join(filepath.Dir(dataPath()),"vk-admin-settings.json")}

func readVKAdminOverlay()(vkAdminOverlay,error){
 overlay:=vkAdminOverlay{Settings:map[string]json.RawMessage{},Providers:map[string]ProviderConfig{}}
 bytes,err:=os.ReadFile(vkAdminOverlayPath())
 if errors.Is(err,os.ErrNotExist){return overlay,nil}
 if err!=nil{return overlay,err}
 if err=json.Unmarshal(bytes,&overlay);err!=nil{return overlay,fmt.Errorf("неверный файл настроек ВК: %w",err)}
 if overlay.Settings==nil{overlay.Settings=map[string]json.RawMessage{}}
 if overlay.Providers==nil{overlay.Providers=map[string]ProviderConfig{}}
 return overlay,nil
}
func mergeVKAdminOverlay(cfg *Config)error{
 overlay,err:=readVKAdminOverlay();if err!=nil{return err}
 if len(overlay.Settings)==0&&len(overlay.Providers)==0{return nil}
 raw,err:=json.Marshal(cfg);if err!=nil{return err}
 data:=map[string]json.RawMessage{}
 if err=json.Unmarshal(raw,&data);err!=nil{return err}
 for field,value:=range overlay.Settings{
  if !vkAdminWritableFields[field]{return fmt.Errorf("неразрешённое поле настроек ВК: %s",field)}
  data[field]=value
 }
 if cfg.Providers==nil{cfg.Providers=map[string]ProviderConfig{}}
 for id,value:=range overlay.Providers{cfg.Providers[id]=value}
 providers,err:=json.Marshal(cfg.Providers);if err!=nil{return err}
 data["providers"]=providers
 combined,err:=json.Marshal(data);if err!=nil{return err}
 return json.Unmarshal(combined,cfg)
}
func writeVKAdminOverlay(overlay vkAdminOverlay)error{
 target:=vkAdminOverlayPath()
 if err:=os.MkdirAll(filepath.Dir(target),0750);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(target),".vk-admin-*")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 raw,err:=json.MarshalIndent(overlay,"","  ");if err!=nil{f.Close();return err}
 if _,err=f.Write(append(raw,'\n'));err!=nil{f.Close();return err}
 if err=f.Chmod(0600);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),target)
}
func saveVKAdminField(field string,value any)(Config,error){
 if !vkAdminWritableFields[field]{return Config{},fmt.Errorf("настройка %s недоступна",field)}
 vkAdminWriteMu.Lock();defer vkAdminWriteMu.Unlock()
 overlay,err:=readVKAdminOverlay();if err!=nil{return Config{},err}
 raw,err:=json.Marshal(value);if err!=nil{return Config{},err}
 overlay.Settings[field]=raw
 if err=writeVKAdminOverlay(overlay);err!=nil{return Config{},err}
 return loadConfig()
}
func saveVKAdminProvider(id string,settings ProviderConfig)(Config,error){
 catalog,err:=newCatalog();if err!=nil{return Config{},err}
 spec,ok:=catalog.Specs[id];if !ok{return Config{},errors.New("неизвестный провайдер")}
 if settings.Enabled&&spec.requiresKey()&&settings.APIKey==""&&spec.AnonymousKey==""{
  return Config{},errors.New("для этого провайдера нужен бесплатный API-ключ")
 }
 vkAdminWriteMu.Lock();defer vkAdminWriteMu.Unlock()
 overlay,err:=readVKAdminOverlay();if err!=nil{return Config{},err}
 overlay.Providers[id]=settings
 if err=writeVKAdminOverlay(overlay);err!=nil{return Config{},err}
 return loadConfig()
}
