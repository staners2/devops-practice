# Модуль 08: подготовка и приёмка Terraform-стенда

Команды выполняет ученик в выделенном lab folder. Этот runbook описывает
платное развёртывание, но наличие файлов не означает, что облако уже запущено.
Работайте из корня checkout. Требуются GitLab.com Maintainer/Owner, Yandex Cloud
с billing account, доступ к runner и разрешённый бюджет. Не используйте home/prod
folder или общий state предыдущих практик.

## 1. Инструменты на ПК

Версии: [Terraform 1.16.5](https://github.com/hashicorp/terraform/releases/tag/v1.16.5),
[provider 0.236.0](https://github.com/yandex-cloud/terraform-provider-yandex/releases/tag/v0.236.0),
[TFLint 0.64.0](https://github.com/terraform-linters/tflint/releases/tag/v0.64.0).
Ansible/collection — из `deploy/ansible-lab/versions.env`.
Linux control node можно запустить в Docker Desktop; Docker socket не монтируется.
Сборка этого image не создаёт облачных VM:

```bash
docker build -f containers/terraform-tools/Dockerfile -t devops-practice-terraform-tools:08 .
```

На Linux bind mount должен быть доступен UID пользователя image (1000);
если UID отличается, используйте native Linux/WSL control node с вашими правами.
Для native установите Terraform/TFLint по официальным релизам указанной версии
с проверкой SHA256; Python 3.12+, jq, OpenSSH, GNU coreutils. Затем:

```bash
python3 -m venv .cache/ansible-08
source .cache/ansible-08/bin/activate
source deploy/ansible-lab/versions.env
python -m pip install "ansible-core==$ANSIBLE_CORE_VERSION" "ansible-lint==$ANSIBLE_LINT_VERSION"
ansible-galaxy collection install "community.docker:$COMMUNITY_DOCKER_VERSION"
```

На macOS GNU timeout устанавливается как `gtimeout` пакетом `coreutils`.
Ansible control node на Windows запускайте из WSL либо контейнера, не PowerShell.
В контейнере repository монтируется read/write для generated files; SSH
каталог и IAM key — только read-only. Контейнер не содержит решения Terraform.

Yandex Cloud CLI установлен отдельно на ПК: `yc version`, `yc init`.
Если CLI нет, скачайте официальный installer, просмотрите его, затем установите
проверенную версию 0.192.0:

```bash
curl -fsSL https://storage.yandexcloud.net/yandexcloud-yc/install.sh -o /tmp/yc-install-08.sh
less /tmp/yc-install-08.sh
CLI_VERSION=0.192.0 bash /tmp/yc-install-08.sh -i "$PWD/.cache/yc-08" -n
export PATH="$PWD/.cache/yc-08/bin:$PATH"
yc version
yc init
```

Путь/profile/billing account выбирайте явно по
[официальному quickstart](https://yandex.cloud/ru/docs/cli/quickstart).
Не запускайте installer из непроверенного сайта. `yc` нужен для ручного
bootstrap и read-only cleanup; CI использует provider, не CLI.

## 2. Folder, права и бюджет

В Console создайте отдельный folder для одного ученика/модуля и привяжите billing
account. Проверьте квоты: 4 vCPU, 6 ГиБ RAM, 40 ГБ HDD, один public IPv4,
один NAT gateway и наличие `standard-v3` в выбранной зоне.
Настройте уведомления бюджета на 300/400 ₽. Они не останавливают ресурсы.
Запишите deadline: максимум 48 календарных часов, удаление при приближении к 400 ₽.

Проверьте актуальные
[Compute](https://yandex.cloud/ru/docs/compute/pricing) /
[VPC](https://yandex.cloud/ru/docs/vpc/pricing) ставки. Исходный расчёт:

| Компонент | ₽/час |
| --- | ---: |
| 4 vCPU × 1,24 | 4,96000 |
| 6 ГиБ × 0,33 | 1,98000 |
| 40 ГБ HDD × 0,0048 | 0,19200 |
| active IPv4 | 0,26352 |
| NAT с маршрутом | 0,39528 |
| Итого | 7,79080 |

187 ₽/24 часа, 374 ₽/48 часов, запас около 126 ₽ до 500 ₽ при этих ставках.
Проверяйте остаток бесплатного трафика; скидки/грант не обязательны и не
гарантированы. Дополнительные snapshots/disks, traffic и overlap не включены.
Остановка VM, закрытие SG и простаивающий стенд не равны cleanup.

Сначала оператор, имеющий право назначать роли, создаёт отдельный service account:

```bash
read -r -p 'Точный lab folder ID: ' LAB_FOLDER_ID
read -r -p 'Уникальное имя SA: ' LAB_SA_NAME
export LAB_FOLDER_ID
LAB_SA_ID=$(yc iam service-account create --folder-id "$LAB_FOLDER_ID" --name "$LAB_SA_NAME" --format json | jq -er '.id')
for role in compute.editor vpc.privateAdmin vpc.publicAdmin vpc.securityGroups.admin vpc.gateways.editor vpc.gateways.user; do
  yc resource-manager folder add-access-binding "$LAB_FOLDER_ID" --role "$role" --subject "serviceAccount:$LAB_SA_ID"
done
umask 077
mkdir -p deploy/terraform-lab/local
yc iam key create --service-account-id "$LAB_SA_ID" --output deploy/terraform-lab/local/yc-key.json
export YC_SERVICE_ACCOUNT_KEY_FILE="$PWD/deploy/terraform-lab/local/yc-key.json"
```

Права ограничены только lab folder:
[Compute IAM](https://yandex.cloud/ru/docs/compute/security/),
[VPC IAM](https://yandex.cloud/ru/docs/vpc/security/).
`vpc.admin` не заменяет специальные роли NAT gateway. CI не получает cloud admin
или editor. SA/key создаются вне Terraform lab state и не прикрепляются к VM.
В конце ключ отзывается отдельно, после проверки cleanup.

## 3. Image, SSH и входные параметры

Получите текущий Ubuntu image один раз и закрепите его ID. Это чтение metadata,
не команда создания VM:

```bash
yc compute image get-latest-from-family ubuntu-2404-lts --folder-id standard-images --format json > deploy/terraform-lab/local/image.json
jq '{id,name,description,os,min_disk_size}' deploy/terraform-lab/local/image.json
export TF_VAR_image_id=$(jq -er '.id' deploy/terraform-lab/local/image.json)
yc compute image get --id "$TF_VAR_image_id" --format json
```

Проверьте Ubuntu 24.04 LTS, amd64 по описанию/Marketplace и minimum disk size
в байтах. Он должен помещаться в 10 ГБ proxy и 30 ГБ app.
Если нет — увеличьте соответствующий диск и пересчитайте бюджет до apply.
Не подменяйте image ID family data source при каждом plan.

Создайте отдельный Ed25519 key на ПК с passphrase. Private key не передаётся
в Terraform, GitLab или на proxy:

```bash
ssh-keygen -t ed25519 -f "$PWD/deploy/terraform-lab/local/id_ed25519" -C devops-08
export LAB_SSH_USER=devops
export TF_VAR_ssh_user=$LAB_SSH_USER
export TF_VAR_ssh_public_key=$(awk '{print $1 " " $2}' deploy/terraform-lab/local/id_ed25519.pub)
read -r -p 'Ваш текущий public IPv4/32: ' TF_VAR_operator_cidr
read -r -p 'Идентификатор ученика (owner): ' TF_VAR_owner
export TF_VAR_operator_cidr TF_VAR_owner
export TF_VAR_folder_id=$LAB_FOLDER_ID
export TF_VAR_egress_mode=bootstrap
export TF_ROOT=terraform
```

Для non-interactive Ansible/passphrase используйте SSH agent на control node,
не agent forwarding. В контейнере можно запустить собственный agent и
`ssh-add /home/lab/.ssh/id_ed25519`; сокет не передавайте на VM.
Локальный каталог SSH для image:

```bash
mkdir -p deploy/terraform-lab/local/ssh
cp deploy/terraform-lab/local/id_ed25519 deploy/terraform-lab/local/ssh/id_ed25519
cp deploy/terraform-lab/local/id_ed25519.pub deploy/terraform-lab/local/ssh/id_ed25519.pub
chmod 700 deploy/terraform-lab/local/ssh
chmod 600 deploy/terraform-lab/local/ssh/id_ed25519
```

Для native добавьте в ваш `~/.ssh/config` две секции Host с точными IP VM:
`User devops`, `IdentityFile /абсолютный/путь/deploy/terraform-lab/local/id_ed25519`,
`IdentitiesOnly yes`, `ForwardAgent no`, `StrictHostKeyChecking yes`.
Это нужно и SSH-процессу ProxyJump, которому Ansible `--private-key` сам по
себе ключ не передаёт. Не отключайте host key checking глобально.

## 4. GitLab state и pipeline

В проекте запретите прямой push default branch, включите успешный pipeline для MR.
Protected cloud variables не выдавайте MR. Отключите Public pipelines в
General pipelines; проверьте права скачивания artifact и state другим аккаунтом.
State/plan содержат чувствительные данные даже при sensitive outputs.

Создайте protected variables:

- `LAB_FOLDER_ID`, `TF_STATE_NAME=studentname-module08`;
- `YC_SERVICE_ACCOUNT_KEY_FILE` типа File — содержимое SA JSON, не его локальный путь;
- `TF_VAR_egress_mode`, `TF_VAR_operator_cidr`, `TF_VAR_image_id`,
  `TF_VAR_ssh_user`, `TF_VAR_ssh_public_key`, `TF_VAR_owner`;
- при необходимости `TF_VAR_name_prefix`, CIDR/disk parameters;
- `TF_REPLACE_PROXY=true` только на один replacement pipeline в bootstrap.

File credential должен быть protected; если GitLab не допускает masking
многострочного JSON, не обещайте его: храните как закрытый File variable,
не печатайте и не включайте trace. После каждого pipeline удалите временный
replacement input. Все inputs одинаковы в plan/apply данного pipeline.

Используйте существующий Kubernetes runner с tag `k8s`; без Docker daemon/DinD.
Для standalone — runner ментора либо подходящий GitLab.com runner при доступной
квоте/требованиях аккаунта; меняйте tag осознанно, бесплатную квоту не предполагаем.
Напишите собственный CI по заданию. Менторский полный CI включается только
выбором файла в Settings → CI/CD → CI/CD configuration file.

Для локального output/plan/import/lock в двух сессиях настройте backend через
environment с PAT scope api пользователя Maintainer/Owner. Не сохраняйте token
в history, tfvars, `-backend-config` или файле shell:

```bash
read -r -p 'GitLab project ID: ' LAB_PROJECT_ID
read -r -p 'GitLab username: ' TF_HTTP_USERNAME
read -r -s -p 'GitLab token: ' TF_HTTP_PASSWORD; printf '\n'
export TF_HTTP_USERNAME TF_HTTP_PASSWORD
export TF_STATE_NAME="$TF_VAR_owner-module08"
export TF_HTTP_ADDRESS="https://gitlab.com/api/v4/projects/$LAB_PROJECT_ID/terraform/state/$TF_STATE_NAME"
export TF_HTTP_LOCK_ADDRESS="$TF_HTTP_ADDRESS/lock"
export TF_HTTP_UNLOCK_ADDRESS="$TF_HTTP_ADDRESS/lock"
export TF_HTTP_LOCK_METHOD=POST TF_HTTP_UNLOCK_METHOD=DELETE
terraform -chdir="$TF_ROOT" init -input=false -lockfile=readonly
```

В CI username `gitlab-ci-token` и password текущего `CI_JOB_TOKEN` каждого job,
не токен предыдущего plan. Не используйте общий state `default`.
Если уже был local state, сначала остановите concurrent jobs и осознанно
выполните `init -migrate-state`; не работайте параллельно с двумя state.
[GitLab state/Free и защита plan](https://docs.gitlab.com/user/infrastructure/iac/terraform_state/).

## 5. Первый запуск и готовая установка

Закончите Terraform-код и lockfile, выполните validator. Ошибочные режим, /0,
пересекающиеся CIDR и неверный SSH user/key проверьте отдельным plan с корректными
cloud credentials, без apply. Один validate не проверяет значения tfvars.

Через MR → merge → plan проверьте folder, две VM, отсутствие public app,
автоудаляемые boot disks и стоимость. Затем manual apply saved plan.
Никаких автоматических `apply -auto-approve` по push.
С ПК сохраните только output lab:

```bash
umask 077
terraform -chdir="$TF_ROOT" output -json lab > deploy/terraform-lab/local/lab.json
python3 scripts/terraform-lab/inventory.py --user "$LAB_SSH_USER" < deploy/terraform-lab/local/lab.json > deploy/terraform-lab/local/inventory.json
export ANSIBLE_CONFIG="$PWD/deploy/terraform-lab/ansible/ansible.cfg"
ansible-inventory -i deploy/terraform-lab/local/inventory.json --graph
```

По доверенному каналу Console/serial output сверьте SSH fingerprints обеих VM.
Для публичного proxy получите кандидат key через `ssh-keyscan`, сверяйте
`ssh-keygen -lf` с cloud console, только потом добавляйте known_hosts.
Для app `ssh-keyscan` через уже доверенный proxy (`ssh proxy 'ssh-keyscan APP_IP'`)
также лишь получает кандидат: доверие устанавливается сравнением fingerprint.
Этот способ требует временно загрузить public keys, не private key.
Разместите verified known_hosts в `local/ssh/known_hosts` для контейнера
и в native SSH known_hosts для маршрута native.

Подготовьте Vault и публичный source:

```bash
ansible-vault create deploy/terraform-lab/local/vault.yml
# В редакторе задайте vault_postgres_password: результат openssl rand -hex 24.
read -r -p 'HTTPS URL публичного учебного GitLab repo (.git): ' LAB_REPO_URL
export LAB_REPO_URL
export LAB_REPO_REF=$(git rev-parse HEAD)
ansible-playbook -i deploy/terraform-lab/local/inventory.json deploy/terraform-lab/ansible/site.yml \
  --private-key deploy/terraform-lab/local/id_ed25519 --ask-vault-pass \
  --extra-vars @deploy/terraform-lab/local/vault.yml
bash scripts/terraform-lab/verify-access.sh --outputs deploy/terraform-lab/local/lab.json \
  --inventory deploy/terraform-lab/local/inventory.json --mode bootstrap
```

Control checkout должен быть чистым и совпадать с опубликованным полным SHA
`LAB_REPO_REF`: иначе VM скачает другие исходники. Для разработчика этого модуля
публикация — отдельная операция, автоматически она не выполнена.
Playbook ставит base, Docker app, клонирует public source в отдельный каталог,
собирает native images existing helper, затем Nginx и приложение.
Пароль остаётся в Vault, .env имеет 0600, данные не удаляются через down -v.
Сохраните UUID заявки `processed` из POST/GET /api/requests отдельно для опыта 10.

Контейнерный вариант запуска control node:

```bash
docker run --rm -it -v "$PWD:/workspace" \
  -v "$PWD/deploy/terraform-lab/local/ssh:/home/lab/.ssh:ro" \
  -v "$YC_SERVICE_ACCOUNT_KEY_FILE:/run/credentials/yc-key.json:ro" \
  -e YC_SERVICE_ACCOUNT_KEY_FILE=/run/credentials/yc-key.json \
  -e LAB_REPO_URL -e LAB_REPO_REF devops-practice-terraform-tools:08
```

Внутри задайте тот же ANSIBLE_CONFIG, запускайте playbook с
`--private-key /home/lab/.ssh/id_ed25519`; Vault prompt вводится локально.
Terraform inputs/backend для container output передайте отдельно через environment;
не передавайте PAT/ключ в аргументе literal. CI Ansible не запускает.

Если Ansible на ПК нет, сначала запустите этот контейнер, затем выполняйте
команды inventory/Vault/playbook раздела 5 внутри него. Для native Vault нужен
локальный редактор; в tools image уже установлен и выбран nano. Cloud CLI
bootstrap/проверка metadata и cleanup остаются на ПК с установленным yc.

## 6. Restricted и routing-опыт

После положительного baseline поменяйте protected input `TF_VAR_egress_mode`
на restricted, новый protected web pipeline → plan → manual apply.
Обновите local TF_VAR и lab.json. Запустите helper с `--mode restricted`.
Он проверяет новые SSH без multiplex reuse, DNS, public health-check, полный
smoke из app через private proxy, внешний hostname и прямой IPv4 с таймаутом.
Control endpoint с ПК проверяется до и после. Curl TLS/HTTP/DNS ошибки вместо
TCP timeout/refused не принимаются за доказательство SG.

Routing-опыт: вернитесь bootstrap, положительный baseline; задайте
`TF_VAR_nat_route_enabled=false`, новый plan/apply. App теряет внешнее соединение
через NAT, но proxy с public IP продолжает работать. Helper полного baseline
здесь ожидаемо не проходит; исследуйте обе VM отдельно через Ansible command.
Верните route=true, убедитесь в baseline и затем restricted. Не удаляйте
DHCP-маршруты внутри ОС и не приписывайте отсутствие маршрута firewall.

## 7. Drift, import, moved, коллекции и replacement

Все изменения — только lab folder, через просмотренный plan. Сохраните
исходные IDs/outputs. Для неверного source /32 заранее проверьте cloud API-доступ:
измените CIDR через input/plan, убедитесь в отказе SSH/HTTP, восстановите
правильный CIDR через новый pipeline. SSH для восстановления не требуется.
Не разрешайте `0.0.0.0/0` «временно» ради обхода проблемы.

Drift: в Console измените label owner только proxy VM. Plan должен показать
возврат к коду, apply возвращает label без replacement.

Import: создайте пустую SG без rules и без attachment только в lab VPC:

```bash
LAB_NETWORK_ID=$(jq -er '.network_id' deploy/terraform-lab/local/lab.json)
LAB_IMPORT_SG_ID=$(yc vpc security-group create --folder-id "$LAB_FOLDER_ID" --network-id "$LAB_NETWORK_ID" --name devops08-import-demo --format json | jq -er '.id')
# Сначала опишите yandex_vpc_security_group.import_demo с теми же folder/network/name.
terraform -chdir="$TF_ROOT" import yandex_vpc_security_group.import_demo "$LAB_IMPORT_SG_ID"
terraform -chdir="$TF_ROOT" plan -input=false -lock-timeout=60s
```

Plan после import должен быть пустым. Добавьте SG в код и сохраните её в state
до final destroy. Не импортируйте attached/чужую SG.
Переименуйте адрес import_demo → imported_demo и добавьте `moved` block
с `from = yandex_vpc_security_group.import_demo`,
`to = yandex_vpc_security_group.imported_demo`. Plan меняет адрес, не ресурс;
примените через обычный Flow и подтвердите state list.

Для `for_each` создайте две пустые unattached SG с ключами `alpha`/`beta`,
измените порядок map, затем удалите только alpha. План затрагивает только этот
ключ. Map VM переставьте без добавления третьей VM: VM IDs не меняются.
Demo SG удаляются final destroy.

Replacement proxy: сохраните UUID processed-заявки, верните bootstrap,
запустите plan с `-replace='module.compute.yandex_compute_instance.vm["proxy-01"]'`
(в reference CI input `TF_REPLACE_PROXY=true`). Адрес приведён для reference;
в своём коде возьмите адрес из state list. Plan не должен заменять app или диск app.
Manual apply прерывает HTTP на время replacement; public IP/host key могут смениться.
Пересоздайте outputs/inventory, сверьте новый fingerprint, удалите лишь старую
строку known_hosts точного proxy и добавьте новую verified. Не выключайте проверку.
Запустите Ansible с `--limit proxy`, затем restricted.

```bash
bash scripts/terraform-lab/verify-access.sh --outputs deploy/terraform-lab/local/lab.json \
  --inventory deploy/terraform-lab/local/inventory.json --mode restricted --request "$LAB_REQUEST_ID"
```

Это дополнительно читает прежнюю заявку из PostgreSQL. Замена app, миграция его
данных, backups и HA не входят в опыт; объясните, почему boot disk auto_delete
опасен при replacement app и почему prevent_destroy не является backup.

## 8. Безопасная демонстрация lock

Не запускайте второй apply. Две сессии с одним HTTP backend; активных jobs нет.
В первой создайте свой уникальный lock и локальный файл под umask:

```bash
umask 077
LAB_LOCK_ID=$(python3 -c 'import uuid; print(uuid.uuid4())')
jq -n --arg id "$LAB_LOCK_ID" --arg who "$TF_HTTP_USERNAME" \
  '{ID:$id,Operation:"OperationTypePlan",Info:"module08-demo",Who:$who,Version:"1.16.5",Created:(now|todate),Path:"module08-demo"}' > deploy/terraform-lab/local/demo-lock.json
curl --fail --silent --show-error --max-time 15 -u "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" \
  -X POST -H 'Content-Type: application/json' --data-binary @deploy/terraform-lab/local/demo-lock.json "$TF_HTTP_LOCK_ADDRESS"
```

Если POST вернул конфликт/ошибку, опыт прекращается: чужой lock не трогайте.
Успех фиксирует ваш ID, operation и Who. Во второй сессии (тот же state/env):

```bash
terraform -chdir="$TF_ROOT" plan -input=false -lock-timeout=5s
```

Ожидается конфликт, не изменение ресурсов. Сразу в первой снимите только свой
lock с тем же ID, тело файла не меняйте:

```bash
curl --fail --silent --show-error --max-time 15 -u "$TF_HTTP_USERNAME:$TF_HTTP_PASSWORD" \
  -X DELETE -H 'Content-Type: application/json' --data-binary @deploy/terraform-lab/local/demo-lock.json "$TF_HTTP_UNLOCK_ADDRESS"
```

После ошибки release проверьте владельца и состояние jobs прежде чем повторять.
Не применяйте force-unlock активному процессу, не редактируйте state вручную.
Demo lock держите только время опыта, не оставляйте на ночь.

## 9. Полный cleanup

До destroy сохраните output отдельным файлом:

```bash
umask 077
terraform -chdir="$TF_ROOT" output -json lab > deploy/terraform-lab/local/lab-before-destroy.json
```

Создайте новый manual destroy-plan, просмотрите VM, disks, NAT, SG/demo/import
и сеть именно этого folder, затем manual destroy-apply saved plan.
Не удаляйте state/SA/key раньше. После apply:

```bash
bash scripts/terraform-lab/verify-cleanup.sh --outputs deploy/terraform-lab/local/lab-before-destroy.json
```

Helper читает VM/disks/snapshots/IP/gateway/route tables/SG/subnets/network в
выделенном folder. Любой остаток, даже без labels, означает незавершённый cleanup.
Он ничего не удаляет. При неудаче разберите точные ID с `yc ... list` и plan,
завершите только учебные операции. Пустой state без live inventory не принимается.
Проверьте Billing по folder и остаточный расход; после завершения отзовите
точный key ID service account через Console, при ненужности удалите сам lab SA.
Не удаляйте другие credential или folder. Backend историю оставьте для отчёта.

## Что не доказывает локальная проверка

fmt/validate/lint/YAML проверяют конфигурацию, не установку Ubuntu, sizing runtime,
фактическую SG-фильтрацию, цены, backend locks или cleanup. Настоящий GitLab CI
Lint/pipeline, оплачиваемые apply/destroy, SSH/Ansible runtime и 12 опытов
проводятся отдельно с разрешением на расходы. Этим runbook они не объявлены выполненными.
