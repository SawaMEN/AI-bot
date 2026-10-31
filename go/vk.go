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
 "sync"
 "time"
)

const vkVersion="5.199"
var vkAPIBaseURL="https://api.vk.com/method/"
type VK struct{token string;groupID int64;client *http.Client}
func newVK(cfg Config)*VK{return &VK{cfg.VKToken,cfg.GroupID,&http.Client{Timeout:40*time.Second}}}
func (v *VK) call(ctx context.Context,method string,args url.Values)(json.RawMessage,error){
 if args==nil{args=url.Values{}};args.Set("access_token",v.token);args.Set("v",vkVersion)
 req,err:=http.NewRequestWithContext(ctx,"POST",vkAPIBaseURL+method,strings.NewReader(args.Encode()))
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
// A callback normally comes from the bot's own message. Refresh that
// keyboard in place instead of filling the group chat with menu messages.
// VK API may refuse editing old messages; caller falls back to messages.send.
func (v *VK) editMenu(ctx context.Context,peer,conversationMessageID int64,text,buttons string)error{
 if peer<=0||conversationMessageID<=0{return errors.New("нет номера сообщения меню")}
 if len([]rune(text))>3500{text=string([]rune(text)[:3500])}
 vals:=url.Values{
  "peer_id":{strconv.FormatInt(peer,10)},
  "conversation_message_id":{strconv.FormatInt(conversationMessageID,10)},
  "message":{text},
 }
 if buttons!=""{vals.Set("keyboard",buttons)}
 _,err:=v.call(ctx,"messages.edit",vals)
 return err
}
// startThinking periodically refreshes VK's 10-second typing indicator.
 // It never sends a chat message and stops when the answer is ready or the context ends.
func (v *VK) startThinking(ctx context.Context,peer int64)func(){
 return v.startThinkingWithInterval(ctx,peer,5*time.Second)
}
func (v *VK) startThinkingWithInterval(ctx context.Context,peer int64,interval time.Duration)func(){
 activityCtx,cancel:=context.WithCancel(ctx)
 done:=make(chan struct{})
 go func(){
  defer close(done)
  ticker:=time.NewTicker(interval)
  defer ticker.Stop()
  for {
   if activityCtx.Err()!=nil{return}
   requestCtx,stop:=context.WithTimeout(activityCtx,3*time.Second)
   _,_ = v.call(requestCtx,"messages.setActivity",url.Values{
    "peer_id":{strconv.FormatInt(peer,10)}, "type":{"typing"},
   })
   stop()
   select{case <-activityCtx.Done():return;case <-ticker.C:}
  }
 }()
 var once sync.Once
 return func(){once.Do(func(){cancel();<-done})}
}
func (v *VK) answerButton(ctx context.Context,peer,user int64,id string)error{
 data,_:=json.Marshal(map[string]string{"type":"show_snackbar","text":"Обновляю меню"})
 vals:=url.Values{"peer_id":{strconv.FormatInt(peer,10)},"user_id":{strconv.FormatInt(user,10)},"event_id":{id},"event_data":{string(data)}}
 _,err:=v.call(ctx,"messages.sendMessageEventAnswer",vals);return err
}
// setLongPollEvents changes only requested events; it is called only after
// explicit confirmation in the local administrator menu. Community tokens
// may lack permissions; a VK API error must never be concealed.
func (v *VK) setLongPollEvents(ctx context.Context) error {
 if v.groupID<1||v.token==""{return errors.New("укажите токен и ID сообщества")}
 raw,err:=v.call(ctx,"groups.setLongPollSettings",url.Values{
  "group_id":{strconv.FormatInt(v.groupID,10)},
  "enabled":{"1"},"message_new":{"1"},"message_event":{"1"},
 })
 if err!=nil{return err}
 var number int
 if json.Unmarshal(raw,&number)==nil&&number==1{return nil}
 var boolean bool
 if json.Unmarshal(raw,&boolean)==nil&&boolean{return nil}
 return errors.New("ВК не подтвердил изменение событий Long Poll")
}

// Some VK API versions return 0/1, others true/false for event flags.
func vkEnabled(raw json.RawMessage)bool{
 var boolean bool
 if json.Unmarshal(raw,&boolean)==nil{return boolean}
 var number int
 if json.Unmarshal(raw,&number)==nil{return number!=0}
 var value string
 if json.Unmarshal(raw,&value)==nil{return value=="1"||strings.EqualFold(value,"true")}
 return false
}

func (v *VK) admin(ctx context.Context,peer,user int64)bool{
 vals:=url.Values{"peer_id":{strconv.FormatInt(peer,10)},"group_id":{strconv.FormatInt(v.groupID,10)}}
 raw,err:=v.call(ctx,"messages.getConversationMembers",vals);if err!=nil{return false}
 var data struct{Items []struct{
  MemberID int64 `json:"member_id"`
  IsAdmin bool `json:"is_admin"`
  IsOwner bool `json:"is_owner"`
  Role string `json:"role"`
 } `json:"items"`}
 if json.Unmarshal(raw,&data)!=nil{return false}
 for _,m:=range data.Items{if m.MemberID==user{return m.IsAdmin||m.IsOwner||m.Role=="admin"||m.Role=="owner"}}
 return false
}
var aiTrigger=regexp.MustCompile(`(?i)^/ai(?:$|[ ,:;.!?])`)
var mentionExpressions sync.Map

func mentionExpression(groupID int64)*regexp.Regexp {
 if cached,ok:=mentionExpressions.Load(groupID);ok{return cached.(*regexp.Regexp)}
 exp:=regexp.MustCompile(`(?i)\[(club|public)`+strconv.FormatInt(groupID,10)+`\|[^]]+\]|@(club|public)`+strconv.FormatInt(groupID,10)+`\b`)
 actual,_:=mentionExpressions.LoadOrStore(groupID,exp)
 return actual.(*regexp.Regexp)
}
func questionFor(cfg Config,m VKMessage) (string,bool) {
 if !m.Valid(){return "",false}
 text:=strings.TrimSpace(m.Text)
 // VK permanent text-keyboard buttons generate message_new rather than a
 // message_event. They carry a trusted *menu shape*, not a user identity.
 // Only use these in private chats to avoid hijacking normal group messages.
 if m.PeerID<2000000000&&m.FromID>0{
  if command:=vkReplyCommand(m.Payload);command!=""{return command,true}
  // Some clients omit payload. Accept exact known menu labels; arbitrary
  // words remain normal messages for the AI.
  if command:=vkReplyLabel(text);command!=""{return command,true}
 }
 if len(text)>=len(cfg.Trigger)&&strings.EqualFold(text[:len(cfg.Trigger)],cfg.Trigger) {
  rest:=text[len(cfg.Trigger):]
  if rest==""||strings.ContainsRune(" ,.:;!?\t\n",rune(rest[0])) {return strings.TrimSpace(strings.TrimLeft(rest," ,.:;!?")),true}
 }
 if aiTrigger.MatchString(text){return strings.TrimSpace(strings.TrimLeft(text[3:]," ,.:;!?")),true}
 loc:=mentionExpression(cfg.GroupID).FindStringIndex(text)
 if loc!=nil{
  before:=strings.TrimRight(text[:loc[0]]," ,.:;!?")
  after:=strings.TrimLeft(text[loc[1]:]," ,.:;!?")
  return strings.TrimSpace(before+" "+after),true
 }
 if m.ReplyMessage!=nil&&m.ReplyMessage.FromID==-cfg.GroupID{return text,true}
 // Inside a private conversation users expect "меню" and settings commands
 // to work without addressing the bot. Do not intercept group conversations.
 if m.PeerID<2000000000&&m.FromID>0 {
  words:=strings.Fields(strings.ToLower(text))
  if len(words)>0 {
   for _,command:=range []string{"меню","старт","помощь","help","настройки","ии","функции","модели","модель",
    "выбор","вопрос","поиск","стиль","контекст","резерв","сброс","права","разрешения","подключение"} {
    if words[0]==command{return text,true}
   }
  }
  // Private messages with the community are a natural one-on-one chat:
  // any other text is an AI question. Group chats still require an
  // explicit mention, /ai or a reply to the bot.
  return text,true
 }
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
// A single unobtrusive menu entry is attached to normal AI answers, so
// users do not need to remember commands or scroll up to old keyboards.
func answerBoard()string{
 return board([][]Btn{{{"⚙ Открыть меню","меню"}}})
}
// One entry point in VK, then concise context-sensitive screens.
// Buttons are stateless callback commands, so the menus survive bot restarts.
func mainBoard()string{
 return board([][]Btn{
  {{"🤖 ИИ","ии"}},
  {{"🧰 Функции","функции"}},
  {{"⚙ Настройки","настройки"},{"👥 Для беседы","диалог"}},
  {{"❓ Помощь","помощь"}},
 })
}
func aiBoard()string{
 return board([][]Btn{
  {{"🤖 Выбрать модель","модели"}},
  {{"✓ Автоматически","модель авто"}},
  {{"🏠 Главное меню","меню"}},
 })
}
func functionsBoard()string{
 return board([][]Btn{
  {{"🔎 Поиск","выбор поиска"},{"✍ Стиль ответа","выбор стиля"}},
  {{"🧠 Контекст","выбор контекста"},{"🔄 Резерв","выбор резерва"}},
  {{"🏠 Главное меню","меню"}},
 })
}
func personalBoard()string{
 return board([][]Btn{
  {{"🤖 ИИ","ии"}},
  {{"🧰 Функции","функции"}},
  {{"↺ Сбросить мои настройки","выбор сброса"}},
  {{"⌂ Главное меню","меню"},{"👥 Для беседы","диалог"}},
 })
}
func chatBoard()string{
 return board([][]Btn{
  {{"🤖 Модель беседы","диалог модели 1"}},
  {{"🔎 Поиск","диалог выбор поиска"},{"✍ Стиль","диалог выбор стиля"}},
  {{"🧠 Контекст","диалог выбор контекста"},{"🔄 Резерв","диалог выбор резерва"}},
  {{"↺ Сброс беседы","диалог выбор сброса"}},
  {{"⌂ Главное меню","меню"},{"⚙ Мои настройки","настройки"}},
 })
}
func helpBoard()string{
 return board([][]Btn{
  {{"🤖 ИИ","ии"},{"🧰 Функции","функции"}},
  {{"⌂ Главное меню","меню"}},
 })
}
func modelBoard(c *Catalog,cfg Config,page int,chat bool)string{
 allowed:=c.enabled(cfg)
 if page<1{page=1}
 maxPage:=(len(allowed)+2)/3;if maxPage<1{maxPage=1}
 if page>maxPage{page=maxPage}
 rows:=[][]Btn{}
 prefix:="";back:="настройки"
 if chat{prefix="диалог ";back="диалог"}
 if len(allowed)==0{
  return board([][]Btn{{{"← Назад",back},{"🏠 Меню","меню"}}})
 }
 for i:=(page-1)*3;i<min(page*3,len(allowed));i++{
  label:=c.label(allowed[i])
  if len([]rune(label))>32{label=string([]rune(label)[:31])+"…"}
  rows=append(rows,[]Btn{{fmt.Sprintf("%d. %s",i+1,label),fmt.Sprintf("%sмодель %d",prefix,i+1)}})
 }
 nav:=[]Btn{}
 route:="модели ";if chat{route="диалог модели "}
 if page>1{nav=append(nav,Btn{"◀ Назад",route+strconv.Itoa(page-1)})}
 if page<maxPage{nav=append(nav,Btn{"Далее ▶",route+strconv.Itoa(page+1)})}
 if len(nav)>0{rows=append(rows,nav)}
 rows=append(rows,[]Btn{{"✓ Авто • рекомендуется",prefix+"модель авто"}})
 if !chat{back="ии"}
 rows=append(rows,[]Btn{{"← Назад",back}})
 return board(rows)
}
func choiceBoard(prefix,name string,cfg Config)string{
 rows:=[][]Btn{}
 switch name {
 case "поиска":
  rows=[][]Btn{
   {{"✓ Авто (рекомендуется)",prefix+"поиск авто"}},
   {{"Искать всегда",prefix+"поиск вкл"},{"Не искать",prefix+"поиск выкл"}},
  }
 case "стиля":
  rows=[][]Btn{
   {{"Кратко",prefix+"стиль кратко"},{"Обычно",prefix+"стиль обычно"}},
   {{"Подробно",prefix+"стиль подробно"}},
  }
 case "контекста":
  values:=[]int{5,10,20,30,50,100}
  unique:=[]int{};seen:=map[int]bool{}
  for _,n:=range values{n=min(cfg.ContextMessages,n);if n>0&&!seen[n]{seen[n]=true;unique=append(unique,n)}}
  for i:=0;i<len(unique);i+=2{
   row:=[]Btn{}
   for j:=i;j<min(i+2,len(unique));j++{
    n:=unique[j]
    row=append(row,Btn{fmt.Sprintf("%d сообщений",n),fmt.Sprintf("%sконтекст %d",prefix,n)})
   }
   rows=append(rows,row)
  }
 case "резерва":
  rows=[][]Btn{
   {{"✓ Резерв включён",prefix+"резерв вкл"},{"✗ Резерв выключен",prefix+"резерв выкл"}},
   {{"Авто • как на сервере",prefix+"резерв авто"}},
  }
 case "сброса":
  rows=[][]Btn{{{"Да, сбросить",prefix+"подтвердить сброс"},{"Отменить",func()string{if prefix=="диалог "{return "диалог"};return "настройки"}()}}}
 default:
  if prefix=="диалог "{return chatBoard()}
  return personalBoard()
 }
 back:="функции";if prefix=="диалог "{back="диалог"}
 if name!="сброса"{rows=append(rows,[]Btn{{"← Назад к настройкам",back}})}
 return board(rows)
}
func buttonsFor(command string,c *Catalog,cfg Config)string{
 words:=strings.Fields(strings.ToLower(command))
 if len(words)==0{return mainBoard()}
 switch words[0]{
 case "меню","старт":return mainBoard()
 case "помощь","help","права","подключение","разрешения":return helpBoard()
 case "настройки":return personalBoard()
 case "ии":return aiBoard()
 case "функции":return functionsBoard()
 case "модели":
  page:=1;if len(words)>1{if n,e:=strconv.Atoi(words[1]);e==nil{page=n}}
  return modelBoard(c,cfg,page,false)
 }
 chat:=false
 if words[0]=="диалог"{
  chat=true;words=words[1:]
  if len(words)==0{return chatBoard()}
 }
 prefix:="";if chat{prefix="диалог "}
 if words[0]=="модели"||(chat&&words[0]=="модели")||chat&&words[0]=="модель"&&len(words)>1&&words[1]!="авто"{
  // "диалог модели <страница>" and "диалог модель <номер>" have
  // different meanings, so only the plural command opens a page.
  if words[0]=="модели"{
   page:=1;if len(words)>1{if n,e:=strconv.Atoi(words[1]);e==nil{page=n}}
   return modelBoard(c,cfg,page,chat)
  }
 }
 if words[0]=="выбор"&&len(words)>1{return choiceBoard(prefix,words[1],cfg)}
 if words[0]=="модель"||words[0]=="поиск"||words[0]=="стиль"||words[0]=="контекст"||
  words[0]=="резерв"||words[0]=="сброс"||words[0]=="сбросить"||words[0]=="подтвердить"{
  if chat{return chatBoard()}
  if words[0]=="модель"{return aiBoard()}
  if words[0]=="поиск"||words[0]=="стиль"||words[0]=="контекст"||words[0]=="резерв"{return functionsBoard()}
  return personalBoard()
 }
 if chat{return chatBoard()}
 return mainBoard()
}
func callbackCommand(raw json.RawMessage)string{
 // VK clients may send payload as JSON object or a JSON-encoded string.
 if len(raw)>0&&raw[0]=='"' {
  var encoded string
  if json.Unmarshal(raw,&encoded)!=nil{return ""}
  raw=json.RawMessage(encoded)
 }
 var obj struct {AIbot int `json:"aibot"`; Command string `json:"command"`}
 if json.Unmarshal(raw,&obj)!=nil||obj.AIbot!=1||len(obj.Command)>90{return ""}
 // Actions are still parsed against an allowlist by command handler, not as shell commands.
 if strings.ContainsAny(obj.Command,"<>{}\n\r") {return ""}
 return obj.Command
}
// vkTimestamp accepts either a VK numeric timestamp or its quoted form.
func vkTimestamp(raw json.RawMessage)(string,error){
 if len(raw)==0{return "",errors.New("VK timestamp отсутствует")}
 var str string
 if json.Unmarshal(raw,&str)!=nil{
  var number json.Number
  if err:=json.Unmarshal(raw,&number);err!=nil{return "",errors.New("неверный VK timestamp")}
  str=number.String()
 }
 if str==""||len(str)>40{return "",errors.New("пустой или слишком длинный VK timestamp")}
 for _,r:=range str {if r<'0'||r>'9'{return "",errors.New("недопустимый VK timestamp")}}
 return str,nil
}
func vkPoll(ctx context.Context,v *VK,s *Store,onMessage func(VKMessage)error,onButton func(int64,int64,string,json.RawMessage,int64))error{
 serverURL,key,ts:="","","";fails:=0
 for ctx.Err()==nil{
  if serverURL==""{
   vals:=url.Values{"group_id":{strconv.FormatInt(v.groupID,10)}}
   raw,err:=v.call(ctx,"groups.getLongPollServer",vals)
   if err!=nil{fails++;if !sleepContext(ctx,time.Duration(min(fails,6))*time.Second){return nil};continue}
   var server struct{Server string `json:"server"`;Key string `json:"key"`;TS json.RawMessage `json:"ts"`}
   if json.Unmarshal(raw,&server)!=nil||!validHTTPS(server.Server){return errors.New("неверный VK Long Poll server")}
   parsedTS,err:=vkTimestamp(server.TS);if err!=nil{return err}
   serverURL,key,ts=server.Server,server.Key,parsedTS;if saved:=s.cursor(v.groupID);saved!=""{ts=saved}
  }
  vals:=url.Values{"act":{"a_check"},"key":{key},"ts":{ts},"wait":{"25"}}
  req,err:=http.NewRequestWithContext(ctx,"GET",serverURL+"?"+vals.Encode(),nil);if err!=nil{return err}
  response,err:=v.client.Do(req)
  if err!=nil{serverURL="";if !sleepContext(ctx,3*time.Second){return nil};continue}
  data,err:=io.ReadAll(io.LimitReader(response.Body,4<<20));response.Body.Close()
  if err!=nil||response.StatusCode!=200{serverURL="";continue}
  var batch struct{Failed int `json:"failed"`;TS json.RawMessage `json:"ts"`;Updates []struct{Type string `json:"type"`;Object json.RawMessage `json:"object"`} `json:"updates"`}
  if json.Unmarshal(data,&batch)!=nil{serverURL="";continue}
  // Failed 2/3 responses do not necessarily include ts.
  if batch.Failed==2||batch.Failed==3{
   if batch.Failed==3{s.clearCursor(v.groupID)}
   serverURL=""
   continue
  }
  parsedTS,err:=vkTimestamp(batch.TS)
  if err!=nil{serverURL="";continue}
  if batch.Failed==1{ts=parsedTS;s.setCursor(v.groupID,ts);continue}
  if batch.Failed!=0{serverURL="";continue}
  fails=0
  for _,evt:=range batch.Updates{
   switch evt.Type {
   case "message_new":
    var envelope struct {Message VKMessage `json:"message"`}
    if json.Unmarshal(evt.Object,&envelope)==nil {
     if err:=onMessage(envelope.Message);err!=nil{
      // Reconnect and replay the entire batch. Atomic persistence and unique
      // event IDs prevent duplicates while preserving unsaved messages.
      return fmt.Errorf("VK event not persisted: %w",err)
     }
    }
   case "message_event":
    var e struct{PeerID int64 `json:"peer_id"`;UserID int64 `json:"user_id"`;EventID string `json:"event_id"`;Payload json.RawMessage `json:"payload"`;ConversationMessageID int64 `json:"conversation_message_id"`}
    if json.Unmarshal(evt.Object,&e)==nil{onButton(e.PeerID,e.UserID,e.EventID,e.Payload,e.ConversationMessageID)}
   }
  }
  ts=parsedTS;s.setCursor(v.groupID,ts)
 }
 return nil
}
func sleepContext(ctx context.Context,d time.Duration)bool{select{case <-ctx.Done():return false;case <-time.After(d):return true}}
