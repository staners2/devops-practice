# Практика DevOps

Для теоретического интенсива согласован план: 14 дней по 4 часа, без
обязательных практических заданий. Отдельный репозиторий интенсива пока
не создан. Этот репозиторий — следующий этап: сначала небольшая
базовая реализация по каждой теме, затем необязательное углубление.

## Порядок прохождения

Linux/сети → Git → Docker → CI/CD → Ansible → Kubernetes → PostgreSQL → Redis → Kafka → Terraform.

В каждой теме `01-basic` — базовое задание. Задания `02-*`, `03-*` и далее
называются по содержанию и являются необязательным углублением. Для прохождения
базового маршрута не требуется завершать сложные задания предыдущей темы.

| Тема | Базовая практика | Необязательное углубление |
| --- | --- | --- |
| [01-linux-networks: Linux и сети](01-linux-networks/README.md) | [01-basic](01-linux-networks/01-basic/README.md) | [02-incidents](01-linux-networks/02-incidents/README.md) |
| [02-git: Git](02-git/README.md) | [01-basic](02-git/01-basic/README.md) | Пока нет заданий |
| [03-docker: Docker](03-docker/README.md) | [01-basic](03-docker/01-basic/README.md) | [02-application](03-docker/02-application/README.md) |
| [04-ci-cd: CI/CD](04-ci-cd/README.md) | [01-basic](04-ci-cd/01-basic/README.md) | [02-gitlab-delivery](04-ci-cd/02-gitlab-delivery/README.md) |
| [05-ansible: Ansible](05-ansible/README.md) | [01-basic](05-ansible/01-basic/README.md) | [02-two-servers](05-ansible/02-two-servers/README.md) |
| [06-kubernetes: Kubernetes](06-kubernetes/README.md) | [01-basic](06-kubernetes/01-basic/README.md) | [02-helm](06-kubernetes/02-helm/README.md), [03-network-storage-access](06-kubernetes/03-network-storage-access/README.md), [04-gitops](06-kubernetes/04-gitops/README.md), [05-observability](06-kubernetes/05-observability/README.md) |
| [07-postgresql: PostgreSQL](07-postgresql/README.md) | [01-basic](07-postgresql/01-basic/README.md) | [02-backup-patroni](07-postgresql/02-backup-patroni/README.md) |
| [08-redis: Redis](08-redis/README.md) | [01-basic](08-redis/01-basic/README.md) | Пока нет заданий |
| [09-kafka: Kafka](09-kafka/README.md) | [01-basic](09-kafka/01-basic/README.md) | [02-consumer-groups](09-kafka/02-consumer-groups/README.md) |
| [10-terraform: Terraform](10-terraform/README.md) | [01-basic](10-terraform/01-basic/README.md) | [02-yandex-cloud](10-terraform/02-yandex-cloud/README.md) |

Сквозной усложнённый маршрут использует приложение: Docker/application →
CI/CD/gitlab-delivery → Kubernetes/helm → gitops → observability →
PostgreSQL/backup-patroni. Другие сложные задания допускают самостоятельную
подготовку по своим условиям. Старые номера в технических именах ресурсов
сохранены; путь задания и навигация выше задают актуальную нумерацию.

## Структура и инициализация

```text
README.md
artifacts/
01-linux-networks/
  README.md
  01-basic/
  02-incidents/
...
06-kubernetes/
  README.md
  01-basic/
  02-helm/
  03-network-storage-access/
  04-gitops/
  05-observability/
```

Требуются Bash и Python 3. После одного клонирования из корня курса выполни:

```bash
bash artifacts/init.sh 06 01
cd 06-kubernetes/01-basic/workspace
```

Первый аргумент — номер темы, второй — номер задания. Для углубления Helm:
`bash artifacts/init.sh 06 02`. Скрипт создаёт `workspace` внутри выбранного
задания и не перезаписывает существующую работу. Рабочие каталоги исключены
из Git курса; для сдачи создай свой Git-репозиторий внутри рабочей копии.

Общие артефакты хранятся в `artifacts/common`; исходная версия приложения
собирается из общих файлов и отличий `artifacts/overrides/00`. Изменение общих
файлов не обновляет ранее созданные рабочие копии. Базовые задания, где ученик
сам пишет конфигурацию, получают пустую рабочую папку со ссылкой на условие;
Kafka получает подготовленный стенд. Сложные задания получают материалы
по своему профилю. Готовые реализации в artifacts служат примерами. Профили копирования
заданы в `artifacts/profiles.json`; при добавлении задания добавь туда его
путь и подходящий профиль.

## Что требуется

Для базового маршрута требования указаны в каждом `01-basic`: учебная Linux VM,
Git, Docker, предоставленный runner для CI/CD, Ansible, готовый Kubernetes-кластер
и Terraform по мере прохождения соответствующих тем. Платное облако и собственная
настройка runner для базового маршрута не требуются.

Для полного углублённого маршрута:

- Компьютер с 16 ГБ RAM для полного локального маршрута. Не держи все стенды
  запущенными одновременно; ограничения конкретного модуля указаны в задании.
- Git, терминал, Docker Desktop либо Docker Engine с Compose и GNU Make
  для стартового приложения. Дополнительные инструменты и закреплённые версии
  устанавливай по инструкциям выбранного модуля.
- Доступ к интернету для получения образов, пакетов и работы с GitLab.
- Аккаунт GitLab.com, учебный проект и собственный runner для CI/CD-модулей;
  Telegram-бот и чат для задания с уведомлениями.
- Для Ansible — гипервизор и две учебные Ubuntu VM, SSH и control node.
- Для Terraform — аккаунт Yandex Cloud с платёжными реквизитами, правами
  создания учебных ресурсов и согласованным бюджетом.
- Для Linux-инцидентов — отдельная Ubuntu Server 24.04 LTS VM со snapshot.

На Windows используй окружение, указанное в инструкции модуля: для Kafka
подходит Git Bash с Docker Desktop в режиме Linux containers, для остальных
инструментов проверь требования в runbook. Поддержка ОС и архитектуры зависит
от выбранного стенда.

Требования к полному сложному маршруту не являются требованиями к каждой
базовой практике: используй раздел «Требуется» выбранного задания. Например,
базовый Terraform работает с local_file и не требует платного облака.

## Материалы и состояние готовности

- [Теория для повторения](artifacts/questions/).
- [Инструкции подготовки стендов](artifacts/common/docs/runbooks/).
- [Описание учебного приложения](artifacts/common/docs/application.md).

Для Linux-инцидентов сформулированы условия, но плейбуки подготовки
неисправностей пока не реализованы. Стенды требуют отдельной проверки на
платформе ученика. Облачное углубление платное; бюджет указан в задании.
