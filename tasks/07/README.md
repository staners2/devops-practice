# Модуль 07. Ansible: управляемое состояние двух серверов

## Подготовка рабочей копии

Из корня курса:

```bash
bash tasks/init.sh 07
cd tasks/07/workspace
```

Все команды задания выполняй из этого рабочего каталога.

Одна обязательная практика уровня Hard. Выполняется отдельно от `00`–`06`:
нужны две локальные Ubuntu VM, но не предыдущий Kubernetes/CI стенд.
Подготовка: [bootstrap VM и control node](../../artifacts/common/docs/runbooks/07-ansible-bootstrap.md).

## Задача от руководителя

> Сервис заявок готов, нам нужно воспроизводимое развёртывание на двух
> Linux-серверах. На `app-01` запускаем API, worker, PostgreSQL, Redis и
> Kafka через Docker Compose. На `proxy-01` устанавливаем обычный Nginx.
> Все обращения пользователя идут через него.
>
> Настройте серверы через Ansible. Разделите общие, групповые и адресные
> параметры; секреты храните в Vault. Приложение должно восстанавливаться
> после перезагрузки. Повторный playbook не должен перезапускать исправный
> runtime. Некорректный Nginx-конфиг нельзя активировать.
>
> Продемонстрируйте прогноз изменений, ограниченное применение, исправление
> ручной правки, выпуск новой версии и rollback без потери заявок. Если новая
> версия не работает, верните принятую конфигурацию и сообщите об ошибке
> обновления: успешное восстановление не делает релиз успешным.
>
> Сдайте код и результаты экспериментов. Решение должно быть понятно другому
> инженеру и повторяться без ручного исправления серверов.

## Что уже подготовлено

| Материал | Для чего |
| --- | --- |
| Go-код и [Dockerfile](../../artifacts/common/Dockerfile) | приложение, worker, миграции, smoke |
| [Базовый Compose](../../artifacts/common/deploy/compose/compose.yaml) | описание сервисов |
| [VM override](../../artifacts/common/deploy/ansible-lab/compose.vm.yaml) | данные, binding, resource limits |
| [Версии](../../artifacts/common/deploy/ansible-lab/versions.env) | Ansible, коллекция, Docker, Compose |
| [Подготовка image](../../artifacts/common/scripts/ansible-lab/prepare-images.sh) | два релиза и controlled failed image |
| [Проверка потока](../../artifacts/common/scripts/ansible-lab/verify.sh) | Nginx → API → PostgreSQL/Kafka → worker → Redis |

Compose менять не требуется. Автоматизация получения пакетов, размещения
файлов, env, Nginx-шаблона, unit и deployment — ваша работа.
Готовые Ansible roles текущего модуля не входят в starter.

## Требуемый flow

1. Подготовить две VM по runbook и проверить SSH/Python/`sudo`.
2. Создать `ansible/` с inventory `app` / `proxy`, `group_vars`, `host_vars`,
   `site.yml` и roles `base`, `docker`, `application`, `nginx`.
3. Применить свою роль Docker на `app-01`, подготовить образы готовым helper.
4. Установить Nginx на `proxy-01`, развернуть приложение на `app-01`.
5. Проверить сервис через Nginx и выполнить эксплуатационные опыты ниже.

Управляйте обеими VM по SSH с `app-01`, даже самой `app-01`.
Используйте `become`, а не root SSH. Не отключайте host key checking.

## Контракт приложения

Размещайте runtime в `/opt/devops-practice-ansible-07`. Каталог содержит
копии двух подготовленных Compose-файлов, root-owned `.env` с mode `0600`
и необходимую вам автоматизацию запуска.

| Переменная env | Значение/источник |
| --- | --- |
| `APPLICATION_IMAGE` | выбранный локальный image, общий для API/worker |
| `TOOL_IMAGE` | `devops-practice:lab-07-v1` для миграций и smoke |
| `API_BIND_IP` | постоянный частный IP `app-01` |
| `PROXY_BASE_URL` | `http://` и частный IP `proxy-01` |
| `POSTGRES_USER` | `devops` |
| `POSTGRES_DB` | `devops_practice` |
| `POSTGRES_PASSWORD` | ваше единожды созданное значение из Vault |

Создайте пароль через `openssl rand -hex 24`, сохраните в Vault, не включайте
в Git plaintext, `.env`, private key и пароль самого Vault. Задачи секретов
не раскрывают diff/log: используйте `no_log` и `diff: false`.
Vault шифрует сохранённый файл, но root на VM читает runtime environment.

Всегда указывайте оба Compose-файла и project `devops-practice-ansible-07`.
PostgreSQL сохраняется в named volume на `/var/lib/postgresql` (образ 18),
Kafka использует собственный named volume. Нельзя применять `down -v`,
volume prune или общий project предыдущего модуля.

Nginx слушает HTTP 80 и проксирует на частный `app-01:8080`. БД, Redis и
Kafka не публикуют порты VM. TLS, внешний DNS и публичный доступ не нужны.
Для проверки template candidate нужен полный main config, не отдельный
`server` fragment. Проверяйте его до замены рабочего файла, затем reload
через handler только при реальном изменении.

## Эксплуатационные опыты

### 1. Inventory и доступ

Покажите inventory graph, результат SSH/Ansible ping и `become` на обеих VM.
Объясните, какие параметры принадлежат `all`, `app`, `proxy`, `host_vars`,
и что произойдёт при переопределении через extra vars.

### 2. Первичная установка

Примените roles к чистой среде. С ПК проверьте адрес Nginx:

```bash
read -r -p 'Частный IP proxy-01: ' PROXY_IP
curl --fail --silent --show-error "http://$PROXY_IP/api/health-check"
```

На `app-01` выполните готовую сквозную проверку:

```bash
read -r -p 'Частный IP proxy-01: ' PROXY_IP
sudo bash scripts/ansible-lab/verify.sh /opt/devops-practice-ansible-07 "http://$PROXY_IP"
```

Smoke создаёт заявку, ждёт `processed`, проверяет Redis, PostgreSQL fallback
и обработку повторного Kafka event. Это проверка с изменением учебных данных,
а не read-only health endpoint. Не запускайте её при каждом неизменённом playbook.

### 3. Идемпотентность

Повторите тот же playbook с теми же переменными. Приложите recap и безопасные
снимки container IDs, image IDs, checksum несекретных конфигов и Nginx worker
processes до/после. Контейнеры не пересоздаются, handler не вызывает reload.
Не обманывайте recap глобальным `changed_when: false`.

### 4. Check и diff

После первичной установки измените несекретный параметр шаблона Nginx,
например заголовок ответа, и выполните `--check --diff`.
Покажите прогноз, неизменность рабочего файла и процессов. Затем примените.
На пустом сервере check mode не создаёт зависимости для следующих tasks;
объясните ограничения. Не выполняйте мутации через `check_mode: false`.

### 5. Limit

Примените изменение только с `--limit proxy`. Докажите, что на `app-01`
не изменились управляемые файлы, сервисы и container IDs. Адрес upstream
можно читать из inventory, но нельзя скрыто менять другой host через delegation.

### 6. Ручное расхождение

Измените управляемый Nginx-конфиг непосредственно на `proxy-01`, сохранив
синтаксическую корректность. Следующий playbook должен вернуть desired state.
Сравните checksum и HTTP-результат. Исправляйте через исходный шаблон,
а не оставляйте ручную серверную правку новым источником истины.

### 7. Отказ до активации

Внесите синтаксическую ошибку в template source. Playbook должен завершиться
ошибкой до изменения live config. Предыдущий checksum и доступность сервиса
сохраняются, reload handler не вызывается. Исправьте source и повторите.
Проверка только после записи в handler не выполняет это требование.

### 8. Vault и вывод

Покажите encrypted-файл, запуск с запросом Vault password, права `.env`
и отсутствие значения пароля в выводе, diff и отчёте. Не показывайте полный
`docker inspect`, environment контейнера или раскрытый `compose config`.
Секрет и его hash в отчёт не нужны.

### 9. Обновление и возврат версии

Создайте контрольную заявку через Nginx и сохраните UUID:

```bash
curl --fail --silent --show-error -X POST "http://$PROXY_IP/api/requests" \
  -H 'Content-Type: application/json' -d '{"title":"Контроль сохранности после релиза"}'
```

Дождитесь `processed` через `GET /api/requests/UUID`. Обновите выбранную
версию с `devops-practice:lab-07-v1` на `lab-07-v2`, затем верните `v1`.
Первые два образа отличаются метаданными и IDs, не схемой или бизнес-функцией.
Tags нельзя переназначать во время опыта.

Сохранность подтвердите непосредственно PostgreSQL, передав UUID helper:

```bash
read -r -p 'UUID контрольной заявки: ' REQUEST_ID
sudo bash scripts/ansible-lab/verify.sh /opt/devops-practice-ansible-07 "http://$PROXY_IP" "$REQUEST_ID"
```

Сравните IDs API/worker, неизменность volumes и возможность обработать новую
заявку после возврата. Ответ только из Redis недостаточен как доказательство БД.

### 10. Неудачный deployment

Сначала укажите несуществующий image: automation должна отклонить его до
остановки исправного сервиса. Затем используйте подготовленный
`devops-practice:lab-07-failed`, который существует, но не запускает API/worker.
Верните предыдущие принятые image и конфигурацию, проверьте полный поток.
Итог попытки обновления — ошибка даже после успешного rollback.
При первом deployment без предыдущей версии честно сообщите невозможность
rollback. Не откатывайте схему БД и не удаляйте данные ради восстановления.

### 11. Перезагрузка

Перезагрузите обе VM. Control node тоже находится на `app-01`, поэтому
ожидается обрыв его SSH-сеанса, а не непрерывная работа Ansible.
После возвращения VM подключитесь заново: Docker, Nginx и приложение
восстановились без ручного `up`, контрольная заявка сохранилась.
Проверьте новую заявку через Nginx. `systemctl is-active` unit не заменяет
health и полный поток; все ожидания должны иметь конечный deadline.

### 12. Заключительная проверка

Выполните `--syntax-check`, `ansible-lint`, повторный playbook и сохраните
результаты. Не скрывайте реальные ошибки через `ignore_errors`.
Запуски package cache refresh отдельно отличайте от перезапуска runtime.

## Что сдавать

- ваш `ansible/`: inventory template без секретов, переменные, roles,
  templates, handlers, unit и процедура запуска;
- команды первоначальной установки, check/diff, limit и rollback;
- результаты всех 12 опытов, с конкретными UUID/IDs/checksums и exit codes;
- ограничения: одна API VM, возможен downtime, локальная сеть, root видит
  runtime secrets, check mode не является полноценным deployment;
- краткие ответы: зачем нужны handlers и Vault, что означает идемпотентность,
  чем возврат image отличается от восстановления БД, где нужна другая
  архитектура для production.

Описывайте это как собственную лабораторную практику, не как вымышленный
production-опыт. Для защиты нужно суметь повторить наблюдения руками.
