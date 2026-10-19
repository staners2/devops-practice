# PostgreSQL: backup, restore и диагностика

Референс [модуля 05](../modules/05-postgresql.md), поверх готового 04.
Команды выполняются из корня репозитория в Bash. Нужны Python 3, jq,
kubectl/Helm и локальный кластер; инфраструктура устанавливается вручную,
приложение изменяется через application/promotion MR и Argo CD.

## Проверить границы

```bash
PG_CONTEXT=k3d-devops-practice
k() { kubectl --context="$PG_CONTEXT" "$@"; }
kubectl config get-contexts "$PG_CONTEXT"
k get nodes
k -n platform get statefulset,pvc,service
k -n devops-practice get deployment
k -n argocd get application devops-practice
source deploy/platform/versions.env
```

Не продолжай, если context отсутствует или найден production. Сначала запиши
источник: Service, DB, версию сервера и известный ID заявки, её status и counts.
Пароли/DSN не попадают в отчёт. Старый Service — `postgresql.platform.svc.cluster.local`;
после переноса — `postgresql-primary.platform.svc.cluster.local`.

## Подготовить backup

Создай Secret интерактивно. В первый раз укажи старый Service и admin password
существующего PostgreSQL; после cutover повтори для нового primary и нового
admin password. Shell tracing выключен, пароль не входит в argv.

```bash
set +x
python3 - <<'PY' | k apply -f -
import getpass, json
host = "postgresql.platform.svc.cluster.local"  # после cutover: postgresql-primary.platform.svc.cluster.local
if host not in ("postgresql.platform.svc.cluster.local", "postgresql-primary.platform.svc.cluster.local"):
    raise SystemExit("Неожиданный учебный Service")
password = getpass.getpass("Admin password PostgreSQL: ")
if not password:
    raise SystemExit("Пустой пароль")
print(json.dumps({"apiVersion":"v1","kind":"Secret","type":"Opaque",
    "metadata":{"name":"postgresql-maintenance-auth","namespace":"platform"},
    "stringData":{"PGHOST":host,"PGPORT":"5432","PGUSER":"postgres",
                  "PGDATABASE":"devops_practice","PGPASSWORD":password}}))
PY
k -n platform create configmap postgresql-maintenance-scripts \
  --from-file=scripts/postgresql/common.sh \
  --from-file=scripts/postgresql/backup.sh \
  --from-file=scripts/postgresql/restore.sh \
  --dry-run=client -o yaml | k apply -f -
k apply -f deploy/postgresql/backup-pvc.yaml
k apply -f deploy/postgresql/backup-cronjob.yaml
```

Service выбирается явно в коде выше, а `getpass` читает пароль через TTY.
Не подставляй пароль в heredoc, команды shell или файл values.

CronJob: каждые 6h UTC, Forbid, timeout 1800s, три успешных bundles, отдельный
PVC 2Gi. Не запускай ручной Job одновременно с CronJob: Forbid не управляет
Jobs, созданными вручную. Для первого опыта:

```bash
k -n platform patch cronjob postgresql-backup --type=merge -p '{"spec":{"suspend":true}}'
k -n platform get jobs,pods
BACKUP_JOB="postgresql-backup-manual-$(date -u +%Y%m%d%H%M%S)"
k -n platform create job "$BACKUP_JOB" --from=cronjob/postgresql-backup
k -n platform wait --for=condition=complete "job/$BACKUP_JOB" --timeout=1800s
k -n platform logs "job/$BACKUP_JOB"
```

Успех содержит BACKUP_ID, не credentials. Bundle: `database.dump`,
`SHA256SUMS`, `metadata.json` (БД, system_identifier, версии client/server,
UTC start/end, size).
Публикация только после успешного dump/checksum; ошибка retention также даёт
failed Job, но новая завершённая копия сохраняется. До следующего backup
проверь ошибку и оставшиеся bundles. Скрипты никогда не исполняют metadata.

## Восстановить новую БД и вынести копию за пределы PVC

Заполни ID из успешного Job и новую цель. Job откажется от существующей,
системной или основной БД до CREATE; повторный запуск требует нового имени.
Не используй `--clean`, `--create`, force или удаление основной БД.
Перед командами задай `KNOWN_ID` реальным UUID, сохранённым до dump.
Независимо от maintenance Secret выбери `RESTORE_HOST` по проверенному
Service. Скрипт сравнивает его с PGHOST и сравнивает system_identifier
целевого сервера с metadata backup до CREATE. Обычный drill выполняется
на том же кластере, с которого сделан dump; перенос между кластерами —
отдельная процедура cutover ниже. Старые bundles без identity не принимаются.

```bash
BACKUP_ID=20261006T100000Z-0123456789abcdef  # замени фактическим ID
RESTORE_DATABASE=devops_practice_restore_drill01
RESTORE_HOST=postgresql.platform.svc.cluster.local  # после cutover: postgresql-primary.platform.svc.cluster.local
RESTORE_JOB="postgresql-restore-$(date -u +%Y%m%d%H%M%S)"
k set env --local -f deploy/postgresql/restore-job.yaml -o json \
  BACKUP_ID="$BACKUP_ID" RESTORE_DATABASE="$RESTORE_DATABASE" EXPECTED_PGHOST="$RESTORE_HOST" \
  | jq --arg name "$RESTORE_JOB" '.metadata.name = $name' \
  | k create -f -
k -n platform wait --for=condition=complete "job/$RESTORE_JOB" --timeout=1800s
k -n platform logs "job/$RESTORE_JOB"
k apply -f deploy/postgresql/backup-reader.yaml
k -n platform wait --for=condition=Ready pod/postgresql-backup-reader --timeout=120s
k -n platform exec -i postgresql-backup-reader -- env PGDATABASE="$RESTORE_DATABASE" \
  psql -X -v ON_ERROR_STOP=1 -v expected_database="$RESTORE_DATABASE" \
  -v known_id="$KNOWN_ID" < scripts/postgresql/verify-restore.sql
BACKUP_EXPORT_DIR="$HOME/devops-practice-backups"
umask 077
mkdir -p "$BACKUP_EXPORT_DIR/$BACKUP_ID"
k -n platform cp "postgresql-backup-reader:/backups/$BACKUP_ID/." "$BACKUP_EXPORT_DIR/$BACKUP_ID/"
(cd "$BACKUP_EXPORT_DIR/$BACKUP_ID" && shasum -a 256 -c SHA256SUMS)
k -n platform delete pod postgresql-backup-reader
```

Если restore упал после создания новой БД, оставь её для расследования и выбери
другую цель. Старая БД/PVC не изменяются. Удалять неуспешную копию можно только
после отдельной проверки точных host/DB/сессий, не широким cleanup.

Внешняя копия на ПК переживает удаление кластера, но не отказ диска самого ПК.
Dump содержит данные: права доступа ограничены, Git/CI 02-interviews-and-practice/artifacts не используются.
Сохраняй metadata и checksum вместе с dump.

## Проверить восстановление приложением

Создай `devops-practice-restore-runtime` в `devops-practice` с DATABASE_URL
на новую БД и прежним REDIS_PASSWORD. Используй интерактивный блок из раздела
«Cutover» ниже, заменив Secret name и DB; host должен соответствовать серверу,
на котором создана restore-БД. До cutover это старый PostgreSQL.

Проверь values: API/worker по одной replica, Redis DB 1,
`requests.restore.created.v1`, `request-worker-restore-v1`, без Ingress.
Не меняй основную Kafka group и не используй FLUSHALL.

```bash
export GITOPS_REPO_URL=https://gitlab.com/your-namespace/devops-practice.git
sed "s|__GITOPS_REPO_URL__|$GITOPS_REPO_URL|g" deploy/argocd/restore-application.yaml | k apply -f -
k -n argocd get application devops-practice-restore
k -n devops-practice get jobs,pods,service
k -n devops-practice logs job/devops-practice-restore-smoke
k -n devops-practice port-forward service/devops-practice-restore-api 18080:8080
```

В другом терминале проверь `http://localhost:18080/api/health-check`, известный
ID и создание/обработку новой заявки. До smoke SQL должен подтвердить
current_database, migrations, known rows и counts. Не считай health-check
доказательством восстановления business data.

После smoke останови port-forward и через MR добавь последним valueFile
ТОЛЬКО в `restore-application.yaml`:
`$values/deploy/environments/maintenance/values.yaml`.
Application-манифест — manual bootstrap, не self-managed ресурс. После merge
MR и обновления локальной ветки повторно примени ТОЛЬКО его конфигурацию:

```bash
sed "s|__GITOPS_REPO_URL__|$GITOPS_REPO_URL|g" deploy/argocd/restore-application.yaml | k apply -f -
```

Теперь дождись Argo sync и
проверь, что `devops-practice-restore-api` и `devops-practice-restore-worker`
имеют replicas 0 и их pods завершились. Основной Application не меняется.
Это освобождает RAM перед cutover/CI, сохраняет restore-БД и backup PVC.
Для следующего drill сначала подготовь новую БД/Secret и затем убери этот
valueFile через MR. Plain delete Application без cascading cleanup оставляет
workloads; вместо него используй этот GitOps shutdown.

## Cutover в Patroni

Сначала установи и проверь [Patroni](patroni.md). Один primary и две streaming
replicas должны быть видны до переноса. Не запускай одновременно restore-copy,
CI и нагрузку без необходимости.

1. В MR добавь к valueFiles основного Application после development файл
   `$values/deploy/environments/maintenance/values.yaml`. После merge обнови
   локальную ветку и примени только изменённый Application-манифест:
   `sed "s|__GITOPS_REPO_URL__|$GITOPS_REPO_URL|g" deploy/argocd/application.yaml | k apply -f -`.
   Сам Application вручную bootstrapped; изменения его YAML в Git без этого
   шага не меняют `spec.valueFiles` в кластере. Сами workloads обновляет Argo CD,
   не эта команда. Дождись sync и
   API/worker replicas 0; hooks migration/smoke отключены. Убедись, что writers
   ушли из pg_stat_activity. Manual scale Argo-managed workloads не подходит.
2. На старом PostgreSQL сделай финальный dump по процедуре выше, сохрани ID,
   known rows/counts и копию на ПК. CronJob остаётся suspended.
3. Пересоздай maintenance Secret на новый primary. Старый backup-reader
   должен быть удалён: env из Secret не обновляется у уже запущенного pod.
   Создай новый reader. Получи system_identifier непосредственно на Patroni
   primary и сравни с целевым соединением; проверь `pg_is_in_recovery() = false`.
4. Основной перенос отличается от restore drill: `devops_practice` уже создана
   bootstrap на НОВОМ сервере. Только после явного подтверждения этой цели,
   server identity и отсутствия таблиц выполни отдельную процедуру:

```bash
EXPECTED_SYSTEM_IDENTIFIER=1234567890123456789  # замени результатом pg_control_system() нового primary
k -n platform exec postgresql-backup-reader -- \
  env BACKUP_ID="$BACKUP_ID" EXPECTED_SYSTEM_IDENTIFIER="$EXPECTED_SYSTEM_IDENTIFIER" \
  sh -eu -c '
    . /scripts/common.sh
    check_connection
    check_directory
    check_bundle
    test "$PGHOST" = postgresql-primary.platform.svc.cluster.local
    test "$PGDATABASE" = devops_practice
    actual=$(psql -X -At -v ON_ERROR_STOP=1 -c "SELECT system_identifier FROM pg_control_system()")
    test "$actual" = "$EXPECTED_SYSTEM_IDENTIFIER"
    test "$(psql -X -At -v ON_ERROR_STOP=1 -c "SELECT pg_is_in_recovery()")" = f
    test "$(psql -X -At -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM pg_tables WHERE schemaname = '\''public'\''")" = 0
    pg_restore --single-transaction --exit-on-error --no-owner --no-acl \
      --role=devops --dbname=devops_practice "$bundle/database.dump"
    psql -X -v ON_ERROR_STOP=1 -c "ANALYZE"
  '
```

Проверяй known rows/counts через verify-restore.sql с expected_database
`devops_practice`. Обычный restore.sh по-прежнему запрещает основную БД;
не добавляй в него исключение/force для cutover.

5. Создай основной runtime Secret, не печатая DSN:

```bash
set +x
python3 - <<'PY' | k apply -f -
import getpass, json
from urllib.parse import quote
password = getpass.getpass("Пароль devops в Patroni: ")
redis_password = getpass.getpass("Прежний Redis password: ")
if not password:
    raise SystemExit("Пустой пароль PostgreSQL")
url = "postgresql://devops:" + quote(password, safe="") + "@postgresql-primary.platform.svc.cluster.local:5432/devops_practice?sslmode=disable"
print(json.dumps({"apiVersion":"v1","kind":"Secret","type":"Opaque",
    "metadata":{"name":"devops-practice-patroni-runtime","namespace":"devops-practice"},
    "stringData":{"DATABASE_URL":url,"REDIS_PASSWORD":redis_password}}))
PY
```

6. В MR поменяй development runtimeSecretName на
   `devops-practice-patroni-runtime`, убери maintenance valueFile. Image остаётся
   immutable. После удаления maintenance valueFile повторно примени Application
   после merge, как в шаге 1: один Git commit сам по себе не удаляет его из live
   spec. Затем дождись migrations/smoke и healthy Argo. Проверь новые записи.
7. Останови только старый инфраструктурный StatefulSet `postgresql`,
   сохрани PVC и финальный backup:
   `k -n platform scale statefulset/postgresql --replicas=0`.
   Не запускай его Helm upgrade с прежним replica count, не удаляй PVC.
8. Backup Secret уже направлен на primary; сделай новый dump/restore drill.
   Сними suspend CronJob. После новых записей простой возврат старого DSN
   потеряет данные. Обратный перенос требует нового maintenance, актуального
   dump и отдельного подтверждённого восстановления, а не «flip DSN».

## SQL, timeout и pool

Для SQL используй client reader с нужным Secret либо локальный psql с env/
PGPASSFILE. Не передавай парольный DSN в argv. `activity.sql`, `locks.sql` и
`replication.sql` запускаются через `psql -X -v ON_ERROR_STOP=1 -f` либо stdin.

Для benchmark создай новую БД `devops_practice_benchmark_<суффикс>` владельца
devops из template0. Через env задай её в PGDATABASE и вызови
`sh scripts/postgresql/measure.sh`. Скрипт откажется от существующей requests,
создаст schema 001, до 100000 маркированных строк и сохранит пять actual JSON
планов чтения/записи до/после additive index 002 и median. UPDATE измеряется
в rollback transaction. Результаты в `.cache/postgresql-benchmark`, не в Git.
Не удаляй индексы основного приложения и не отключай enable_seqscan.

В отчёт включи buffers, план, median и размер индекса. Rollback не отменяет
генерацию WAL и dead tuples; для строго сравнимой нагрузки используй свежие
benchmark-БД с одинаковыми данными и учитывай порядок прогрева/ANALYZE.

Для блокировки: в своей сессии `BEGIN; LOCK TABLE requests IN ACCESS EXCLUSIVE MODE;`,
в другой выполни SELECT. Найди blocking PID; закончи первую через ROLLBACK.
Для pool pressure держи несколько собственных запросов занятыми; увеличение
PG_STATEMENT_TIMEOUT через MR для опыта должно затем быть отменено.
Не убивай чужие PID и не перезапускай БД.

Application defaults: max/min pool 5/1, statement 1s, lock 500ms,
idle-in-transaction 5s. REQUEST_TIMEOUT 2s — отдельный deadline. Две API и
один worker могут занять до 15 соединений; добавь бюджеты migrator/exporter/admin.
Migrator использует 120s statement и 5s lock timeout, общий context 120s.
Acquire duration — ожидание pool, не длительность SQL; смотри также canceled
acquires, pg_stat_activity, locks и API latency.

## Мониторинг и alerts

После cutover получи `PRIMARY_POD` по инструкции [Patroni](patroni.md).
Создай отдельную login role с `pg_monitor`, не superuser. Команда читает
admin password из окружения Patroni, а новый пароль — через TTY:

```bash
set +x
k -n platform exec -it "$PRIMARY_POD" -- /opt/patroni/bin/python -c '
import getpass, os, psycopg
password = getpass.getpass("Пароль devops_exporter: ")
if not password:
    raise SystemExit("Пустой пароль")
try:
    with psycopg.connect(host="127.0.0.1", user="postgres", dbname="postgres",
                        password=os.environ["PATRONI_SUPERUSER_PASSWORD"],
                        autocommit=True, cursor_factory=psycopg.ClientCursor) as conn:
        with conn.cursor() as cur:
            cur.execute("SELECT 1 FROM pg_roles WHERE rolname = %s", ("devops_exporter",))
            if cur.fetchone() is None:
                cur.execute("CREATE ROLE devops_exporter")
            cur.execute("ALTER ROLE devops_exporter LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %s", (password,))
            cur.execute("GRANT pg_monitor TO devops_exporter")
except psycopg.Error as exc:
    raise SystemExit("Ошибка PostgreSQL: " + type(exc).__name__ + " SQLSTATE=" + str(exc.sqlstate))
'
python3 - <<'PY' | k apply -f -
import getpass, json
password = getpass.getpass("Тот же пароль devops_exporter: ")
if not password:
    raise SystemExit("Пустой пароль")
print(json.dumps({"apiVersion":"v1", "kind":"Secret", "type":"Opaque",
    "metadata":{"name":"postgresql-exporter-auth", "namespace":"monitoring"},
    "stringData":{"username":"devops_exporter", "password":password}}))
PY
```

Затем установи готовый chart:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update prometheus-community
helm pull prometheus-community/prometheus-postgres-exporter \
  --version "$POSTGRES_EXPORTER_CHART_VERSION" --untar --untardir .cache/charts
helm upgrade --install postgresql-exporter .cache/charts/prometheus-postgres-exporter \
  --kube-context "$PG_CONTEXT" --namespace monitoring \
  -f deploy/platform/values/prometheus-postgres-exporter.yaml --wait --timeout 5m
```

Обнови ConfigMaps vmagent-scrape, vmalert-rules, monitoring-dashboards и
alloy-config командами из [observability.md](observability.md); перезапусти
vmagent/Alloy согласно их способу reload. Dashboard UID: postgresql-operations.
Scrape Patroni — каждый pod:8008, не primary Service; exporter — один primary.
Alertmanager/Telegram уже настроены в 04, delivery notifier GitLab не заменяет их.

Новые сигналы: PostgreSQLUnavailable, PostgreSQLPrimaryMissing,
PatroniMemberMissing, PatroniReplicationLag, PostgreSQLPoolPressure,
PostgreSQLConnectionsHigh, PostgreSQLBackupFailed/Stale/Missing.
Проверь target каждого участника, pg_up, роли/timeline, actual WAL byte lag,
connections/activity/locks, pool/API и backup age. Пустой backup success до
первого расписанного запуска не должен отображаться как успешный ноль.

Ручной Job не обновляет last_successful_time CronJob и не входит в owner-based
alert. Для проверки scheduled alerts временно поставь расписание каждую минуту,
сними suspend, дождись Job с CronJob ownerReference, success/failure и resolved;
верни исходный backup-cronjob.yaml. Не оставляй частое расписание.
Failed status — gauge, не counter: старый failed Job не должен тревожить после
нового успешного. Missing target — не здоровая replica с lag=0.

## Приёмка и PITR

Три отчёта: учебное удаление/restore, SQL-деградация, primary loss. В каждом:
impact, UTC события, known IDs, причина и подтверждающие SQL/метрики/логи,
восстановление, prevention. RPO считай по подтверждённым записям, RTO — от
отказа до успешного нового business flow; dump completion не равен service RTO.
Замерь idle/backup/restore/одну CI job; цели RAM 10/12Gi не считаются выполненными
без фактических замеров.

Backup в 12:00 не восстановит данные прямо перед ошибочным DELETE в 14:37.
PITR использует physical base backup и непрерывную цепочку WAL с нужной
timeline и recovery target/timezone. pgBackRest/WAL-G автоматизируют backup,
archive и restore; logical pg_dump не заменяет physical base. Репликация
повторяет DELETE, HA не заменяет backup. PITR здесь только теория, без установки
MinIO/S3/WAL-G. [Документация PostgreSQL](https://www.postgresql.org/docs/18/continuous-archiving.html).
