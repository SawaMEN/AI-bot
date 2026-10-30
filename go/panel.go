package main

import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "net"
 "os"
 "os/exec"
 "os/user"
 "path/filepath"
 "regexp"
 "strings"
)

const panelBackendService="ai-bot-panel.service"
const panelTLSService="ai-bot-panel-tls.service"

type PanelConfig struct {
 Domain string `json:"domain"`
 PasswordHash string `json:"password_hash"`
}

var domainLabel=regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
func validPanelDomain(input string)bool {
 if input!=strings.ToLower(input)||len(input)>253||len(input)<4||strings.Contains(input,"..")||net.ParseIP(input)!=nil{return false}
 labels:=strings.Split(input,".")
 if len(labels)<2{return false}
 for _,label:=range labels{if !domainLabel.MatchString(label){return false}}
 last:=labels[len(labels)-1]
 if len(last)<2||last=="local"||last=="internal"||last=="localhost"||last=="test"||last=="invalid"||last=="example"{return false}
 return true
}
func panelPath()string{return filepath.Join(filepath.Dir(configPath()),"panel.json")}
// Caddy must never need read access to /etc/ai-bot (contains API keys).
func panelCaddyfilePath()string{return "/etc/ai-bot-panel/Caddyfile"}

func newPanelCredential()(password,hash string,err error){
 key:=make([]byte,32)
 if _,err=rand.Read(key);err!=nil{return "","",err}
 password=base64.RawURLEncoding.EncodeToString(key)
 return password,panelDigest(password),nil
}
func panelDigest(password string)string {
 hash:=sha256.Sum256([]byte(password))
 return hex.EncodeToString(hash[:])
}
func loadPanelConfig()(PanelConfig,error){
 var p PanelConfig
 data,err:=os.ReadFile(panelPath());if err!=nil{return p,err}
 if err=json.Unmarshal(data,&p);err!=nil{return p,err}
 if !validPanelDomain(p.Domain)||len(p.PasswordHash)!=64{return p,errors.New("неверные настройки доменной панели")}
 if _,err=hex.DecodeString(p.PasswordHash);err!=nil{return p,err}
 return p,nil
}
func writeProtected(path string,data []byte,mode os.FileMode)error{
 if err:=os.MkdirAll(filepath.Dir(path),0750);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(path),".ai-bot-panel-*")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 if _,err=f.Write(data);err!=nil{f.Close();return err}
 if err=f.Chmod(mode);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),path)
}
func savePanelConfig(p PanelConfig)error{
 if !validPanelDomain(p.Domain)||len(p.PasswordHash)!=64{return errors.New("недопустимая конфигурация панели")}
 blob,err:=json.MarshalIndent(p,"","  ");if err!=nil{return err}
 return writeProtected(panelPath(),append(blob,'\n'),0600)
}
func caddyPanelConfig(domain string)string{
 // The upstream only binds to loopback; the public port is exclusively TLS.
 return domain+" {\n"+
  "  encode gzip\n"+
  "  reverse_proxy 127.0.0.1:8765\n"+
  "  header {\n"+
  "    Strict-Transport-Security \"max-age=31536000\"\n"+
  "    X-Content-Type-Options \"nosniff\"\n"+
  "    Referrer-Policy \"no-referrer\"\n"+
  "  }\n"+
  "}\n"
}
func portFree(port int)bool{
 listener,err:=net.Listen("tcp",fmt.Sprintf(":%d",port))
 if err!=nil{return false}
 listener.Close();return true
}
func panelUnitFiles(caddyBin string)map[string]string{
 backend:=`# Managed by AI-bot panel
[Unit]
Description=AI-bot panel (local HTTP, password protected)
After=network.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/lib/ai-bot/ai-bot panel serve
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/etc/ai-bot
MemoryMax=140M

[Install]
WantedBy=multi-user.target
`
 tls:="# Managed by AI-bot panel\n"+
  "[Unit]\nDescription=AI-bot automatic HTTPS panel\nAfter=network-online.target "+panelBackendService+
  "\nWants=network-online.target\nRequires="+panelBackendService+
  "\n\n[Service]\nType=simple\nUser=caddy\nGroup=caddy\n"+
  "Environment=HOME=/var/lib/ai-bot-panel-tls\n"+
  "Environment=XDG_DATA_HOME=/var/lib/ai-bot-panel-tls/data\n"+
  "Environment=XDG_CONFIG_HOME=/var/lib/ai-bot-panel-tls/config\n"+
  "StateDirectory=ai-bot-panel-tls\nStateDirectoryMode=0700\n"+
  "ExecStart="+caddyBin+" run --config /etc/ai-bot-panel/Caddyfile --adapter caddyfile\n"+
  "Restart=on-failure\nRestartSec=5\n"+
  "AmbientCapabilities=CAP_NET_BIND_SERVICE\n"+
  "CapabilityBoundingSet=CAP_NET_BIND_SERVICE\n"+
  "NoNewPrivileges=true\nPrivateTmp=true\nProtectHome=true\n"+
  "ProtectSystem=strict\nReadWritePaths=/var/lib/ai-bot-panel-tls\n"+
  "MemoryMax=180M\n\n[Install]\nWantedBy=multi-user.target\n"
 return map[string]string{
  "/etc/systemd/system/"+panelBackendService:backend,
  "/etc/systemd/system/"+panelTLSService:tls,
 }
}
func checkedSystemd(args ...string)error{
 command:=exec.Command("systemctl",args...)
 output,err:=command.CombinedOutput()
 if err!=nil{return fmt.Errorf("systemctl %s: %w: %s",strings.Join(args," "),err,strings.TrimSpace(string(output)))}
 return nil
}
func systemdActive(service string)bool{
 return exec.Command("systemctl","is-active","--quiet",service).Run()==nil
}
func installedByUs(path string)error{
 data,err:=os.ReadFile(path)
 if errors.Is(err,os.ErrNotExist){return nil}
 if err!=nil{return err}
 if !strings.HasPrefix(string(data),"# Managed by AI-bot panel\n"){
  return fmt.Errorf("файл %s существует и не принадлежит AI-bot; он не будет перезаписан",path)
 }
 return nil
}
func panelSetupDomain(domain string)(string,error){
 if !validPanelDomain(domain){return "",errors.New("укажите публичный домен, например bot.example.com, без протокола, порта и пробелов")}
 if systemdActive(panelBackendService)||systemdActive(panelTLSService){
  return "",errors.New("панель уже запущена: сначала выполните sudo aibot panel off")
 }
 // Never stop an existing VPN, Caddy, nginx, HAProxy or firewall.
 // Do not hijack an existing local panel process either.
 if !portFree(8765){
  return "",errors.New("локальный порт панели 8765 занят. Закройте другой экземпляр панели; настройки VPN не меняются")
 }
 if !portFree(80)||!portFree(443){
  return "",errors.New("80 или 443 уже занят. Ничего не изменено; используйте sudo aibot menu или SSH-панель. VPN не трогаем")
 }
 if exec.Command("systemctl","is-enabled","--quiet","caddy.service").Run()==nil{
  return "",errors.New("системный caddy.service включён и может занять 80/443 после перезагрузки. Проверьте, нужен ли он другим сайтам; AI-bot его не отключает")
 }
 caddyBin,err:=exec.LookPath("caddy")
 if err!=nil{return "",errors.New("Caddy не установлен. Установите его из репозитория своего дистрибутива (например, sudo apt install caddy), затем повторите команду; если порт 443 занят VPN, используйте терминал")}
 if _,err=user.Lookup("caddy");err!=nil{return "",errors.New("не найден системный пользователь caddy; установите официальный пакет Caddy")}
 units:=panelUnitFiles(caddyBin)
 for path:=range units{if err:=installedByUs(path);err!=nil{return "",err}}
 if _,err=os.Stat(panelPath());err==nil{return "",errors.New("панель уже настроена; для смены домена используйте sudo aibot panel off и panel reset")}
 if err!=nil&&!errors.Is(err,os.ErrNotExist){return "",err}
 // Validate Caddy syntax before creating service units or credentials.
 caddyText:=caddyPanelConfig(domain)
 configFile,err:=os.CreateTemp("","ai-bot-caddy-*.Caddyfile")
 if err!=nil{return "",err}
 defer os.Remove(configFile.Name())
 if _,err=configFile.WriteString(caddyText);err!=nil{configFile.Close();return "",err}
 if err=configFile.Close();err!=nil{return "",err}
 output,err:=exec.Command(caddyBin,"validate","--config",configFile.Name(),"--adapter","caddyfile").CombinedOutput()
 if err!=nil{return "",fmt.Errorf("Caddy не принял конфигурацию: %w (%s)",err,strings.TrimSpace(string(output)))}
 password,hash,err:=newPanelCredential();if err!=nil{return "",err}
 // Public reverse-proxy config lives separately from the secret API-key dir.
 // The caddy service cannot traverse /etc/ai-bot (root:aibot 2750).
 if err=os.MkdirAll("/etc/ai-bot-panel",0755);err!=nil{return "",err}
 if err=os.Chmod("/etc/ai-bot-panel",0755);err!=nil{return "",err}
 if err=savePanelConfig(PanelConfig{Domain:domain,PasswordHash:hash});err!=nil{return "",err}
 if err=writeProtected(panelCaddyfilePath(),[]byte(caddyText),0644);err!=nil{return "",err}
 for path,body:=range units{if err=writeProtected(path,[]byte(body),0644);err!=nil{return "",err}}
 if err=checkedSystemd("daemon-reload");err!=nil{return "",err}
 if err=checkedSystemd("enable","--now",panelBackendService);err!=nil{return "",err}
 if err=checkedSystemd("enable","--now",panelTLSService);err!=nil{return "",err}
 if !systemdActive(panelBackendService)||!systemdActive(panelTLSService){
  return "",errors.New("панель не запустилась; проверьте journalctl -u ai-bot-panel-tls")
 }
 return password,nil
}
func panelOff()error{
 // Do not stop the system caddy.service or anything owned by the VPN.
 var failures []string
 for _,unit:=range []string{panelTLSService,panelBackendService}{
  if err:=checkedSystemd("disable","--now",unit);err!=nil{failures=append(failures,err.Error())}
 }
 if len(failures)>0{return errors.New(strings.Join(failures,"; "))}
 return nil
}
func panelReset()error{
 if systemdActive(panelTLSService)||systemdActive(panelBackendService){
  return errors.New("сначала выполните sudo aibot panel off")
 }
 if err:=os.Remove(panelPath());err!=nil&&!errors.Is(err,os.ErrNotExist){return err}
 return nil
}
func panelStatus()string{
 cfg,err:=loadPanelConfig()
 if err!=nil{
  return "✗ Доменная панель не настроена.\nТерминальное меню уже доступно: sudo aibot menu\nЧтобы открыть веб-панель, выберите пункт 8 → 1."
 }
 backend,tls:=systemdActive(panelBackendService),systemdActive(panelTLSService)
 result:=fmt.Sprintf("Адрес: https://%s\n%s  Веб-панель бота\n%s  HTTPS и сертификаты Caddy\n",
  cfg.Domain,switchLabel(backend),switchLabel(tls))
 if !backend||!tls{result+="Если что-то выключено, проверьте: sudo journalctl -u ai-bot-panel-tls -n 40 --no-pager\n"}
 result+="Caddy автоматически получает и продлевает сертификат, когда DNS и порты 80/443 доступны."
 return result
}
func (w Wizard) panelMenu(){
 for{
  choice:=w.chooseMenu("УПРАВЛЕНИЕ ВЕБ-ПАНЕЛЬЮ",panelStatus(),[]menuOption{
   {1,"Настроить HTTPS на своём домене","Проверка домена, Caddy и TLS; доступ по паролю",""},
   {2,"Проверить состояние панели","Показать адрес, статус службы и настройки",""},
   {3,"Отключить доменную панель","Бот ВК и остальные сервисы продолжат работать",""},
   {4,"Временная панель через SSH","Панель только через локальный SSH-туннель",""},
   {5,"Сбросить домен и пароль","Возможен только после отключения веб-панели","Осторожно"},
   {0,"Назад","Вернуться к главному меню",""},
  })
  switch choice{
  case 0:return
  case 1:
   fmt.Fprintln(w.out,"Для доменной панели нужно:")
   fmt.Fprintln(w.out,"  1) Владеть доменом (например ai.example.com).")
   fmt.Fprintln(w.out,"  2) У регистратора/в DNS домена добавить A-запись на IP вашего VDS.")
   fmt.Fprintln(w.out,"  3) Освободить порты 80 и 443, если на них нет других сервисов.")
   fmt.Fprintln(w.out,"  4) Установить Caddy: sudo apt install caddy")
   fmt.Fprintln(w.out,"AI-bot не будет отключать VPN, изменять файрвол или занимать уже используемый порт.")
   domain:=strings.ToLower(strings.TrimSpace(w.ask("Домен (без https://, например ai.example.com)","")))
   if w.ask("Начать установку? Напишите ДА","нет")!="ДА"{fmt.Fprintln(w.out,"Отменено.");continue}
   var pass string;var err error
   consoleSpinner(w.out,"Настройка HTTPS-панели и службы",func(){pass,err=panelSetupDomain(domain)})
   if err!=nil{
    fmt.Fprintln(w.out,"✗ Доменная панель не настроена:",err)
    fmt.Fprintln(w.out,"Можно использовать только терминал: sudo aibot menu")
    continue
   }
   fmt.Fprintf(w.out,"\n✓ Панель запущена.\nАдрес: https://%s\nЛогин: admin\nПароль (запишите сейчас): %s\n",domain,pass)
   fmt.Fprintln(w.out,"Далее: откройте адрес в браузере. Если сертификат пока не выдан, проверьте DNS, 80/443 и журнал Caddy.")
  case 2:fmt.Fprintln(w.out,panelStatus())
  case 3:
   if w.ask("Для отключения веб-панели напишите ДА","нет")=="ДА"{
    if err:=panelOff();err!=nil{fmt.Fprintln(w.out,"✗ Не удалось отключить:",err)
    }else{fmt.Fprintln(w.out,"✓ Доменная веб-панель выключена. Сам AI-bot продолжает отвечать в ВК.")}
   }
  case 4:
   fmt.Fprintln(w.out,"Откройте на своём компьютере SSH-туннель: ssh -L 8765:127.0.0.1:8765 USER@SERVER")
   fmt.Fprintln(w.out,"Затем откройте http://127.0.0.1:8765 в браузере. Логин и временный пароль появятся ниже.")
   cfg,err:=loadConfig();if err!=nil{fmt.Fprintln(w.out,"✗ Конфигурация:",err);continue}
   catalog,err:=newCatalog();if err!=nil{fmt.Fprintln(w.out,"✗ Каталог моделей:",err);continue}
   if err:=webServe(cfg,catalog);err!=nil{fmt.Fprintln(w.out,"✗ Панель:",err)}
  case 5:
   if w.ask("Для сброса домена и пароля напишите ДА","нет")=="ДА"{
    if err:=panelReset();err!=nil{fmt.Fprintln(w.out,"✗ Сброс не выполнен:",err)
    }else{fmt.Fprintln(w.out,"✓ Сброшено. Чтобы настроить новый домен, выберите пункт 1.")}
   }
  }
 }
}
func panelCommand(w Wizard,args []string)error{
 if len(args)==0{w.panelMenu();return nil}
 switch args[0]{
 case "status":fmt.Fprintln(w.out,panelStatus());return nil
 case "off":return panelOff()
 case "reset":return panelReset()
 case "setup":
  if len(args)!=2{return errors.New("использование: sudo aibot panel setup bot.example.com")}
  password,err:=panelSetupDomain(strings.ToLower(args[1]));if err!=nil{return err}
  fmt.Fprintf(w.out,"HTTPS: https://%s\nЛогин: admin\nОдноразово показанный пароль: %s\nСертификат появится после проверки DNS и портов ACME.\n",args[1],password)
  return nil
 case "serve":
  return errors.New("команда panel serve запускается только через системную службу")
 default:
  return errors.New("команды: panel, panel setup ДОМЕН, panel status, panel off, panel reset")
 }
}
