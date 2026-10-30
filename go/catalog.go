package main

import (
 "context"
 _ "embed"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "regexp"
 "sort"
 "strings"
 "sync"
 "time"
)

//go:embed providers.json
var registryJSON []byte

type Model struct {ID string; Label string}
type ProviderSpec struct {
 ID string `json:"id"`
 Name string `json:"name"`
 KeyURL string `json:"key_url"`
 Type string `json:"type"`
 Endpoint string `json:"endpoint"`
 CatalogURL string `json:"catalog_url"`
 FreePolicy string `json:"free_policy"`
 AnonymousKey string `json:"anonymous_key"`
 Models [][]string `json:"models"`
}
type Catalog struct {
 Specs map[string]ProviderSpec
 Order []string
 verified map[string][]Model
 mu sync.RWMutex
 client *http.Client
 health *ModelHealth
}
// Cloudflare Workers AI model IDs begin with @cf/. Preserve this safe prefix.
var safeModelID=regexp.MustCompile(`^(?:[A-Za-z0-9]|@[A-Za-z0-9])[A-Za-z0-9_.:/@+\-]{0,178}$`)
func newCatalog() (*Catalog,error) {
 var blob struct {Providers []ProviderSpec `json:"providers"`};if err:=json.Unmarshal(registryJSON,&blob);err!=nil{return nil,err}
 c:=&Catalog{Specs:map[string]ProviderSpec{},verified:map[string][]Model{},client:&http.Client{Timeout:20*time.Second}}
 for _,p:=range blob.Providers {
  if p.ID==""||!validHTTPS(p.KeyURL)|| (p.Type=="openai"&&!validHTTPS(p.Endpoint)) {return nil,fmt.Errorf("неверный провайдер %q",p.ID)}
  c.Specs[p.ID]=p;c.Order=append(c.Order,p.ID)
 }
 return c,nil
}
func splitModel(key string)(string,string,error){
 bits:=strings.SplitN(key,":",2);if len(bits)!=2||!safeModelID.MatchString(bits[1]){return "","",errors.New("неверный ID модели")}
 return bits[0],bits[1],nil
}
func (c *Catalog) modelAllowedID(provider,id string) bool {
 spec,ok:=c.Specs[provider];if !ok||!safeModelID.MatchString(id){return false}
 switch spec.FreePolicy {
 case "curated":for _,m:=range spec.Models{if len(m)>=1&&m[0]==id{return true}};return false
 case "priced_openrouter":return strings.HasSuffix(id,":free")&&russianCandidate(id)
 case "priced_pollinations":return russianCandidate(id)
 case "volunteer":return false
 default:return false
 }
}
// Dynamic catalogs cannot prove language ability from prices or model IDs alone.
// Only shortlist broadly multilingual text families; an authenticated Russian
// probe and response guard perform the functional language check.
func russianCandidate(id string)bool{
 name:=strings.ToLower(id)
 for _,family:=range []string{"gpt-oss","gemini","qwen","glm-","deepseek","mistral","gemma","llama","kimi","minimax","nemotron"}{
  if strings.Contains(name,family){return true}
 }
 return false
}
func (c *Catalog) label(key string)string {
 provider,model,_:=splitModel(key);spec:=c.Specs[provider]
 for _,m:=range spec.Models{if len(m)>1&&m[0]==model{return spec.Name+" • "+m[1]}}
 return spec.Name+" • "+model
}
func (c *Catalog) candidates(cfg Config) []string {
 c.mu.RLock();defer c.mu.RUnlock()
 result:=[]string{};seen:=map[string]bool{}
 for _,name:=range c.Order {
  spec:=c.Specs[name];settings,ok:=cfg.Providers[name];if !ok||!settings.Enabled||settings.APIKey==""&&spec.AnonymousKey==""{continue}
  if name=="cloudflare"&&settings.CloudflareAccount==""{continue}
  verified:=map[string]bool{};for _,m:=range c.verified[name]{verified[m.ID]=true}
  add:=func(model string) {
   key:=name+":"+model
   if !seen[key]&&!contains(cfg.RussianBlockedModels,key)&&c.modelAllowedID(name,model){seen[key]=true;result=append(result,key)}
  }
  for _,key:=range cfg.EnabledModels {
   p,id,err:=splitModel(key);if err!=nil||p!=name||!c.modelAllowedID(p,id){continue}
   if spec.FreePolicy=="curated"||verified[id]{add(id)}
  }
  auto:=false;for _,p:=range cfg.AutoModelsProviders{if p==name{auto=true;break}}
  if auto {
   if spec.FreePolicy=="curated"{for _,m:=range spec.Models{add(m[0])}
   }else{
    for _,m:=range c.verified[name]{add(m.ID)}
   }
  }
 }
 return result
}
// In production a model is selectable only after a successful authenticated
// test from this server. Bare Catalog instances in unit tests have no health
// store and preserve the historical catalogue behavior.
func (c *Catalog) enabled(cfg Config) []string {
 candidates:=c.candidates(cfg)
 if c.health==nil{return candidates}
 ready:=make([]string,0,len(candidates))
 for _,key:=range candidates {
  if c.health.available(cfg,key){ready=append(ready,key)}
 }
 return ready
}
func (c *Catalog) defaultModel(cfg Config)string {allowed:=c.enabled(cfg);for _,k:=range allowed{if k==cfg.DefaultModel{return k}};if len(allowed)>0{return allowed[0]};return ""}
func freeZero(v any)bool {switch n:=v.(type){case string:return n=="0"||n=="0.0"||n=="0.000";case float64:return n==0;case json.Number:return n.String()=="0";default:return false}}
func freeItem(p ProviderSpec,item map[string]any)bool{
 if item["paid_only"]==true||item["is_premium"]==true||item["requires_paid"]==true||item["premium"]==true{return false}
 price,_:=item["pricing"].(map[string]any)
 switch p.FreePolicy {
 case "priced_openrouter":
  id,_:=item["id"].(string);if !strings.HasSuffix(id,":free"){return false}
  if !freeZero(price["prompt"])||!freeZero(price["completion"]){return false}
  if arch,ok:=item["architecture"].(map[string]any);ok{
   if modalities,ok:=arch["output_modalities"].([]any);ok{
    for _,m:=range modalities{if m=="text"{return true}};return false
   }
  }
  return true
 case "priced_pollinations":
  if freeZero(price["input"])&&freeZero(price["output"]){return true}
  if freeZero(price["prompt"])&&freeZero(price["completion"]){return true}
  if txt,ok:=price["text"].(map[string]any);ok{return freeZero(txt["input"])&&freeZero(txt["output"])}
  return false
 case "volunteer":return true
 }
 return false
}
func (c *Catalog) discover(ctx context.Context,p ProviderSpec,key string)([]Model,error) {
 req,err:=http.NewRequestWithContext(ctx,"GET",p.CatalogURL,nil);if err!=nil{return nil,err}
 if key!=""{req.Header.Set("Authorization","Bearer "+key)}
 response,err:=c.client.Do(req);if err!=nil{return nil,err};defer response.Body.Close()
 if response.StatusCode!=200{return nil,fmt.Errorf("HTTP %d",response.StatusCode)}
 data,err:=io.ReadAll(io.LimitReader(response.Body,2<<20));if err!=nil{return nil,err}
 var root any;if err=json.Unmarshal(data,&root);err!=nil{return nil,err}
 items,ok:=root.([]any);if !ok {
  entry,good:=root.(map[string]any);if !good{return nil,errors.New("неверный каталог")}
  for _,key:=range []string{"data","models","items"}{if list,yes:=entry[key].([]any);yes{items=list;break}}
  if items==nil{return nil,errors.New("нет списка моделей")}
 }
 models:=[]Model{};found:=map[string]bool{}
 for _,raw:=range items {
  item,ok:=raw.(map[string]any);if !ok||!freeItem(p,item){continue}
  id,_:=item["id"].(string);if id==""{id,_=item["name"].(string)}
  if !safeModelID.MatchString(id)||found[id]||!c.modelAllowedID(p.ID,id){continue}
  found[id]=true;title,_:=item["display_name"].(string);if title==""{title,_=item["name"].(string)}
  if title==""{title=id};if len(title)>80{title=title[:80]}
  models=append(models,Model{id,title});if len(models)>=150{break}
 }
 sort.Slice(models,func(i,j int)bool{return models[i].Label<models[j].Label})
 return models,nil
}
// Refresh dynamic catalogs in parallel. A slow/offline provider must not
// delay all other providers or VK startup by multiple 20-second timeouts.
func (c *Catalog) refresh(ctx context.Context,cfg Config)map[string]string {
 errorsBy:=map[string]string{}
 targets:=map[string]bool{}
 for _,p:=range cfg.AutoModelsProviders{targets[p]=true}
 for _,key:=range cfg.EnabledModels{
  p,_,err:=splitModel(key);if err==nil{targets[p]=true}
 }
 var wg sync.WaitGroup
 var errorMu sync.Mutex
 for _,name:=range c.Order {
  p:=c.Specs[name]
  if !targets[name]||p.FreePolicy=="curated"{continue}
  wg.Add(1)
  go func(name string,p ProviderSpec){
   defer wg.Done()
   settings:=cfg.Providers[name]
   key:=settings.APIKey;if key==""{key=p.AnonymousKey}
   var models []Model
   var err error
   if settings.Enabled&&key!=""{models,err=c.discover(ctx,p,key)}
   c.mu.Lock()
   // Temporary catalog outages must not discard previously verified models.
   if err==nil&&settings.Enabled&&key!=""{c.verified[name]=models}
   c.mu.Unlock()
   if err!=nil{
    errorMu.Lock();errorsBy[name]=err.Error();errorMu.Unlock()
   }
  }(name,p)
 }
 wg.Wait()
 return errorsBy
}
