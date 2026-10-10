# GitLab notifier

`cmd/notifier` — вспомогательный компонент модуля 01. Он раз в заданный интервал
читает GitLab REST API и отправляет новые успешные события в Telegram. У
контейнера нет входящего порта и webhook endpoint.

## Конфигурация

| Переменная | Обязательна | Назначение |
|---|---:|---|
| `GITLAB_API_URL` | нет | Base URL API, по умолчанию `https://gitlab.com/api/v4` |
| `GITLAB_PROJECT_ID` | да | Числовой ID или path проекта |
| `GITLAB_READ_TOKEN` | нет | Локальный read-only token для закрытого проекта |
| `TELEGRAM_BOT_TOKEN` | да | Token Telegram bot |
| `TELEGRAM_CHAT_ID` | да | ID тестового chat |
| `NOTIFIER_POLL_INTERVAL` | нет | Интервал опроса, по умолчанию `15s` |
| `NOTIFIER_STATE_PATH` | нет | State file, по умолчанию `/var/lib/notifier/state.json` |

Tokens и chat ID должны находиться только в `deploy/notifier/.env`. Их нельзя
передавать runner или CI jobs, печатать в logs и добавлять в Git.

## Типы сообщений

- `pipeline:<id>:mr` — MR прошёл проверки и готов к ревью;
- `pipeline:<id>:ready` — job `publish-image` опубликовал image, артефакт
  готов к продвижению, но ещё не развёрнут;
- `deployment:<id>` — job `verify-development` подтвердил, что Argo CD
  синхронизировал нужную revision, workloads используют ожидаемый image,
  smoke и ingress checks успешны.

Pipeline с source `pipeline`, запущенная для validation promotion MR, не
публикует image и поэтому не создаёт ложное событие `pipeline:<id>:ready`.
Изменение desired state становится deployment только после успешной read-only
проверки окружения, а не в момент создания или merge MR.

При первом успешном опросе notifier сохраняет уже существующие keys без
отправки сообщений. После успешного `sendMessage` key атомарно записывается в
state file. Повреждённый state не перезаписывается автоматически.
Этот же state store подавляет повторную отправку уже подтверждённых
`pipeline:*` и `deployment:*` событий.

Telegram Bot API не поддерживает idempotency key. Если Telegram принял
сообщение, но network timeout не позволил получить ответ, следующая попытка
может создать один повтор. Подтверждённые ответы повторно не отправляются.
