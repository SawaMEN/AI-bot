package main

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "strings"
 "time"
)

type Message struct {Role string `json:"role"`; Content string `json:"content"`}
type AI struct{client *http.Client; catalog *Catalog}
func newAI(c *Catalog)*AI{return &AI{client:&http.Client{Timeout:65*time.Second},catalog:c}}
func (ai *AI) generate(ctx context.Context,cfg Config,key string,messages []Message)(string,error){
 enabled:=false;for _,id:=range ai.catalog.candidates(cfg){if id==key{enabled=true;break}}
 if !enabled{return "",errors.New("модель запрещена или больше не бесплатна")}
 provider,model,err:=splitModel(key);if err!=nil{return "",err}
 spec:=ai.catalog.Specs[provider];setting:=cfg.Providers[provider];token:=setting.APIKey;if token==""{token=spec.AnonymousKey}
 endpoint:=spec.Endpoint;body:=map[string]any{"model":model,"messages":messages,"max_tokens":1500}
 headers:=map[string]string{"Authorization":"Bearer "+token}
 if spec.Type=="gemini"{
  endpoint="https://generativelanguage.googleapis.com/v1beta/models/"+url.PathEscape(model)+":generateContent"
  var prompt strings.Builder;for _,m:=range messages{prompt.WriteString(strings.ToUpper(m.Role)+":\n"+m.Content+"\n\n")}
  body=map[string]any{"contents":[]any{map[string]any{"parts":[]any{map[string]string{"text":prompt.String()}}}},"generationConfig":map[string]int{"maxOutputTokens":1500}}
  headers=map[string]string{"x-goog-api-key":token}
 }else if spec.Type=="cloudflare"{
  if setting.CloudflareAccount==""{return "",errors.New("Cloudflare Account ID отсутствует")}
  endpoint="https://api.cloudflare.com/client/v4/accounts/"+url.PathEscape(setting.CloudflareAccount)+"/ai/run/"+model
  body=map[string]any{"messages":messages,"max_tokens":1500}
 }
 if !validHTTPS(endpoint){return "",errors.New("небезопасный адрес API")}
 raw,err:=json.Marshal(body);if err!=nil{return "",err}
 req,err:=http.NewRequestWithContext(ctx,"POST",endpoint,bytes.NewReader(raw));if err!=nil{return "",err}
 req.Header.Set("Content-Type","application/json");for k,v:=range headers{req.Header.Set(k,v)}
 response,err:=ai.client.Do(req);if err!=nil{return "",err};defer response.Body.Close()
 data,err:=io.ReadAll(io.LimitReader(response.Body,2<<20));if err!=nil{return "",err}
 // Report actionable diagnostics without printing API bodies or credentials.
 switch response.StatusCode {
 case 400:return "",errors.New("API отклонил формат запроса (HTTP 400)")
 case 401,403:return "",errors.New("ключ недействителен или доступ запрещён (HTTP 401/403)")
 case 402:return "",errors.New("бесплатный баланс исчерпан или требуется оплата (HTTP 402)")
 case 404,410:return "",errors.New("модель не найдена или удалена у провайдера (HTTP 404/410)")
 case 422:return "",errors.New("API отклонил параметры запроса (HTTP 422)")
 case 429:return "",errors.New("бесплатная квота или лимит частоты исчерпаны (HTTP 429)")
 }
 if response.StatusCode>=500{return "",fmt.Errorf("сервер ИИ временно недоступен (HTTP %d)",response.StatusCode)}
 if response.StatusCode<200||response.StatusCode>299{return "",fmt.Errorf("API отклонил запрос (HTTP %d)",response.StatusCode)}
 var result map[string]any;if err=json.Unmarshal(data,&result);err!=nil{return "",err}
 if spec.Type=="gemini"{
  if candidates,ok:=result["candidates"].([]any);ok&&len(candidates)>0{
   if candidate,ok:=candidates[0].(map[string]any);ok{
    if content,ok:=candidate["content"].(map[string]any);ok{
     if parts,ok:=content["parts"].([]any);ok{
      var text strings.Builder;for _,part:=range parts{if p,ok:=part.(map[string]any);ok{if v,ok:=p["text"].(string);ok{text.WriteString(v)}}}
      if text.Len()>0{return text.String(),nil}
     }
    }
   }
  }
 }else if spec.Type=="cloudflare"{
  if output,ok:=result["result"].(map[string]any);ok {
   if text,ok:=output["response"].(string);ok&&text!=""{return text,nil}
   if text,ok:=output["output"].(string);ok&&text!=""{return text,nil}
  }
 }else if choices,ok:=result["choices"].([]any);ok&&len(choices)>0{
  if choice,ok:=choices[0].(map[string]any);ok{
   if msg,ok:=choice["message"].(map[string]any);ok{
    if text,ok:=msg["content"].(string);ok&&strings.TrimSpace(text)!=""{return text,nil}
    // Some OpenAI-compatible free providers return text blocks as a
    // content array instead of a plain string. Do not mistake these
    // successful responses for "model unavailable".
    if parts,ok:=msg["content"].([]any);ok{
     var combined strings.Builder
     for _,part:=range parts{
      item,ok:=part.(map[string]any);if !ok{continue}
      if kind,ok:=item["type"].(string);ok&&kind!="text"&&kind!="output_text"{continue}
      if value,ok:=item["text"].(string);ok{combined.WriteString(value)}
     }
     if combined.Len()>0{return combined.String(),nil}
    }
   }
   if reason,ok:=choice["finish_reason"].(string);ok&&reason=="length"{
    return "",errors.New("API не вернул текст: лимит выходных токенов израсходован")
   }
  }
 }
 return "",errors.New("API не вернул текст")
}
