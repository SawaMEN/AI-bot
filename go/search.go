package main

import (
 "context"
 "encoding/json"
 "fmt"
 "html"
 "io"
 "net/http"
 "net/url"
 "regexp"
 "strings"
 "time"
)

type Result struct{Title,Body,URL string}
var searchIntent=regexp.MustCompile(`(?i)(найди|поищи|проверь|новости|погугли|ссылки|источники|в интернете|за сегодня|актуальн)`)
var linkRe=regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
var snippetRe=regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
var tagsRe=regexp.MustCompile(`<[^>]+>`)
func needsSearch(q string)bool{return searchIntent.MatchString(q)}
func cleanupHTML(s string)string{return strings.TrimSpace(html.UnescapeString(tagsRe.ReplaceAllString(s," ")))}
func webSearch(ctx context.Context,q string)[]Result {
 if len(q)>250{q=q[:250]}
 req,err:=http.NewRequestWithContext(ctx,"GET","https://html.duckduckgo.com/html/?q="+url.QueryEscape(q),nil);if err!=nil{return nil}
 req.Header.Set("User-Agent","Mozilla/5.0 (compatible; AI-bot/1.0)")
 client:=&http.Client{Timeout:12*time.Second};resp,err:=client.Do(req);if err!=nil{return nil};defer resp.Body.Close()
 if resp.StatusCode!=200{return nil}
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,512<<10));if err!=nil{return nil}
 htmlText:=string(raw);links:=linkRe.FindAllStringSubmatch(htmlText,5);snips:=snippetRe.FindAllStringSubmatch(htmlText,5)
 result:=[]Result{};for i,link:=range links {
  target:=html.UnescapeString(link[1]);parsed,err:=url.Parse(target);if err!=nil{continue}
  if parsed.Host=="duckduckgo.com"||parsed.Host=="html.duckduckgo.com" {
   if real:=parsed.Query().Get("uddg");real!=""{target=real}
  }
  parsed,err=url.Parse(target);if err!=nil||parsed.Scheme!="https"&&parsed.Scheme!="http"{continue}
  body:="";if i<len(snips){body=cleanupHTML(snips[i][1])}
  result=append(result,Result{cleanupHTML(link[2]),body,target})
 }
 return result
}
func sourcesText(results []Result)string {
 var b strings.Builder;for i,r:=range results{
  fmt.Fprintf(&b,"%d. %s\n%s\n%s\n",i+1,r.Title,r.Body,r.URL)
 };return b.String()
}
func publicInfo(ctx context.Context,q string)string{
 // A lightweight optional fallback for search engines that block scraping.
 // No credentials, no private data, no arbitrary URL fetching.
 endpoint:="https://ru.wikipedia.org/w/api.php?action=query&list=search&format=json&srlimit=3&srsearch="+url.QueryEscape(q)
 req,err:=http.NewRequestWithContext(ctx,"GET",endpoint,nil);if err!=nil{return ""}
 response,err:=(&http.Client{Timeout:8*time.Second}).Do(req);if err!=nil{return ""};defer response.Body.Close()
 if response.StatusCode!=200{return ""}
 var payload struct{Query struct{Search []struct{Title string `json:"title"`;Snippet string `json:"snippet"`} `json:"search"`} `json:"query"`}
 if json.NewDecoder(io.LimitReader(response.Body,128<<10)).Decode(&payload)!=nil{return ""}
 if len(payload.Query.Search)==0{return ""}
 var b strings.Builder;for _,item:=range payload.Query.Search{
  fmt.Fprintf(&b,"%s — %s (Википедия)\n",item.Title,cleanupHTML(item.Snippet))
 }
 return b.String()
}
