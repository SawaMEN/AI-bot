package main

import "fmt"

// The console follows the same separation as the VK menu: AI services,
// assistant functions, and VK integration are independent destinations.
func consoleMainOptions(cfg Config,c *Catalog)[]menuOption{
 active:=0
 for _,id:=range c.Order{if cfg.Providers[id].Enabled{active++}}
 vkStatus:="Не настроен"
 if cfg.VKToken!=""&&cfg.GroupID>0{vkStatus="Подключён"}
 return []menuOption{
  {1,"ИИ и модели","Провайдеры, бесплатные модели, выбор основной модели",fmt.Sprintf("%d сервисов • %d моделей",active,len(c.enabled(cfg)))},
  {2,"Функции бота","Поиск, резерв, контекст, история и лимиты",""},
  {3,"ВКонтакте","Сообщество, токен и получение событий",vkStatus},
  {4,"Диагностика","Проверка ИИ и состояния, сеть, журнал",""},
  {5,"Веб-панель","Управление через браузер",""},
  {6,"Обслуживание","Перезапуск и очистка истории",""},
  {7,"Помощь","Инструкция по работе бота в беседе",""},
  {8,"Мастер настройки","Первое подключение ВК и ИИ",""},
  {0,"Выход","Закрыть управление",""},
 }
}

func(w Wizard) consoleAIHub(cfg *Config,c *Catalog){
 w.setupModels(cfg,c)
 // A chosen provider or default model has been persisted in the wizard,
 // but the running VK bot loads the config only on startup.
 applySettings(w,*cfg,c)
}

func(w Wizard) consoleVKHub(cfg *Config,c *Catalog){
 for {
  state:="Не настроен"
  if cfg.VKToken!=""&&cfg.GroupID>0{state=fmt.Sprintf("ID %d",cfg.GroupID)}
  action:=w.chooseMenu("ВКОНТАКТЕ","Подключение сообщества и обработка событий — отдельно от ИИ и функций.",[]menuOption{
   {1,"Подключить сообщество","Токен и числовой ID",state},
   {2,"Настроить события","Long Poll: входящие сообщения и кнопки",""},
   {3,"Инструкция по ВК","Как включить сообщения и добавить бота в беседу",""},
   {0,"Назад","Главное меню",""},
  })
  switch action{
  case 0:return
  case 1:w.setupVK(cfg);applySettings(w,*cfg,c)
  case 2:w.configureVKEvents(*cfg)
  case 3:fmt.Fprintln(w.out,vkSetupGuide(cfg.GroupID))
  }
  w.pauseMenu()
 }
}

func(w Wizard) consoleFunctionsHub(cfg *Config,c *Catalog){
 for{
  options:=[]menuOption{
   {1,"Поиск и резерв","Веб-поиск и переключение на запасной ИИ",fmt.Sprintf("Поиск %s • резерв %s",switchLabel(cfg.SearchEnabled),switchLabel(cfg.FreeFallback))},
   {2,"Контекст и история","Число сообщений, время контекста, хранение",""},
   {3,"Лимиты и частота","Запросы по пользователю, беседе и серверу",""},
   {0,"Назад","Главное меню",""},
  }
  action:=w.chooseMenu("ФУНКЦИИ БОТА","Здесь нет ключей, моделей и настроек сообщества.",options)
  if action==0{return}
  switch action{
  case 1:
   cfg.SearchEnabled=w.yes("Искать в интернете",cfg.SearchEnabled)
   cfg.FreeFallback=w.yes("Использовать другой подтверждённый ИИ при ошибке",cfg.FreeFallback)
  case 2:
   cfg.ContextMessages=w.integer("Число сообщений в контексте (1–100)",cfg.ContextMessages,1,100)
   cfg.ContextHours=w.integer("Возраст сообщений, часов (1–168)",cfg.ContextHours,1,168)
   cfg.RetentionDays=w.integer("Хранить историю, дней (1–30)",cfg.RetentionDays,1,30)
  case 3:
   cfg.MaxRequestsHour=w.integer("Запросы бота в час",cfg.MaxRequestsHour,1,10000)
   cfg.MaxUserRequestsHour=w.integer("Запросы участника в час",cfg.MaxUserRequestsHour,1,1000)
   cfg.MaxChatRequestsHour=w.integer("Запросы беседы в час",cfg.MaxChatRequestsHour,1,1000)
   cfg.CooldownSeconds=w.integer("Пауза между ответами, секунд (0–3600)",cfg.CooldownSeconds,0,3600)
  default:continue
  }
  applySettings(w,*cfg,c)
  w.pauseMenu()
 }
}
