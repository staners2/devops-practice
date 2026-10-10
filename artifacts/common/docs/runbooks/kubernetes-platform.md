# Локальная Kubernetes-платформа

> Module 03 добавляет Argo CD как отдельный platform component. Его установка
> описана в [`argocd-gitops.md`](argocd-gitops.md); существующие PostgreSQL,
> Redis, Kafka, ingress-nginx и GitLab Runner продолжают устанавливаться по
> этому runbook.

## Требования

Установи Docker-совместимый runtime, `k3d 5.8.3`, `kubectl 1.35.8` и
`helm 4.3.0`. Все закреплённые версии перечислены в
`deploy/platform/versions.env`.

## Создание кластера

```bash
k3d cluster create --config deploy/k3d/config.yaml

for namespace in ingress-nginx platform gitlab-runner devops-practice; do
  kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f -
done

kubectl get nodes
kubectl get storageclass
```

Кластер содержит один server и два agent nodes. Встроенный Traefik отключён,
а host port `8080` направлен на порт `80` k3d load balancer.

## ingress-nginx

> ingress-nginx завершил поддержку в марте 2026 года и больше не получает
> исправления, включая security fixes. В этой лаборатории он оставлен только
> для простого локального Ingress. Для новой production-платформы выбери
> поддерживаемую реализацию Gateway API.

```bash
mkdir -p .cache/charts
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update ingress-nginx
helm pull ingress-nginx/ingress-nginx --version 4.15.1 \
  --untar --untardir .cache/charts
helm upgrade --install ingress-nginx .cache/charts/ingress-nginx \
  --namespace ingress-nginx \
  -f deploy/platform/values/ingress-nginx.yaml \
  --wait --timeout 10m

kubectl -n ingress-nginx get pods,service
```

## PostgreSQL, Redis и Kafka

Создай локальные пароли без вывода в терминал:

```bash
printf 'PostgreSQL password: ' >&2
IFS= read -r -s POSTGRES_PASSWORD
printf '\nRedis password: ' >&2
IFS= read -r -s REDIS_PASSWORD
printf '\n' >&2

kubectl -n platform create secret generic postgresql-auth \
  --from-literal=postgres-password="$POSTGRES_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n platform create secret generic redis-auth \
  --from-literal=redis-password="$REDIS_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Скачай закреплённые версии charts и установи каждый компонент отдельно:

```bash
helm pull oci://registry-1.docker.io/cloudpirates/postgres \
  --version 0.13.6 --untar --untardir .cache/charts
helm pull oci://registry-1.docker.io/cloudpirates/redis \
  --version 0.35.4 --untar --untardir .cache/charts
helm pull oci://registry-1.docker.io/cloudpirates/kafka \
  --version 0.1.6 --untar --untardir .cache/charts

helm upgrade --install postgresql .cache/charts/postgres \
  --namespace platform -f deploy/platform/values/postgresql.yaml \
  --wait --timeout 10m
helm upgrade --install redis .cache/charts/redis \
  --namespace platform -f deploy/platform/values/redis.yaml \
  --wait --timeout 10m
helm upgrade --install kafka .cache/charts/kafka \
  --namespace platform -f deploy/platform/values/kafka.yaml \
  --wait --timeout 10m

kubectl -n platform get statefulset,pod,service,pvc
kubectl -n platform wait --for=condition=Ready pod --all --timeout=10m
```

После установки очисти локальные пароли инфраструктуры:

```bash
unset POSTGRES_PASSWORD REDIS_PASSWORD
```

## Runtime Secret приложения

GitLab Runner не создаёт и не изменяет Secrets. Введи готовую PostgreSQL URL
для пользователя `devops`, базы `devops_practice` и адреса
`postgresql.platform.svc.cluster.local:5432`, а также текущий пароль Redis:

```bash
printf 'DATABASE_URL: ' >&2
IFS= read -r -s DATABASE_URL
printf '\nRedis password: ' >&2
IFS= read -r -s REDIS_PASSWORD
printf '\n' >&2

kubectl -n devops-practice create secret generic devops-practice-runtime \
  --from-literal=DATABASE_URL="$DATABASE_URL" \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -

unset DATABASE_URL REDIS_PASSWORD
```

Secret создаётся один раз как bootstrap-объект. Argo CD управляет workloads,
которые на него ссылаются, но не самим Secret. External Secrets, SOPS, Vault
и Sealed Secrets рассматриваются в следующих модулях. Не добавляй значения в
GitLab variables, Git, artifacts или job logs.

Kafka работает одним узлом в KRaft mode без ZooKeeper и client
authentication. Это учебная конфигурация, а не production HA.

После deployment приложения проверь маршрут с компьютера:

```bash
curl --fail --show-error \
  -H 'Host: devops-practice.localhost' \
  http://127.0.0.1:8080/api/health-check
```

Приложением управляет Argo CD. В штатном потоке GitLab публикует неизменяемый
image, открывает promotion MR с новым tag в development values, а после merge
job `verify-development` только проверяет sync, workloads, hooks и ingress.
Runtime Secret остаётся ручным bootstrap-объектом и не создаётся из CI.

## Приёмка приложения

Сначала зафиксируй состав и потребление ресурсов:

```bash
kubectl get pods -A
kubectl -n devops-practice get deploy,job,service,ingress,pdb,networkpolicy
kubectl top pods -A
helm -n devops-practice history devops-practice
curl --fail --show-error \
  -H 'Host: devops-practice.localhost' \
  http://127.0.0.1:8080/api/health-check
```

Сложи колонку `MEMORY(bytes)` из `kubectl top pods -A --no-headers`. В отчёте
приложи снимки в покое и во время одной CI job. Критерии: не более 8 ГБ в
покое, не более 10 ГБ во время job. Если metrics-server ещё не начал отдавать
метрики, дождись его готовности, а не подменяй измерение суммой requests.

Последний migration и smoke hooks должны завершиться успешно:

```bash
kubectl -n devops-practice logs job/devops-practice-migrate
kubectl -n devops-practice logs job/devops-practice-smoke
kubectl -n devops-practice get deploy devops-practice-api \
  -o jsonpath='{.status.readyReplicas}{" ready replicas\n"}'
```

Smoke job создаёт заявку и проверяет весь поток PostgreSQL → Kafka → worker →
Redis, включая статус `processed` и `/api/stats`. Для проверки доступности
двух replicas запусти в первом терминале цикл:

```bash
for attempt in $(seq 1 100); do
  curl --fail --silent --show-error \
    -H 'Host: devops-practice.localhost' \
    http://127.0.0.1:8080/api/health-check >/dev/null || exit 1
  sleep 0.2
done
```

Во втором терминале во время цикла удали один готовый API pod:

```bash
api_pod=$(kubectl -n devops-practice get pod \
  -l app.kubernetes.io/instance=devops-practice,app.kubernetes.io/component=api \
  -o jsonpath='{.items[0].metadata.name}')
kubectl -n devops-practice delete pod "$api_pod" --wait=false
kubectl -n devops-practice rollout status deployment/devops-practice-api \
  --timeout=3m
```

Цикл должен завершиться без ошибок, а Deployment снова получить две готовые
replicas.

## Проверка probes при отказе PostgreSQL

Сохрани текущее количество replicas PostgreSQL, останови StatefulSet и
дождись, когда API станет `NotReady`:

```bash
postgres_sts=$(kubectl -n platform get statefulset \
  -l app.kubernetes.io/instance=postgresql \
  -o jsonpath='{.items[0].metadata.name}')
test -n "$postgres_sts"
postgres_replicas=$(kubectl -n platform get statefulset "$postgres_sts" \
  -o jsonpath='{.spec.replicas}')
kubectl -n platform scale statefulset "$postgres_sts" --replicas=0

api_pod=$(kubectl -n devops-practice get pod \
  -l app.kubernetes.io/instance=devops-practice,app.kubernetes.io/component=api \
  -o jsonpath='{.items[0].metadata.name}')
kubectl -n devops-practice port-forward "pod/$api_pod" 18080:8080
```

Пока `port-forward` работает, во втором терминале проверь разделение probes:

```bash
curl -i http://127.0.0.1:18080/livez
curl -i http://127.0.0.1:18080/readyz
kubectl -n devops-practice get endpointslice \
  -l kubernetes.io/service-name=devops-practice-api -o yaml
kubectl -n devops-practice get pod "$api_pod" \
  -o jsonpath='{.status.containerStatuses[0].restartCount}{" restarts\n"}'
```

Ожидается `livez=200`, `readyz=503`, `ready: false` у адресов API и отсутствие
новых restart. Затем останови `port-forward`, верни PostgreSQL и дождись
восстановления:

```bash
kubectl -n platform scale statefulset "$postgres_sts" \
  --replicas="$postgres_replicas"
kubectl -n platform rollout status statefulset/"$postgres_sts" --timeout=5m
kubectl -n devops-practice wait --for=condition=Ready pod \
  -l app.kubernetes.io/component=api --timeout=3m
```

## Проверка rollback и ошибки миграции

После включения Argo CD не запускай локальный `helm upgrade` поверх
управляемого release. Failed image, ошибка migration и rollback через Git
revert воспроизводятся по
[`argocd-gitops.md`](argocd-gitops.md#эксперимент-failed-image-и-migration).
Предыдущая готовая версия должна оставаться доступной, а намеренно сломанные
изменения — удаляться из финальной ветки через MR.

## Проверка NetworkPolicy

Получи IP одного API pod и попробуй подключиться к нему из namespace
`default`, который не входит в allowlist:

```bash
api_ip=$(kubectl -n devops-practice get pod \
  -l app.kubernetes.io/instance=devops-practice,app.kubernetes.io/component=api \
  -o jsonpath='{.items[0].status.podIP}')
kubectl -n default run network-check --rm -i --restart=Never \
  --image=curlimages/curl:8.19.0 -- \
  curl --connect-timeout 3 --fail "http://$api_ip:8080/livez"
```

Команда должна завершиться ошибкой подключения. Маршрут через разрешённый
ingress-nginx при этом остаётся доступен:

```bash
curl --fail --show-error \
  -H 'Host: devops-practice.localhost' \
  http://127.0.0.1:8080/livez
```

## Модуль 05: PostgreSQL и Patroni

После модуля 04 старый PostgreSQL переносится в новый трёхузловой кластер.
Установка собственного [Helm chart Patroni](patroni.md) выполняется вручную,
без оператора. [Runbook PostgreSQL](postgresql-operations.md) описывает
maintenance через GitOps, финальный dump, проверку переноса и отдельный
restore release. Старый PostgreSQL/PVC сохраняются до подтверждения результата;
поверх Argo CD локальный Helm upgrade приложения не выполняется.
Эти команды предназначены только для учебного context `k3d-devops-practice`.
