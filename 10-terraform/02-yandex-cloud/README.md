# Terraform и Yandex Cloud

Уровень: **углублённый, необязательный**.

Исторические номера внутри технических имён стендов сохраняются для совместимости.

## Подготовка рабочей копии

Из корня курса:

```bash
bash artifacts/init.sh 10 02
cd 10-terraform/02-yandex-cloud/workspace
```

Все команды задания выполняй из этого рабочего каталога.

Уровень: Hard. Один обязательный маршрут. Можно выполнить отдельно от
предыдущих модулей: приложение и готовый Ansible уже предоставлены.
На ПК с 16 ГБ RAM работают только инструменты; две VM находятся в облаке.

## Задача от руководителя

> У нас есть сервис заявок с PostgreSQL, Redis и Kafka. Подготовьте воспроизводимый
> стенд в Yandex Cloud: отдельную сеть, приватный сервер приложения и входной
> Nginx. Ресурсами управляйте через Terraform и GitLab.com.
>
> Изменения должны проходить MR, lint и security checks. Создание, изменение и
> удаление применяйте вручную по просмотренному saved plan. State храните в
> GitLab, не в Git и не только на своём ноутбуке.
>
> Разрешите загрузку пакетов и образов на время установки, затем запретите новые
> исходящие соединения VM в интернет. SSH через входной сервер и сквозной поток
> заявок должны остаться рабочими.
>
> Покажите drift, import, рефакторинг без пересоздания и замену входной VM.
> Уложитесь в 500 ₽ на весь короткий стенд. После сдачи удалите ресурсы и
> подтвердите cleanup по фактическому облаку.

## Что выдаётся

- Приложение API/worker и Compose с PostgreSQL, Redis, Kafka.
- Готовые roles `reference/module-07-ansible/roles/` и adapter
  `deploy/terraform-lab/ansible/`; это входной материал, а не задание на Ansible.
- Генератор inventory, проверки доступа и cleanup в `scripts/terraform-lab/`.
- [Runbook подготовки](../../artifacts/common/docs/runbooks/08-terraform-bootstrap.md), закреплённые
  инструменты и validator.
- Существующий runner курса с Kubernetes executor. Для отдельного маршрута
  заранее согласуйте runner ментора; доступная SaaS-квота не гарантируется.

Вы реализуете `terraform/`, два модуля и CI Flow. Ответ
`reference/module-08-terraform/` предназначен ментору. В отчёте не выдавайте
учебную практику за опыт работы в production; рассказывайте, что проверили руками.

## Требуемая архитектура

```text
ПК инженера /32 ── SSH, HTTP ── proxy-01 (public IPv4, Nginx)
                                  │ SSH 22 / HTTP 8080
                                  ▼
                              app-01 (private IPv4, Compose)
                                  │ bootstrap TCP 80/443
                                  ▼
                            managed NAT → интернет
```

Одна выделенная VPC, две /24 подсети одной зоны: по умолчанию
`10.80.10.0/24` и `10.80.20.0/24`. При конфликте с VPN/локальной сетью
используйте другие непересекающиеся RFC1918 CIDR.
`standard-v3`, обычные VM, 100% vCPU, Ubuntu 24.04 LTS amd64 по закреплённому
image ID. Проверка minimum disk size обязательна до apply.

| VM | Ресурсы | Доступ |
| --- | --- | --- |
| `proxy-01` | 2 vCPU, 2 ГиБ, HDD 10 ГБ | один public IPv4; Nginx и jump host, без Docker |
| `app-01` | 2 vCPU, 4 ГиБ, HDD 30 ГБ | только private IPv4; API, worker и три зависимости |

Порты PostgreSQL/Redis/Kafka не публикуйте на VM. Прикрепляйте только одну
custom SG каждой роли, не default SG. IAM service account CI к VM не привязывайте.
Boot disks принадлежат VM, имеют prefix и `auto_delete=true`; disk IDs входят
в output. Не включайте `create_before_destroy` для удвоения платных VM.

## Два режима сети

Безопасный default `egress_mode=restricted`; для первой установки явно задайте
`bootstrap`. NAT route переключается отдельным `nat_route_enabled`, а не
вместе с режимом firewall. В restricted NAT и маршрут сохраняются.

| Соединение | Разрешение |
| --- | --- |
| инженер public IPv4 /32 → proxy | TCP 22, 80 |
| proxy SG → app SG | TCP 22, 8080, ingress и egress |
| app SG → private proxy | TCP 80, ingress и egress; нужен существующему smoke |
| каждая VM → DNS своей подсети | второй адрес подсети /32, UDP и TCP 53 |
| каждая VM → metadata | `169.254.169.254/32`, TCP 80 |
| обе VM → интернет | TCP 80/443 только в bootstrap |
| прочие новые соединения | разрешающих правил нет |

SG stateful: ответ на разрешённый входящий запрос не требует широкого egress.
Новый запрос Nginx → app требует разрешения обеих SG. Их правила объединяются:
дополнительная SG с широким egress разрушает ограничение.
Взаимные ссылки оформляйте отдельными SG rule resources, не двумя зависимыми
inline groups. IPv6 не настраивайте.

Это не air gap: cloud DNS допускает DNS exfiltration, доверенный инженер может
организовать SSH tunnel. HTTP без TLS предназначен только для учебных данных
и одного IP. Здесь нет HA, zero-downtime или защиты от всех каналов вывода данных.

## Terraform и контракт установки

Модуль `network` создаёт VPC, subnets, SG, rules, NAT и route table.
Модуль `compute` получает outputs сети и создаёт две VM с map/`for_each`
по стабильным ключам `app-01` и `proxy-01`, а не индексам списка.
Входы типизированы, ошибочные CIDR, operator не /32, SSH user/key и режим
отклоняются validation. Метки `owner`, `project=devops-practice`, `module=08`
ставьте на поддерживающие их ресурсы.

Закрепите Terraform/provider; сгенерируйте lockfile для Linux/Darwin amd64/arm64.
Provider credential получайте из `YC_SERVICE_ACCOUNT_KEY_FILE`.
Cloud-init создаёт пользователя с публичным ключом, sudo и выключенным password
SSH. Не устанавливайте приложение через provisioner или metadata.

Root должен выдать ровно этот объект командой `terraform output -json lab`:

```text
lab = {
  folder_id, name_prefix, owner, egress_mode,
  network_id, gateway_id, route_table_id,
  app   = { id, boot_disk_id, private_ip },
  proxy = { id, boot_disk_id, private_ip, public_ip }
}
```

Не выгружайте весь state ради двух адресов. Inventory и outputs локальные,
под `umask 077`; пароль PostgreSQL остаётся в Ansible Vault.
Приватный SSH key — только на control node, ProxyJump без agent forwarding,
host key checking включён. После установки в restricted запускайте проверки,
не полную переустановку с apt refresh.

## Обязательный GitLab Flow

1. Feature branch → MR: fmt, validate с `-backend=false`, TFLint, Trivy IaC
   HIGH/CRITICAL и Gitleaks. Ошибка блокирует merge. Fork MR без cloud credentials
   и remote state. Не добавляйте blanket security exceptions.
2. Merge в protected default branch → новый plan итогового commit, не plan из MR.
   Remote HTTP state отдельный для ученика/модуля; запись и lock — Maintainer/Owner.
3. Сохраните plan и manifest: commit, pipeline, project, folder, state, mode,
   hash inputs и SHA256 plan. Artifacts Developer+, один час, logs не публичные.
4. Manual apply проверяет manifest, checksum и актуальный SHA default branch.
   Несовпадение входов, другой folder, старый commit/state или expired artifact —
   отказ, новый pipeline. Apply использует saved plan и не перепланирует его.
5. Один `resource_group` на state плюс backend lock, non-interruptible cloud jobs
   и ограниченные timeout. Credentials HTTP backend только через environment
   каждого job, не `-backend-config` и не artifact.
6. Отдельные manual destroy-plan и destroy-apply. Обычный push не удаляет стенд.

Cloud jobs запускаются только из проверенной protected ветки. Даже plan из
непроверенного MR может раскрыть credential. Ручной apply не заменяет code review.
Runner уже существует: третью VM/контейнер runner здесь не создавайте.

## Опыты и результат для сдачи

Последовательность команд установки, восстановления, lock и удаления находится
в [runbook](../../artifacts/common/docs/runbooks/08-terraform-bootstrap.md). Выполните все 12 опытов.

| № | Опыт | Обязательное доказательство |
| --- | --- | --- |
| 1 | Первый bootstrap plan → manual apply | состав/стоимость, точный folder, две VM, app без public IP |
| 2 | Готовый Ansible | ProxyJump, inventory, успешный health-check и заявка processed в PostgreSQL |
| 3 | restricted | diff SG; новые внешние hostname/IPv4 соединения блокируются на обеих VM; SSH/DNS/smoke работают |
| 4 | Маршрут vs firewall | bootstrap, отключён/возвращён NAT route; app теряет интернет, proxy продолжает работать |
| 5 | Неверный source /32 | доступ не проходит; восстановление через cloud API новым корректным plan |
| 6 | Drift label | ручная правка VM label, обнаружение plan, восстановление декларативного значения |
| 7 | Import | пустая unattached SG только lab folder; описание в коде, import, пустой plan |
| 8 | moved | новый адрес ресурса без destroy/create, адрес изменён в state |
| 9 | for_each | перестановка map не заменяет VM; удаление одного из бесплатных demo SG не затрагивает другой |
| 10 | Replacement proxy | bootstrap, `-replace` только proxy; новый host key, повторная установка, restricted; прежняя заявка читается |
| 11 | State lock | ваш краткий demo lock мешает второй сессии получить lock; чужой lock не снимается |
| 12 | Cleanup | просмотренный destroy plan, live inventory: нет VM/disks/IP/NAT, удалены demo SG и сеть |

Сначала положительный baseline внешнего доступа bootstrap, потом отрицательный
restricted. Новый SSH и новый curl обязательны, существующий keep-alive не
доказывает правила. Проверяйте внешний endpoint одновременно с ПК: упавший сайт
не является доказательством SG. В routing-опыте сохраняйте DNS и SG baseline.

Сохраните UUID контрольной заявки до replacement. Замена proxy не доказывает
сохранность данных при замене app. Объясните роль persistent storage, backup,
`prevent_destroy`, refresh, lock и ограничения Terraform.

## Бюджет и приёмка

Расчёт на 7 октября 2026, Россия/рубли с НДС: около 7,7908 ₽/час,
187 ₽/24 часа, 374 ₽/48 часов для указанного sizing, HDD, одного active IPv4
и одного NAT. Это оценка, не ограничитель счёта:
[Compute](https://github.com/yandex-cloud/docs/blob/master/md-docs/compute/pricing.md),
[VPC](https://github.com/yandex-cloud/docs/blob/master/md-docs/vpc/pricing.md).
Повторно проверьте ставки и неистраченный бесплатный объём трафика перед запуском.
Дополнительные диски/snapshots, traffic и overlap replacement в оценку не входят.
Увеличение minimum disk size требует пересчёта.

Максимум 48 календарных часов, включая простои между занятиями. Уведомления
300/400 ₽ не останавливают расход; при 400 ₽ завершайте и удаляйте.
Остановка VM и закрытие SG не прекращают все расходы.

Сдайте MR, конфигурацию, результаты 12 опытов и короткий отчёт:
ожидание → команда/изменение → наблюдение → причинное объяснение.
Без IAM keys, private keys, Vault, токенов, state и plan в публичном отчёте.
Static validate не доказывает ошибки input plan, работу сети или стоимость.
Приёмка завершена только после live cleanup; удалить state вместо ресурсов нельзя.
