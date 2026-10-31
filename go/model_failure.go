package main

import (
 "context"
 "errors"
 "fmt"
 "strings"
)

// Only structured, secret-free status codes are written to model-health.json
// or displayed in VK. Never include the provider's response body, URL query
// or the API key in a model diagnosis.
func classifyModelFailure(err error,englishOnly int)string{
 if err==nil{
  if englishOnly==2{return "russian_unsupported"}
  return "invalid_response"
 }
 message:=strings.ToLower(err.Error())
 switch{
 case strings.Contains(message,"http 401"),strings.Contains(message,"http 403"):
  return "auth"
 case strings.Contains(message,"http 429"):
  return "rate_limit"
 case strings.Contains(message,"http 402"):
  return "billing"
 case strings.Contains(message,"http 404"),strings.Contains(message,"http 410"):
  return "not_found"
 case strings.Contains(message,"http 400"),strings.Contains(message,"http 422"):
  return "request_rejected"
 case strings.Contains(message,"api не вернул текст"),strings.Contains(message,"api вернул пустой"):
  return "empty_response"
 case errors.Is(err,context.DeadlineExceeded),errors.Is(err,context.Canceled),
  strings.Contains(message,"timeout"),strings.Contains(message,"deadline exceeded"),
  strings.Contains(message,"network"),strings.Contains(message,"tls"),strings.Contains(message,"dial tcp"),
  strings.Contains(message,"no such host"),strings.Contains(message,"connection refused"):
  return "network"
 case strings.Contains(message,"http 5"):
  return "server_error"
 default:return "api_error"
 }
}

func modelFailureLabel(reason string)string{
 switch reason{
 case "auth":return "API отклонил ключ или права доступа (HTTP 401/403)"
 case "rate_limit":return "Исчерпана бесплатная квота или превышен лимит частоты (HTTP 429)"
 case "billing":return "Не хватает бесплатного баланса / требуется оплата (HTTP 402)"
 case "not_found":return "Модель не существует или не доступна по этому ключу (HTTP 404/410)"
 case "request_rejected":return "Провайдер отклонил формат запроса (HTTP 400/422)"
 case "empty_response":return "API ответил, но не вернул текст"
 case "network":return "Ошибка сети, TLS или превышено время ответа"
 case "server_error":return "Временная ошибка сервера провайдера (HTTP 5xx)"
 case "russian_unsupported":return "Дважды не подтвердился ответ на русском языке"
 case "invalid_response":return "Ответ получен, но не прошёл проверку"
 case "api_error":return "Ошибка API; точный код не сохранён"
 default:return "Проверка не завершена"
 }
}

func modelFailureRecommendation(reason string)string{
 switch reason{
 case "auth":return "Проверьте действительность API-ключа и разрешения на доступ"
 case "rate_limit":return "Дождитесь обновления квоты или проверьте лимит провайдера"
 case "billing":return "Проверьте баланс и бесплатный тариф провайдера"
 case "not_found":return "Обновите список моделей у провайдера"
 case "request_rejected","empty_response":return "Возможна несовместимость формата API; проверьте версию бота и провайдера"
 case "network":return "Проверьте интернет и TLS на VDS: sudo aibot check-ai network"
 case "server_error":return "Сбой на стороне провайдера; повторите проверку позже"
 case "russian_unsupported","invalid_response":return "Модель не подтвердила корректный русский ответ; проверьте другую"
 default:return "Проверьте API с VDS: sudo aibot check-ai"
 }
}

func modelFailureSummary(reason string)string{
 return fmt.Sprintf("%s. %s.",modelFailureLabel(reason),modelFailureRecommendation(reason))
}
