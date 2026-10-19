# Решение ученика: модуль 08

Здесь реализуйте Terraform root и модули `network` / `compute` по
[заданию руководителя](../docs/modules/08-terraform-yandex-cloud.md).
Готовая установка приложения дана в
[runbook](../docs/runbooks/08-terraform-bootstrap.md): Ansible заново писать не нужно.

Создайте `versions.tf`, пустой HTTP backend, typed variables, modules и output
`lab`; сгенерируйте и добавьте `.terraform.lock.hcl`. Проверки:

```bash
terraform -chdir=terraform init -backend=false -input=false
terraform -chdir=terraform providers lock -platform=linux_amd64 -platform=linux_arm64 -platform=darwin_amd64 -platform=darwin_arm64
bash scripts/ci/validate-terraform.sh terraform
```

Результаты plan и state, `.terraform/`, локальные tfvars, inventory, Vault и
ключи в Git не добавляйте. Полный Terraform-ответ находится в отдельном
менторском reference и не является входным материалом задания.
