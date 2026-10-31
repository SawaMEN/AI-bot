package main

import (
 "fmt"
 "sort"
 "strings"
 "time"
)

type usageStat struct{
 Calls24h int
 ActiveChats24h int
 Messages24h int
 Queued int
 OldestWaitMinutes int
 TotalModelAttempts int
 TotalModelSuccesses int
 MeanResponseMS int64
 Models []modelUsageStat
}
type modelUsageStat struct{
 Key string
 Attempts int
 Successes int
 AvgMS int64
}
func(s *Store) usageStats()usageStat{
 var stats usageStat
 since:=time.Now().Add(-24*time.Hour).Unix()
 _=s.DB.QueryRow("SELECT COUNT(*),COUNT(DISTINCT peer_id) FROM messages WHERE created_at>=?",since).Scan(&stats.Messages24h,&stats.ActiveChats24h)
 _=s.DB.QueryRow("SELECT COUNT(*) FROM calls WHERE created_at>=?",since).Scan(&stats.Calls24h)
 var oldest int64
 _=s.DB.QueryRow("SELECT COUNT(*),COALESCE(MIN(created_at),0) FROM pending").Scan(&stats.Queued,&oldest)
 if oldest>0{stats.OldestWaitMinutes=int(time.Since(time.Unix(oldest,0)).Minutes());if stats.OldestWaitMinutes<0{stats.OldestWaitMinutes=0}}
 metrics:=s.modelMetrics()
 var totalMS int64
 for key,m:=range metrics{
  if m.Attempts==0{continue}
  stats.TotalModelAttempts+=m.Attempts
  stats.TotalModelSuccesses+=m.Successes
  totalMS+=m.TotalLatencyMS
  stats.Models=append(stats.Models,modelUsageStat{Key:key,Attempts:m.Attempts,Successes:m.Successes,AvgMS:m.TotalLatencyMS/int64(m.Attempts)})
 }
 if stats.TotalModelAttempts>0{stats.MeanResponseMS=totalMS/int64(stats.TotalModelAttempts)}
 sort.Slice(stats.Models,func(i,j int)bool{return stats.Models[i].Attempts>stats.Models[j].Attempts})
 return stats
}
func percent(part,total int)int{
 if total<=0{return 0}
 return int(float64(part)*100/float64(total)+0.5)
}
func(s *Store) vkUsageStats(cfg Config,c *Catalog)string{
 st:=s.usageStats()
 return fmt.Sprintf("СТАТИСТИКА БОТА\n\nЗа 24 часа:\n• Запросов к ИИ: %d\n• Активных бесед: %d\n• Сообщений: %d\n\nОчередь: %d сообщений\nРаботающих ИИ: %d\nУспешных ответов ИИ: %d%%\nСредняя задержка ИИ: %.1f с\n\nСтатистика ошибок и квот провайдеров доступна только в консоли администратора.",
  st.Calls24h,st.ActiveChats24h,st.Messages24h,st.Queued,len(c.enabled(cfg)),
  percent(st.TotalModelSuccesses,st.TotalModelAttempts),float64(st.MeanResponseMS)/1000)
}
func(s *Store) consoleUsageStats(cfg Config,c *Catalog)string{
 st:=s.usageStats()
 var b strings.Builder
 b.WriteString(s.vkUsageStats(cfg,c))
 fmt.Fprintf(&b,"\n\nЗадержка самой старой задачи: %d мин\nВсего запросов моделей: %d (успешных: %d)\n",
  st.OldestWaitMinutes,st.TotalModelAttempts,st.TotalModelSuccesses)
 for i,m:=range st.Models{
  if i>=20{fmt.Fprintf(&b,"...и ещё %d моделей\n",len(st.Models)-i);break}
  fmt.Fprintf(&b,"%s: %d запросов, %d%% успешных, %.1f с в среднем\n",
   c.label(m.Key),m.Attempts,percent(m.Successes,m.Attempts),float64(m.AvgMS)/1000)
 }
 b.WriteString("\nПровайдеры могут применять собственные лимиты; здесь учитываются реальные обращения бота, но не остаток внешних квот.")
 return b.String()
}
