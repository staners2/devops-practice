# Наблюдаемость development-кластера

Этот runbook устанавливает и проверяет результат модуля 04 поверх работающих
модулей 00–03. Требуются локальный `k3d-devops-practice`, application
`devops-practice` в Argo CD и успешный предыдущий promotion. Команды выполняй
из корня репозитория в Bash; на Windows используй WSL2.

Условие задания находится в [`../modules/04-observability.md`](../modules/04-observability.md).
Все файлы доступны в текущей версии репозитория. Для последовательного
маршрута сначала выполните модули 00–03; переключать Git-теги не требуется.

## Границы и подготовка

Monitoring устанавливается вручную отдельными upstream Helm charts.
Приложение получает изменения только через существующий flow: application MR
→ checks → merge → image → promotion MR → Argo CD → `verify-development`.
CI проверяет observability 02-interviews-and-practice/artifacts, но не устанавливает stack и не получает
дополнительных прав записи в кластер. Namespace `monitoring` не входит в
application Argo CD Project.

На компьютере требуется 16 ГБ RAM. Цель полного стенда — до 10 GiB в покое и
до 12 GiB во время одной CI job; оставшаяся память нужна ОС и runtime.
Single-node VictoriaMetrics и monolithic Loki не дают HA. Их PVC сохраняют
историю при перезапуске pod, но удаление k3d-кластера уничтожает локальные
данные. Retention метрик и логов — три дня, PVC каждого хранилища — `5Gi`.

Проверь context до любой записи. Функция `k` всегда передаёт его явно:

```bash
OBS_CONTEXT=k3d-devops-practice
k() { kubectl --context="$OBS_CONTEXT" "$@"; }
kubectl config get-contexts "$OBS_CONTEXT"
k get nodes
k get storageclass local-path
k -n argocd get application devops-practice
k -n devops-practice get deployment devops-practice-api devops-practice-worker
k -n platform get statefulset redis
source deploy/platform/versions.env
```

Если нужного context нет, вернись к [`kubernetes-platform.md`](kubernetes-platform.md).
Не подставляй текущий production context вместо учебного.

## Скачать закреплённые charts

Chart versions и app versions находятся в `deploy/platform/versions.env`.
Восемь charts устанавливают девять компонентов: Alertmanager входит в
`victoria-metrics-alert`. Operators, `ServiceMonitor`, `PodMonitor` и
`PrometheusRule` не используются.

```bash
mkdir -p .cache/charts
helm repo add vm https://victoriametrics.github.io/helm-charts/
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add grafana-community https://grafana-community.github.io/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update vm grafana grafana-community prometheus-community

helm pull vm/victoria-metrics-single \
  --version "$VICTORIA_METRICS_SINGLE_CHART_VERSION" --untar --untardir .cache/charts
helm pull vm/victoria-metrics-agent \
  --version "$VICTORIA_METRICS_AGENT_CHART_VERSION" --untar --untardir .cache/charts
helm pull vm/victoria-metrics-alert \
  --version "$VICTORIA_METRICS_ALERT_CHART_VERSION" --untar --untardir .cache/charts
helm pull grafana-community/grafana \
  --version "$GRAFANA_CHART_VERSION" --untar --untardir .cache/charts
helm pull grafana-community/loki \
  --version "$LOKI_CHART_VERSION" --untar --untardir .cache/charts
helm pull grafana/alloy \
  --version "$ALLOY_CHART_VERSION" --untar --untardir .cache/charts
helm pull prometheus-community/kube-state-metrics \
  --version "$KUBE_STATE_METRICS_CHART_VERSION" --untar --untardir .cache/charts
helm pull prometheus-community/prometheus-node-exporter \
  --version "$PROMETHEUS_NODE_EXPORTER_CHART_VERSION" --untar --untardir .cache/charts
```

`.cache` игнорируется Git. Перед установкой изучи repository values в
`deploy/platform/values/`; upstream charts в кеше являются скачанными
зависимостями. Для документации upstream используй официальные repositories.

## Namespace, RBAC и конфигурация

Сначала создай namespace и ограниченный RBAC для vmagent. Его ServiceAccount
создаст chart. Discovery разрешает `get/list/watch` для Kubernetes resources
и `get` для `nodes/proxy`; доступа к Secrets и mutating verbs нет.

```bash
k apply -f deploy/monitoring/bootstrap

k -n monitoring create configmap vmagent-scrape \
  --from-file=scrape.yml=deploy/monitoring/scrape/vmagent.yaml \
  --dry-run=client -o yaml | k apply -f -
k -n monitoring create configmap vmalert-rules \
  --from-file=deploy/monitoring/rules/alerts.yaml \
  --from-file=deploy/monitoring/rules/recording.yaml \
  --dry-run=client -o yaml | k apply -f -
k -n monitoring create configmap alertmanager-config \
  --from-file=deploy/monitoring/alertmanager/alertmanager.yaml \
  --from-file=deploy/monitoring/alertmanager/telegram.tmpl \
  --dry-run=client -o yaml | k apply -f -
k -n monitoring create configmap alloy-config \
  --from-file=config.alloy=deploy/monitoring/alloy/config.alloy \
  --dry-run=client -o yaml | k apply -f -
k -n monitoring create configmap monitoring-dashboards \
  --from-file=deploy/monitoring/dashboards \
  --dry-run=client -o yaml | k apply -f -
```

Эти файлы — source of truth. После изменения повторно создай соответствующий
ConfigMap из Git. Для scrape configuration перезапусти Deployment vmagent;
для Alertmanager — его workload, если reload не применён. vmalert проверяет
изменения rules каждую минуту; Grafana перечитывает dashboard files каждые
30 секунд. Изменения dashboards через UI в этой конфигурации не сохраняются.

## Создать Secrets интерактивно

До установки подготовь Telegram bot и разрешённый chat: добавь bot в чат и
отправь ему сообщение. Введи bot token и числовой chat ID; у группы ID может
начинаться с `-`. Не включай shell tracing, не помещай значения в values,
GitLab variables, 02-interviews-and-practice/artifacts, отчёт или команды `helm --set`.

Блок работает в subshell, не печатает введённые значения и удаляет временные
файлы после создания Secrets:

```bash
(
  set -euo pipefail
  set +x
  umask 077
  secret_dir=$(mktemp -d)
  trap 'rm -f "$secret_dir/admin-password" "$secret_dir/bot_token" "$secret_dir/chat_id"; rmdir "$secret_dir"' EXIT

  read -r -s -p 'Grafana admin password: ' grafana_password
  printf '\n' >&2
  read -r -s -p 'Telegram bot token: ' telegram_token
  printf '\n' >&2
  read -r -s -p 'Telegram chat ID: ' telegram_chat_id
  printf '\n' >&2
  test -n "$grafana_password"
  test -n "$telegram_token"
  [[ "$telegram_chat_id" =~ ^-?[0-9]+$ ]]

  printf '%s' "$grafana_password" > "$secret_dir/admin-password"
  printf '%s' "$telegram_token" > "$secret_dir/bot_token"
  printf '%s' "$telegram_chat_id" > "$secret_dir/chat_id"
  k -n monitoring create secret generic grafana-admin \
    --from-literal=admin-user=admin \
    --from-file=admin-password="$secret_dir/admin-password" \
    --dry-run=client -o yaml | k apply -f -
  k -n monitoring create secret generic alertmanager-telegram \
    --from-file=bot_token="$secret_dir/bot_token" \
    --from-file=chat_id="$secret_dir/chat_id" \
    --dry-run=client -o yaml | k apply -f -
  unset grafana_password telegram_token telegram_chat_id
)
```

Grafana читает `admin-user`/`admin-password` из `grafana-admin`. Alertmanager
монтирует `alertmanager-telegram` read-only в `/etc/alertmanager/telegram` и
читает `bot_token_file`/`chat_id_file`. Проверяй наличие Secrets через
`k -n monitoring get secret grafana-admin alertmanager-telegram`, без `-o yaml`.

## Установить восемь releases

Устанавливай каждый chart отдельно. Дополнительные `--set-file` не нужны:
values ссылаются на подготовленные ConfigMaps и Secrets.

```bash
helm upgrade --install victoriametrics .cache/charts/victoria-metrics-single \
  --kube-context "$OBS_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/victoria-metrics-single.yaml --wait --timeout 10m
helm upgrade --install loki .cache/charts/loki \
  --kube-context "$OBS_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/loki.yaml --wait --timeout 10m
helm upgrade --install vmalert .cache/charts/victoria-metrics-alert \
  --kube-context "$OBS_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/victoria-metrics-alert.yaml --wait --timeout 10m
helm upgrade --install kube-state-metrics .cache/charts/kube-state-metrics \
  --kube-context "$OBS_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/kube-state-metrics.yaml --wait --timeout 10m
helm upgrade --install node-exporter .cache/charts/prometheus-node-exporter \
  --kube-context "$OBS_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/prometheus-node-exporter.yaml --wait --timeout 10m
helm upgrade --install vmagent .cache/charts/victoria-metrics-agent \
  --kube-context "$OBS_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/victoria-metrics-agent.yaml --wait --timeout 10m
helm upgrade --install alloy .cache/charts/alloy \
  --kube-context "$OBS_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/alloy.yaml --wait --timeout 10m
helm upgrade --install grafana .cache/charts/grafana \
  --kube-context "$OBS_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/grafana.yaml --wait --timeout 10m

k -n monitoring get deployment,statefulset,daemonset,pod,service,pvc
k -n monitoring wait --for=condition=Ready pod --all --timeout=10m
k auth can-i list pods --as=system:serviceaccount:monitoring:vmagent --all-namespaces
k auth can-i get nodes/proxy --as=system:serviceaccount:monitoring:vmagent
k auth can-i get secrets --as=system:serviceaccount:monitoring:vmagent --all-namespaces
k auth can-i create deployments --as=system:serviceaccount:monitoring:vmagent --all-namespaces
```

Первые два RBAC ответа должны быть `yes`, последние два — `no`.
VictoriaMetrics и Loki PVC должны быть `Bound`. Все observability Services —
`ClusterIP`; Ingress для этих UI/API не создаётся.

В обновлённых values ingress-nginx и Argo CD включён metrics endpoint. Примени
их теми же platform Helm upgrades из
[`kubernetes-platform.md`](kubernetes-platform.md#ingress-nginx) и
[`argocd-gitops.md`](argocd-gitops.md#установка-argo-cd). Application chart
`deploy/helm/devops-practice` обновляй через application MR и promotion,
без ручного Helm deployment. Новый image должен экспортировать `/metrics`
на `METRICS_ADDR=:9090` у API и worker, иметь named port `metrics`, annotations
и NetworkPolicy для namespace `monitoring`.

## UI, endpoints и targets

В четырёх отдельных терминалах объяви `OBS_CONTEXT`/`k` из первого раздела и
запусти:

```bash
k -n monitoring port-forward service/victoriametrics 8428:8428
```

```bash
k -n monitoring port-forward service/grafana 3000:80
```

```bash
k -n monitoring port-forward service/alertmanager 9093:9093
```

```bash
k -n monitoring port-forward service/loki 3100:3100
```

Открой `http://localhost:8428/vmui`, `http://localhost:3000` и
`http://localhost:9093`. В Grafana войди как `admin` с введённым паролем.
Loki проверяется API, самостоятельного Grafana-подобного UI у него нет.

```bash
curl --fail --silent --show-error http://localhost:8428/health
curl --fail --silent --show-error http://localhost:3000/api/health | jq
curl --fail --silent --show-error http://localhost:9093/-/ready
curl --fail --silent --show-error http://localhost:3100/ready
curl --fail --silent --show-error --get http://localhost:8428/api/v1/query \
  --data-urlencode 'query=count by (job,service) (up{cluster="devops-practice"})' | jq
curl --fail --silent --show-error --get http://localhost:8428/api/v1/query \
  --data-urlencode 'query=up{cluster="devops-practice"} == 0' | jq
curl --fail --silent --show-error --get http://localhost:8428/api/v1/query \
  --data-urlencode 'query=devops_practice_http_requests_total{namespace="devops-practice"}' | jq
curl --fail --silent --show-error --get http://localhost:8428/api/v1/query \
  --data-urlencode 'query=kube_deployment_spec_replicas{namespace="devops-practice"}' | jq
```

После нескольких scrape intervals запрос `up == 0` должен вернуть пустой
result, а остальные — application и Kubernetes series. Пустой `up == 0` не
доказывает полноту discovery: отдельно проверь jobs `application`,
`kube-state-metrics`, `node-exporter`, `kubelet`, `cadvisor`, `ingress-nginx`,
`argocd`, `meta-monitoring`, `alloy` и ожидаемые API/worker pods.

Для конкретного failed target дополнительно открой vmagent targets:

```bash
k -n monitoring port-forward service/vmagent 8429:8429
```

`http://localhost:8429/targets` показывает scrape URL и последнюю ошибку.
Сопоставь её с `k -n monitoring logs deployment/vmagent --tail=100`, pod
annotations, named port, NetworkPolicy и разрешением `nodes/proxy`.

Для rules открой ещё один port-forward:

```bash
k -n monitoring port-forward service/vmalert 8880:8880
```

Проверь `http://localhost:8880/api/v1/rules` и
`http://localhost:8880/api/v1/alerts`: rules должны загружаться без evaluation
errors. Лог vmalert подтверждает доступность datasource, remote write и
Alertmanager, но отсутствие ошибок в логе не заменяет проверку series.

## Datasources, dashboards и request ID

В Grafana открой `Connections → Data sources`: `VictoriaMetrics` имеет UID
`victoriametrics`, `Loki` — `loki`. URL указывают на cluster Services;
`localhost` внутри Grafana pod означал бы сам pod. В `Dashboards → DevOps
practice` должны появиться три provisioned dashboards:

| Dashboard | URL и роль |
| --- | --- |
| Service overview | `/d/service-overview`: RED, dependencies, events, API/worker resources |
| Kubernetes operations | `/d/kubernetes-operations`: nodes, replicas, requests/limits, ingress, targets |
| Incident drill-down | `/d/incident-drilldown`: namespace/component/pod, точный request ID, timeline и logs |

Создай заявку через штатный Ingress и сохрани бизнесовый ID:

```bash
created_request=$(curl --fail --silent --show-error \
  -H 'Host: devops-practice.localhost' -H 'Content-Type: application/json' \
  -d '{"title":"Проверка наблюдаемости модуля 04"}' \
  http://127.0.0.1:8080/api/requests)
request_id=$(printf '%s' "$created_request" | jq -er '.id')
printf 'Business request ID: %s\n' "$request_id"
logql=$(printf '{namespace="devops-practice",app="devops-practice"} | json | request_id = "%s"' "$request_id")
curl --fail --silent --show-error --get http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode "query=$logql" --data-urlencode 'since=15m' \
  --data-urlencode 'direction=forward' --data-urlencode 'limit=100' | jq
```

Дождись ingestion и обработки: найди API `request created` и worker `request
processed` с одинаковым бизнесовым `request_id`. Введи ID в textbox dashboard
Incident drill-down; поле фильтруется точным равенством после `| json`, без
substring или regex поиска. HTTP `X-Request-ID` относится к одному HTTP
обращению; API business log содержит также `http_request_id` для связи с
middleware. Response `.id` и Kafka `request_id` связывают создание с worker.

В Explore используй datasource Loki и выражения:

```logql
{namespace="devops-practice",app="devops-practice"} | json | request_id = "ID из response"
{namespace="devops-practice",app="devops-practice",component="worker",level="error"} | json
```

`level` JSON slog обычно uppercase; Loki label `level` нормализован lowercase.
Второй запрос использует именно label. После `| json` одноимённое JSON-поле
может называться `level_extracted`; не сравнивай lowercase label с `ERROR`.
`request_id`, error text и Kafka offset остаются JSON fields, без Loki labels
и metric labels. Alloy читает только namespaces `devops-practice`, `platform`,
`argocd`, `ingress-nginx`, `gitlab-runner`, `monitoring`; `kube-system` сюда
не отправляется. API диагностики логов описано в
[Loki HTTP API](https://grafana.com/docs/loki/latest/reference/loki-http-api/).

## Проверить firing и resolved в Telegram

Сначала выполни offline validation, описанную ниже. Затем проверь всю цепочку
Alertmanager → Telegram временным alert через
[Alertmanager API v2](https://github.com/prometheus/alertmanager/blob/v0.34.1/api/v2/openapi.yaml).
Тест не меняет rules и не вызывает отказ приложения:

```bash
delivery_started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
jq -n --arg started "$delivery_started_at" '
  [{labels: {alertname: "ObservabilityDeliveryTest", namespace: "monitoring",
             service: "observability-test", severity: "warning"},
    annotations: {summary: "Проверка доставки Module 04",
                  description: "Временный тест firing/resolved",
                  dashboard_url: "http://localhost:3000/d/kubernetes-operations",
                  runbook_url: "docs/runbooks/observability.md"},
    startsAt: $started, endsAt: ((now + 600) | strftime("%Y-%m-%dT%H:%M:%SZ"))}]' \
  | curl --fail --silent --show-error -H 'Content-Type: application/json' \
      --data-binary @- http://localhost:9093/api/v2/alerts
curl --fail --silent --show-error --get http://localhost:9093/api/v2/alerts \
  --data-urlencode 'filter=alertname="ObservabilityDeliveryTest"' | jq
```

Дождись сообщения `firing`: `group_wait=30s`. Затем отправь тот же label set
и `startsAt` с текущим `endsAt`, сохранив время начала в этом терминале:

```bash
jq -n --arg started "$delivery_started_at" '
  [{labels: {alertname: "ObservabilityDeliveryTest", namespace: "monitoring",
             service: "observability-test", severity: "warning"},
    annotations: {summary: "Проверка доставки Module 04",
                  description: "Временный тест firing/resolved"},
    startsAt: $started, endsAt: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))}]' \
  | curl --fail --silent --show-error -H 'Content-Type: application/json' \
      --data-binary @- http://localhost:9093/api/v2/alerts
unset delivery_started_at
```

Дождись отдельного сообщения `resolved`; `group_interval=2m` может задержать
уведомление. Сохрани оба сообщения с UTC временем, названием alert и labels.
Отсутствие alert в API после завершения само по себе не подтверждает доставку.
Если сообщения нет, проверь Alertmanager pod events и logs, наличие Secret,
права bot в чате и `alertmanager_notifications_failed_total`. Не копируй
Secret contents или token в диагностический вывод.

Группировка использует `alertname`, `namespace`, `service`; critical подавляет
warning того же service/namespace. Поэтому symptom warning может быть firing
в vmalert, но inhibited в Alertmanager. Существующий Go notifier GitLab
сообщает о delivery pipeline и не участвует в этой alert цепочке.

## Инцидент 1: Redis unavailable

Перед опытом сохрани baseline: healthy targets, последние успешные заявки,
deployment image/revision, UTC время, API/worker restarts и screenshots
dashboards. Создай несколько заявок и проверь, что worker их обработал.
Эксперимент меняет только StatefulSet `platform/redis` в context
`k3d-devops-practice`. PVC и Secrets остаются на месте.

Запусти блок в отдельном Bash-терминале. Он сохраняет число replicas и
восстанавливает Redis при выходе или `Ctrl+C`; не закрывай весь runtime:

```bash
(
  set -euo pipefail
  OBS_CONTEXT=k3d-devops-practice
  k() { kubectl --context="$OBS_CONTEXT" "$@"; }
  k -n platform get statefulset redis
  redis_replicas=$(k -n platform get statefulset redis -o jsonpath='{.spec.replicas}')
  test "$redis_replicas" -gt 0
  api_pod=$(k -n devops-practice get pod \
    -l app.kubernetes.io/instance=devops-practice,app.kubernetes.io/component=api \
    -o jsonpath='{.items[0].metadata.name}')
  test -n "$api_pod"
  api_forward_pid=
  restore_redis() {
    trap - EXIT INT TERM
    k -n platform scale statefulset redis --replicas="$redis_replicas"
    k -n platform rollout status statefulset/redis --timeout=5m
    if [[ -n "$api_forward_pid" ]]; then kill "$api_forward_pid"; fi
  }
  trap restore_redis EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  mkdir -p .cache/observability
  k -n devops-practice port-forward "pod/$api_pod" 18080:8080 \
    > .cache/observability/api-port-forward.log 2>&1 &
  api_forward_pid=$!
  for attempt in $(seq 1 30); do
    if curl --fail --silent http://127.0.0.1:18080/livez >/dev/null; then break; fi
    sleep 1
  done
  curl --fail --silent --show-error http://127.0.0.1:18080/livez >/dev/null
  k -n platform scale statefulset redis --replicas=0
  date -u +%Y-%m-%dT%H:%M:%SZ
  for attempt in $(seq 1 240); do
    curl --silent --show-error --output /dev/null --write-out '%{http_code}\n' \
      http://127.0.0.1:18080/api/stats
    sleep 2
  done
  read -r -p 'Зафиксируй firing и диагностику; Enter восстановит Redis: ' acknowledgement
)
```

В другом терминале во время отказа проверь:

```bash
curl -i http://127.0.0.1:18080/livez
curl -i http://127.0.0.1:18080/readyz
curl -i http://127.0.0.1:18080/api/health-check
curl -i http://127.0.0.1:18080/api/stats
redis_request=$(curl --fail --silent --show-error \
  -H 'Content-Type: application/json' \
  -d '{"title":"Заявка во время отказа Redis"}' \
  http://127.0.0.1:18080/api/requests)
redis_request_id=$(printf '%s' "$redis_request" | jq -er '.id')
printf 'Redis incident request ID: %s\n' "$redis_request_id"
k -n devops-practice get endpointslice \
  -l kubernetes.io/service-name=devops-practice-api -o yaml
k -n devops-practice get pods
k -n devops-practice logs deployment/devops-practice-worker --tail=100
curl --fail --silent --show-error --get http://localhost:8428/api/v1/query \
  --data-urlencode 'query=devops_practice_dependency_up{namespace="devops-practice",dependency="redis"}' | jq
curl --fail --silent --show-error http://localhost:8880/api/v1/alerts | jq
curl --fail --silent --show-error http://localhost:9093/api/v2/alerts | jq
```

Ожидаются `/livez=200`, `/readyz=503`, `/api/health-check=503`, `/api/stats=503`, Redis gauge `0`
и `DependencyUnavailable` после `for: 1m`. Readiness снимает все API pods с
Service endpoints: Ingress обычно возвращает собственный `503`, который не
попадает в application HTTP counter. Поэтому тестовый `/api/stats`
трафик идёт через pod port-forward, а не через Service или Ingress.

Поддерживаемый восемь минут трафик позволяет увидеть `APIHighErrorRate`
после его `for: 5m`; warning может быть inhibited critical dependency alert.
POST в этом приложении возвращает `201` и публикует event даже при отказе
Redis: cache invalidation пишет warning и не отменяет создание. Сопоставь
`redis_request_id` с structured error/retries worker и последующей успешной
обработкой. Если вместо ожидаемого `201` публикация не удалась, зафиксируй
регрессию и её причину. `WorkerStalled` также требует
ненулевого rate успешной публикации и не гарантирован при полном отсутствии
нового трафика.

В dashboard сравни latency/error rate/dependency health и worker records. В
Loki используй `{namespace="devops-practice",level="error"} | json`; затем
сузь до точного бизнесового request ID. Причину подтверждает отказ Redis и
dependency error; рост API `5xx` является симптомом.

Нажми Enter в терминале эксперимента. После восстановления:

```bash
k -n platform rollout status statefulset/redis --timeout=5m
k -n devops-practice wait --for=condition=Ready pod \
  -l app.kubernetes.io/component=api --timeout=3m
curl --fail --silent --show-error -H 'Host: devops-practice.localhost' \
  http://127.0.0.1:8080/api/health-check
```

Проверь Redis gauge `1`, обработку отложенной заявки, healthy targets и
отдельное `resolved` в Telegram. При потере терминала восстанови сохранённое
число replicas явно; штатное значение этой лаборатории — `1`:
`k -n platform scale statefulset redis --replicas=1`. Не удаляй Redis PVC.

## Инцидент 2: broken worker rollout

Намеренный startup defect вводится config-only GitOps MR, а восстановление —
новым Git revert MR. Перед началом зафиксируй рабочий commit/image,
`argocd app get devops-practice` и replica baseline. Создай отдельную учебную
ветку от актуального integration branch, используемого репозиторием.

В `deploy/environments/development/values.yaml` добавь к существующему
`worker` ровно эти значения, сохранив остальные поля:

```yaml
worker:
  replicas: 1
  strategy: Recreate
  command: ["/usr/local/bin/does-not-exist"]
```

Рабочая команда в image — `/usr/local/bin/worker`. `Recreate` нужен именно
для опыта: RollingUpdate мог бы оставить старый healthy worker, и очередь
продолжила бы обрабатываться. После review и merge наблюдай automated Argo CD
sync и job `verify-development`. Здесь изменяются только environment values:
image не пересобирается, новый promotion MR не создаётся. Argo CD следит за
integration branch и применяет config-only commit напрямую. Image promotion
из модуля 03 нужен при изменении кода/image, а не при этом эксперименте.
Не запускай ручной application Helm upgrade.

В терминале с Argo CD CLI, подключённым к локальному UI по
[`argocd-gitops.md`](argocd-gitops.md#локальный-доступ-к-ui), собери:

```bash
argocd app get devops-practice
argocd app history devops-practice
k -n argocd get application devops-practice \
  -o jsonpath='{.status.sync.revision}{" "}{.status.sync.status}{" "}{.status.health.status}{"\n"}'
k -n devops-practice get deployment devops-practice-worker
k -n devops-practice get pods -l app.kubernetes.io/component=worker
k -n devops-practice get events --sort-by=.metadata.creationTimestamp
worker_pod=$(k -n devops-practice get pod -l app.kubernetes.io/component=worker \
  -o jsonpath='{.items[0].metadata.name}')
k -n devops-practice describe pod "$worker_pod"
k -n devops-practice get pod "$worker_pod" \
  -o jsonpath='{.status.containerStatuses[0].state}{"\n"}{.status.containerStatuses[0].lastState}{"\n"}'
```

Ready/available worker replicas должны упасть до нуля, новый pod не должен
стартовать. Синхронизация может быть `Synced`, а health — `Degraded` или
`Progressing`: успешное применение manifest не равно готовности workload.
Для несуществующего executable runtime может дать `CreateContainerError` и
events без application logs. Не требуй Loki log от процесса, который не
запустился; причина в `describe`, events и container state. Restart alert
сработает только если счётчик действительно вырос до условия rules.

В течение минимум пяти минут продолжай создавать небольшое число заявок
через доступный API. Сохраняй ID и сравнивай опубликованные events с worker
records/freshness. `ApplicationTargetDown`, `DeploymentReplicasMismatch` и
`WorkerStalled` должны подтверждаться соответствующими условиями; inhibition
может сократить число Telegram notifications. API dependency health остаётся
нормальным, но заявки задерживаются в `created`.

Восстанови весь intentional defect, включая `Recreate`, через Git revert
config-only commit. Пример команд для обычного, не merge commit:

```bash
git fetch origin
read -r -p 'Integration branch: ' integration_branch
read -r -p 'Commit с startup defect: ' defect_commit
git switch -c practice/04-worker-recovery "origin/$integration_branch"
git revert --no-edit "$defect_commit"
git diff --check
git push -u origin practice/04-worker-recovery \
  -o merge_request.create -o "merge_request.target=$integration_branch" \
  -o 'merge_request.title=Restore worker after observability experiment'
```

Для merge commit сначала проверь его родителей и примени согласованный
`git revert -m 1`, а не угадывай parent. После review/merge дождись automated
Argo CD sync и `verify-development`; revert environment values также не создаёт
новый image или promotion MR. Desired state снова содержит рабочую command и исходную
strategy; не исправляй workload через `kubectl edit`, `set image`, удаление pod
или Argo rollback.

```bash
argocd app wait devops-practice --sync --health --timeout 600
k -n devops-practice rollout status deployment/devops-practice-worker --timeout=5m
k -n devops-practice logs deployment/devops-practice-worker --tail=100
```

Подтверди processing сохранённых IDs, восстановление свежести, successful
`verify-development` и `resolved`. Изменение command/strategy не должно
оставаться в финальном desired state.

## Действия по alerts

Сначала выбери UTC интервал инцидента в dashboard, проверь связанные labels
и последнюю Argo revision. `runbook_url` в rules — относительный путь от корня
репозитория; открывай его в том Git repository, где лежит alert definition.
Dashboard links используют локальный Grafana port-forward.

<a id="applicationtargetdown"></a>
### ApplicationTargetDown

Проверь `up` по `component`, присутствие target в vmagent и pod state.
Сравни annotations/port `9090`, metrics listener `/metrics` и `/-/healthy`,
NetworkPolicy и scrape error. Падение listener требует исправления image/
config через MR; ошибка discovery требует исправления scrape/RBAC в Git и
manual bootstrap apply. Восстановление: ожидаемые API и worker targets `up=1`.

<a id="dependencyunavailable"></a>
### DependencyUnavailable

Выбери label `dependency`, проверь `platform` pods, Service endpoints и
application readiness. Сопоставь JSON errors API/worker с Redis/PostgreSQL/
Kafka events; Secret values не выводи. Для лабораторного Redis восстанови
сохранённые replicas по сценарию выше. Восстановление: gauge `1`, API Ready
и successful request processing.

<a id="apihigherrorrate"></a>
### APIHighErrorRate

Сравни `devops_practice:http_error_ratio5m`, request rate и breakdown HTTP
status/route. Учитываются только `/api/*`; `unmatched`, probes и ingress
ошибки вне API counter не являются этим SLI. Найди exact request ID в Loki,
проверь dependency health и последний rollout. Восстановление: error ratio
ниже порога на ненулевом трафике после исправления причины.

<a id="apihighlatency"></a>
### APIHighLatency

Сравни p50/p95/p99 с request rate, dependency errors, CPU/memory и limits API.
Проверь timeout/retries; высокий p95 на редком трафике требует оценки числа
запросов. Локализуй route и интервал до поиска конкретного request ID.
Восстановление: p95 ниже `500ms` при репрезентативном трафике.

<a id="workerstalled"></a>
### WorkerStalled

Проверь успешную публикацию events, worker freshness и worker records.
Отсутствие успешных records при отсутствии публикации не является backlog.
Проверь worker Deployment/events, Kafka и Redis; связать заявку можно по
response `.id` в JSON logs. При startup defect выполни Git revert/promotion.
Восстановление: сохранённые заявки `processed`, новые records успешны.

<a id="deploymentreplicasmismatch"></a>
### DeploymentReplicasMismatch

Сравни desired/available replicas выбранного Deployment, scheduling events,
readiness и Argo CD diff/revision. Отдели нехватку node resources от ошибки
image/command и от недоступной dependency. Startup defect восстанавливай
через MR; Redis readiness — восстановлением зависимости. Критерий:
available равно desired, Argo `Synced/Healthy`.

<a id="podrestartingtoooften"></a>
### PodRestartingTooOften

Проверь restart increase за `10m`, `lastState.terminated.reason`, pod events
и previous logs выбранного pod (`k -n devops-practice logs pod/ИМЯ --previous`),
если контейнер уже запускался. `OOMKilled`, liveness failure и process exit требуют разных
исправлений; удаление pod сбрасывает наблюдение, но не устраняет причину.
Восстановление: отсутствуют новые restarts, workload Ready, дефект исправлен.

<a id="victoriametricsstoragelow"></a>
### VictoriaMetricsStorageLow

Проверь `vm_free_disk_space_bytes`, `k -n monitoring get pvc`, node filesystem
в Kubernetes operations и retention/ingestion rate. `5Gi` PVC на `local-path`
не гарантирует отдельный физический диск: заполниться может общий node
filesystem. Не удаляй PVC ради снятия alert. Устрани нецелевое потребление
диска или согласуй ёмкость/retention в values и выполни manual Helm upgrade.
Критерий: свободно больше `512MiB` и новые samples принимаются.

## SLI/SLO и tracing

SLI availability — доля `/api/*` ответов без `5xx`; цель `99%`. Latency SLI —
доля запросов быстрее `500ms`; цель `95%`. Recording rules рассчитывают оба
значения за `5m` и `30m`. Без запросов ratio не доказывает доступность:
сопоставляй его с traffic rate. Worker freshness и ready/desired replicas
показывают другие аспекты обслуживания.

В production rolling 30-day SLO оценивает непрерывные последние 30 дней,
а не календарный месяц. Error budget для availability `99%` допускает `1%`
неуспешных запросов. Burn rate равен фактической доле ошибок, делённой на
`0.01`: при `5%` ошибок он равен `5`. При неизменном трафике такой rate
расходует месячный budget примерно за шесть дней. Multi-window burn-rate
alert сочетает короткое окно для скорости обнаружения с длинным для
устойчивости сигнала. Лабораторные `5m`/`30m` и threshold `5%` служат быстрой
практике; три дня retention не позволяют вычислить реальный 30-day SLO.

Trace объединяет spans с parent/child связями; context передаётся HTTP и
Kafka headers. Sampling ограничивает объём, trace ID связывает spans с
structured logs. OpenTelemetry SDK/Collector и Tempo помогли бы найти
задержку между несколькими сервисами, когда агрегированные metrics и logs
недостаточны. Здесь реализована correlation по request ID, без практического
trace pipeline, Collector или Tempo.

## Проверка и доказательства

До изменения кластера выполни repository validation:

```bash
scripts/ci/validate-observability.sh
git diff --check
```

Скрипт использует закреплённые validation tools и charts, проверяет Go
instrumentation/cardinality, Helm render, Kubernetes manifests, rules,
Alertmanager config с UTF-8 strict mode и фиктивными secret files,
grouping/inhibition, Alloy и dashboard JSON/UID. Он не заменяет live
acceptance и не устанавливает monitoring.

Замерь память отдельно в покое и во время одной Runner job (`concurrent=1`):

```bash
k top nodes
k top pods -A --no-headers
k top pods -A --no-headers | awk '
  { value=$4; unit=value; sub(/^[0-9.]+/, "", unit); sub(/[A-Za-z]+$/, "", value);
    if (unit=="Ki") total+=value/1024;
    else if (unit=="Mi") total+=value;
    else if (unit=="Gi") total+=value*1024;
    else { print "Unexpected memory unit: " unit > "/dev/stderr"; exit 1; } }
  END { if (total>0) printf "Pod memory total: %.1f MiB (%.2f GiB)\n", total, total/1024; }'
```

Сохрани UTC timestamp, job ID и оба исходных вывода, а не только сумму.
Сумма pod usage не включает весь overhead ОС/runtime: приложи также замер
памяти Docker VM/host штатным монитором ОС. Не подменяй usage requests/limits
и не складывай node totals с pod totals: это перекрывающиеся измерения.
Если metrics-server не готов, сначала восстанови его и повтори замер.

Приёмка требует подтверждений всех пунктов:

- Все восемь releases нужных versions готовы; VM/Loki PVC `Bound`, retention три дня.
- Полный discovery, targets `up`, application/Kubernetes/ingress/Argo series в VM.
- Metrics `9090` закрыт для пользовательского Ingress и разрешён monitoring NetworkPolicy.
- Известный request ID найден в API/worker Loki logs; labels не содержат request IDs/secrets.
- Два datasources с фиксированными UID и три provisioned dashboards работают.
- Recording rules и восемь alerts загружены; Telegram получил `firing` и `resolved`.
- Оба инцидента воспроизведены, причины подтверждены, исходное состояние восстановлено.
- Два postmortem содержат Git/MR evidence, UTC timeline и влияние на SLI.
- Полный стенд укладывается в 10 GiB idle и 12 GiB при одной CI job на host с 16 ГБ RAM.

Repository validation, ручная приёмка, доставка Telegram и измерение памяти —
отдельные результаты. До выполнения этих команд в локальном кластере live
acceptance и resource budget считаются неподтверждёнными.

## Postmortem для каждого эксперимента

Создай отдельный Markdown-отчёт для Redis и worker, заполнив реальные данные:

| Поле | Что фиксировать |
| --- | --- |
| Impact | Какие запросы/заявки пострадали, число/доля, affected SLI |
| UTC timeline | Изменение → обнаружение → firing → диагноз → восстановление → resolved |
| Symptom | Наблюдаемое отклонение RED/freshness/replicas |
| Root cause | Подтверждённая причина с events/logs/query evidence |
| Queries | Выполненные MetricsQL/PromQL и exact LogQL с интервалом |
| Delivery | Application/recovery commit, MR, promotion MR, Argo revision, job |
| Recovery | Команды/изменения, отсутствие нового backlog, healthy targets |
| Improvement | Конкретное изменение rule/dashboard/runbook и ожидаемое снижение MTTD/MTTR |

Скриншоты дополняют запросы и вывод команд. Не прикладывай token, Secret YAML,
payload title или raw credentials. Вывод «инцидент устранён» требует проверки
обработки заявок и resolved, а не только зелёного pod в UI.
