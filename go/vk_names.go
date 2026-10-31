package main

import (
 "context"
 "encoding/json"
 "net/url"
 "strconv"
 "strings"
 "time"
)

// Names are optional decoration for the current chat context, not a hard
// dependency of AI processing. Failures fall back to numeric sender IDs.
func(v *VK) fetchUserNames(ctx context.Context,ids []int64)map[int64]string{
 names:=map[int64]string{}
 if len(ids)==0{return names}
 values:=[]string{}
 for _,id:=range ids{
  if id>0{values=append(values,strconv.FormatInt(id,10))}
  if len(values)>=20{break}
 }
 if len(values)==0{return names}
 response,err:=v.call(ctx,"users.get",url.Values{"user_ids":{strings.Join(values,",")}})
 if err!=nil{return names}
 var people []struct{ID int64 `json:"id"`;First string `json:"first_name"`;Last string `json:"last_name"`}
 if json.Unmarshal(response,&people)!=nil{return names}
 for _,person:=range people{
  if person.ID<=0{continue}
  display:=safeExcerpt(person.First+" "+person.Last,55)
  if display!=""{names[person.ID]=display}
 }
 return names
}

func(b *Bot) chatNames(ctx context.Context,turns []Turn)map[int64]string{
 needed:=[]int64{}
 result:=map[int64]string{}
 b.mu.Lock()
 if b.names==nil{b.names=map[int64]string{}}
 seen:=map[int64]bool{}
 for _,t:=range turns{
  if t.Sender<=0||seen[t.Sender]{continue}
  seen[t.Sender]=true
  if name,ok:=b.names[t.Sender];ok{
   if name!=""{result[t.Sender]=name}
  }else if len(needed)<20{needed=append(needed,t.Sender)}
 }
 b.mu.Unlock()
 if len(needed)==0{return result}
 timeout,stop:=context.WithTimeout(ctx,2*time.Second)
 found:=b.vk.fetchUserNames(timeout,needed);stop()
 b.mu.Lock()
 for _,id:=range needed{
  b.names[id]=found[id]
  if found[id]!=""{result[id]=found[id]}
 }
 b.mu.Unlock()
 return result
}
