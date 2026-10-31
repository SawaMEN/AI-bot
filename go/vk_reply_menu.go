package main

import (
 "encoding/json"
 "fmt"
 "strconv"
 "strings"
)

// VK supports two different keyboards:
// * an always-visible reply keyboard below the input field (private chats);
// * an inline callback keyboard attached to a message (shared group chats).
// The permanent menu resembles the quick-action, few-tap flow used by
// Dai Vinchik; users never need to remember command names.
type replyButton struct {
 Label string
 Command string
 Color string
}

func replyKeyboard(rows [][]replyButton) string {
 if len(rows)==0||len(rows)>10{return ""}
 buttons:=make([][]any,0,len(rows))
 for _,row:=range rows{
  if len(row)==0||len(row)>4{return ""}
  line:=make([]any,0,len(row))
  for _,button:=range row{
   if button.Command==""||button.Label==""||len([]rune(button.Label))>40{return ""}
   color:=button.Color
   switch color{
   case "primary","secondary","positive","negative":
   default:color="secondary"
   }
   payload,_:=json.Marshal(map[string]any{"aibot":1,"command":button.Command})
   line=append(line,map[string]any{
    "action":map[string]string{"type":"text","label":button.Label,"payload":string(payload)},
    "color":color,
   })
  }
  buttons=append(buttons,line)
 }
 encoded,_:=json.Marshal(map[string]any{
  "one_time":false,"inline":false,"buttons":buttons,
 })
 return string(encoded)
}

func replyMainBoard()string{
 return replyKeyboard([][]replyButton{
  {{"💬 Задать вопрос","вопрос","positive"}},
  {{"🤖 Выбрать ИИ","модели","primary"},{"⚙ Мои настройки","настройки","primary"}},
  {{"❓ Помощь","помощь","secondary"}},
 })
}
func replyPersonalBoard()string{
 return replyKeyboard([][]replyButton{
  {{"🤖 Модель","модели","primary"},{"🔎 Поиск","выбор поиска","primary"}},
  {{"✍ Стиль","выбор стиля","secondary"},{"🧠 Контекст","выбор контекста","secondary"}},
  {{"🔄 Резерв","выбор резерва","secondary"},{"↺ Сброс","выбор сброса","negative"}},
  {{"🏠 Главное меню","меню","positive"}},
 })
}
func replyHelpBoard()string{
 return replyKeyboard([][]replyButton{
  {{"⚙ Настройки","настройки","primary"},{"🤖 Выбрать ИИ","модели","primary"}},
  {{"🏠 Главное меню","меню","positive"}},
 })
}
func replyModelBoard(c *Catalog,cfg Config,page int)string{
 allowed:=c.enabled(cfg)
 maxPage:=(len(allowed)+2)/3
 if maxPage<1{maxPage=1}
 if page<1{page=1}
 if page>maxPage{page=maxPage}
 rows:=make([][]replyButton,0,7)
 if len(allowed)==0{
  return replyKeyboard([][]replyButton{
   {{"↻ Обновить список","модели","primary"}},
   {{"⚙ Настройки","настройки","secondary"},{"🏠 Меню","меню","positive"}},
  })
 }
 for i:=(page-1)*3;i<min(page*3,len(allowed));i++{
  label:=c.label(allowed[i])
  if len([]rune(label))>31{label=string([]rune(label)[:30])+"…"}
  rows=append(rows,[]replyButton{{fmt.Sprintf("%d. %s",i+1,label),"модель "+strconv.Itoa(i+1),"primary"}})
 }
 navigation:=[]replyButton{}
 if page>1{navigation=append(navigation,replyButton{"◀ Назад","модели "+strconv.Itoa(page-1),"secondary"})}
 if page<maxPage{navigation=append(navigation,replyButton{"Далее ▶","модели "+strconv.Itoa(page+1),"secondary"})}
 if len(navigation)>0{rows=append(rows,navigation)}
 rows=append(rows,[]replyButton{{"✓ Автоматически","модель авто","positive"}})
 rows=append(rows,[]replyButton{{"⚙ Настройки","настройки","secondary"},{"🏠 Меню","меню","secondary"}})
 return replyKeyboard(rows)
}
func replyChoiceBoard(name string,cfg Config)string{
 var rows [][]replyButton
 switch name{
 case "поиска":
  rows=[][]replyButton{
   {{"✓ Авто","поиск авто","positive"}},
   {{"Искать","поиск вкл","primary"},{"Не искать","поиск выкл","secondary"}},
  }
 case "стиля":
  rows=[][]replyButton{
   {{"Кратко","стиль кратко","primary"},{"Обычно","стиль обычно","positive"}},
   {{"Подробно","стиль подробно","primary"}},
  }
 case "контекста":
  values:=[]int{5,10,20,30,50,100}
  seen:=map[int]bool{}
  all:=[]replyButton{}
  for _,n:=range values{
   n=min(n,cfg.ContextMessages)
   if n<1||seen[n]{continue}
   seen[n]=true
   all=append(all,replyButton{strconv.Itoa(n)+" сообщений","контекст "+strconv.Itoa(n),"primary"})
  }
  for i:=0;i<len(all);i+=2{rows=append(rows,all[i:min(i+2,len(all))])}
 case "резерва":
  rows=[][]replyButton{
   {{"✓ Включить","резерв вкл","positive"},{"✗ Отключить","резерв выкл","secondary"}},
   {{"Авто","резерв авто","primary"}},
  }
 case "сброса":
  rows=[][]replyButton{
   {{"Да, сбросить","подтвердить сброс","negative"},{"Отмена","настройки","positive"}},
  }
 default:return replyPersonalBoard()
 }
 if name!="сброса"{
  rows=append(rows,[]replyButton{{"⬅ Назад","настройки","secondary"},{"🏠 Меню","меню","secondary"}})
 }
 return replyKeyboard(rows)
}

func replyButtonsFor(command string,c *Catalog,cfg Config)string{
 words:=strings.Fields(strings.ToLower(command))
 if len(words)==0{return replyMainBoard()}
 switch words[0]{
 case "меню","старт","вопрос":return replyMainBoard()
 case "настройки":return replyPersonalBoard()
 case "помощь","help","права","разрешения","подключение":return replyHelpBoard()
 case "модели":
  page:=1
  if len(words)>1{if n,e:=strconv.Atoi(words[1]);e==nil{page=n}}
  return replyModelBoard(c,cfg,page)
 case "выбор":
  if len(words)>1{return replyChoiceBoard(words[1],cfg)}
 }
 // When a setting changes, always return to the settings screen, not to
 // the same choice form. Users can change another parameter immediately.
 for _,name:=range []string{"модель","поиск","стиль","контекст","резерв","сброс","сбросить","подтвердить"}{
  if words[0]==name{return replyPersonalBoard()}
 }
 return replyMainBoard()
}
func keyboardForPeer(peer int64,command string,c *Catalog,cfg Config)string{
 if peer>0&&peer<2000000000{return replyButtonsFor(command,c,cfg)}
 return buttonsFor(command,c,cfg)
}
func answerKeyboardForPeer(peer int64)string{
 if peer>0&&peer<2000000000{return replyMainBoard()}
 return answerBoard()
}
func homeKeyboardForPeer(peer int64)string{
 if peer>0&&peer<2000000000{return replyMainBoard()}
 return mainBoard()
}

// VK message_new for reply-keyboard text buttons contains the JSON payload
// alongside the visible label; treat the payload as a navigation command.
func vkReplyCommand(payload json.RawMessage)string{
 command:=callbackCommand(payload)
 if command==""{return ""}
 // Never interpret arbitrary payload as an AI prompt. Only menu commands
 // and setting operations are supported by the regular command handler.
 fields:=strings.Fields(strings.ToLower(command))
 if len(fields)==0{return ""}
 switch fields[0]{
 case "меню","старт","вопрос","помощь","настройки","права","модели","модель",
  "выбор","поиск","стиль","контекст","резерв","сброс","сбросить","подтвердить":
  return command
 }
 return ""
}

func vkReplyLabel(text string)string{
 switch strings.ToLower(strings.TrimSpace(text)){
 case "🏠 меню","🏠 главное меню":return "меню"
 case "⚙ мои настройки","⚙ настройки":return "настройки"
 case "🤖 выбрать ии","🤖 модель":return "модели"
 case "❓ помощь":return "помощь"
 case "💬 задать вопрос":return "вопрос"
 case "🔎 поиск":return "выбор поиска"
 case "✍ стиль":return "выбор стиля"
 case "🧠 контекст":return "выбор контекста"
 case "🔄 резерв":return "выбор резерва"
 case "↺ сброс":return "выбор сброса"
 }
 return ""
}
