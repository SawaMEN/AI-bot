package main

import (
 "encoding/json"
 "fmt"
 "strconv"
 "strings"
)

// A VK group conversation has one shared bottom keyboard. Consequently the
// group keyboard exposes only chat-wide settings; personal preferences live
// in the community's private messages. Admin rights are checked on every
// group-setting mutation, never inferred from the button payload.
func replyGroupMainBoard()string{
 return replyKeyboard([][]replyButton{
  {{"🤖 ИИ беседы","диалог модели 1","primary"}},
  {{"🧰 Функции беседы","диалог","primary"}},
  {{"📊 Статистика","статистика","secondary"},{"❓ Помощь","помощь","secondary"}},
 })
}

func replyGroupChatBoard()string{
 return replyKeyboard([][]replyButton{
  {{"🤖 Модель беседы","диалог модели 1","primary"}},
  {{"🔎 Поиск","диалог выбор поиска","primary"},{"✍ Стиль","диалог выбор стиля","primary"}},
  {{"🧠 Контекст","диалог выбор контекста","secondary"},{"🔄 Резерв","диалог выбор резерва","secondary"}},
  {{"↺ Сбросить настройки беседы","диалог выбор сброса","negative"}},
  {{"📊 Статистика","статистика","secondary"},{"🏠 Главное меню","меню","positive"}},
 })
}

func replyGroupModelBoard(c *Catalog,cfg Config,page int)string{
 allowed:=c.enabled(cfg)
 maxPage:=(len(allowed)+2)/3
 if maxPage<1{maxPage=1}
 if page<1{page=1}
 if page>maxPage{page=maxPage}
 rows:=[][]replyButton{}
 for i:=(page-1)*3;i<min(page*3,len(allowed));i++{
  label:=c.label(allowed[i])
  if len([]rune(label))>30{label=string([]rune(label)[:29])+"…"}
  rows=append(rows,[]replyButton{{
   Label:fmt.Sprintf("%d. %s",i+1,label),
   Command:"диалог модель "+strconv.Itoa(i+1),
   Color:"primary",
  }})
 }
 if len(allowed)>0{
  nav:=[]replyButton{}
  if page>1{nav=append(nav,replyButton{"◀ Назад","диалог модели "+strconv.Itoa(page-1),"secondary"})}
  if page<maxPage{nav=append(nav,replyButton{"Далее ▶","диалог модели "+strconv.Itoa(page+1),"secondary"})}
  if len(nav)>0{rows=append(rows,nav)}
  rows=append(rows,[]replyButton{{"✓ Автоматически","диалог модель авто","positive"}})
 }
 rows=append(rows,[]replyButton{{"← Функции беседы","диалог","secondary"},{"🏠 Меню","меню","secondary"}})
 return replyKeyboard(rows)
}

// Reuse the same server-enforced ranges/choices as the private keyboard,
// while routing every setting to the chat-scoped command handler.
func replyGroupChoiceBoard(name string,cfg Config)string{
 raw:=replyChoiceBoard(name,cfg)
 var original struct{
  Buttons [][]struct{
   Action struct{Label,Payload string}
   Color string
  }
 }
 if err:=json.Unmarshal([]byte(raw),&original);err!=nil{return replyGroupChatBoard()}
 rows:=[][]replyButton{}
 for _,line:=range original.Buttons{
  row:=[]replyButton{}
  for _,b:=range line{
   command:=vkReplyCommand(json.RawMessage(b.Action.Payload))
   if command==""{return replyGroupChatBoard()}
   if command=="настройки"||command=="функции"{command="диалог"
   }else if command!="меню"{
    command="диалог "+command
   }
   row=append(row,replyButton{b.Action.Label,command,b.Color})
  }
  rows=append(rows,row)
 }
 return replyKeyboard(rows)
}

func replyGroupButtonsFor(command string,c *Catalog,cfg Config)string{
 fields:=strings.Fields(strings.ToLower(command))
 if len(fields)==0{return replyGroupMainBoard()}
 switch fields[0]{
 case "меню","старт","вопрос","помощь","help","статистика":
  return replyGroupMainBoard()
 case "диалог":
  if len(fields)==1{return replyGroupChatBoard()}
  if fields[1]=="модели"{
   page:=1
   if len(fields)>2{if n,err:=strconv.Atoi(fields[2]);err==nil{page=n}}
   return replyGroupModelBoard(c,cfg,page)
  }
  if fields[1]=="выбор"&&len(fields)>2{
   return replyGroupChoiceBoard(fields[2],cfg)
  }
  return replyGroupChatBoard()
 // Old inline actions and manual commands must also install a bottom menu.
 case "модели","ии":
  return replyGroupModelBoard(c,cfg,1)
 case "функции","настройки":
  return replyGroupChatBoard()
 default:return replyGroupMainBoard()
 }
}

// Only these group-level keyboard payloads may bypass the mention requirement.
// Typed, unmentioned group messages still pass through the normal AI-mention
// gate, even if their text looks like a keyboard label.
func vkGroupReplyCommand(raw json.RawMessage)string{
 command:=vkReplyCommand(raw)
 if command==""{return ""}
 words:=strings.Fields(strings.ToLower(command))
 if len(words)==0{return ""}
 switch words[0]{
 case "меню","старт","помощь","вопрос","ии","функции","настройки","модели","статистика":
  return command
 case "диалог":
  if len(words)==1{return command}
  switch words[1]{
  case "модели","модель","выбор","поиск","стиль","контекст","резерв","подтвердить","сброс":
   return command
  }
 }
 return ""
}
