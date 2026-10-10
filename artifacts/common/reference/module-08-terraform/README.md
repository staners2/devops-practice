# Менторский референс Terraform

Полное решение [модуля 08](../../docs/modules/08-terraform-yandex-cloud.md).
Это не starter ученика. Для отдельного прохождения выдаются приложение,
`deploy/ansible-lab/`, roles модуля 07, `deploy/terraform-lab/`, helpers и
runbook, но не этот каталог. Содержимое публичных refs и история Git не секретны.

## Проверки без облачного аккаунта

```bash
bash scripts/ci/validate-terraform.sh reference/module-08-terraform
shellcheck reference/module-08-terraform/ci.sh scripts/terraform-lab/*.sh scripts/ci/*terraform*.sh
```

Root использует Terraform 1.16.5 и provider 0.236.0; lockfile сгенерирован
для Linux/Darwin amd64/arm64. TFLint 0.64.0 использует встроенный ruleset.
Все параметры перечислены в `variables.tf`; значения пользовательского
folder/image/key не выбраны автоматически. CIDR этого референса — только
канонические IPv4 /24 сети: разные сети этой длины не пересекаются.
Default egress — `restricted`, первый запуск явно требует `bootstrap`.

Ровно две VM со стабильными ключами; sizing из задания, HDD с auto_delete.
Замена proxy создаёт чистый boot disk и может менять public IP.
Замена app удаляет данные на его boot disk: в практику она не входит.
Cloud-init содержит только публичный ключ и SSH-пользователя с sudo.

## CI

Основной CI выполняет только static job. Чтобы проверить полный Flow в отдельном
учебном GitLab.com проекте, в Settings → CI/CD → General pipelines задайте
CI/CD configuration file `reference/module-08-terraform/.gitlab-ci.yml`.
Это намеренный выбор конфигурации, а не автоматический include.
В решении ученика скопируйте собственный Flow в корневой CI и задайте
`TF_ROOT=terraform`; `ci.sh` здесь — референс guards, не скрытая зависимость.

Настройте protected variables по [runbook](../../docs/runbooks/08-terraform-bootstrap.md).
MR запускает validate/Trivy/Gitleaks без cloud credentials.
Protected default branch создаёт новый plan итогового commit. Manual apply
получает artifact этого pipeline, проверяет checksum, входы, folder, state,
project, operation и актуальный commit ветки. Изменившиеся inputs требуют нового
pipeline. Apply никогда не делает новый plan. Backend token берётся из environment
нового job; Terraform backend config не содержит password.

Все cloud jobs имеют один `resource_group` на state и backend lock.
Manual jobs блокирующие: unattended pipeline может оставаться blocked — это
ожидаемо, а не повод включить автоматическое применение. Destroy-plan зависит
только от static checks, поэтому доступен и без выполнения обычного apply.
После обычного apply destroy-plan всё равно строится заново по текущему state.
Если artifact исчез или state изменился, повторно запустите планирование.

Plan содержит потенциально чувствительные данные: Developer+, срок один час,
public pipeline visibility отключена. Не запускайте `CI_DEBUG_TRACE`.
Не публикуйте JSON plan, state, IAM key, Vault или весь Terraform output.
IaC scan не заменяет review SG: покрытие конкретного provider зависит от Trivy.

Локально проверены конфигурации и helpers; облачный runtime, расходы, GitLab CI
Lint и настоящий pipeline требуют отдельной приёмки и разрешения на расходы.
