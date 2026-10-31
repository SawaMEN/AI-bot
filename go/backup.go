package main

import (
 "fmt"
 "io"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "syscall"
 "time"
)

// backupState creates consistent SQLite snapshots via VACUUM INTO rather than
// copying live WAL files, and preserves a private config copy for recovery.
// Only the newest seven dated snapshots are retained.
func backupState(store *Store)(string,error){
 root:=filepath.Join(filepath.Dir(dataPath()),"backups")
 if err:=os.MkdirAll(root,0700);err!=nil{return "",err}
 // A root-run manual backup must not make future service backups unwritable.
 if os.Geteuid()==0{
  if info,err:=os.Stat(filepath.Dir(root));err==nil{
   if owner,ok:=info.Sys().(*syscall.Stat_t);ok{
    if err:=os.Chown(root,int(owner.Uid),int(owner.Gid));err!=nil{return "",err}
   }
  }
 }
 dest:=filepath.Join(root,time.Now().UTC().Format("20060102T150405.000000000Z"))
 if err:=os.Mkdir(dest,0700);err!=nil{return "",err}
 ok:=false
 defer func(){if !ok{_ =os.RemoveAll(dest)}}()
 dbpath:=filepath.Join(dest,"history.db")
 if _,err:=store.DB.Exec("VACUUM INTO ?",dbpath);err!=nil{return "",fmt.Errorf("SQLite backup: %w",err)}
 if err:=os.Chmod(dbpath,0600);err!=nil{return "",err}
 if raw,err:=os.Open(configPath());err==nil{
  defer raw.Close()
  cfgFile,err:=os.OpenFile(filepath.Join(dest,"config.json"),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
  if err!=nil{return "",err}
  _,err=io.Copy(cfgFile,io.LimitReader(raw,2<<20))
  if closeErr:=cfgFile.Close();err==nil{err=closeErr}
  if err!=nil{return "",err}
 }else if !os.IsNotExist(err){return "",err}
 entries,err:=os.ReadDir(root);if err!=nil{return "",err}
 old:=[]string{}
 for _,entry:=range entries{
  if entry.IsDir()&&strings.HasSuffix(entry.Name(),"Z")&&len(entry.Name())==26{old=append(old,entry.Name())}
 }
 sort.Strings(old)
 for len(old)>7{
  _=os.RemoveAll(filepath.Join(root,old[0]))
  old=old[1:]
 }
 ok=true
 return dest,nil
}

// The background task intentionally runs infrequently to avoid CPU and IO
// pressure on small VDS. A failure does not stop the VK receiver.
func periodicBackup(ctxDone <-chan struct{},store *Store,report func(error)){
 if _,err:=backupState(store);err!=nil{report(err)}
 ticker:=time.NewTicker(24*time.Hour);defer ticker.Stop()
 for{
  select{
  case <-ctxDone:return
  case <-ticker.C:
   if _,err:=backupState(store);err!=nil{report(err)}
  }
 }
}

