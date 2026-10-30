package main

import (
 "fmt"
 "strconv"
 "strings"
)

// Multiple selection uses Space to toggle and Enter to accept. A line-input
// fallback is retained for redirected terminals, scripts and accessibility.
func(w Wizard) chooseMany(title string,labels []string)[]int{
 if len(labels)==0{return nil}
 if !terminalEnabled(w.out){
  fmt.Fprintln(w.out,"\n"+title)
  for i,label:=range labels{fmt.Fprintf(w.out," %d. %s\n",i+1,cleanTerminal(label))}
  raw:=w.ask("Введите номера через запятую (например 1,3), 0 — отменить","1")
  picked:=map[int]bool{}
  for _,entry:=range strings.Split(raw,","){
   n,e:=strconv.Atoi(strings.TrimSpace(entry))
   if e==nil&&n>=1&&n<=len(labels){picked[n-1]=true}
  }
  selected:=[]int{}
  for i:=range labels{if picked[i]{selected=append(selected,i)}}
  return selected
 }
 restore,err:=rawTerm();if err!=nil{return nil}
 defer restore()
 fmt.Fprint(w.out,"\x1b[?25l")
 defer fmt.Fprint(w.out,"\x1b[?25h\x1b[0m")
 checked:=make([]bool,len(labels))
 selected:=0
 filter:="";searching:=false
 for{
  visible:=make([]int,0,len(labels))
  for i,name:=range labels{
   if filter==""||strings.Contains(strings.ToLower(name),strings.ToLower(filter)){visible=append(visible,i)}
  }
  if len(visible)>0{
   matched:=false
   for _,idx:=range visible{if idx==selected{matched=true}}
   if !matched{selected=visible[0]}
  }
  width,height:=terminalDimensions()
  maxWidth:=width-4;if maxWidth>100{maxWidth=100}
  rows:=height-11;if rows>18{rows=18};if rows<4{rows=4}
  selectedPos:=0;for i,v:=range visible{if v==selected{selectedPos=i}}
  offset:=selectedPos-rows/2
  if offset<0{offset=0}
  if offset+rows>len(visible){offset=len(visible)-rows}
  if offset<0{offset=0}
  fmt.Fprint(w.out,"\x1b[H\x1b[2J")
  fmt.Fprintln(w.out,tone("╭"+strings.Repeat("─",maxWidth-2)+"╮",tuiAccent))
  fmt.Fprintln(w.out,"  "+tone(compactLine(title,maxWidth-4),tuiAccent))
  count:=0;for _,v:=range checked{if v{count++}}
  fmt.Fprintf(w.out,"  Отмечено: %d из %d  •  У всех моделей проверена доступность\n\n",count,len(labels))
  if len(visible)==0{fmt.Fprintln(w.out,"  Нет совпадений")}
  for i:=offset;i<len(visible)&&i<offset+rows;i++{
   j:=visible[i]
   tick:="□";if checked[j]{tick="■"}
   line:=padLine(compactLine(fmt.Sprintf(" %2d  %s  %s",j+1,tick,labels[j]),maxWidth-4),maxWidth-4)
   if j==selected{fmt.Fprintln(w.out,tone("▸"+line,tuiSelected))
   }else{fmt.Fprintln(w.out," "+line)}
  }
  if len(visible)>rows{fmt.Fprintf(w.out,"  %d–%d из %d\n",offset+1,min(offset+rows,len(visible)),len(visible))}
  if searching{fmt.Fprintf(w.out,"\n  Поиск: %s_\n",filter)
  }else{fmt.Fprintln(w.out,"\n  ↑ ↓ — перемещение  •  Space — отметить  •  Enter — применить  •  Esc — отмена  •  / — поиск")}
  key,e:=w.in.ReadByte();if e!=nil{return nil}
  if key==27{
   if !keyReady(w.in,65){
    if searching{searching=false;filter=""}else{return nil}
    continue
   }
   next,e:=w.in.ReadByte();if e!=nil{return nil}
   if next!='['{if searching{searching=false;filter=""}else{return nil};continue}
   if !keyReady(w.in,65){continue}
   key,e=w.in.ReadByte();if e!=nil{continue}
   switch key{
   case 'A':selected=moveSelected(visible,selected,-1)
   case 'B':selected=moveSelected(visible,selected,1)
   }
   continue
  }
  if searching{
   switch key{
   case '\r','\n':searching=false
   case 8,127:if len(filter)>0{filter=filter[:len(filter)-1]}
   default:if key>=32{filter+=string(key)}
   }
   continue
  }
  switch key {
  case '/':searching=true
  case 'j','J':selected=moveSelected(visible,selected,1)
  case 'k','K':selected=moveSelected(visible,selected,-1)
  case ' ':if len(visible)>0{checked[selected]=!checked[selected]}
  case '\r','\n':
   result:=[]int{}
   for i,on:=range checked{if on{result=append(result,i)}}
   return result
  }
 }
}
