# Локальный GitLab Runner

Runner выполняет jobs публичного GitLab project на компьютере ученика, поэтому
платные compute minutes не используются. Сам runner работает в контейнере с
лимитом 256 МБ и создаёт job containers через Docker executor.

## Подготовка

1. Скопируй `deploy/runner/.env.example` в `deploy/runner/.env`.
2. Укажи настоящий `GITLAB_PROJECT_ID`, Telegram bot token и test chat ID.
3. Для Docker Desktop или OrbStack проверь путь к Docker socket. Если он
   отличается от `/var/run/docker.sock`, задай его в `DOCKER_HOST_SOCKET`.
4. Запусти stack командой `make runner-up`.

`GITLAB_PROJECT_ID` принимает числовой ID или path вида
`group/devops-practice`. Для публичного project `GITLAB_READ_TOKEN` не нужен.

## Регистрация

В GitLab открой `Settings → CI/CD → Runners`, создай project runner без tags и
скопируй одноразовый authentication token с префиксом `glrt-`.

Выполни:

```bash
make runner-register
```

Скрипт запросит token без отображения на экране, зарегистрирует Docker executor
и проверит `privileged = false`, `concurrent = 1` и отсутствие Docker socket в
job volumes. Не сохраняй одноразовый token в `.env`, shell history или Git.

Проверить состояние:

```bash
make runner-status
make runner-logs
```

Runner container получает host Docker socket, потому что Docker executor должен
создавать изолированные job containers. Внутри самих jobs socket отсутствует;
сборка image выполняется rootless BuildKit без Docker-in-Docker.

## Notifier

Notifier запускается тем же stack, но не получает Docker socket и не принимает
входящие соединения. Первый успешный опрос создаёт baseline без исторических
сообщений. Логи доступны через:

```bash
make notifier-logs
```

State хранится в volume `devops-practice-notifier-state`. Перед переносом
runner этот volume можно отдельно сохранить средствами Docker. Удаление volume
сбросит дедупликацию и снова создаст baseline при следующем запуске.

## Остановка и ротация

`make runner-down` останавливает containers, сохраняя configuration и state
volumes. Для ротации удалить runner в GitLab, остановить stack, удалить только
volume `devops-practice-runner-config`, запустить stack и зарегистрировать новый
token. Telegram и GitLab read tokens заменяются в игнорируемом `.env`, после
чего notifier пересоздаётся командой `make runner-up`.
