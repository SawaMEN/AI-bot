package main

import (
 "sort"
 "strings"
 "time"
)

type modelMetrics struct{
 Attempts int
 Successes int
 TotalLatencyMS int64
 LastFailure int64
 LastSuccess int64
}

func(s *Store) recordModelAttempt(model string,success bool,latency time.Duration){
 if model==""{return}
 ms:=latency.Milliseconds();if ms<0{ms=0};if ms>300000{ms=300000}
 passed:=0;lastFailure,lastSuccess:=int64(0),int64(0)
 if success{passed=1;lastSuccess=time.Now().Unix()}else{lastFailure=time.Now().Unix()}
 _,_ =s.DB.Exec(`INSERT INTO model_metrics(model,attempts,successes,total_latency_ms,last_failure,last_success)
 VALUES(?,1,?,?,?,?)
 ON CONFLICT(model) DO UPDATE SET attempts=attempts+1,
 successes=successes+excluded.successes,
 total_latency_ms=total_latency_ms+excluded.total_latency_ms,
 last_failure=MAX(last_failure,excluded.last_failure),
 last_success=MAX(last_success,excluded.last_success)`,model,passed,ms,lastFailure,lastSuccess)
}
func(s *Store) modelMetrics()map[string]modelMetrics{
 result:=map[string]modelMetrics{}
 rows,err:=s.DB.Query("SELECT model,attempts,successes,total_latency_ms,last_failure,last_success FROM model_metrics")
 if err!=nil{return result}
 defer rows.Close()
 for rows.Next(){
  var key string;var m modelMetrics
  if rows.Scan(&key,&m.Attempts,&m.Successes,&m.TotalLatencyMS,&m.LastFailure,&m.LastSuccess)==nil{result[key]=m}
 }
 return result
}
func complexQuestion(question string)bool{
 q:=strings.ToLower(question)
 if len([]rune(q))>330{return true}
 for _,term:=range []string{"объясни подробно","сравни","проанализируй","напиши код","пошагово","доказательство","разбери","алгоритм","план разработки"}{
  if strings.Contains(q,term){return true}
 }
 return false
}
func modelHeuristic(key string,complex bool)float64{
 id:=strings.ToLower(key)
 large:=false
 for _,part:=range []string{"120b","70b","72b","80b","32b","35b","deepseek-r1","thinking","reasoner","-pro"}{
  if strings.Contains(id,part){large=true;break}
 }
 if complex{if large{return 12};return 0}
 if large{return -4}
 for _,part:=range []string{"lite","flash","mini","8b","4b","3b","20b"}{
  if strings.Contains(id,part){return 5}
 }
 return 0
}
func modelReliabilityScore(m modelMetrics)float64{
 if m.Attempts<3{return 0}
 success:=float64(m.Successes)/float64(m.Attempts)
 latency:=float64(m.TotalLatencyMS)/float64(m.Attempts)/1000
 if latency>60{latency=60}
 penalty:=0.0
 if m.LastFailure>m.LastSuccess&&time.Since(time.Unix(m.LastFailure,0))<15*time.Minute{penalty=25}
 return success*32-latency/3-penalty
}
func(s *Store) rankedModels(allowed []string,question string)[]string{
 output:=append([]string(nil),allowed...)
 metrics:=s.modelMetrics()
 complex:=complexQuestion(question)
 sort.SliceStable(output,func(i,j int)bool{
  score:=func(k string)float64{return modelHeuristic(k,complex)+modelReliabilityScore(metrics[k])}
  return score(output[i])>score(output[j])
 })
 return output
}
func(s *Store) explicitAIModel(cfg Config,peer,user int64,c *Catalog)bool{
 available:=map[string]bool{}
 for _,key:=range c.enabled(cfg){available[key]=true}
 if available[s.getPref("user",user).Model]{return true}
 if peer>=2000000000&&available[s.getPref("chat",peer).Model]{return true}
 return false
}
