package main

// Keep the administrator's default as initial automatic preference, while
// allowing measured reliability and latency to improve future decisions.
func(s *Store) chooseAutoModel(cfg Config,allowed []string,question string)string{
 ranked:=s.rankedModels(allowed,question)
 if len(ranked)==0{return ""}
 if cfg.DefaultModel!=""{
  valid:=false
  for _,key:=range allowed{if key==cfg.DefaultModel{valid=true;break}}
  if valid{
   observed:=false
   for _,stats:=range s.modelMetrics(){if stats.Attempts>=3{observed=true;break}}
   if !observed{return cfg.DefaultModel}
  }
 }
 return ranked[0]
}
