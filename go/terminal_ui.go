package main

import (
 "bufio"
 "fmt"
 "io"
 "os"
 "strconv"
 "strings"
 "syscall"
 "time"
 "unicode"
 "unsafe"
)

const (
 tuiReset = "\x1b[0m"
 tuiAccent = "\x1b[38;5;81m"
 tuiMuted = "\x1b[38;5;245m"
 tuiGreen = "\x1b[38;5;114m"
 tuiYellow = "\x1b[38;5;221m"
 tuiSelected = "\x1b[48;5;238m\x1b[38;5;117m"
)
type menuOption struct {
 Number int
 Title string
 Detail string
 Status string
}
type screenSize struct {Rows,Cols,X,Y uint16}

func terminalEnabled(out io.Writer) bool {
 f,ok:=out.(*os.File)
 if !ok||f!=os.Stdout||os.Getenv("TERM")==""||os.Getenv("TERM")=="dumb"||os.Getenv("AIBOT_PLAIN")!=""{return false}
 in,err:=os.Stdin.Stat()
 if err!=nil||in.Mode()&os.ModeCharDevice==0{return false}
 info,err:=f.Stat()
 if err!=nil||info.Mode()&os.ModeCharDevice==0{return false}
 _,err=termGet(int(os.Stdin.Fd()))
 return err==nil
}
func termGet(fd int)(syscall.Termios,error){
 var term syscall.Termios
 _,_,errno:=syscall.Syscall(syscall.SYS_IOCTL,uintptr(fd),uintptr(syscall.TCGETS),uintptr(unsafe.Pointer(&term)))
 if errno!=0{return term,errno}
 return term,nil
}
func termSet(fd int,t *syscall.Termios)error{
 _,_,errno:=syscall.Syscall(syscall.SYS_IOCTL,uintptr(fd),uintptr(syscall.TCSETS),uintptr(unsafe.Pointer(t)))
 if errno!=0{return errno}
 return nil
}
func rawTerm() (func(),error){
 original,err:=termGet(int(os.Stdin.Fd()))
 if err!=nil{return nil,err}
 raw:=original
 raw.Lflag &^= syscall.ICANON|syscall.ECHO
 raw.Iflag &^= syscall.IXON|syscall.ICRNL
 if err=termSet(int(os.Stdin.Fd()),&raw);err!=nil{return nil,err}
 return func(){_ =termSet(int(os.Stdin.Fd()),&original)},nil
}
func terminalDimensions()(int,int){
 size:=screenSize{}
 _,_,errno:=syscall.Syscall(syscall.SYS_IOCTL,uintptr(int(os.Stdout.Fd())),uintptr(syscall.TIOCGWINSZ),uintptr(unsafe.Pointer(&size)))
 if errno!=0||size.Cols<20||size.Rows<10{return 80,24}
 return int(size.Cols),int(size.Rows)
}
func useColors()bool{return os.Getenv("NO_COLOR")==""&&os.Getenv("TERM")!="dumb"}
func tone(v,color string)string{
 if !useColors(){return v}
 return color+v+tuiReset
}
// Terminal descriptions may include API-provided model labels. Never let
// escape sequences or control bytes drive the terminal cursor.
func cleanTerminal(s string)string{
 return strings.Map(func(r rune)rune{
  if unicode.IsControl(r)||r==0x7f||r>=0x80&&r<=0x9f{return -1}
  return r
 },s)
}
func compactLine(s string,limit int)string{
 s=cleanTerminal(s)
 r:=[]rune(s)
 if limit<4{return ""}
 if len(r)<=limit{return s}
 return string(r[:limit-1])+"…"
}
func padLine(s string,width int)string{
 n:=len([]rune(cleanTerminal(s)))
 if n>=width{return compactLine(s,width)}
 return cleanTerminal(s)+strings.Repeat(" ",width-n)
}
func canReadTTY(reader *bufio.Reader)bool{
 return reader!=nil
}
func keyReady(reader *bufio.Reader,timeoutMs int)bool{
 if reader.Buffered()>0{return true}
 fd:=int(os.Stdin.Fd())
 if fd<0||fd>=1024{return false}
 var readSet syscall.FdSet
 readSet.Bits[fd/64]|=1<<uint(fd%64)
 timeout:=syscall.NsecToTimeval(int64(timeoutMs)*int64(time.Millisecond))
 n,err:=syscall.Select(fd+1,&readSet,nil,nil,&timeout)
 return err==nil&&n>0
}
func (w Wizard) chooseMenu(title,subtitle string,options []menuOption)int{
 if len(options)==0{return 0}
 if !terminalEnabled(w.out)||!canReadTTY(w.in){
  fmt.Fprintln(w.out,"\n"+title)
  if subtitle!=""{fmt.Fprintln(w.out,subtitle)}
  maxNumber:=0
  for _,item:=range options{
   fmt.Fprintf(w.out," %2d. %s",item.Number,item.Title)
   if item.Status!=""{fmt.Fprintf(w.out,"  [%s]",item.Status)}
   if item.Detail!=""{fmt.Fprintf(w.out," — %s",item.Detail)}
   fmt.Fprintln(w.out)
   if item.Number>maxNumber{maxNumber=item.Number}
  }
  for{
   selected:=w.integer("Номер пункта",0,0,maxNumber)
   for _,item:=range options{if item.Number==selected{return selected}}
   fmt.Fprintln(w.out,"Такого пункта нет.")
  }
 }
 restore,err:=rawTerm()
 if err!=nil{return w.chooseMenuPlain(title,subtitle,options)}
 defer restore()
 fmt.Fprint(w.out,"\x1b[?25l")
 defer fmt.Fprint(w.out,"\x1b[?25h\x1b[0m")
 selected:=0
 for i,item:=range options{if item.Number==0{selected=i;break}}
 // Prefer the first action, not Exit/Back.
 for i,item:=range options{if item.Number>0{selected=i;break}}
 filter:=""
 number:=""
 searching:=false
 for {
  visible:=make([]int,0,len(options))
  for i,item:=range options{
   if filter==""||strings.Contains(strings.ToLower(item.Title+" "+item.Detail+" "+item.Status),strings.ToLower(filter)){visible=append(visible,i)}
  }
  if len(visible)>0{
   shown:=false
   for _,idx:=range visible{if idx==selected{shown=true;break}}
   if !shown{selected=visible[0]}
  }
  width,height:=terminalDimensions()
  boxWidth:=width-4
  if boxWidth>102{boxWidth=102}
  if boxWidth<36{boxWidth=36}
  viewHeight:=height-12-strings.Count(subtitle,"\n")
  if viewHeight<4{viewHeight=4}
  if viewHeight>16{viewHeight=16}
  position:=0;for i,idx:=range visible{if idx==selected{position=i;break}}
  start:=position-viewHeight/2
  if start<0{start=0}
  if start+viewHeight>len(visible){start=len(visible)-viewHeight}
  if start<0{start=0}
  fmt.Fprint(w.out,"\x1b[H\x1b[2J")
  border:=strings.Repeat("─",boxWidth-2)
  fmt.Fprintf(w.out,"%s\n",tone("╭"+border+"╮",tuiAccent))
  fmt.Fprintf(w.out,"%s  %s\n",tone("│",tuiAccent),tone("AI-bot  •  КОНСОЛЬ УПРАВЛЕНИЯ",tuiAccent))
  fmt.Fprintf(w.out,"%s  %s\n",tone("│",tuiAccent),tone(compactLine(title,boxWidth-5),tuiGreen))
  fmt.Fprintf(w.out,"%s\n",tone("├"+border+"┤",tuiAccent))
  for _,line:=range strings.Split(subtitle,"\n"){
   if line!=""{fmt.Fprintf(w.out," %s\n",tone(compactLine(line,boxWidth-3),tuiMuted))}
  }
  fmt.Fprintln(w.out)
  if len(visible)==0{fmt.Fprintln(w.out,"  Совпадений нет — измените поисковый запрос")}
  for i:=start;i<len(visible)&&i<start+viewHeight;i++{
   item:=options[visible[i]]
   label:=fmt.Sprintf(" %2d   %s",item.Number,item.Title)
   if item.Status!=""{label+="   "+item.Status}
   label=padLine(compactLine(label,boxWidth-5),boxWidth-5)
   if visible[i]==selected{
    fmt.Fprintf(w.out," %s\n",tone("▸"+label,tuiSelected))
    if item.Detail!=""{fmt.Fprintf(w.out,"     %s\n",tone(compactLine(item.Detail,boxWidth-8),tuiMuted))}
   }else{fmt.Fprintf(w.out,"  %s\n",label)}
  }
  if len(visible)>viewHeight{fmt.Fprintf(w.out,"  %s\n",tone(fmt.Sprintf("Строки %d–%d из %d",start+1,min(start+viewHeight,len(visible)),len(visible)),tuiMuted))}
  fmt.Fprintln(w.out)
  if searching{fmt.Fprintf(w.out," %s %s_\n",tone("Поиск:",tuiAccent),filter)
  }else if number!=""{fmt.Fprintf(w.out," %s %s  (Enter — перейти)\n",tone("Номер:",tuiAccent),number)
  }else{fmt.Fprintln(w.out,tone("  ↑ ↓ / j k — выбор    Enter — открыть    / — поиск    0 / Esc — назад",tuiMuted))}
  fmt.Fprintln(w.out,tone("  Подсказка: управление только с клавиатуры; API-ключи не выводятся.",tuiMuted))
  key,readErr:=w.in.ReadByte()
  if readErr!=nil{return 0}
  if key==3||key==4{return 0}
  if key==27{
   if !keyReady(w.in,65){
    if searching{searching=false;filter=""}else{return 0}
    continue
   }
   next,e:=w.in.ReadByte();if e!=nil{return 0}
   if next!='['{if searching{searching=false;filter=""}else{return 0};continue}
   if !keyReady(w.in,65){continue}
   last,e:=w.in.ReadByte();if e!=nil{continue}
   switch last{
   case 'A':selected=moveSelected(visible,selected,-1)
   case 'B':selected=moveSelected(visible,selected,1)
   case 'H':if len(visible)>0{selected=visible[0]}
   case 'F':if len(visible)>0{selected=visible[len(visible)-1]}
   }
   number=""
   continue
  }
  if searching{
   switch key{
   case '\r','\n':searching=false
   case 127,8:if len(filter)>0{filter=filter[:len(filter)-1]}
   default:if key>=32{filter+=string(key)}
   }
   continue
  }
  switch key{
  case '/':searching=true;number=""
  case 'j','J':selected=moveSelected(visible,selected,1);number=""
  case 'k','K':selected=moveSelected(visible,selected,-1);number=""
  case '\r','\n':
   if number!=""{
    n,e:=strconv.Atoi(number)
    if e==nil{for _,item:=range options{if item.Number==n{return n}}}
    number=""
   }else if len(visible)>0{return options[selected].Number}
  case 127,8:if len(number)>0{number=number[:len(number)-1]}
  default:
   if key>='0'&&key<='9'{
    number+=string(key)
    if key=='0'&&len(number)==1{
     for _,item:=range options{if item.Number==0{return 0}}
    }
    if len(number)>4{number=""}
   }
  }
 }
}
func moveSelected(visible []int,selected,step int)int{
 if len(visible)==0{return selected}
 for i,idx:=range visible{if idx==selected{return visible[(i+step+len(visible))%len(visible)]}}
 return visible[0]
}
func(w Wizard) chooseMenuPlain(title,subtitle string,options []menuOption)int{
 fmt.Fprintln(w.out,title)
 if subtitle!=""{fmt.Fprintln(w.out,subtitle)}
 maxNumber:=0
 for _,item:=range options{
  fmt.Fprintf(w.out,"%d. %s  %s\n",item.Number,item.Title,item.Status)
  if item.Number>maxNumber{maxNumber=item.Number}
 }
 for{
  number:=w.integer("Номер пункта",0,0,maxNumber)
  for _,item:=range options{if item.Number==number{return number}}
 }
}

// Determinate progress is used only when the total number of jobs is known.
func consoleProgress(out io.Writer,done,total int,label string){
 if total<=0{return}
 if done<0{done=0}
 if done>total{done=total}
 if !terminalEnabled(out){
  fmt.Fprintf(out,"  [%d/%d] %s\n",done,total,cleanTerminal(label))
  return
 }
 width,_:=terminalDimensions()
 barSize:=22
 if width<66{barSize=12}
 filled:=done*barSize/total
 bar:=strings.Repeat("━",filled)+strings.Repeat("─",barSize-filled)
 fmt.Fprintf(out,"\r\x1b[2K%s [%s] %3d%%  %s",tone("Проверка",tuiAccent),bar,done*100/total,compactLine(label,max(12,width-barSize-25)))
 if done==total{fmt.Fprint(out,"\n")}
}

// Spinner represents indeterminate network calls. No fabricated percentage.
func consoleSpinner(out io.Writer,label string,task func()) {
 if !terminalEnabled(out){fmt.Fprintln(out,cleanTerminal(label)+"...");task();return}
 done:=make(chan struct{})
 stopped:=make(chan struct{})
 go func(){
  defer close(stopped)
  glyphs:=[]string{"◐","◓","◑","◒"}
  ticker:=time.NewTicker(125*time.Millisecond);defer ticker.Stop()
  i:=0
  for{
   select{
   case <-done:return
   case <-ticker.C:
    fmt.Fprintf(out,"\r\x1b[2K%s %s",tone(glyphs[i%len(glyphs)],tuiAccent),compactLine(label,65));i++
   }
  }
 }()
 task()
 close(done)
 <-stopped
 fmt.Fprintf(out,"\r\x1b[2K%s %s\n",tone("•",tuiMuted),cleanTerminal(label))
}
