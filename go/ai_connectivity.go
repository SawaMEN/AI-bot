package main

import (
 "context"
 "crypto/tls"
 "errors"
 "fmt"
 "io"
 "net"
 "net/url"
 "strings"
 "time"
)

type networkProbe func(context.Context,string)(string,error)
type modelProbe func(context.Context,Config,string,[]Message)(string,error)

// This probes the direct network path of the machine executing the command.
// Real API requests may additionally use Go HTTP_PROXY / HTTPS_PROXY settings.
func probeDirectHTTPS(ctx context.Context,endpoint string)(string,error){
 u,err:=url.Parse(endpoint)
 if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil{return "",errors.New("неверный HTTPS-адрес API")}
 host:=u.Hostname();port:=u.Port();if port==""{port="443"}
 dnsCtx,done:=context.WithTimeout(ctx,5*time.Second)
 addresses,err:=net.DefaultResolver.LookupIPAddr(dnsCtx,host);done()
 if err!=nil{return "",fmt.Errorf("DNS: %w",err)}
 if len(addresses)==0{return "",errors.New("DNS: не найдено IP-адресов")}
 status:=fmt.Sprintf("DNS OK (%d IP)",len(addresses))
 dialCtx,done:=context.WithTimeout(ctx,6*time.Second)
 conn,err:=(&net.Dialer{Timeout:6*time.Second,KeepAlive:-1}).DialContext(dialCtx,"tcp",net.JoinHostPort(host,port))
 done()
 if err!=nil{return status,fmt.Errorf("TCP: %w",err)}
 defer conn.Close()
 status+=", TCP OK"
 tlsCtx,done:=context.WithTimeout(ctx,6*time.Second);defer done()
 secure:=tls.Client(conn,&tls.Config{ServerName:host,MinVersion:tls.VersionTLS12})
 if err=secure.HandshakeContext(tlsCtx);err!=nil{return status,fmt.Errorf("TLS/сертификат: %w",err)}
 return status+", TLS OK",nil
}
func aiProviderEndpoint(p ProviderSpec)string{
 switch p.Type {
 case "gemini":return "https://generativelanguage.googleapis.com/"
 case "cloudflare":return "https://api.cloudflare.com/"
 default:return p.Endpoint
 }
}
func selectedProbeModel(c *Catalog,cfg Config,provider string)string{
 chosen:=""
 for _,key:=range c.enabled(cfg){
  p,_,err:=splitModel(key);if err!=nil||p!=provider{continue}
  if chosen==""{chosen=key}
  if key==cfg.DefaultModel{return key}
 }
 return chosen
}
func checkAIConnectivity(ctx context.Context,cfg Config,c *Catalog,out io.Writer,networkOnly bool)error{
 ai:=newAI(c)
 return checkAIConnectivityWith(ctx,cfg,c,out,networkOnly,probeDirectHTTPS,ai.generate)
}
func checkAIConnectivityWith(ctx context.Context,cfg Config,c *Catalog,out io.Writer,networkOnly bool,probe networkProbe,generate modelProbe)error{
 if !networkOnly{
  refreshCtx,stop:=context.WithTimeout(ctx,25*time.Second)
  problems:=c.refresh(refreshCtx,cfg);stop()
  for id,reason:=range problems{fmt.Fprintf(out,"Каталог %s: %s\n",id,reason)}
 }
 if networkOnly{fmt.Fprintln(out,"Проверка DNS/TCP/TLS с этого сервера (без запросов к ИИ)")}else{
  fmt.Fprintln(out,"Проверка DNS/TCP/TLS и реальных ответов ИИ с этого сервера")
 }
 checked,healthy,failed:=0,0,0
 for _,id:=range c.Order{
  settings:=cfg.Providers[id]
  if !settings.Enabled{continue}
  if err:=ctx.Err();err!=nil{return err}
  p:=c.Specs[id]
  checked++
  fmt.Fprintf(out,"\n%s (%s)\n",p.Name,id)
  endpoint:=aiProviderEndpoint(p)
  if endpoint==""{fmt.Fprintln(out,"  ✗ Адрес API отсутствует");failed++;continue}
  nctx,cancel:=context.WithTimeout(ctx,20*time.Second)
  steps,err:=probe(nctx,endpoint);cancel()
  if steps!=""{fmt.Fprintf(out,"  %s\n",steps)}
  if err!=nil{
   fmt.Fprintf(out,"  ✗ Сетевая ошибка: %v\n",err)
   failed++;continue
  }
  if networkOnly{
   fmt.Fprintln(out,"  ✓ Сеть доступна (авторизация и генерация не проверялись)")
   healthy++;continue
  }
  if settings.APIKey==""&&p.AnonymousKey==""{
   fmt.Fprintln(out,"  ✗ Не указан API-ключ")
   failed++;continue
  }
  if p.Type=="cloudflare"&&settings.CloudflareAccount==""{
   fmt.Fprintln(out,"  ✗ Не указан Cloudflare Account ID")
   failed++;continue
  }
  model:=selectedProbeModel(c,cfg,id)
  if model==""{
   fmt.Fprintln(out,"  ✗ Не найдено включённой бесплатной модели; проверьте каталог")
   failed++;continue
  }
  fmt.Fprintf(out,"  Модель: %s\n",c.label(model))
  requestCtx,cancel:=context.WithTimeout(ctx,30*time.Second)
  started:=time.Now()
  answer,apiErr:=generate(requestCtx,cfg,model,[]Message{
   {Role:"system",Content:"Проверка API. Отвечай кратко и только по-русски."},
   {Role:"user",Content:"Какой город является столицей России?"},
  })
  cancel()
  duration:=time.Since(started).Round(time.Millisecond)
  if apiErr!=nil{
   fmt.Fprintf(out,"  ✗ Ошибка API/ключа/квоты: %v (%s)\n",apiErr,duration)
   failed++;continue
  }
  if strings.TrimSpace(answer)==""{
   fmt.Fprintln(out,"  ✗ API вернул пустой ответ");failed++;continue
  }
  if !russianProbeOK(answer){
   fmt.Fprintf(out,"  ✗ Ответ получен, но проверка русского языка не прошла (%s)\n",duration)
   failed++;continue
  }
  fmt.Fprintf(out,"  ✓ API доступен, ключ принят, ответ по-русски (%s)\n",duration)
  healthy++
 }
 if checked==0{return errors.New("нет включённых ИИ-провайдеров: sudo aibot menu")}
 fmt.Fprintf(out,"\nИтог: проверено %d, успешно %d, проблем %d.\n",checked,healthy,failed)
 if failed>0{return fmt.Errorf("проблемы с %d из %d провайдеров",failed,checked)}
 return nil
}
