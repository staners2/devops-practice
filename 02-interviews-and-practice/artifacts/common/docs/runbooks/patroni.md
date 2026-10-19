# Patroni: установка и репликация

Учебный трёхузловой PostgreSQL для [модуля 05](../modules/05-postgresql.md).
Перед переносом данных прочитай [backup/cutover runbook](postgresql-operations.md).
Все команды ниже выполняются из корня репозитория в Bash, только в локальном
стенде. Инфраструктура устанавливается вручную; приложение остаётся под Argo CD.

## Образ и границы стенда

```bash
PG_CONTEXT=k3d-devops-practice
k() { kubectl --context="$PG_CONTEXT" "$@"; }
kubectl config get-contexts "$PG_CONTEXT"
k get nodes
k -n platform get statefulset,pvc,service
source deploy/platform/versions.env
```

Не продолжай в другом context. Все три pod могут находиться на одном ПК:
практика проверяет отказ участника, а не отказ компьютера или диска.

CI собирает `containers/patroni` rootless BuildKit в отдельный OCI artifact,
сканирует его и публикует в `$CI_REGISTRY_IMAGE/patroni`. PostgreSQL 18.6 и
Patroni 4.1.5 закреплены, Python зависимости устанавливаются по hash lock.
Bootstrap self-test выполняется при сборке образа. Используй фактически
успешную публикацию, а не SHA ещё не собранного commit.

В `deploy/platform/values/postgresql-patroni.yaml` укажи свой registry/repository
и `image.tag: <40-символьный SHA>@sha256:<digest из patroni-image.env>`.
Пустой tag/latest запрещены chart. Для приватного registry добавь existing
imagePullSecret в `imagePullSecrets`; пароль registry не хранится в values.
Мультиплатформенная сборка должна пройти для обеих архитектур: не объявляй
поддержку ARM по одному наличию ARM-образа базового PostgreSQL.

## Secret и установка

Создай три разных пароля: postgres, replicator, devops. Первый также защищает
изменяющие REST-запросы Patroni. Secret не выводится в terminal, YAML или Git:

```bash
set +x
python3 - <<'PY' | k apply -f -
import getpass, json
keys = ("postgres-password", "replication-password", "password")
data = {key: getpass.getpass(key + ": ") for key in keys}
if not all(data.values()) or len(set(data.values())) != 3:
    raise SystemExit("Требуются три непустых разных пароля")
print(json.dumps({"apiVersion":"v1", "kind":"Secret", "type":"Opaque",
    "metadata":{"name":"postgresql-patroni-auth", "namespace":"platform"},
    "stringData":data}))
PY
helm lint deploy/helm/postgresql-patroni \
  -f deploy/platform/values/postgresql-patroni.yaml --strict
mkdir -p .cache/rendered
helm template postgresql-patroni deploy/helm/postgresql-patroni \
  --namespace platform -f deploy/platform/values/postgresql-patroni.yaml \
  > .cache/rendered/postgresql-patroni.yaml
helm upgrade --install postgresql-patroni deploy/helm/postgresql-patroni \
  --kube-context "$PG_CONTEXT" --namespace platform \
  -f deploy/platform/values/postgresql-patroni.yaml --wait --timeout 10m
k -n platform get pods -l app.kubernetes.io/name=postgresql-patroni -L role
```

`helm lint` использует служебное release name; проверка фиксированного имени
может появиться в его INFO, но реальный `helm template postgresql-patroni`
должен пройти.
Chart не содержит operator/CRD или отдельного etcd. DCS хранится в Kubernetes
ConfigMaps; для него используются Kubernetes API и etcd control plane.

## Роли, DCS, Service и PVC

```bash
PRIMARY_POD=$(k -n platform get pod \
  -l app.kubernetes.io/name=postgresql-patroni,role=primary \
  -o jsonpath='{.items[0].metadata.name}')
k -n platform exec "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml list
k -n platform exec "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml show-config
k -n platform get configmap postgresql-patroni-leader -o yaml
k -n platform get endpointslice -l kubernetes.io/service-name=postgresql-primary -o yaml
k -n platform get endpointslice -l kubernetes.io/service-name=postgresql-replicas -o yaml
k -n platform get pvc -l app.kubernetes.io/name=postgresql-patroni
k -n platform exec "$PRIMARY_POD" -- id
k -n platform exec "$PRIMARY_POD" -- psql -X -U postgres -d postgres \
  -c 'SELECT pg_is_in_recovery(), system_identifier FROM pg_control_system();'
k -n platform exec -i "$PRIMARY_POD" -- psql -X -U postgres -d postgres \
  -v ON_ERROR_STOP=1 < scripts/postgresql/replication.sql
```

Leader lock, обновление pod label и reconciliation EndpointSlice не атомарны.
Нужны все три свидетельства: Patroni list, SQL `pg_is_in_recovery()` и endpoint.
Приложение подключается к `postgresql-primary.platform.svc.cluster.local`.
`postgresql-replicas` показывает read-only узлы; не направляй туда записи.

Readiness `/readiness` допускает работоспособную replica, а не только primary.
Liveness `/liveness` проверяет loop Patroni, а startup `/health` даёт время
bootstrap. По умолчанию 3 PVC по 2Gi, non-root UID/GID 999 и fsGroup 999.
При permission error проверь CSI/ownership; не используй `chmod 777`.
Обновление StatefulSet — OnDelete: смена image не означает автоматическую
перезагрузку всех участников. Обновляй по одному, сначала replicas, после
подтверждения их здоровья — switchover и бывший primary.

REST GET `/metrics` доступен мониторингу. Изменяющие операции требуют
authentication и allowlist localhost/members. NetworkPolicy разрешает
PostgreSQL приложению, backup/restore и exporter. Разрешение API egress
`0.0.0.0/0` на 443/6443 — локальный компромисс DNAT/CNI, не production baseline.

## WAL и репликация

На primary смотри `pg_stat_replication`: sent/write/flush/replay LSN,
state/sync_state и разность `pg_wal_lsn_diff`. На replicas —
`pg_stat_wal_receiver`, replay LSN, recovery и timeline. Скрипт выше выбирает
запросы по роли. Physical slots удерживают нужный WAL, но не бесконечно:
`max_slot_wal_keep_size=256MB` ограничивает рост ценой риска потери WAL для
долго отсутствующей replica. Изучай slots и свободное место вместе.

Возраст последней replay transaction растёт и на полностью догнавшей idle
replica. Это не lag; сравнивай актуальные LSN/байты при нагрузке. Ограниченный
`maximum_lag_on_failover` — критерий выбора кандидата, не гарантия нулевого RPO.
После cutover проверь весь business flow: POST, worker, GET, stats. Зафиксируй
известные UUID и подтверждённые записи до эксперимента.

## Асинхронное переключение

Сначала примени baseline и проверь фактический DCS:

```bash
k -n platform exec -i "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml \
  edit-config --apply - --force < deploy/patroni/async.yaml
k -n platform exec "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml show-config
```

Для воспроизводимого удаления текущего leader через scale сначала сделай
switchover на ordinal 2. Если он уже primary, switchover пропусти.
Проверь list и endpoints после команды:

```bash
k -n platform exec "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml \
  switchover --candidate postgresql-patroni-2 --force
PRIMARY_POD=postgresql-patroni-2
k -n platform exec "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml list
date -u +%FT%TZ
k -n platform scale statefulset/postgresql-patroni --replicas=2
k -n platform get pods -l app.kubernetes.io/name=postgresql-patroni -L role -w
```

Останови watch после нового primary, проверь новый endpoint, dashboard и
успешную новую заявку. Проверь все подтверждённые UUID через PostgreSQL,
не только Redis. Запиши UTC, фактические RPO/RTO, timeline и влияние на API.
Оставь ordinal 2 выключенным как минимум дольше действующего TTL из DCS
(baseline 30s) и зафиксируй время. При graceful shutdown Patroni может
отпустить leader lock раньше TTL: не выдавай этот опыт за TTL-driven election.
`scale` завершает pod штатно: это controlled member-loss, не доказательство
поведения при crash, network partition или split brain. PDB не запрещает
scale StatefulSet.

```bash
k -n platform scale statefulset/postgresql-patroni --replicas=3
k -n platform wait --for=condition=Ready pod/postgresql-patroni-2 --timeout=10m
PRIMARY_POD=$(k -n platform get pod \
  -l app.kubernetes.io/name=postgresql-patroni,role=primary \
  -o jsonpath='{.items[0].metadata.name}')
k -n platform exec "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml list
k -n platform logs postgresql-patroni-2 --tail=100
```

Проверь rejoin, timeline и применение `pg_rewind`. Checksums/WAL hints уже
включены. Chart не удаляет data directory при ошибке rewind/diverged timeline.
Не удаляй PVC и не запускай destructive reinit для получения зелёного списка:
сначала диагностика, затем отдельное подтверждение цели/потери данных.

## Strict synchronous: доступность против подтверждённых данных

При трёх здоровых узлах примени dynamic config; bootstrap values не меняют
уже созданный DCS:

```bash
k -n platform exec -i "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml \
  edit-config --apply - --force < deploy/patroni/strict-sync.yaml
k -n platform exec "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml show-config
k -n platform exec -i "$PRIMARY_POD" -- psql -X -U postgres -d postgres \
  -v ON_ERROR_STOP=1 < scripts/postgresql/replication.sql
```

Нужна одна synchronous replica. Чтобы убрать обе replicas, сначала переведи
primary на ordinal 0; если он уже primary, switchover пропусти. После
подтверждения роли можно scale до 1:

```bash
k -n platform exec "$PRIMARY_POD" -- patronictl -c /etc/patroni/patroni.yaml \
  switchover --candidate postgresql-patroni-0 --force
k -n platform exec postgresql-patroni-0 -- patronictl -c /etc/patroni/patroni.yaml list
k -n platform scale statefulset/postgresql-patroni --replicas=1
```

Новая запись ожидает synchronous acknowledgement. Сравни API timeout, SQL
wait_event, sync_state, метрики и отсутствие успешного ответа. Timeout клиенту
не гарантирует rollback: транзакция могла уже записать commit, ожидая ack.
Проверь конкретный ID после возврата replicas; не повторяй создание вслепую.

```bash
k -n platform scale statefulset/postgresql-patroni --replicas=3
k -n platform wait --for=condition=Ready pod \
  -l app.kubernetes.io/name=postgresql-patroni --timeout=10m
k -n platform exec -i postgresql-patroni-0 -- patronictl -c /etc/patroni/patroni.yaml \
  edit-config --apply - --force < deploy/patroni/async.yaml
k -n platform exec postgresql-patroni-0 -- patronictl -c /etc/patroni/patroni.yaml show-config
k -n platform exec postgresql-patroni-0 -- patronictl -c /etc/patroni/patroni.yaml list
```

Оставь три участника и async baseline. В отчёте объясни, когда приемлемы
async, synchronous и strict synchronous: latency/availability/потеря
подтверждённых записей, а не только переключатели config.

## Границы HA

Patroni loop/TTL влияют на election, но TTL 30s не равен обещанию RTO 30s:
учитывай detection, promotion, endpoints, pool reconnection и business flow.
Не запускай ручной `pg_ctl promote` параллельно Patroni. DCS/fencing важны
при partition; watchdog здесь отключён, production split-brain safety не
доказана. HA повторяет ошибочный DELETE на replicas и не заменяет backup/PITR.

Источники: [Patroni replication modes](https://patroni.readthedocs.io/en/latest/replication_modes.html),
[PostgreSQL 18: warm standby](https://www.postgresql.org/docs/18/warm-standby.html).
