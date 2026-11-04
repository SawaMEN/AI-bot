package main

import (
 "encoding/json"
 "strings"
)

// VK keyboards are delivered per message. Attach the administrator shortcut
// only to the explicitly configured account's private messages. A group-chat
// keyboard is shared and must never expose bot-wide management actions.
func adminKeyboardShortcut(raw string)string{
 var board map[string]any
 if json.Unmarshal([]byte(raw),&board)!=nil{return raw}
 rows,ok:=board["buttons"].([]any);if !ok||len(rows)>=10{return raw}
 payload,_:=json.Marshal(map[string]any{"aibot":1,"command":"админ"})
 rows=append(rows,[]any{map[string]any{
  "action":map[string]any{"type":"text","label":"🛠 Меню админа","payload":string(payload)},
  "color":"primary",
 }})
 board["buttons"]=rows
 output,err:=json.Marshal(board);if err!=nil{return raw}
 return string(output)
}
func keyboardForViewer(peer,user int64,command string,c *Catalog,cfg Config)string{
 raw:=keyboardForPeer(peer,command,c,cfg)
 if !isBotAdministrator(cfg,peer,user){return raw}
 if first:=strings.Fields(strings.ToLower(command));len(first)>0&&first[0]=="админ"{
  return adminMenuButtons(c,cfg,command)
 }
 return adminKeyboardShortcut(raw)
}
func answerKeyboardForViewer(peer,user int64,cfg Config)string{
 raw:=answerKeyboardForPeer(peer)
 if !isBotAdministrator(cfg,peer,user){return raw}
 return adminKeyboardShortcut(raw)
}
func homeKeyboardForViewer(peer,user int64,cfg Config)string{
 raw:=homeKeyboardForPeer(peer)
 if !isBotAdministrator(cfg,peer,user){return raw}
 return adminKeyboardShortcut(raw)
}
