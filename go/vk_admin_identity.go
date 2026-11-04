package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/url"
 "regexp"
 "strconv"
 "strings"
)

var vkScreenName=regexp.MustCompile(`^[A-Za-z0-9_.]{3,32}$`)
var vkMentionID=regexp.MustCompile(`^\[id([1-9][0-9]{0,9})\|[^\]\r\n]{1,100}\]$`)

// parseVKUserReference accepts numeric IDs, id123, @screen_name,
// vk.com/id123, vk.com/username, mobile VK links, and VK mentions.
// No arbitrary URL is followed and group/public identifiers are rejected.
func parseVKUserReference(raw string)(string,error){
 input:=strings.TrimSpace(raw)
 if input==""||len(input)>500{return "",errors.New("укажите ID или ссылку на страницу человека ВК")}
 if match:=vkMentionID.FindStringSubmatch(input);match!=nil{return normalizeVKScreenName("id"+match[1])}
 input=strings.TrimPrefix(input,"@")
 low:=strings.ToLower(input)
 if strings.HasPrefix(low,"vk.com/")||strings.HasPrefix(low,"m.vk.com/")||strings.HasPrefix(low,"mobile.vk.com/")||strings.HasPrefix(low,"vk.ru/")||strings.HasPrefix(low,"m.vk.ru/")||strings.HasPrefix(low,"vk.me/"){
  input="https://"+input
 }
 if strings.Contains(input,"://"){
  address,err:=url.Parse(input);if err!=nil{return "",errors.New("неверная ссылка ВК")}
  if address.Scheme!="https"&&address.Scheme!="http"{return "",errors.New("нужна ссылка https://vk.com/...")}
  host:=strings.ToLower(address.Hostname())
  switch host{case "vk.com","www.vk.com","m.vk.com","mobile.vk.com","vk.ru","www.vk.ru","m.vk.ru","vk.me":default:return "",errors.New("ссылка должна вести на профиль VK")}
  if address.Port()!=""||address.User!=nil{return "",errors.New("неверная ссылка профиля VK")}
  path:=strings.Trim(address.EscapedPath(),"/")
  if strings.EqualFold(path,"profile.php"){
   id:=address.Query().Get("id")
   return normalizeVKScreenName(id)
  }
  if strings.Contains(path,"/"){return "",errors.New("нужна ссылка именно на профиль ВК")}
  unescaped,err:=url.PathUnescape(path);if err!=nil{return "",errors.New("неверный адрес профиля")}
  input=strings.TrimPrefix(unescaped,"@")
 }
 return normalizeVKScreenName(input)
}
func normalizeVKScreenName(input string)(string,error){
 name:=strings.TrimSpace(input)
 if strings.HasPrefix(strings.ToLower(name),"club")||strings.HasPrefix(strings.ToLower(name),"public")||strings.HasPrefix(strings.ToLower(name),"event"){
  return "",errors.New("укажите профиль человека, не сообщество ВК")
 }
 id:=name
 if strings.HasPrefix(strings.ToLower(id),"id"){id=id[2:]}
 if len(id)>0&&strings.Trim(id,"0123456789")==""{
  number,err:=strconv.ParseInt(id,10,64)
  if err!=nil||number<1||number>2147483647{return "",errors.New("неверный числовой ID профиля ВК")}
  return strconv.FormatInt(number,10),nil
 }
 if !vkScreenName.MatchString(name){return "",errors.New("неверный короткий адрес профиля ВК")}
 return name,nil
}
// Resolve screen names only through the authenticated VK API and accept
// user objects exclusively; a community can never be granted bot admin.
func resolveVKAdminID(ctx context.Context,v *VK,raw string)(int64,error){
 name,err:=parseVKUserReference(raw);if err!=nil{return 0,err}
 if number,err:=strconv.ParseInt(name,10,64);err==nil{return number,nil}
 if v==nil||v.token==""{return 0,errors.New("для короткой ссылки сначала укажите токен сообщества ВК")}
 reply,err:=v.call(ctx,"utils.resolveScreenName",url.Values{"screen_name":{name}})
 if err!=nil{return 0,fmt.Errorf("ВК не смог разрешить короткий адрес: %w",err)}
 var user struct{Type string `json:"type"`; ObjectID int64 `json:"object_id"`}
 if err:=json.Unmarshal(reply,&user);err!=nil||user.Type!="user"||user.ObjectID<1{
  return 0,errors.New("ссылка не ведёт на доступный профиль человека ВК")
 }
 return user.ObjectID,nil
}
