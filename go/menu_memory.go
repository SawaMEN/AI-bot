package main

import (
 "strings"
 "time"
)

func normalizeMenuSection(command string)string{
 fields:=strings.Fields(strings.ToLower(command))
 if len(fields)==0{return "меню"}
 switch fields[0]{
 case "меню","старт","статистика":return "меню"
 case "помощь","help":return "помощь"
 case "ии","модель":return "ии"
 case "модели":return "модели"
 case "функции","поиск","стиль","контекст","резерв":return "функции"
 case "настройки":return "настройки"
 case "выбор":
  if len(fields)>1{return "выбор "+fields[1]}
 case "диалог":
  if len(fields)>1{
   if fields[1]=="модели"||fields[1]=="модель"{return "диалог модели 1"}
   if fields[1]=="выбор"&&len(fields)>2{return "диалог выбор "+fields[2]}
  }
  return "диалог"
 }
 return "меню"
}
func(s *Store) rememberMenu(peer int64,command string){
 if peer<=0{return}
 section:=normalizeMenuSection(command)
 _,_=s.DB.Exec(`INSERT INTO menu_state(peer_id,section,updated_at) VALUES(?,?,?)
 ON CONFLICT(peer_id) DO UPDATE SET section=excluded.section,updated_at=excluded.updated_at`,peer,section,time.Now().Unix())
}
func(s *Store) activeMenu(peer int64)string{
 var section string;var updated int64
 if s.DB.QueryRow("SELECT section,updated_at FROM menu_state WHERE peer_id=?",peer).Scan(&section,&updated)!=nil{return ""}
 if time.Since(time.Unix(updated,0))>12*time.Hour{return ""}
 return section
}
