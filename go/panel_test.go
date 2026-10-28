package main

import (
 "encoding/base64"
 "net"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestPanelDomainValidation(t *testing.T){
 valid:=[]string{"bot.example.com","a-b.example.org","vpn-ai.sub.example.net"}
 for _,s:=range valid{if !validPanelDomain(s){t.Errorf("valid domain rejected: %s",s)}}
 invalid:=[]string{"","localhost","localhost.local","internal.local","192.168.1.1","127.0.0.1","example","EXAMPLE.COM",
  "-a.example.com","a-.example.com","a..example.com","a.example.com:443",
  "https://example.com","one.com\nreverse_proxy evil","*.example.com","a.example.test"}
 for _,s:=range invalid{if validPanelDomain(s){t.Errorf("unsafe domain accepted: %q",s)}}
}
func TestPanelCredentialSavedAsHashOnly(t *testing.T){
 t.Setenv("AIBOT_CONFIG",filepath.Join(t.TempDir(),"config.json"))
 pass,hash,err:=newPanelCredential()
 if err!=nil{t.Fatal(err)}
 if len(pass)<40||len(hash)!=64||panelDigest(pass)!=hash{t.Fatal("invalid random password or hash")}
 if _,err=base64.RawURLEncoding.DecodeString(pass);err!=nil{t.Fatal(err)}
 p:=PanelConfig{Domain:"admin.example.org",PasswordHash:hash}
 if err=savePanelConfig(p);err!=nil{t.Fatal(err)}
 read,err:=loadPanelConfig();if err!=nil||read!=p{t.Fatalf("panel config lost: %+v %v",read,err)}
 raw,err:=os.ReadFile(panelPath());if err!=nil{t.Fatal(err)}
 if strings.Contains(string(raw),pass){t.Fatal("plaintext password leaked to disk")}
 st,err:=os.Stat(panelPath());if err!=nil||st.Mode().Perm()!=0600{t.Fatalf("bad config permissions: %v %v",st,err)}
}
func TestCaddyOnlyOwnDomainAndLoopback(t *testing.T){
 caddy:=caddyPanelConfig("panel.example.org")
 if !strings.HasPrefix(caddy,"panel.example.org {")||!strings.Contains(caddy,"reverse_proxy 127.0.0.1:8765"){
  t.Fatalf("Caddy is not restricted to domain and local service:\n%s",caddy)
 }
 if strings.Contains(caddy,"tls internal")||strings.Contains(caddy,"http://"){
  t.Fatal("public TLS not automatic")
 }
 units:=panelUnitFiles("/usr/bin/caddy")
 if len(units)!=2{t.Fatal("expected only own two units")}
 for path,unit:=range units{
  if !strings.HasPrefix(unit,"# Managed by AI-bot panel\n"){t.Fatalf("%s: unmanaged unit",path)}
  if strings.Contains(unit,"systemctl stop caddy")||strings.Contains(unit,"iptables"){
   t.Fatal("other services or firewall must remain untouched")
  }
 }
}
func TestDomainPanelHostAuthAndHTTPSProxy(t *testing.T){
 t.Setenv("AIBOT_CONFIG",filepath.Join(t.TempDir(),"config.json"))
 if err:=saveConfig(defaults());err!=nil{t.Fatal(err)}
 catalog,err:=newCatalog();if err!=nil{t.Fatal(err)}
 secret:="random-test-secret"
 handler:=panelHTTPHandler(catalog,panelDigest(secret),"panel.example.org")
 query:=func(host,proto,user,password string)*httptest.ResponseRecorder{
  req:=httptest.NewRequest("GET","http://127.0.0.1:8765/",nil)
  req.Host=host
  if proto!=""{req.Header.Set("X-Forwarded-Proto",proto)}
  if user!=""{req.SetBasicAuth(user,password)}
  recorder:=httptest.NewRecorder()
  handler.ServeHTTP(recorder,req)
  return recorder
 }
 cases:=[]struct{host,proto,user,pass string;status int}{
  {"panel.example.org","https","admin",secret,http.StatusOK},
  {"panel.example.org","","admin",secret,http.StatusBadRequest},
  {"other.example.org","https","admin",secret,http.StatusBadRequest},
  {"panel.example.org","http","admin",secret,http.StatusBadRequest},
  {"panel.example.org","https","admin","wrong",http.StatusUnauthorized},
  {"panel.example.org","https","admin","",http.StatusUnauthorized},
  {"localhost:8765","","admin",secret,http.StatusOK},
 }
 for _,tc:=range cases{
  got:=query(tc.host,tc.proto,tc.user,tc.pass)
  if got.Code!=tc.status{t.Errorf("%q/%q/%q: status %d, wanted %d",tc.host,tc.proto,tc.user,got.Code,tc.status)}
  if tc.status==http.StatusOK&&strings.Contains(got.Body.String(),secret){t.Fatal("password echoed to browser")}
 }
}
func TestPanelOccupiedPortNeverConfigures(t *testing.T){
 // No write to /etc or systemd: reject invalid domains before any effect.
 t.Setenv("AIBOT_CONFIG",filepath.Join(t.TempDir(),"config.json"))
 _,err:=panelSetupDomain("bad.com; rm -rf /")
 if err==nil{t.Fatal("unsafe domain accepted")}
 if _,statErr:=os.Stat(panelPath());!os.IsNotExist(statErr){t.Fatalf("panel config created despite rejection: %v",statErr)}
 socket,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{t.Fatal(err)};defer socket.Close()
 if portFree(socket.Addr().(*net.TCPAddr).Port){t.Fatal("occupied socket reported free")}
}
