# 04. Наблюдаемость приложения и расследование инцидентов

## Подготовка рабочей копии

Из корня курса:

```bash
bash tasks/init.sh 04
cd tasks/04/workspace
```

Все команды задания выполняй из этого рабочего каталога.

Уровень: **Hard**. Продолжение модуля 03 либо самостоятельный старт с
материалами текущей версии репозитория без переключения веток.

## Заявка руководителя

Development-кластер получает релизы через GitLab и Argo CD, но команда узнаёт
о сбоях из ручных проверок. Нужна возможность связать деградацию API с
состоянием Kubernetes, конкретным релизом и логами API/worker.

Постройте локальную платформу наблюдаемости на VictoriaMetrics, Grafana и Loki.
Добавьте метрики приложения, dashboards as code, SLI/SLO и уведомления в
Telegram. Воспроизведите отказ зависимости и неуспешный rollout, расследуйте
их по сигналам платформы и восстановите сервис через принятый процесс.

## Исходные условия

- Один бесплатный локальный кластер, компьютер с 16 ГБ RAM. Цель: до 10 ГБ
  RAM в покое и до 12 ГБ при одной CI job; Runner concurrent=1.
- Готовые API, worker, PostgreSQL, Redis, Kafka, Helm chart, GitLab CI,
  Kubernetes Runner, promotion MR и Argo CD из предыдущего модуля.
- Один environment `development`. Инфраструктура устанавливается вручную,
  изменения приложения проходят application MR → image → promotion MR → Argo CD.
- Linux на основном компьютере не обязателен; допускаются Docker Desktop/k3d.

## Обязательные работы

1. Инструментируйте Go API и worker. Экспортируйте RED-метрики HTTP,
   dependency health для PostgreSQL/Redis/Kafka, результат публикации событий,
   результат и длительность обработки, timestamp последней успешной обработки,
   стандартные Go/process metrics. API и worker слушают отдельный порт 9090.
   Ошибка запуска metrics listener должна останавливать процесс. Проверьте
   graceful shutdown.
2. Добавьте HTTP correlation ID в header и JSON-логи. Сохраняйте связь API и
   worker по бизнесовому ID заявки. Не включайте payload, пароли, Cookie и
   Authorization в логи. Метрики используют шаблон маршрута, а не URL заявки.
   UUID, error text, title, offset и query string не могут быть metric labels.
3. В namespace `monitoring` установите отдельными upstream Helm charts:
   VictoriaMetrics Single, vmagent, vmalert с Alertmanager, Grafana, Loki
   monolithic, Alloy, kube-state-metrics и node-exporter. Закрепите версии.
   Operators, ServiceMonitor, PodMonitor и PrometheusRule не используются.
   Скачивайте charts, изучайте values и выполняйте Helm-команды самостоятельно.
4. Настройте scrape каждые 15 секунд: приложение, kube-state-metrics,
   node-exporter, kubelet/cAdvisor, ingress-nginx и сама monitoring-платформа.
   Метрики Kubernetes собираются cluster-wide. RBAC scraper разрешает чтение
   discovery и kubelet metrics, но не Secrets или изменение объектов.
5. Добавьте metrics container ports и annotations в application chart.
   NetworkPolicy допускает scraping только из `monitoring`; пользовательский
   Ingress не открывает metrics. UI monitoring открываются через port-forward.
6. VM и Loki сохраняют данные на PVC по 5Gi в `local-path`, retention — три
   дня, server replicas — одна. Alloy и node-exporter работают DaemonSet.
   Задайте requests/limits. Отключите лишние caches, MinIO и distributed mode.
7. Alloy отправляет логи только из `devops-practice`, `platform`, `argocd`,
   `ingress-nginx`, `gitlab-runner`, `monitoring`. Не отправляйте `kube-system`.
   Разбирайте JSON slog. ID заявки и error остаются JSON-полями либо structured
   metadata, но не Loki labels.
8. Provisioning Grafana хранится в Git. Datasource UIDs: `victoriametrics` и
   `loki`. Подготовьте три dashboards: service overview, Kubernetes operations
   и incident drill-down. В последнем нужны namespace/component/pod filters,
   точный поиск request ID в JSON, события deployment/Argo CD и переход в Explore.
   Покажите RED, зависимости, публикацию/обработку событий, freshness,
   ресурсы/restarts/replicas, failed Jobs, ingress и состояние scrape targets.
9. Определите SLI: availability API (не 5xx), доля запросов быстрее 500ms,
   worker freshness, deployment availability. Цели: 99% availability и 95%
   запросов быстрее 500ms. Используйте recording rules и окна 5m/30m.
   Отдельно объясните поведение SLI при отсутствии трафика.
10. Настройте alerts и их unit tests. Проверьте inactive/pending/firing и
    annotations; отсутствие трафика не должно давать ложный WorkerStalled.

| Alert | Сигнал | Минимальный for |
| --- | --- | --- |
| ApplicationTargetDown | Недоступен metrics target приложения | 2m |
| DependencyUnavailable | Dependency gauge равен нулю | 1m |
| APIHighErrorRate | Более 5% 5xx при ненулевом трафике | 5m |
| APIHighLatency | p95 выше 500ms | 5m |
| WorkerStalled | Публикация продолжается, обработки нет | 2m |
| DeploymentReplicasMismatch | Available replicas меньше desired | 3m |
| PodRestartingTooOften | Растёт restart counter | 5m |
| VictoriaMetricsStorageLow | Мало свободного места | 5m |

Каждый alert содержит severity, service, namespace, summary, description,
dashboard_url и runbook_url. Alertmanager группирует уведомления, подавляет
warning при critical того же сервиса и отправляет firing/resolved в Telegram.
Токен/chat ID монтируются из Secret файлами; пароль Grafana также берётся из
Secret. Delivery notifier GitLab остаётся отдельным каналом.

## Два обязательных инцидента

### Отказ Redis

Сохраните baseline и временно остановите учебный Redis без удаления PVC.
Покажите readiness 503 при liveness 200, dependency failure, ошибки API,
повторы worker, alerts и подтверждение причины в Loki. Восстановите Redis и
покажите восстановление обработки и resolved notification.

### Неуспешный rollout worker

Создайте отдельный MR с воспроизводимым startup defect worker. Обоснуйте,
будет ли старая replica продолжать работу при выбранной rollout strategy.
Покажите проблему в Argo CD, replicas/restarts, freshness, events/logs.
Исправьте Git revert через MR; удаление pod или live edit не устраняют причину.

Для каждого инцидента подготовьте postmortem: impact/SLI, UTC timeline,
symptom отдельно от root cause, диагностические запросы, MR/commit,
восстановление, конкретное улучшение alert/dashboard/runbook и влияние на
MTTD/MTTR. Скриншоты дополняют воспроизводимые доказательства.

## Проверки и сдача

Добавьте в CI проверки Go instrumentation, cardinality, charts/render/schema,
rules, UTF-8 strict Alertmanager config/routes/inhibition, Alloy config,
dashboard JSON/UID/panels, secrets и незакреплённых images. CI не устанавливает
monitoring и не получает новые права записи в Kubernetes.

Сдайте MR с кодом и конфигурацией, успешный pipeline/promotion, healthy targets,
application/Kubernetes series, поиск известного ID в Loki, три provisioned
dashboards, Telegram firing/resolved, два postmortem, замеры RAM idle/CI.
Изменения dashboards через UI экспортируйте в Git и проведите через MR.

На защите объясните:

- почему labels нельзя заполнять ID заявки и сырым URL;
- различия readiness/liveness/target health/SLI;
- SLI/SLO, 30-day rolling window, error budget и burn rate;
- ограничения single-node и почему PVC не является backup;
- trace/span, propagation через HTTP/Kafka, sampling, связь с logs,
  роль OpenTelemetry Collector и Tempo. Tracing здесь только теория:
  устанавливать runtime-компоненты и SDK не требуется.

Модуль принимается по цепочке «симптом → alert → dashboard → logs → причина →
Git fix/revert → resolved», а не по факту открытия Grafana.
