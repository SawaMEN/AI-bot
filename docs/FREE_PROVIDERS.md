# Бесплатные модели VK-бота — ноябрь 2026

Рабочий каталог содержит **Gemini, LLM7, G4F, OpenRouter и Cloudflare Workers AI**. Другие прежние провайдеры исключены. Существующие ключи Gemini, OpenRouter и Cloudflare, а также Cloudflare Account ID сохраняются при обновлении.

| Провайдер | Доступ | Модели |
| --- | --- | --- |
| Google Gemini | Бесплатная квота с Google API-ключом | Сохранённые Gemini Flash и Flash Lite |
| LLM7 | Публичный сторонний API без регистрации и ключа | DeepSeek V4 Flash, Gemma 4 31B |
| G4F Groq | Публичный сторонний API без регистрации и ключа | GPT-OSS 120B |
| OpenRouter | **Аккаунт и бесплатный API-ключ обязательны** | Только текущие модели с суффиксом `:free` и нулевой стоимостью входных и выходных токенов |
| Cloudflare Workers AI | **Cloudflare Account ID и API-токен обязательны** | GLM 4.7 Flash, Gemma 4 26B, Nemotron 3 120B |

### Бесплатность и пределы

- У OpenRouter модели `:free` доступны без покупки кредитов, но действуют ограничения около **50 запросов в сутки** и **20 запросов в минуту** (по актуальным правилам бесплатного уровня); даже неудачные попытки расходуют дневную квоту. Каталог обновляется, кандидаты проверяются отдельно.
- Cloudflare Workers Free предоставляет **10 000 neurons в сутки**, с запретом использования после исчерпания лимита. Отобраны три модели, которые официально сохраняют доступ на Free; более дорогие, включая некоторые DeepSeek V4, требуют платного плана. Бот не повышает тариф автоматически.
- У LLM7 и G4F нет гарантии стабильной доступности, постоянства модели или сохранения условий. VK показывает только **прошедшие фактическую проверку ответа на русском языке с VDS**.

Источники условий: https://openrouter.ai/collections/free-models ; https://openrouter.ai/pricing ; https://developers.cloudflare.com/workers-ai/platform/pricing/ ; https://developers.cloudflare.com/changelog/post/2026-07-28-models-require-workers-paid/ .

### Настройка

Запустите `sudo aibot menu` → «Подключения» → «Бесплатные ИИ». Для OpenRouter создайте бесплатный ключ в https://openrouter.ai/settings/keys , для Cloudflare — API-токен и получите Account ID в https://dash.cloudflare.com/ . Для Gemini сохранённый ключ продолжает использоваться. Затем `sudo aibot check-ai` для реальной проверки с сервера. Список доступных моделей в ВК не покажет кандидатов без успешной проверки.

**Claude:** бесплатный гостевой доступ в Duck.ai не является официальным публичным API для VK-бота. Автоматическое использование Duck.ai в качестве стороннего ИИ-сервиса запрещено условиями. Поэтому Claude через Duck.ai не добавлен. Официальные API Claude и DeepSeek не дают анонимный бесплатный доступ; DeepSeek в каталоге предоставляется сторонним LLM7.

Источники: https://duckduckgo.com/duckai/privacy-terms , https://api-docs.deepseek.com/api/deepseek-api/ , https://support.claude.com/en/articles/8114521-how-can-i-access-the-claude-api .

### Приватность

При отправке ответа часть истории беседы ВК уходит выбранному внешнему провайдеру. Ключи API не печатаются в ВК, не передаются в LLM7/G4F и не вставляются в ответы. Бесплатный доступ не гарантируется в регионах VDS или при исчерпании квот. Платного запасного маршрута нет.

Проверки: `go test ./go/...`, `go test -race ./go/...`, `sudo aibot check-ai`.
