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
 "sort"
 "strings"
 "time"
 "unicode/utf8"
)

type Result struct{Title,Body,URL string}

// Production uses fixed endpoints; variables permit isolated httptest coverage.
var duckEndpoint="https://html.duckduckgo.com/html/"
var wikiEndpoint="https://ru.wikipedia.org/w/api.php"
var searchHTTPClient=&http.Client{Timeout:12*time.Second}

var searchIntent=regexp.MustCompile(`(?i)(найди|поищи|проверь|новости|погугли|ссылки|источники|в интернете|за сегодня|за вчера|актуальн)`)
var anchors=regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
var hrefAttribute=regexp.MustCompile(`(?is)\bhref\s*=\s*["']([^"']+)["']`)
var classAttribute=regexp.MustCompile(`(?is)\bclass\s*=\s*["']([^"']+)["']`)
var tagsRe=regexp.MustCompile(`(?s)<[^>]+>`)
var snippetRe=regexp.MustCompile(`(?is)<(?:a|div|span)\b([^>]*\bclass=["'][^"']*result__snippet[^"']*["'][^>]*)>(.*?)</(?:a|div|span)>`)

func needsSearch(q string)bool{return searchIntent.MatchString(q)}
func cleanupHTML(s string)string{return strings.Join(strings.Fields(html.UnescapeString(tagsRe.ReplaceAllString(s," ")))," ")}
func searchURL(raw string)string{
 raw=html.UnescapeString(strings.TrimSpace(raw))
 if strings.HasPrefix(raw,"//"){raw="https:"+raw}
 parsed,err:=url.Parse(raw)
 if err!=nil{return ""}
 if strings.EqualFold(parsed.Hostname(),"duckduckgo.com")||strings.EqualFold(parsed.Hostname(),"html.duckduckgo.com"){
  if actual:=parsed.Query().Get("uddg");actual!=""{
   parsed,err=url.Parse(actual)
   if err!=nil{return ""}
  }else{return ""}
 }
 if (parsed.Scheme!="http"&&parsed.Scheme!="https")||parsed.Hostname()==""||parsed.User!=nil{return ""}
 return parsed.String()
}
func parseDuckHTML(raw []byte)[]Result {
 // DDG may reorder attributes; do not assume href follows class.
 snippets:=[]string{}
 for _,snippet:=range snippetRe.FindAllSubmatch(raw,10){snippets=append(snippets,cleanupHTML(string(snippet[2])))}
 results:=[]Result{}
 for _,a:=range anchors.FindAllSubmatch(raw,150){
  attrs:=string(a[1]);class:=classAttribute.FindStringSubmatch(attrs)
  if len(class)<2||!strings.Contains(" "+class[1]+" "," result__a "){continue}
  href:=hrefAttribute.FindStringSubmatch(attrs);if len(href)<2{continue}
  link:=searchURL(href[1]);if link==""{continue}
  title:=cleanupHTML(string(a[2]));if title==""{continue}
  body:="";if len(snippets)>len(results){body=snippets[len(results)]}
  results=append(results,Result{Title:title,Body:body,URL:link})
  if len(results)>=4{break}
 }
 return results
}
func duckResults(ctx context.Context,q string)[]Result {
 addr,err:=url.Parse(duckEndpoint);if err!=nil{return nil}
 params:=addr.Query();params.Set("q",q);addr.RawQuery=params.Encode()
 req,err:=http.NewRequestWithContext(ctx,"GET",addr.String(),nil);if err!=nil{return nil}
 req.Header.Set("User-Agent","Mozilla/5.0 (compatible; AI-bot/1.0)")
 response,err:=searchHTTPClient.Do(req);if err!=nil{return nil};defer response.Body.Close()
 if response.StatusCode!=http.StatusOK{return nil}
 raw,err:=io.ReadAll(io.LimitReader(response.Body,512<<10));if err!=nil{return nil}
 return parseDuckHTML(raw)
}
func wikiResults(ctx context.Context,q string)[]Result{
 endpoint,err:=url.Parse(wikiEndpoint);if err!=nil{return nil}
 params:=endpoint.Query()
 params.Set("action","query");params.Set("list","search")
 params.Set("format","json");params.Set("srlimit","4");params.Set("srsearch",q)
 endpoint.RawQuery=params.Encode()
 req,err:=http.NewRequestWithContext(ctx,"GET",endpoint.String(),nil);if err!=nil{return nil}
 req.Header.Set("User-Agent","AI-bot/1.0 (community assistant; public search)")
 response,err:=searchHTTPClient.Do(req);if err!=nil{return nil};defer response.Body.Close()
 if response.StatusCode!=200{return nil}
 var parsed struct{Query struct{Search []struct{
  Title string `json:"title"`
  Snippet string `json:"snippet"`
 } `json:"search"`} `json:"query"`}
 if json.NewDecoder(io.LimitReader(response.Body,256<<10)).Decode(&parsed)!=nil{return nil}
 output:=[]Result{}
 for _,item:=range parsed.Query.Search {
  if strings.TrimSpace(item.Title)==""{continue}
  output=append(output,Result{
   Title:item.Title+" (Википедия)",
   Body:cleanupHTML(item.Snippet),
   URL:"https://ru.wikipedia.org/wiki/"+url.PathEscape(strings.ReplaceAll(item.Title," ","_")),
  })
  if len(output)>=4{break}
 }
 return output
}
// Query independent sources concurrently. The result URLs are references,
// never proof that a page was fetched, and the model is told not to invent
// publication dates or claim that results are necessarily current.
func webSearch(ctx context.Context,q string)[]Result{
 q=strings.TrimSpace(q);if q==""{return nil}
 if utf8.RuneCountInString(q)>250{q=string([]rune(q)[:250])}
 child,cancel:=context.WithTimeout(ctx,10*time.Second);defer cancel()
 results:=make(chan []Result,2)
 for _,fetch:=range []func(context.Context,string)[]Result{duckResults,wikiResults}{
  fn:=fetch
  go func(){results<-fn(child,q)}()
 }
 merged:=[]Result{}
 seen:=map[string]bool{}
 for i:=0;i<2;i++{
  var next []Result
  if i==0{
   select{case next=<-results:case <-child.Done():return merged}
  }else{
   // Avoid making users wait for one sluggish secondary search provider.
   grace:=time.NewTimer(1500*time.Millisecond)
   select{
   case next=<-results:
   case <-grace.C:
    grace.Stop();return merged
   case <-child.Done():
    grace.Stop();return merged
   }
   grace.Stop()
  }
  for _,item:=range next{
   link:=searchURL(item.URL);if link==""{continue}
   parsed,err:=url.Parse(link);if err!=nil{continue}
   parsed.Fragment=""
   dedup:=strings.ToLower(strings.TrimSuffix(parsed.String(),"/"))
   if seen[dedup]{continue}
   seen[dedup]=true
   item.URL=link
   item.Title=safeExcerpt(item.Title,140)
   item.Body=safeExcerpt(item.Body,350)
   merged=append(merged,item)
  }
 }
 words:=strings.Fields(strings.ToLower(q))
 rank:=func(item Result)int{
  corpus:=strings.ToLower(item.Title+" "+item.Body)
  score:=0
  for _,word:=range words{
   if len([]rune(word))<4{continue}
   if strings.Contains(corpus,word){score++}
   if strings.Contains(strings.ToLower(item.Title),word){score+=2}
  }
  return score
 }
 sort.SliceStable(merged,func(i,j int)bool{return rank(merged[i])>rank(merged[j])})
 if len(merged)>6{merged=merged[:6]}
 return merged
}
func sourcesText(results []Result)string {
 var b strings.Builder
 for i,r:=range results {fmt.Fprintf(&b,"%d. %s\n%s\n%s\n",i+1,r.Title,r.Body,r.URL)}
 return b.String()
}
