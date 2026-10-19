# Перевести поставку на GitOps с Argo CD

Уровень: **03 — дополнительные задачи, необязательно**. Начинай после соответствующей базовой практики второго уровня.

Исторические номера внутри технических имён стендов сохраняются для совместимости.

## Подготовка рабочей копии

Из корня курса:

```bash
bash 02-interviews-and-practice/artifacts/init.sh 03 06 04
cd 03-advanced/06-kubernetes/04-gitops/workspace
```

Все команды задания выполняй из этого рабочего каталога.

## Заявка от руководителя

Сейчас GitLab Runner напрямую разворачивает приложение в Kubernetes командой
`helm upgrade`. Из-за этого CI имеет права записи в namespace приложения,
ручные изменения кластера не исправляются автоматически, а желаемая версия
окружения определяется параметрами завершившейся job.

Переведи development-окружение на Argo CD и GitOps. Новая версия должна
попадать в кластер только после Merge Request с изменением immutable image
tag. Покажи ручную и автоматическую синхронизацию, восстановление drift,
удаление ресурсов через prune и возврат предыдущей версии через Git. GitLab
должен зафиксировать deployment и отправить уведомление только после проверки
реально работающей версии.

## Текущее состояние

В качестве исходных материалов доступны:

- Go API, worker, migrator и smoke-команда;
- PostgreSQL, Redis и Kafka в локальном Kubernetes;
- endpoint `GET /api/health-check`, liveness и readiness probes;
- собственный Helm chart приложения;
- migration и smoke Jobs в виде Helm hooks;
- один GitLab Runner с Kubernetes executor;
- проверки Go-кода, Helm chart, OCI-образа и зависимостей;
- публикация image с tag `$CI_COMMIT_SHA` в GitLab Container Registry;
- прямой deploy приложения из `main` командой `helm upgrade --install`;
- GitLab environment `development` и Telegram notifier.

Argo CD, GitOps manifests, promotion Merge Request и read-only verification в
стартовом состоянии отсутствуют.

## Рабочая задача

1. Вручную скачай и установи официальный Argo CD Helm chart закреплённой
   версии в namespace `argocd`.
2. Отключи Dex, ApplicationSet, Notifications и high availability. UI
   открывай только локально через `kubectl port-forward`.
3. Создай отдельный `AppProject`, разрешающий только текущий публичный
   repository, in-cluster destination и namespace `devops-practice`.
4. Создай multi-source `Application`: Helm chart находится в
   `deploy/helm/devops-practice`, а значения development-окружения — в
   `deploy/environments/development/values.yaml` того же repository.

### Чекпоинт: источники ограничены

- [ ] **Результат:** Argo CD установлен, AppProject и Application указывают нужные repository, chart, values и destination; diff изучен.
- [ ] **Понимание:** Могу объяснить desired state и границу разрешений AppProject.

5. Первый sync выполни вручную после изучения `OutOfSync` diff. Затем включи
   automated sync с `prune`, `selfHeal` и `allowEmpty: false`.

### Чекпоинт: синхронизация работает

- [ ] **Результат:** Первый manual sync завершён; после перехода к automated sync записаны ожидаемые статусы и настройки.
- [ ] **Понимание:** Могу объяснить OutOfSync, Synced и Healthy и не считаю их взаимозаменяемыми.

6. Удали прямой `helm upgrade`, создание runtime Secret и другие изменения
   Kubernetes из GitLab pipeline.
7. Ограничь Runner чтением workloads приложения и статуса одного Argo CD
   Application. Доступ к Secrets и mutating verbs запрещён.

### Чекпоинт: CI лишён прямого deploy

- [ ] **Результат:** В CI нет изменения workloads; положительные и отрицательные auth can-i подтверждают observer-права runner.
- [ ] **Понимание:** Могу объяснить, кто теперь изменяет кластер и почему успешная сборка сама по себе ничего не развёртывает.

8. После публикации image создавай служебную ветку и promotion Merge Request,
   изменяющий только image repository/tag development-окружения.
9. Используй `CI_JOB_TOKEN` без постоянного personal/project access token.
   Учти, что push с job token сам не запускает pipeline.
10. После merge promotion MR дождись reconciliation Argo CD, проверь
    `Synced/Healthy`, фактический image, rollout, smoke и Ingress. Только после
    этого создай GitLab deployment `development`.

### Чекпоинт: promotion дошёл до приложения

- [ ] **Результат:** До merge promotion MR версия не менялась; после merge проверены реальный image, готовность и GitLab deployment.
- [ ] **Понимание:** Могу связать application MR, image SHA, promotion MR и результат reconciliation.

11. Runtime Secret создай один раз вручную из интерактивно введённых значений.
    Не сохраняй его содержимое в Git, CI 02-interviews-and-practice/artifacts или logs.
12. Воспроизведи drift, prune, несуществующий image, ошибку migration и
    rollback через Git revert.

### Чекпоинт: восстановление следует Git

- [ ] **Результат:** Bootstrap Secret не попал в Git; drift, prune и failed sync проверены, Git revert вернул рабочую версию.
- [ ] **Понимание:** Могу объяснить отличие исправления desired state от разовой live-правки и границы self-heal.

## Разные подходы

На ревью нужно не только показать итоговый Argo CD, но и сравнить:

| Сравнение | Что показать |
| --- | --- |
| Push-based CD / pull-based GitOps | кто инициирует deployment, где desired state и какие права нужны CI |
| Manual / automated sync | `OutOfSync`, ручной sync и последующий автоматический reconciliation |
| Live hotfix / изменение в Git | временный `kubectl scale`, self-healing и устойчивое изменение replicas через MR |
| Helm rollback / Git revert | operational rollback Module 02 и возврат immutable tag через проверяемую Git history |

Объясни, почему изменение live object не является исправлением desired state и
почему `argocd app rollback` не используется вместе с automated sync.

## Ограничения

- один обязательный уровень сложности Hard;
- только локальные ресурсы и публичный GitLab.com Free, затраты 0 ₽;
- компьютер с 16 ГБ RAM: до 8 ГБ в покое и до 10 ГБ с одной CI job;
- один environment `development`;
- один GitLab Runner с Kubernetes executor и `concurrent = 1`;
- Argo CD устанавливается вручную отдельным upstream Helm chart;
- ApplicationSet, app-of-apps и управление Argo CD самим Argo CD не входят в
  модуль;
- operators для PostgreSQL, Redis и Kafka не добавляются;
- Argo Rollouts, canary, blue/green, TLS, SSO и внешний Ingress для Argo CD не
  входят в модуль;
- private repository credentials не рассматриваются;
- External Secrets, SOPS, Vault и Sealed Secrets не добавляются;
- production Kubernetes, observability, backup, Terraform и Yandex Cloud
  остаются за рамками модуля;
- application image использует только immutable commit SHA; `latest`
  запрещён;
- прямой commit image tag в `main` и automatic merge promotion MR запрещены.

## Что предоставить на ревью

- ссылку на обычный Merge Request и успешный application pipeline;
- ссылку на promotion Merge Request и его GitOps validation pipeline;
- `AppProject`, `Application`, Argo CD values и development values;
- GitLab pipeline без прямого deploy;
- observer RBAC Runner и результаты позитивных/негативных `kubectl auth can-i`;
- Argo CD `Synced/Healthy`, sync history и фактический image Deployments;
- GitLab environment/deployment и Telegram-сообщение;
- команды и результаты manual/automated sync;
- команды и результаты drift/self-heal и prune;
- failed sync с несуществующим image или ошибкой migration;
- Merge Request с Git revert на предыдущий immutable image;
- объяснение bootstrap-границы runtime Secret;
- замеры памяти стенда в покое и во время одной CI job.

Скриншоты допустимы только как дополнение. Основное доказательство —
воспроизводимые manifests, commits, pipelines и команды проверки.

## Критерии приёмки

1. Argo CD установлен официальным закреплённым chart в namespace `argocd`.
2. Dex, ApplicationSet и Notifications отключены.
3. `AppProject` ограничивает source repository и destination приложения.
4. `Application` получает chart и development values из Git без parameter
   overrides.
5. Первый manual sync и переход к automated sync документированы.
6. После merge promotion MR приложение становится `Synced/Healthy` с image tag
   ожидаемого commit SHA.
7. До merge promotion MR новая версия в кластер не попадает.
8. CI не выполняет `helm upgrade`, не изменяет workloads и не читает Secrets.
9. Runner не может создать, изменить или удалить Deployment.
10. Ручное изменение replicas автоматически исправляется.
11. Удалённый из Git учебный ресурс автоматически удаляется через prune.
12. Ошибка migration или rollout не делает недоступной предыдущую готовую
    версию API.
13. Git revert возвращает предыдущий immutable image tag и работоспособную
    версию.
14. Повторный promotion для одного SHA не создаёт дублирующий Merge Request.
15. GitLab environment содержит подтверждённый deployment.
16. Telegram получает ровно одно сообщение после подтверждённой доставки, а
    не после создания MR.
17. Runtime Secret отсутствует в Git и создаётся только bootstrap-командой.
18. На ревью объяснены различия четырёх пар подходов без подмены GitOps
    обычным запуском Helm из CI.
