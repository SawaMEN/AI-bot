package main

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "time"
)

const vkVersion="5.199"
type VK struct{token string;groupID int64;client *http.Client}
func newVK(cfg Config)*VK{return &VK{cfg.VKToken,cfg.GroupID,&http.Client{Timeout:40*time.Second}}}
func (v *VK) call(ctx context.Context,method string,args url.Values)(json.RawMessage,error){
 if args==nil{args=url.Values{}};args.Set("access_token",v.token);args.Set("v",vkVersion)
 req,err:=http.NewRequestWithContext(ctx,"POST","https://api.vk.com/method/"+method,strings.NewReader(args.Encode()))
 if err!=nil{return nil,err};req.Header.Set("Content-Type","application/x-www-form-urlencoded")
 response,err:=v.client.Do(req);if err!=nil{return nil,err};defer response.Body.Close()
 if response.StatusCode!=200{return nil,fmt.Errorf("ВК HTTP %d",response.StatusCode)}
 raw,err:=io.ReadAll(io.LimitReader(response.Body,2<<20));if err!=nil{return nil,err}
 var payload struct{Response json.RawMessage `json:"response"`;Error *struct{Message string `json:"error_msg"`} `json:"error"`}
 if err=json.Unmarshal(raw,&payload);err!=nil{return nil,err}
 if payload.Error!=nil{return nil,errors.New("ВК: "+payload.Error.Message)}
 return payload.Response,nil
}
func stableID(id string)int64 {
 h:=sha256.Sum256([]byte(id));result:=int64(h[0])<<24|int64(h[1])<<16|int64(h[2])<<8|int64(h[3])
 return result&0x7fffffff
}
func (v *VK) send(ctx context.Context,peer int64,text,buttons string,stable string)error{
 if len([]rune(text))>3500{text=string([]rune(text)[:3500])}
 id:=time.Now().UnixNano()&0x7fffffff;if stable!=""{id=stableID(stable)}
 vals:=url.Values{"peer_id":{strconv.FormatInt(peer,10)},"random_id":{strconv.FormatInt(id,10)},"message":{text}}
 if buttons!=""{vals.Set("keyboard",buttons)}
 _,err:=v.call(ctx,"messages.send",vals);return err
}
func (v *VK) answerButton(ctx context.Context,peer,user int64,id string)error{
 data,_:=json.Marshal(map[string]string{"type":"show_snackbar","text":"Настройка открыта"})
 vals:=url.Values{"peer_id":{strconv.FormatInt(peer,10)},"user_id":{strconv.FormatInt(user,10)},"event_id":{id},"event_data":{string(data)}}
 _,err:=v.call(ctx,"messages.sendMessageEventAnswer",vals);return err
}
func (v *VK) admin(ctx context.Context,peer,user int64)bool{
 vals:=url.Values{"peer_id":{strconv.FormatInt(peer,10)},"group_id":{strconv.FormatInt(v.groupID,10)}}
 raw,err:=v.call(ctx,"messages.getConversationMembers",vals);if err!=nil{return false}
 var data struct{Items []struct{
  MemberID int64 `json:"member_id"`
  IsAdmin bool `json:"is_admin"`
  IsOwner bool `json:"is_owner"`
  CanKick bool `json:"can_kick"`
  Role string `json:"role"`
 } `json:"items"`}
 if json.Unmarshal(raw,&data)!=nil{return false}
 for _,m:=range data.Items{if m.MemberID==user{return m.IsAdmin||m.IsOwner||m.CanKick||m.Role=="admin"||m.Role=="owner"}}
 return false
}
var aiTrigger=regexp.MustCompile(`(?i)^/ai(?:$|[ ,:;.!?])`)
func questionFor(cfg Config,m VKMessage) (string,bool) {
 if !m.Valid(){return "",false}
 text:=strings.TrimSpace(m.Text)
 if strings.HasPrefix(strings.ToLower(text),strings.ToLower(cfg.Trigger)+" ")||strings.EqualFold(text,cfg.Trigger) {
  return strings.TrimSpace(strings.TrimLeft(text[len([]byte(cfg.Trigger)):]," ,.:;!?")),true
 }
 if aiTrigger.MatchString(text){return strings.TrimSpace(strings.TrimLeft(text[3:]," ,.:;!?")),true}
 group:=strconv.FormatInt(cfg.GroupID,10)
 re:=regexp.MustCompile(`(?i)\[(club|public)`+group+`\|[^]]+\]|@(club|public)`+group+`\b`)
 loc:=re.FindStringIndex(text)
 if loc!=nil{
  before:=strings.TrimRight(text[:loc[0]]," ,.:;!?")
  after:=strings.TrimLeft(text[loc[1]:]," ,.:;!?")
  return strings.TrimSpace(before+" "+after),true
 }
 if m.ReplyMessage!=nil&&m.ReplyMessage.FromID==-cfg.GroupID{return text,true}
 return "",false
}
func keyboard(rows [][]struct{Label,Command string})string{
 if len(rows)>6{return ""}
 buttons:=[][]any{}
 for _,row:=range rows{
  line:=[]any{}
  for _,b:=range row{
   payload,_:=json.Marshal(map[string]any{"aibot":1,"command":b.Command})
   line=append(line,map[string]any{"action":map[string]string{"type":"callback","label":b.Label,"payload":string(payload)}})
  }
  buttons=append(buttons,line)
 }
 raw,_:=json.Marshal(map[string]any{"inline":true,"buttons":buttons});return string(raw)
}
type Btn struct{Label,Command string}
func board(rows [][]Btn)string{
 typed:=[][]struct{Label,Command string}{};for _,row:=range rows{
  line:=[]struct{Label,Command string}{};for _,b:=range row{line=append(line,struct{Label,Command string}{b.Label,b.Command})};typed=append(typed,line)
 };return keyboard(typed)
}
func mainBoard()string{
 return board([][]Btn{
  {{"Выбрать ИИ","модели 1"},{"Мои настройки","настройки"}},
  {{"Поиск","выбор поиска"},{"Стиль ответа","выбор стиля"}},
  {{"Контекст","выбор контекста"},{"Настройки беседы","диалог"}},
  {{"Резервный ИИ","выбор резерва"},{"Помощь","помощь"}},
  {{"Права и подключение","права"}},
 })
}
func chatBoard()string {
 return board([][]Btn{
  {{"Модель беседы","диалог модели 1"},{"Поиск","диалог выбор поиска"}},
  {{"Стиль","диалог выбор стиля"},{"Контекст","диалог выбор контекста"}},
  {{"Резерв","диалог выбор резерва"},{"Сбросить беседу","диалог сброс"}},
  {{"Меню","меню"}},
 })
}
func modelBoard(c *Catalog,cfg Config,page int,chat bool)string{
 allowed:=c.enabled(cfg);if page<1{page=1};maxPage:=(len(allowed)+3)/4;if maxPage<1{maxPage=1};if page>maxPage{page=maxPage}
 rows:=[][]Btn{}
 for i:=(page-1)*4;i<min(page*4,len(allowed));i++{
  prefix:="";if chat{prefix="диалог "}
  label:=c.label(allowed[i]);if len([]rune(label))>37{label=string([]rune(label)[:37])}
  rows=append(rows,[]Btn{{fmt.Sprintf("%d. %s",i+1,label),fmt.Sprintf("%sмодель %d",prefix,i+1)}})
 }
 nav:=[]Btn{};prefix:="модели ";if chat{prefix="диалог модели "}
 if page>1{nav=append(nav,Btn{"Назад",prefix+strconv.Itoa(page-1)})}
 if page<maxPage{nav=append(nav,Btn{"Далее",prefix+strconv.Itoa(page+1)})}
 if len(nav)>0{rows=append(rows,nav)}
 rows=append(rows,[]Btn{{"Меню","меню"}})
 return board(rows)
}
func choiceBoard(prefix,name string,cfg Config)string{
 rows:=[][]Btn{}
 switch name {
 case "поиска":rows=[][]Btn{{{"Авто",prefix+"поиск авто"},{"Искать всегда",prefix+"поиск вкл"}},{{"Без поиска",prefix+"поиск выкл"}}}
 case "стиля":rows=[][]Btn{{{"Кратко",prefix+"стиль кратко"},{"Обычно",prefix+"стиль обычно"}},{{"Подробно",prefix+"стиль подробно"}}}
 case "контекста":
  values:=[]int{5,10,20,30,50,100};used:=map[int]bool{}
  for _,n:=range values{n=min(cfg.ContextMessages,n);if n<1||used[n]{continue};used[n]=true;rows=append(rows,[]Btn{{fmt.Sprintf("%d сообщений",n),fmt.Sprintf("%sконтекст %d",prefix,n)}});if len(rows)>=5{break}}
 case "резерва":rows=[][]Btn{{{"Разрешить резерв",prefix+"резерв вкл"},{"Запретить",prefix+"резерв выкл"}},{{"По умолчанию",prefix+"резерв авто"}}}
 }
 rows=append(rows,[]Btn{{"Меню","меню"}})
 return board(rows)
}
func buttonsFor(command string,c *Catalog,cfg Config)string{
 words:=strings.Fields(strings.ToLower(command));if len(words)==0{return mainBoard()}
 if words[0]=="меню"||words[0]=="помощь"||words[0]=="права"||words[0]=="настройки"{return mainBoard()}
 chat:=false;if words[0]=="диалог"{
  chat=true;words=words[1:];if len(words)==0{return chatBoard()}
 }
 prefix:="";if chat{prefix="диалог "}
 if words[0]=="модели"{
  page:=1;if len(words)>1{if i,err:=strconv.Atoi(words[1]);err==nil{page=i}}
  return modelBoard(c,cfg,page,chat)
 }
 if words[0]=="выбор"&&len(words)>1{return choiceBoard(prefix,words[1],cfg)}
 if words[0]=="поиск"{return choiceBoard(prefix,"поиска",cfg)}
 if words[0]=="стиль"{return choiceBoard(prefix,"стиля",cfg)}
 if words[0]=="контекст"{return choiceBoard(prefix,"контекста",cfg)}
 if words[0]=="резерв"{return choiceBoard(prefix,"резерва",cfg)}
 return mainBoard()
}
func callbackCommand(raw json.RawMessage)string{
 var obj struct {AIbot int `json:"aibot"`; Command string `json:"command"`}
 if json.Unmarshal(raw,&obj)!=nil||obj.AIbot!=1||len(obj.Command)>90{return ""}
 // Actions are still parsed against an allowlist by command handler, not as shell commands.
 if strings.ContainsAny(obj.Command,"<>{}\n\r") {return ""}
 return obj.Command
}
func vkPoll(ctx context.Context,v *VK,s *Store,onMessage func(VKMessage),onButton func(int64,int64,string,json.RawMessage))error{
 serverURL,key,ts:="","","";fails:=0
 for ctx.Err()==nil{
  if serverURL==""{
   vals:=url.Values{"group_id":{strconv.FormatInt(v.groupID,10)}}
   raw,err:=v.call(ctx,"groups.getLongPollServer",vals)
   if err!=nil{fails++;if !sleepContext(ctx,time.Duration(min(fails,6))*time.Second){return nil};continue}
   var server struct{Server string `json:"server"`;Key string `json:"key"`;TS string `json:"ts"`}
   if json.Unmarshal(raw,&server)!=nil||!validHTTPS(server.Server){return errors.New("неверный VK Long Poll server")}
   serverURL,key,ts=server.Server,server.Key,server.TS;if saved:=s.cursor(v.groupID);saved!=""{ts=saved}
  }
  vals:=url.Values{"act":{"a_check"},"key":{key},"ts":{ts},"wait":{"25"}}
  req,err:=http.NewRequestWithContext(ctx,"GET",serverURL+"?"+vals.Encode(),nil);if err!=nil{return err}
  response,err:=v.client.Do(req)
  if err!=nil{serverURL="";if !sleepContext(ctx,3*time.Second){return nil};continue}
  data,err:=io.ReadAll(io.LimitReader(response.Body,4<<20));response.Body.Close()
  if err!=nil||response.StatusCode!=200{serverURL="";continue}
  var batch struct{Failed int `json:"failed"`;TS string `json:"ts"`;Updates []struct{Type string `json:"type"`;Object json.RawMessage `json:"object"`} `json:"updates"`}
  if json.Unmarshal(data,&batch)!=nil{serverURL="";continue}
  if batch.Failed!=0{
   if batch.Failed==1{ts=batch.TS;s.setCursor(v.groupID,ts)}else{if batch.Failed==3{s.clearCursor(v.groupID)};serverURL=""};continue
  }
  fails=0
  for _,evt:=range batch.Updates{
   switch evt.Type {
   case "message_new":
    var envelope struct {Message VKMessage `json:"message"`}
    if json.Unmarshal(evt.Object,&envelope)==nil {onMessage(envelope.Message)}
   case "message_event":
    var e struct{PeerID int64 `json:"peer_id"`;UserID int64 `json:"user_id"`;EventID string `json:"event_id"`;Payload json.RawMessage `json:"payload"`}
    if json.Unmarshal(evt.Object,&e)==nil{onButton(e.PeerID,e.UserID,e.EventID,e.Payload)}
   }
  }
  ts=batch.TS;s.setCursor(v.groupID,ts)
 }
 return nil
}
func sleepContext(ctx context.Context,d time.Duration)bool{select{case <-ctx.Done():return false;case <-time.After(d):return true}}
