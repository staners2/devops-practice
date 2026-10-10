# Kafka: события, consumers и сроки хранения

Уровень: **углублённый, необязательный**.

Исторические номера внутри технических имён стендов сохраняются для совместимости.

## Подготовка рабочей копии

Из корня курса:

```bash
bash artifacts/init.sh 09 02
cd 09-kafka/02-consumer-groups/workspace
```

Все команды задания выполняй из этого рабочего каталога.

## Заявка от руководителя

> События о заявках должны независимо получать обработчик заявок,
> уведомления и аналитика. Сервисы работают с разной скоростью и иногда
> недоступны. Нам нужен способ накопить события, прочитать их позже
> и повторить обработку после исправления ошибки.
>
> Подготовьте локальный стенд Kafka в контейнере. Проверьте topics,
> распределение работы между consumers, продолжение чтения после остановки
> и сроки хранения. Покажите, что происходит с отставшим consumer.
>
> Предоставьте настройки и наблюдения из экспериментов. Объясните,
> какую проблему решает Kafka, когда она избыточна и какие ограничения
> есть у вашего стенда.

Это одна обязательная практика уровня Hard. Её можно выполнить отдельно
от модулей `00`–`05`: Go API/worker, Kubernetes, PostgreSQL и Redis не нужны.
Тема заявок общая с проектом, но события здесь демонстрационные и не
отправляются в существующий worker.

## Что вы получаете и что нужно сделать

Готовы [Compose](../../artifacts/common/deploy/kafka-lab/compose.yaml) и два файла событий:
[requests.events](../../artifacts/common/deploy/kafka-lab/fixtures/requests.events),
[followup.events](../../artifacts/common/deploy/kafka-lab/fixtures/followup.events).
Topics и consumer groups не создаются автоматически.

Ваша работа — изучить настройки, запустить стенд, выполнить девять опытов
и объяснить результаты. Команды ниже помогают воспроизвести опыт, но не
заменяют выводы инженера. Не сдавайте чужие offsets или заранее заполненный
отчёт: фиксируйте фактические значения своего стенда.

Нужны Git, Docker Desktop либо Docker Engine с Compose и несколько окон
терминала. На Linux/macOS команды выполняются в `sh`-совместимой оболочке.
На Windows подойдёт Git Bash с Docker Desktop в режиме Linux containers;
WSL — ещё один вариант, не обязательное требование. Java и Kafka на основную
ОС устанавливать не нужно. В Git Bash дополнительно выполните:

```bash
export MSYS_NO_PATHCONV=1
```

Это отключает преобразование контейнерных путей `/opt/kafka/...` в пути
Windows. Все команды выполняйте из корня своей копии этого репозитория.

## Термины, которые понадобятся в опытах

| Термин | Значение в этом стенде |
|---|---|
| Broker | Сервер, который принимает, хранит и отдаёт записи |
| Controller / KRaft | Управление metadata кластера без ZooKeeper |
| Record | Событие: key, value, timestamp и необязательные headers |
| Producer | Клиент, записывающий события |
| Consumer | Клиент, читающий события |
| Topic | Именованный поток событий |
| Partition | Независимый упорядоченный журнал внутри topic |
| Segment | Файл с частью журнала partition |
| Key | Ключ события; у нас ID заявки, используемый для выбора partition |
| Offset | Позиция записи внутри конкретной partition |
| Consumer group | Независимый подписчик, работа которого делится между consumers |
| Commit | Сохранение позиции продолжения чтения группы для partition |
| Lag | Разница между конечной позицией журнала и committed offset группы |
| Rebalance | Перераспределение partitions при изменении состава группы |
| Retention | Условия удаления старых сегментов |
| Replication factor | Количество копий partition на brokers |
| Leader / ISR | Принимающая запись replica и набор синхронизированных replicas |

Offset не является глобальным ID: `partition=0, offset=5` и
`partition=1, offset=5` — разные позиции. Committed offset обозначает
**следующую** позицию чтения. Он не доказывает успешность бизнес-операции:
consumer мог сохранить позицию, но не выполнить нужное действие в БД.

Порядок гарантируется внутри partition, а не между всеми событиями topic.
Одинаковые keys попадают в одну partition при стандартном распределении
и неизменном числе partitions. Увеличение их числа может изменить
распределение keys. [Основные понятия Kafka](https://kafka.apache.org/41/getting-started/introduction/).

## Подготовка терминалов и запуск

В **каждом** окне терминала выполните:

```bash
export COMPOSE_FILE=deploy/kafka-lab/compose.yaml
export COMPOSE_PROJECT_NAME=devops-practice-kafka-06
export KAFKA_HEAP_OPTS='-Xms64m -Xmx128m'
```

Только в первом окне создайте суффикс для групп этой попытки:

```bash
KAFKA_LAB_RUN="practice06-$(date +%s)"
export KAFKA_LAB_RUN
printf 'export KAFKA_LAB_RUN=%s\n' "$KAFKA_LAB_RUN"
```

Скопируйте напечатанную строку `export` в остальные окна. Значение должно
совпадать: иначе вы создадите независимые группы вместо участников одной
группы. При новом прохождении создайте новый суффикс. Topics и volume при
этом остаются прежними — это не сброс данных.

Проверьте конфигурацию и запустите стенд:

```bash
docker compose config
docker compose up -d --wait --wait-timeout 180
docker compose ps
docker compose logs --tail=60 kafka
```

Ожидается один healthy сервис `kafka`. Healthcheck обращается к Kafka API,
а не просто проверяет наличие процесса. Перед долгими опытами проверьте
утилиту ограничения времени внутри контейнера:

```bash
docker compose exec kafka timeout --help
```

Разберите настройки [compose.yaml](../../artifacts/common/deploy/kafka-lab/compose.yaml):

- Broker и controller работают в одном контейнере: это учебный KRaft-стенд.
- `listeners` задаёт адреса прослушивания; `advertised.listeners` — адрес,
  который Kafka сообщает клиенту; `bootstrap.servers` — первоначальный
  адрес подключения клиента. Здесь CLI работает **внутри** контейнера,
  поэтому `localhost:9092` подходит. С ПК этот адрес к Kafka не ведёт:
  порты не опубликованы.
- `kafka-data` — отдельный volume этого Compose project. `log.dirs` явно
  указывает в `/var/lib/kafka/data`; там сохраняются data и KRaft metadata.
- `node.id` и `CLUSTER_ID` сохраняются при пересоздании. ID кластера —
  не пароль; менять его поверх существующих данных не нужно.
- Автоматическое создание topics отключено: опечатка не должна создавать
  новый topic незаметно для инженера.
- Лимит контейнера — 2 GiB RAM, heap broker — 256 MiB. Это настройки,
  не обещание фактического расхода. `exec -e KAFKA_HEAP_OPTS` передаёт CLI
  меньший heap из вашего терминала; broker сохраняет свой.

Одновременно запускайте не больше четырёх consumers: каждый CLI — отдельная
JVM в том же лимите контейнера. Для измерения используйте
`docker stats --no-stream`, ориентируясь на контейнер, показанный
`docker compose ps`. PLAINTEXT и отсутствие авторизации допустимы только
в этом изолированном локальном стенде.

Обычная остановка сохраняет данные:

```bash
docker compose down
```

После неё для продолжения нужен `docker compose up -d --wait --wait-timeout 180`.
Во время проверки распределения останавливайте **consumer через Ctrl+C**,
а не сам broker.

## Опыт 1. Topics, первая запись и повторное чтение

Создайте два topics с хранением по времени на сутки:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --create \
  --topic practice06.requests.single --partitions 1 --replication-factor 1 \
  --config cleanup.policy=delete --config retention.ms=86400000 \
  --config retention.bytes=-1

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --create \
  --topic practice06.requests.created --partitions 3 --replication-factor 1 \
  --config cleanup.policy=delete --config retention.ms=86400000 \
  --config retention.bytes=-1

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --describe --topic practice06.requests.single

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --describe --topic practice06.requests.created
```

При повторном прохождении ошибка `TopicExistsException` означает, что topic
уже существует. Проверьте его partitions, replication factor и configs через
`--describe`; не считайте ошибку успешным созданием и не удаляйте topic
автоматически. При несовпадении настроек остановитесь и разберите состояние.

Отправьте девять событий. Строка fixture состоит из `key|JSON`; `parse.key`
и `key.separator` задают разбор. `--sync` ждёт результат каждой отправки,
`acks=all` — подтверждение ISR. Это не означает несколько replicas:
broker у нас один.

```bash
docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created \
  --sync --property parse.key=true --property 'key.separator=|' \
  --producer-property acks=all \
  < deploy/kafka-lab/fixtures/requests.events

docker compose exec -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created \
  --group "${KAFKA_LAB_RUN}-inspect" --from-beginning \
  --consumer-property enable.auto.commit=false \
  --property print.key=true --property print.partition=true \
  --property print.offset=true --property print.timestamp=true
```

Consumer читает доступный журнал и затем ждёт новых сообщений. Остановите
его через Ctrl+C. Auto-commit здесь намеренно отключён: это просмотр,
а не опыт с продолжением чтения. Повторите команду с группой
`${KAFKA_LAB_RUN}-inspect-again`: те же сохранённые события доступны другой
группе. При повторном запуске fixture добавляет записи, а не заменяет их.

Посмотрите диапазоны offsets:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-get-offsets.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created --time -2

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-get-offsets.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created --time -1
```

Формат — `topic:partition:offset`. `-2` показывает начало доступного журнала,
`-1` — конец, следующую позицию записи. Зафиксируйте их для каждой partition.
Объясните, почему чтение первой группой не уменьшило доступный журнал.

## Опыт 2. Keys и порядок

Отправьте те же события в single-partition topic и прочитайте их:

```bash
docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.single \
  --sync --property parse.key=true --property 'key.separator=|' \
  --producer-property acks=all \
  < deploy/kafka-lab/fixtures/requests.events

docker compose exec -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.single \
  --group "${KAFKA_LAB_RUN}-single" --from-beginning \
  --consumer-property enable.auto.commit=false \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

Сравните с опытом 1, затем остановите consumer. Для `req-001`, `req-002`
и `req-003` сопоставьте key, partition, offsets и `sequence=1,2,3`.
В topic с одной partition виден общий порядок добавления этой партии.
В topic с тремя partitions порядок между их выводами не гарантирован.
Три keys могут занять меньше трёх partitions: разные keys могут совпасть
по выбранной partition.

Ответьте: почему нельзя сортировать все события topic только по offset?
Что изменится для распределения keys при увеличении числа partitions?

## Опыт 3. Одна группа, несколько consumers и независимый подписчик

Остановите consumers просмотра. В первом окне задайте `worker-1`, во втором
`worker-2`; остальные переменные уже подготовлены и `KAFKA_LAB_RUN` одинаков:

```bash
export KAFKA_LAB_CLIENT=worker-1
```

Во втором окне значение задаётся так:

```bash
export KAFKA_LAB_CLIENT=worker-2
```

В каждом окне запустите одну и ту же команду:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created \
  --group "${KAFKA_LAB_RUN}-workers" --from-beginning \
  --consumer-property client.id="$KAFKA_LAB_CLIENT" \
  --consumer-property enable.auto.commit=true \
  --consumer-property auto.commit.interval.ms=1000 \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

В отдельном окне проверьте участников и назначения:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe \
  --group "${KAFKA_LAB_RUN}-workers" --members --verbose
```

Добавьте ещё два consumers той же командой, с `KAFKA_LAB_CLIENT=worker-3`
и `worker-4` в своих окнах. Дождитесь стабильного назначения. При трёх
partitions один из четырёх участников будет без назначения. Это проверяется
по assignment, а не по отсутствию вывода: назначенная partition может быть
пустой или уже прочитанной.

Остановите одного участника, повторите `--members --verbose` и покажите
rebalance. Имена владельцев могут отличаться между запусками. Делится
владение partitions, не равное количество сообщений по кругу.

Оставьте один worker; остальные остановите. Запустите независимого подписчика:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created \
  --group "${KAFKA_LAB_RUN}-analytics" \
  --consumer-property auto.offset.reset=earliest \
  --consumer-property enable.auto.commit=true \
  --consumer-property auto.commit.interval.ms=1000 \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

Analytics читает сохранённые события независимо от workers. Сравните offsets
двух групп командами `--describe --group`, затем остановите analytics.
Объясните отличие «ещё один worker обработчика» от «новый подписчик».
Один владелец partition в группе не исключает повторную обработку после сбоя.

## Опыт 4. Commit, остановка и lag

Пока оставшийся worker работает, дождитесь видимого commit:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe --group "${KAFKA_LAB_RUN}-workers"
```

Для каждой partition запишите `CURRENT-OFFSET`, `LOG-END-OFFSET` и `LAG`.
Там, где записи есть, commit должен быть числом, не `-`; после чтения партии
lag становится нулевым. Auto-commit с интервалом 1000 ms не гарантирует,
что закрытый сразу после вывода сообщения consumer уже сохранил позицию.
Доказательство — команда `--describe`, не секундомер и не `--max-messages`.

Остановите **всех** workers группы и проверьте её состояние:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe \
  --group "${KAFKA_LAB_RUN}-workers" --state
```

Дождитесь `Empty` и `#MEMBERS=0`. Затем отправьте три новые заявки:

```bash
docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created \
  --sync --property parse.key=true --property 'key.separator=|' \
  --producer-property acks=all \
  < deploy/kafka-lab/fixtures/followup.events
```

Повторите `--describe`: committed offsets остались прежними, конец журнала
вырос, появился lag. Запустите одного worker прежней командой и тем же
`group.id`. Он продолжает с сохранённой позиции, а не читает весь журнал
заново. Проверьте новый commit и уменьшение lag.

В опыте console consumer только выводит строки. Auto-commit выбран для
наглядности. Для реального изменения БД нужно отдельно выбирать момент
commit и обеспечивать идемпотентность, если сообщение придёт повторно.

## Опыт 5. Earliest, latest и управляемый replay

Остановите workers. В двух подготовленных окнах запустите новые группы:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created \
  --group "${KAFKA_LAB_RUN}-fresh-earliest" \
  --consumer-property auto.offset.reset=earliest \
  --consumer-property enable.auto.commit=false \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

Во втором окне:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created \
  --group "${KAFKA_LAB_RUN}-fresh-latest" \
  --consumer-property auto.offset.reset=latest \
  --consumer-property enable.auto.commit=true \
  --consumer-property auto.commit.interval.ms=1000 \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

У новой группы earliest читает с начала **доступного** журнала, latest —
с его конца при выборе начальной позиции. Проверьте назначения latest:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe \
  --group "${KAFKA_LAB_RUN}-fresh-latest" --members --verbose

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe \
  --group "${KAFKA_LAB_RUN}-fresh-latest"
```

Дождитесь числового `CURRENT-OFFSET`, равного `LOG-END-OFFSET`, для каждой
partition. Повторяйте вторую команду, пока это условие не выполнено;
назначения без commit недостаточно. Другие producers пока не запускайте.
Теперь latest выбрал начальную позицию: отправьте marker и сравните вывод:

```bash
printf 'req-replay|{"request_id":"req-replay","event":"replay.marker","sequence":1}\n' | \
  docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
    /opt/kafka/bin/kafka-console-producer.sh \
    --bootstrap-server localhost:9092 --topic practice06.requests.created \
    --sync --property parse.key=true --property 'key.separator=|' \
    --producer-property acks=all
```

Latest не должен показывать старую партию; новый marker должны получить
обе группы. Затем остановите обе группы.

`auto.offset.reset` действует, когда позиции ещё нет или она недоступна.
Если у workers есть допустимый commit, `earliest` и `--from-beginning`
не перематывают эту группу назад. Для replay остановленной workers-группы
проверьте `Empty`/ноль участников и сначала выполните dry-run:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe \
  --group "${KAFKA_LAB_RUN}-workers" --state

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --group "${KAFKA_LAB_RUN}-workers" \
  --topic practice06.requests.created --reset-offsets --to-earliest --dry-run
```

Проверьте конкретные topic, group и предложенные offsets. Только если группа
неактивна и результат dry-run подходит, примените сброс:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --group "${KAFKA_LAB_RUN}-workers" \
  --topic practice06.requests.created --reset-offsets --to-earliest --execute
```

Снова запустите worker и покажите повторное чтение сохранённой партии.
Не используйте `--all-groups` или `--all-topics`. Reset не возвращает
удалённые события и может повторно выполнить внешние действия обработчика.
[Настройки consumer](https://kafka.apache.org/41/configuration/consumer-configs/).

## Опыт 6. Retention по времени

Остановите consumers предыдущих опытов. Создайте отдельный topic:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --create \
  --topic practice06.retention.time --partitions 1 --replication-factor 1 \
  --config cleanup.policy=delete --config retention.ms=60000 \
  --config retention.bytes=-1 --config segment.ms=10000 \
  --config segment.bytes=1048576

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-configs.sh \
  --bootstrap-server localhost:9092 --entity-type topics \
  --entity-name practice06.retention.time --describe
```

При существующем topic сначала проверьте настройки. Не используйте topic
заявок для этого опыта. `retention.ms=60000` — 60 секунд; `segment.ms=10000`
помогает закрывать небольшие сегменты. Broker проверяет retention каждые
5 секунд. Удаление выполняется сегментами и не зависит от commit consumers.

Перед отправкой marker сохраните конец журнала — будущий offset этого
события. Producer этого topic на время измерения только один:

```bash
KAFKA_LAB_TIME_MARKER_OFFSET="$(docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
  timeout 10s /opt/kafka/bin/kafka-get-offsets.sh \
  --bootstrap-server localhost:9092 --topic practice06.retention.time --time -1 | \
  cut -d: -f3)"
export KAFKA_LAB_TIME_MARKER_OFFSET
printf 'Offset первого marker: %s\n' "$KAFKA_LAB_TIME_MARKER_OFFSET"

printf 'req-time|{"request_id":"req-time","event":"retention.first","sequence":1}\n' | \
  docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
    /opt/kafka/bin/kafka-console-producer.sh \
    --bootstrap-server localhost:9092 --topic practice06.retention.time \
    --sync --property parse.key=true --property 'key.separator=|' \
    --producer-property acks=all

sleep 11

printf 'req-time|{"request_id":"req-time","event":"retention.next","sequence":2}\n' | \
  docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
    /opt/kafka/bin/kafka-console-producer.sh \
    --bootstrap-server localhost:9092 --topic practice06.retention.time \
    --sync --property parse.key=true --property 'key.separator=|' \
    --producer-property acks=all
```

Проверьте оба marker чтением новой группой, затем остановите consumer:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.retention.time \
  --group "${KAFKA_LAB_RUN}-time-inspect" \
  --consumer-property auto.offset.reset=earliest \
  --consumer-property enable.auto.commit=false \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

Значение переменной должно быть числом; пустой вывод — ошибка
команды, а не offset 0. Если другие producers успели записать события,
повторите измерение без них: старый конец не доказывает позицию вашего marker.

Теперь дождитесь фактического удаления первой записи. Block запускает
отдельный `sh` **внутри контейнера**, не меняет режим вашего терминала и не
создаёт постоянного скрипта. Общий deadline — 180 секунд. `run_bounded`
ограничивает каждый CLI и паузу оставшимся временем, не больше 10 секунд
на один вызов. `timeout -s KILL` принудительно завершает одноразовый процесс:
shutdown hook JVM не продлевает ожидание. Внешний timeout дополнительно
ограничивает сам shell:

```bash
docker compose exec -T -e KAFKA_HEAP_OPTS -e KAFKA_LAB_TIME_MARKER_OFFSET kafka \
  timeout -s KILL 180s sh -s <<'SH'
set -eu
deadline=$(( $(date +%s) + 180 ))
run_bounded() {
  remaining=$(( deadline - $(date +%s) ))
  if [ "$remaining" -le 0 ]; then
    printf '%s\n' 'Общий бюджет опыта исчерпан' >&2
    return 1
  fi
  if [ "$remaining" -gt 10 ]; then
    remaining=10
  fi
  timeout -s KILL "$remaining" "$@"
}
case "${KAFKA_LAB_TIME_MARKER_OFFSET:-}" in
  ''|*[!0-9]*) printf '%s\n' 'Offset контрольной записи не определён' >&2; exit 1 ;;
esac
while :; do
  result=$(run_bounded /opt/kafka/bin/kafka-get-offsets.sh \
    --bootstrap-server localhost:9092 --topic practice06.retention.time --time -2)
  start=$(printf '%s\n' "$result" | cut -d: -f3)
  case "$start" in
    ''|*[!0-9]*) printf '%s\n' 'Не получен начальный offset' >&2; exit 1 ;;
  esac
  printf 'Marker=%s, начало доступного журнала=%s\n' \
    "$KAFKA_LAB_TIME_MARKER_OFFSET" "$start"
  if [ "$start" -gt "$KAFKA_LAB_TIME_MARKER_OFFSET" ]; then
    printf '%s\n' 'Первая контрольная запись больше недоступна'
    break
  fi
  run_bounded sleep 5
done
SH
```

Ненулевой exit или срабатывание timeout — **опыт не подтверждён**. Сохраните
ошибку, настройки и логи, а не добавляйте `|| true`. После успеха проверьте
диапазон `--time -2` / `--time -1` и повторное чтение новой группой.
Старого marker уже нет. Если задержались и истёк срок второй записи,
она также может исчезнуть — это не нарушение retention.

Ответьте: почему запись не обязана исчезнуть ровно через 60 секунд?
Kafka учитывает timestamps записей при удалении сегментов; закрытие сегмента
и фоновая проверка влияют на наблюдение. Timestamp внутри JSON value —
обычные данные, не Kafka record timestamp. Недоступность для чтения и
физическое удаление файла отличаются: `file.delete.delay.ms` добавляет
задержку удаления с диска. Не удаляйте файлы Kafka вручную.

## Опыт 7. Retention по размеру

Создайте ещё один отдельный topic, теперь без ограничения по времени:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --create \
  --topic practice06.retention.size --partitions 1 --replication-factor 1 \
  --config cleanup.policy=delete --config retention.ms=-1 \
  --config retention.bytes=3145728 --config segment.bytes=1048576

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-configs.sh \
  --bootstrap-server localhost:9092 --entity-type topics \
  --entity-name practice06.retention.size --describe
```

Здесь порог хранения — 3 MiB **на partition**, размер сегмента — 1 MiB.
Зафиксируйте начальный offset до подачи данных:

```bash
KAFKA_LAB_SIZE_START="$(docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
  timeout 10s /opt/kafka/bin/kafka-get-offsets.sh \
  --bootstrap-server localhost:9092 --topic practice06.retention.size --time -2 | \
  cut -d: -f3)"
export KAFKA_LAB_SIZE_START
printf 'Начало до отправки: %s\n' "$KAFKA_LAB_SIZE_START"

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-producer-perf-test.sh \
  --topic practice06.retention.size --num-records 8192 \
  --record-size 1024 --throughput 1024 \
  --producer-props bootstrap.servers=localhost:9092 compression.type=none acks=all
```

Это 8 MiB входных данных без сжатия, не benchmark. Не увеличивайте поток
сверх 16 MiB за запуск: для удаления сегментов достаточно этого опыта.
При повторном прохождении исходный offset может быть ненулевым.

Ожидайте рост начального offset, проверяя каждую partition самостоятельно:

```bash
docker compose exec -T -e KAFKA_HEAP_OPTS -e KAFKA_LAB_SIZE_START kafka \
  timeout -s KILL 180s sh -s <<'SH'
set -eu
deadline=$(( $(date +%s) + 180 ))
run_bounded() {
  remaining=$(( deadline - $(date +%s) ))
  if [ "$remaining" -le 0 ]; then
    printf '%s\n' 'Общий бюджет опыта исчерпан' >&2
    return 1
  fi
  if [ "$remaining" -gt 10 ]; then
    remaining=10
  fi
  timeout -s KILL "$remaining" "$@"
}
case "${KAFKA_LAB_SIZE_START:-}" in
  ''|*[!0-9]*) printf '%s\n' 'Исходный offset не определён' >&2; exit 1 ;;
esac
while :; do
  result=$(run_bounded /opt/kafka/bin/kafka-get-offsets.sh \
    --bootstrap-server localhost:9092 --topic practice06.retention.size --time -2)
  start=$(printf '%s\n' "$result" | cut -d: -f3)
  case "$start" in
    ''|*[!0-9]*) printf '%s\n' 'Не получен начальный offset' >&2; exit 1 ;;
  esac
  printf 'Начало до опыта=%s, начало сейчас=%s\n' "$KAFKA_LAB_SIZE_START" "$start"
  if [ "$start" -gt "$KAFKA_LAB_SIZE_START" ]; then
    printf '%s\n' 'Удаление старых записей по размеру подтверждено'
    break
  fi
  run_bounded sleep 5
done
SH
```

Если topic уже содержит старые данные, убедитесь, что рост связан с новой
партией: зафиксируйте диапазон перед/после отправки и отсутствие других
producers. Прочитайте оставшуюся часть с начала новой группой:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-get-offsets.sh \
  --bootstrap-server localhost:9092 --topic practice06.retention.size --time -1

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.retention.size \
  --group "${KAFKA_LAB_RUN}-size-inspect" --from-beginning \
  --consumer-property enable.auto.commit=false \
  --property print.partition=true --property print.offset=true
```

Остановите consumer после наблюдения. Новые записи доступны, старое начало
журнала — уже нет. Размер входных данных, размер Kafka log и объём volume
различаются. Retention удаляет целые сегменты асинхронно; это не строгая
дисковая квота. Для оценки места учитывайте число partitions, replicas,
индексы и активный сегмент. Если включены оба ограничения `delete`,
сегмент может быть удалён по времени **или** размеру.
[Настройки topic](https://kafka.apache.org/41/configuration/topic-configs/).

## Опыт 8. Consumer отстал дальше окна хранения

Проводите этот опыт непрерывно: короткий retention действует, пока вы
читаете инструкции. Сначала подготовьте два окна с командами consumers
ниже и третье для диагностики. Остановите consumers остальных опытов.

Используется прежний `practice06.retention.time`. Отправьте свежую
контрольную партию:

```bash
docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
  /opt/kafka/bin/kafka-console-producer.sh \
  --bootstrap-server localhost:9092 --topic practice06.retention.time \
  --sync --property parse.key=true --property 'key.separator=|' \
  --producer-property acks=all \
  < deploy/kafka-lab/fixtures/requests.events
```

В первом окне задайте:

```bash
export KAFKA_LAB_EXPIRED_GROUP="${KAFKA_LAB_RUN}-expired-earliest"
```

Во втором:

```bash
export KAFKA_LAB_EXPIRED_GROUP="${KAFKA_LAB_RUN}-expired-latest"
```

В **обоих** окнах сначала запустите чтение с earliest:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.retention.time \
  --group "$KAFKA_LAB_EXPIRED_GROUP" \
  --consumer-property auto.offset.reset=earliest \
  --consumer-property enable.auto.commit=true \
  --consumer-property auto.commit.interval.ms=1000 \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

В третьем окне дождитесь одинакового числового `CURRENT-OFFSET` обеих групп:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe \
  --group "${KAFKA_LAB_RUN}-expired-earliest"

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe \
  --group "${KAFKA_LAB_RUN}-expired-latest"
```

Остановите оба consumers и проверьте `--state` для каждой группы: `Empty`,
ноль участников. Теперь создайте новые события, которых эти группы ещё
не читали. Следующий block сам получает commits, проверяет совпадение
и поддерживает свежую партию, пока старые сегменты удаляются:

```bash
docker compose exec -T -e KAFKA_HEAP_OPTS -e KAFKA_LAB_RUN kafka \
  timeout -s KILL 180s sh -s <<'SH'
set -eu
deadline=$(( $(date +%s) + 180 ))
run_bounded() {
  remaining=$(( deadline - $(date +%s) ))
  if [ "$remaining" -le 0 ]; then
    printf '%s\n' 'Общий бюджет опыта исчерпан' >&2
    return 1
  fi
  if [ "$remaining" -gt 10 ]; then
    remaining=10
  fi
  timeout -s KILL "$remaining" "$@"
}
case "${KAFKA_LAB_RUN:-}" in
  practice06-[0-9]*) ;;
  *) printf '%s\n' 'Не задан суффикс этой попытки' >&2; exit 1 ;;
esac
earliest_group="${KAFKA_LAB_RUN}-expired-earliest"
latest_group="${KAFKA_LAB_RUN}-expired-latest"
earliest_description=$(run_bounded /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe --group "$earliest_group")
latest_description=$(run_bounded /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe --group "$latest_group")
earliest_commit=$(printf '%s\n' "$earliest_description" | \
  awk -v g="$earliest_group" \
    '$1 == g && $2 == "practice06.retention.time" && $3 == "0" { print $4 }')
latest_commit=$(printf '%s\n' "$latest_description" | \
  awk -v g="$latest_group" \
    '$1 == g && $2 == "practice06.retention.time" && $3 == "0" { print $4 }')
for position in "$earliest_commit" "$latest_commit"; do
  case "$position" in
    ''|*[!0-9]*) printf '%s\n' 'Не подтверждён числовой commit' >&2; exit 1 ;;
  esac
done
if [ "$earliest_commit" != "$latest_commit" ]; then
  printf '%s\n' 'Commits групп различаются: повторите подготовку' >&2
  exit 1
fi
sequence=1
while :; do
  printf 'req-retention|{"request_id":"req-retention","event":"retention.marker","sequence":%s}\n' \
    "$sequence" | \
    run_bounded /opt/kafka/bin/kafka-console-producer.sh \
      --bootstrap-server localhost:9092 --topic practice06.retention.time \
      --sync --property parse.key=true --property 'key.separator=|' \
      --producer-property acks=all
  start_result=$(run_bounded /opt/kafka/bin/kafka-get-offsets.sh \
    --bootstrap-server localhost:9092 --topic practice06.retention.time --time -2)
  end_result=$(run_bounded /opt/kafka/bin/kafka-get-offsets.sh \
    --bootstrap-server localhost:9092 --topic practice06.retention.time --time -1)
  start=$(printf '%s\n' "$start_result" | cut -d: -f3)
  end=$(printf '%s\n' "$end_result" | cut -d: -f3)
  for position in "$start" "$end"; do
    case "$position" in
      ''|*[!0-9]*) printf '%s\n' 'Не получен диапазон offsets' >&2; exit 1 ;;
    esac
  done
  printf 'Commit=%s, earliest=%s, latest=%s\n' "$earliest_commit" "$start" "$end"
  if [ "$earliest_commit" -lt "$start" ] && [ "$start" -lt "$end" ]; then
    printf '%s\n' 'Commit уже недоступен, но свежие непрочитанные записи ещё есть'
    break
  fi
  sequence=$(( sequence + 1 ))
  run_bounded sleep 10
done
SH
```

Сохраните три числа `committed < earliest < latest`. Если условие не
получено или CLI завершился ошибкой, опыт не подтверждён. Активный consumer
во время ожидания может менять commit, поэтому группы должны быть остановлены.

Сразу возобновите **первую** группу командой первоначального чтения с
`earliest`. **Вторую** возобновите с latest, без `--from-beginning`:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.retention.time \
  --group "${KAFKA_LAB_RUN}-expired-latest" \
  --consumer-property auto.offset.reset=latest \
  --consumer-property enable.auto.commit=true \
  --consumer-property auto.commit.interval.ms=1000 \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

Earliest читает оставшуюся партию; latest пропускает её. Перед новой отправкой
проверьте назначение обеих групп, а для latest — числовой commit на текущем
конце журнала через `--describe`: это подтверждает завершение reset, не
только membership. Затем отправьте ещё один marker:

```bash
printf 'req-after-retention|{"request_id":"req-after-retention","event":"retention.after","sequence":1}\n' | \
  docker compose exec -T -e KAFKA_HEAP_OPTS kafka \
    /opt/kafka/bin/kafka-console-producer.sh \
    --bootstrap-server localhost:9092 --topic practice06.retention.time \
    --sync --property parse.key=true --property 'key.separator=|' \
    --producer-property acks=all
```

Обе группы читают новый marker, но не возвращают удалённую партию.
Если задержались дольше окна хранения, сначала снова проверьте
`earliest < latest`: без оставшейся партии сравнение не доказывает разницу
политик. Повторите подготовку с новыми именами групп и свежими данными.
Остановите consumers после опыта.

`auto.offset.reset=none` вместо автоматического выбора позиции приводит
к ошибке при отсутствующем/недоступном offset. Это позволяет явно обработать
потерю окна чтения. `earliest` — начало оставшихся данных, не обещание
восстановить все события за историю topic. Срок жизни committed offsets
группы — отдельная настройка, не срок жизни событий topic.

## Опыт 9. Пересоздание контейнера без потери volume

Используйте основной `practice06.requests.created` с хранением на сутки,
не короткий time topic. Остановите все остальные consumers. Запустите
отдельную группу, дождитесь чтения текущих данных и commit:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created \
  --group "${KAFKA_LAB_RUN}-persistent" --from-beginning \
  --consumer-property enable.auto.commit=true \
  --consumer-property auto.commit.interval.ms=1000 \
  --property print.key=true --property print.partition=true \
  --property print.offset=true
```

В другом окне сохраните `--describe` этой группы, диапазоны offsets topic
и имя volume из `docker compose config`. Затем остановите consumer и
пересоздайте контейнер:

```bash
docker compose down
docker compose up -d --wait --wait-timeout 180
docker compose ps
```

Не добавляйте `-v`. До возобновления consumer проверьте прежние topics,
начальные/конечные offsets и commit:

```bash
docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --describe --topic practice06.requests.created

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-get-offsets.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created --time -2

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-get-offsets.sh \
  --bootstrap-server localhost:9092 --topic practice06.requests.created --time -1

docker compose exec -e KAFKA_HEAP_OPTS kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe --group "${KAFKA_LAB_RUN}-persistent"
```

Новой группой чтения подтвердите доступность прежних контрольных событий.
Возобновите persistent с тем же именем, отправьте новую заявку через
`followup.events` и покажите продолжение с сохранённой позиции.
Повторение fixture добавляет новые записи с прежними keys — это не
автоматическая дедупликация.

Volume обеспечивает сохранение после этого корректного пересоздания,
но не HA и не backup. Потеря диска/ПК затрагивает все данные стенда.
Если прервать практику на сутки, retention основных topics тоже может
удалить старые события: сохранение volume не отключает retention.

## Зачем применять Kafka и где она избыточна

Свяжите выводы с рабочими ситуациями:

- Обработчик заявок, уведомления и аналитика должны читать события
  независимо — отдельные consumer groups, опыт 3.
- Обработчик временно недоступен или работает медленнее producer — журнал
  накапливает события, растёт lag, опыт 4. Догнать можно лишь в пределах
  доступного окна хранения, опыт 8.
- После исправления ошибки нужно пересчитать результат — replay, опыт 5.
  Внешние эффекты должны выдерживать повторную обработку.
- Нужен поток событий для аналитики или интеграции систем — один поток
  читается несколькими независимыми подписчиками.
- Растёт нагрузка — partitions дают параллелизм, но четвёртый обычный
  consumer не ускоряет чтение topic с тремя partitions, опыт 3.

Компромиссы: дополнительные ресурсы и эксплуатация, eventual consistency,
выбор retention, возможные повторы и необходимость наблюдать lag. Kafka не
делает SQL-запись и публикацию события атомарными автоматически. Exactly-once
для произвольного изменения внешней БД или отправки уведомления из самого
факта использования Kafka не следует.

Для маленького синхронного CRUD без независимых подписчиков, replay и
потоковой нагрузки Kafka может быть лишней. Для простой очереди задач
с подтверждениями, retry и маршрутизацией стоит сравнить её с обычным
message broker. Kafka не заменяет PostgreSQL как БД приложения.

`cleanup.policy=delete` удаляет старые сегменты по retention. `compact`
сохраняет последнее значение по ключу при фоновой очистке: не немедленно
и не с гарантией отсутствия старых значений при чтении. Для удаления ключа
используются tombstones с собственными правилами хранения. В этой практике
compaction только объясняется, не настраивается.

В production обсуждают несколько brokers и controller quorum, replicas,
ISR, `min.insync.replicas`, доступ, monitoring и восстановление. У нас
replication factor `1`: `acks=all` получает подтверждение единственной ISR
replica, но не превращает её в три копии. Broker/controller в одном
контейнере выбран для обучения, не для критичного deployment.
[KRaft](https://kafka.apache.org/41/operations/kraft/).

## Что предоставить на ревью

Приложите ссылку на свою ветку/MR в публичном GitLab и короткий Markdown
отчёт. Для каждого опыта укажите настройки, вывод команд до/после,
наблюдение и его объяснение. Не нужны огромные dumps или только скриншот
запущенного контейнера. Настоящие персональные данные не используйте.

Критерии приёмки:

1. Kafka healthy, вы объяснили Compose, адреса подключения, heap и volume.
2. Созданы topics с одной и тремя partitions; сохранены configs и события.
3. По keys и offsets подтверждён порядок внутри partition, без общего
   порядка между partitions.
4. Показаны назначения двух/трёх/четырёх consumers, rebalance и независимая
   группа. Простой доказан assignment, не только отсутствием вывода.
5. Показаны commit, рост/снижение lag, продолжение чтения и управляемый replay.
6. По времени и размеру подтверждён рост начального доступного offset;
   объяснена сегментная очистка и её задержки.
7. Для отставших групп зафиксировано `committed < earliest < latest`;
   объяснены earliest/latest/none и невосстановимость удалённой партии.
8. События, metadata и commit сохранились после пересоздания без удаления
   volume. Вы отличаете этот результат от HA и backup.
9. Обоснованы use cases Kafka и случаи, в которых её выбирать не стоит.

## Если результат не совпал с ожиданием

| Наблюдение | Что проверить |
|---|---|
| Kafka не healthy | `docker compose ps`, `docker compose logs --tail=100 kafka`; image, startup, доступность Docker |
| CLI завершается или JVM сообщает OOM | Heap переменной терминала, `exec -e`, число CLI-процессов, `docker stats --no-stream` |
| Получились разные группы вместо распределения работы | Сравнить `printf '%s\n' "$KAFKA_LAB_RUN"` во всех окнах |
| Consumer не показывает старые записи при earliest | Существующий commit группы и доступное начало журнала; earliest не является командой replay |
| CURRENT-OFFSET равен `-` | Подтвердить auto-commit и дождаться commit; вывод записи не доказывает его |
| Reset отказался применяться | `--state` и `--members --verbose`; группа должна быть неактивной |
| Retention не подтверждён за 180 секунд | `kafka-configs.sh --describe`, `segment.ms`, `segment.bytes`, timestamps и логи broker; не удалять data вручную |
| Earliest и latest ничего не показали | Сравнить начальный/конечный offsets: все события могли истечь; повторить со свежими данными |
| Новое событие не появилось у latest | Подтвердить числовой commit на конце журнала до отправки и успешный producer |

Для диагностики OOM только своего контейнера:

```bash
docker inspect --format '{{.State.OOMKilled}}' "$(docker compose ps -a -q kafka)"
```

Отсутствующий Docker daemon не исправляется командами Kafka. Сначала нужен
работающий локальный Docker. Ошибка CLI, ненулевой exit или timeout остаются
ошибкой опыта — не скрывайте их успешным сообщением.

## Остановка и необязательное удаление стенда

Для перерыва используйте `docker compose down` без `-v`. Если практика
закончена и данные точно больше не нужны, проверьте целевой project:

```bash
printf 'Compose file: %s\nProject: %s\n' "$COMPOSE_FILE" "$COMPOSE_PROJECT_NAME"
docker compose config
```

Должны быть `deploy/kafka-lab/compose.yaml`, `devops-practice-kafka-06`,
один `kafka` и его собственный volume. Только после этой проверки можно
выполнить необратимое удаление **данных этого стенда**:

```bash
docker compose -p devops-practice-kafka-06 -f deploy/kafka-lab/compose.yaml down -v
```

Это не часть проверки сохранности. `docker system prune`, очистка чужих
volumes и массовое удаление topics/groups для прохождения модуля не нужны.
