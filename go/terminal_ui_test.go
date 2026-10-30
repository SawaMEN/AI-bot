package main

import (
 "bufio"
 "bytes"
 "strings"
 "testing"
)

func TestConsoleMenuNumericFallback(t *testing.T){
 var output bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("7\n2\n")),out:&output}
 selected:=w.chooseMenu("Главное меню","Состояние бота",[]menuOption{
  {1,"Включить бота","Подсказка","Готово"},
  {2,"Настроить API","Бесплатный доступ",""},
  {0,"Назад","",""},
 })
 if selected!=2{t.Fatalf("selected %d, expected 2",selected)}
 if !strings.Contains(output.String(),"Настроить API")||!strings.Contains(output.String(),"Бесплатный доступ"){t.Fatal(output.String())}
}

func TestConsoleMenuBackAndFallback(t *testing.T){
 var output bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("0\n")),out:&output}
 if got:=w.chooseMenu("Settings","",[]menuOption{{1,"First","",""},{0,"Back","",""}});got!=0{t.Fatalf("back returned %d",got)}
}

func TestConsoleModelMultiSelectPlain(t *testing.T){
 var output bytes.Buffer
 w:=Wizard{in:bufio.NewReader(strings.NewReader("1,3,3,99\n")),out:&output}
 got:=w.chooseMany("Выбор бесплатных моделей",[]string{"Gemini","Qwen","Mistral"})
 if len(got)!=2||got[0]!=0||got[1]!=2{t.Fatalf("selection = %v",got)}
}

func TestConsoleProgressIsDeterminate(t *testing.T){
 var output bytes.Buffer
 consoleProgress(&output,3,4,"Проверен ИИ")
 result:=output.String()
 if !strings.Contains(result,"[3/4]")||!strings.Contains(result,"Проверен ИИ"){t.Fatal(result)}
 if strings.Contains(result,"\x1b["){t.Fatal("escape sequences in plain output")}
}

func TestConsoleSpinnerHasPlainFallback(t *testing.T){
 var out bytes.Buffer
 calls:=0
 consoleSpinner(&out,"Обновление каталога",func(){calls++})
 if calls!=1||!strings.Contains(out.String(),"Обновление каталога"){t.Fatal(out.String())}
}

func TestConsoleUntrustedModelLabelsSanitized(t *testing.T){
 label:="Qwen\x1b[31m\x00\nready"
 if got:=cleanTerminal(label);strings.ContainsAny(got,"\x1b\x00\n")||got!="Qwen[31mready"{t.Fatalf("bad sanitization: %q",got)}
}

func TestConsoleSelectionNavigationWraps(t *testing.T){
 visible:=[]int{1,3,5}
 if moveSelected(visible,1,-1)!=5||moveSelected(visible,5,1)!=1||moveSelected(visible,3,1)!=5{t.Fatal("navigation wrap")}
}
