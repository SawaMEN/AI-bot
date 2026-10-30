package main

import (
 "bufio"
 "bytes"
 "strings"
 "testing"
)

func TestReadableSwitchAndSetupStatus(t *testing.T){
 if switchLabel(true)!="✓ ВКЛ"||switchLabel(false)!="✗ ВЫКЛ"{
  t.Fatal("unreadable switch labels")
 }
 c,err:=newCatalog();if err!=nil{t.Fatal(err)}
 cfg:=defaults()
 if !strings.Contains(nextSetupStep(cfg,c),"пункт 1"){t.Fatal("VK onboarding step missing")}
 if !strings.Contains(setupSummary(cfg,c),"✗ Не настроено"){t.Fatal("missing initial X markers")}
 cfg.VKToken="private-vk-token";cfg.GroupID=12345
 if !strings.Contains(nextSetupStep(cfg,c),"пункт 2"){t.Fatal("AI onboarding step missing")}
 cfg.Providers["groq"]=ProviderConfig{APIKey:"private-api-key",Enabled:true}
 cfg.AutoModelsProviders=[]string{"groq"}
 if !strings.Contains(nextSetupStep(cfg,c),"добавьте своё сообщество ВК в беседу"){t.Fatal("chat invitation step missing")}
 summary:=setupSummary(cfg,c)
 if !strings.Contains(summary,"✓ Готово"){t.Fatal("missing ready markers")}
 if strings.Contains(summary,"private-api-key")||strings.Contains(summary,"private-vk-token"){
  t.Fatal("API secret echoed in status")
 }
}

func TestVKGuideExplainsCommunityVersusChat(t *testing.T){
 guide:=vkSetupGuide(123)
 required:=[]string{
  "Это НЕ беседа","vk.com/club123","Работа с API","Ключи доступа",
  "Long Poll API","message_new","message_event",
  "добавьте","Доступ ко всей переписке","sudo aibot doctor",
 }
 for _,item:=range required{
  if !strings.Contains(guide,item){t.Errorf("VK guide misses %q",item)}
 }
 if strings.Contains(guide,"api_key"){t.Error("guide exposes internal setting")}
}

func TestMenuNavigationHasGuidesAndExit(t *testing.T){
 cfg:=defaults();catalog,err:=newCatalog();if err!=nil{t.Fatal(err)}
 var output bytes.Buffer
 wizard:=Wizard{in:bufio.NewReader(strings.NewReader("10\n11\n0\n")),out:&output}
 menu(wizard,cfg,catalog)
 body:=output.String()
 for _,fragment:=range []string{
  "AI-bot · УПРАВЛЕНИЕ","✓ ВКЛ","✗ ВЫКЛ",
  "Где найти ID и токен ВК","ЧТО ДЕЛАТЬ ДАЛЬШЕ","ШAГ 1",
 }{
  if fragment=="ШAГ 1"{continue}
  if !strings.Contains(body,fragment){t.Errorf("menu does not contain %q",fragment)}
 }
 if !strings.Contains(body,"ШАГ 1")||!strings.Contains(body,"ШАГ 6"){
  t.Fatal("step-by-step guide not available from main menu")
 }
}

func TestDisabledProviderKeepsExistingSecret(t *testing.T){
 cfg:=defaults();cfg.Providers["groq"]=ProviderConfig{Enabled:true,APIKey:"private-api-key"}
 cfg.AutoModelsProviders=[]string{"groq"}
 catalog,_:=newCatalog()
 var output bytes.Buffer
 wizard:=Wizard{in:bufio.NewReader(strings.NewReader("3\n")),out:&output}
 wizard.provider(&cfg,catalog,"groq")
 if cfg.Providers["groq"].Enabled{t.Fatal("provider not switched off")}
 if cfg.Providers["groq"].APIKey!="private-api-key"{t.Fatal("provider secret lost on disable")}
 if strings.Contains(output.String(),"private-api-key"){t.Fatal("secret exposed in provider menu")}
 if !strings.Contains(output.String(),"✗ ВЫКЛ"){t.Fatal("OFF state not indicated")}
}

func TestVKHelpSettingsArePlainLanguage(t *testing.T){
 if !strings.Contains(permissionHelp(Config{GroupID:123}),"сообщество"){
  t.Fatal("VK permissions need an understandable explanation")
 }
 if !strings.Contains(helpText(),"беседе ВКонтакте"){
  t.Fatal("VK command help needs context")
 }
 if readableSearch("on")!="✓ ВКЛ"||readableSearch("off")!="✗ ВЫКЛ"{
  t.Fatal("search states not localized")
 }
 if readableFallback("off")!="✗ ВЫКЛ"{t.Fatal("fallback off state missing")}
}
