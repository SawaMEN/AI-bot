package main

import (
 "fmt"
 "strings"
)

func switchLabel(on bool)string {
 if on{return "✓ ВКЛ"}
 return "✗ ВЫКЛ"
}
func readyLabel(on bool)string {
 if on{return "✓ Готово"}
 return "✗ Не настроено"
}

// A VK community is an owned page (bot identity), NOT the chat the bot joins.
// Keep the instructions available in both the terminal and VK help screen.
func vkSetupGuide(groupID int64)string{
 idHint:="ID сообщества пока не указан."
 exampleID:=int64(123456)
 if groupID>0{exampleID=groupID;idHint=fmt.Sprintf("Текущий ID сообщества: %d. Ссылка: https://vk.com/club%d",groupID,groupID)}
 return fmt.Sprintf(`
КАК ПОДКЛЮЧИТЬ ВКОНТАКТЕ

Что такое «сообщество»?
  Это отдельная страница ВК (группа/паблик), от имени которой отвечает
  AI-bot. Это НЕ беседа с друзьями. Бота можно добавить в беседу от
  имени созданного вами сообщества.

ШАГ 1. Создайте или выберите сообщество
  Откройте vk.com/groups → «Создать сообщество» (если его ещё нет).
  У вас должны быть права администратора этого сообщества.

ШАГ 2. Найдите ID сообщества
  Откройте страницу сообщества. Если адрес выглядит как
  vk.com/club123456 или vk.com/public123456, ID — число 123456.
  Если адрес красивый (например vk.com/my_bot), откройте
  «Управление» и найдите числовой ID в сведениях сообщества.
  Не вводите адрес, символы club или знак минус — только число.
  %s

ШАГ 3. Создайте токен (секретный ключ)
  В сообществе: «Управление» → «Настройки» → «Работа с API»
  → «Ключи доступа» → «Создать ключ».
  Разрешите доступ к сообщениям. Скопируйте токен и вставьте
  его ТОЛЬКО в терминал сервера; никому не пересылайте.

ШАГ 4. Разрешите сообщения и события
  В «Управление» включите «Сообщения сообщества».
  В разделе «Настройки для бота» разрешите возможности ботов
  и добавление сообщества в беседы, если эти пункты доступны.
  В «Работа с API» → «Long Poll API» включите его и события
  message_new и message_event (в некоторых версиях ВК названия
  и расположение пунктов немного отличаются).

ШАГ 5. Добавьте сообщество в нужную беседу
  В беседе ВК откройте её название/настройки → добавление
  участников или ботов → выберите ваше сообщество.
  Администратор беседы должен разрешить боту читать сообщения
  («Доступ ко всей переписке», если этот пункт доступен).
  Сообщения до добавления бота ему недоступны.

ШАГ 6. Проверьте ответ
  Напишите в беседе: [club%d|Бот], привет!
  Затем на сервере: sudo aibot doctor
  Если ответа нет, проверьте журнал: sudo aibot logs

Название пунктов в интерфейсе ВК может отличаться.
`,idHint,exampleID)
}
func setupSummary(cfg Config,c *Catalog)string{
 vk:=cfg.VKToken!=""&&cfg.GroupID>0
 provider:=false
 for id,p:=range cfg.Providers{
  spec,ok:=c.Specs[id]
  if ok&&p.Enabled&&(p.APIKey!=""||spec.AnonymousKey!=""){provider=true;break}
 }
 models:=len(c.enabled(cfg))
 var b strings.Builder
 fmt.Fprintf(&b,"  %s  Сообщество ВК (токен и ID)\n",readyLabel(vk))
 fmt.Fprintf(&b,"  %s  Провайдер бесплатного ИИ\n",readyLabel(provider))
 fmt.Fprintf(&b,"  %s  Доступная модель ИИ (%d)\n",readyLabel(models>0),models)
 return b.String()
}
func nextSetupStep(cfg Config,c *Catalog)string{
 if cfg.GroupID<1||cfg.VKToken==""{
  return "Далее: выберите пункт 1 — подключите сообщество ВК (инструкция появится на экране)."
 }
 configured:=false
 for name,p:=range cfg.Providers{
  spec,ok:=c.Specs[name]
  if ok&&p.Enabled&&(p.APIKey!=""||spec.AnonymousKey!=""){configured=true;break}
 }
 if !configured{return "Далее: пункт 2 — выберите бесплатный ИИ, получите его ключ по указанной ссылке и включите."}
 if len(c.enabled(cfg))==0{
  return "Далее: пункт 2 — включите автоматический подбор бесплатных моделей и проверьте их доступность."
 }
 return "Далее: добавьте своё сообщество ВК в беседу, дайте доступ к сообщениям и отправьте упоминание. Затем проверьте пункт 4 (диагностика)."
}
