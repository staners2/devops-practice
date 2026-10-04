# Модуль 02. Развернуть приложение в Kubernetes

## Заявка от руководителя

У нас есть Go-приложение, которое собирается и проверяется в GitLab CI,
публикуется в GitLab Container Registry и локально работает вместе с
PostgreSQL, Redis и Kafka. Следующий шаг — подготовить сервис к эксплуатации в
Kubernetes.

Разверни локальный Kubernetes-кластер, установи необходимые инфраструктурные
компоненты и оформи приложение собственным Helm chart'ом. Перенеси GitLab
Runner с локального Docker executor в Kubernetes executor. После merge в
`main` новая версия должна автоматически устанавливаться в кластер, проходить
миграцию базы и smoke-проверку.

## Текущее состояние

В стартовом теге находятся:

- Go API, worker, migrator и smoke-команда;
- PostgreSQL, Redis и Kafka для локального запуска через Docker Compose;
- endpoint `GET /api/health-check`;
- GitLab Flow, проверки качества и безопасности;
- rootless-сборка OCI-образа и публикация в GitLab Container Registry;
- локальный container runner с Docker executor;
- notifier, отправляющий события GitLab в Telegram.

Готового Kubernetes-кластера, Helm chart приложения и deploy job нет.

## Рабочая задача

1. Создай локальный кластер `k3d`: один server и два agent nodes, без Traefik.
2. Вручную скачай и установи отдельными Helm-релизами ingress-nginx,
   PostgreSQL, Redis, Kafka и GitLab Runner. Используй предоставленные
   values-файлы, не применяй operators и umbrella chart.
3. Полностью переведи pipeline на один GitLab Runner с Kubernetes executor и
   удали прежний container runner после успешной проверки нового.
4. Добавь `/livez` для состояния процесса и `/readyz` для проверки
   PostgreSQL, Redis и Kafka. Существующий `/api/health-check` сохрани.
5. Разработай Helm chart приложения с двумя replicas API и одной replica
   worker, probes, ресурсными ограничениями, безопасным security context,
   PodDisruptionBudget и NetworkPolicy.
6. Выполняй миграцию PostgreSQL перед обновлением workloads, а smoke-проверку —
   после него. Ошибка любого hook должна блокировать release.
7. Добавь проверки Helm chart в Merge Request pipeline.
8. После merge в `main` публикуй образ с tag `$CI_COMMIT_SHA` и автоматически
   выполняй `helm upgrade --install` с `--atomic` и `--wait`.
9. Создай GitLab environment `development` с URL
   `http://devops-practice.localhost:8080` и сохрани уведомление о deployment в
   существующем Telegram flow.
10. Воспроизведи отказ зависимости, неудачный rollout, ошибку миграции и
    блокировку трафика NetworkPolicy. Объясни диагностику и восстановление.

## Ограничения

- один обязательный уровень сложности Hard;
- только локальные ресурсы и публичный GitLab.com Free, затраты 0 ₽;
- компьютер с 16 ГБ RAM: до 8 ГБ в покое и до 10 ГБ с одной CI job;
- одновременно выполняется только одна CI job;
- Kubernetes устанавливается через `k3d`; production-кластер в этот модуль не
  входит;
- инфраструктурные приложения устанавливаются готовыми Helm charts вручную;
- приложение устанавливается собственным chart только через pipeline;
- запрещены Kubernetes operators, CRD, DinD, privileged mode и Docker socket
  внутри jobs;
- приложение публикуется только с неизменяемым tag commit SHA; `latest`
  запрещён;
- credentials, runner token, kubeconfig и локальные конфигурации не попадают в
  Git, artifacts и job logs;
- один Runner выполняет MR и deploy jobs. Это сознательное упрощение локальной
  практики: его доступ ограничивается namespace приложения, а риск выполнения
  недоверенного MR-кода необходимо описать;
- TLS, cert-manager, Argo CD, autoscaling, observability, backup, HA stateful
  services и Yandex Cloud не входят в модуль;
- ingress-nginx применяется только для упрощённой локальной практики. Проект
  завершил поддержку в марте 2026 года и не подходит как новая
  production-рекомендация; для production рассмотри поддерживаемую реализацию
  Gateway API.

## Что предоставить на ревью

- ссылку на Merge Request и успешный MR pipeline;
- ссылку на успешный main pipeline;
- ссылку на GitLab deployment и environment;
- Helm chart приложения и values-файлы инфраструктуры;
- вывод migration и smoke hooks;
- `helm history` и доказательство atomic rollback;
- команды и результаты проверки probes, двух replicas и NetworkPolicy;
- runbook установки локальной платформы;
- runbook переноса GitLab Runner;
- краткий разбор отказа: симптом, причина, диагностика и восстановление;
- подтверждение удаления прежнего container runner без потери notifier state.

## Критерии приёмки

1. MR pipeline проверяет Go-код, OCI-образ и Helm chart.
2. Main pipeline публикует неизменяемый image tag и автоматически выполняет
   deploy.
3. Migration Job успешно завершается до обновления workloads.
4. API доступен через `http://devops-practice.localhost:8080`.
5. Основной поток проходит через PostgreSQL, Kafka, worker и Redis.
6. Удаление одного API pod не делает Service недоступным.
7. Падение PostgreSQL меняет readiness, но не liveness.
8. Неверный image tag не заменяет предыдущую рабочую версию.
9. Ошибка миграции блокирует release.
10. NetworkPolicy блокирует неразрешённый pod.
11. `helm history` содержит проверяемую историю релиза и rollback.
12. GitLab содержит deployment и environment `development`.
13. Telegram получает одно уведомление об успешном deploy.
14. Старый Docker/container runner отсутствует в GitLab, Compose и локальных
    volumes.
15. Charts, runtime images и CI images закреплены версиями; инфраструктурные
    images закреплены digest.
16. Стенд соблюдает ограничение памяти и выполняет только одну CI job
    одновременно.
