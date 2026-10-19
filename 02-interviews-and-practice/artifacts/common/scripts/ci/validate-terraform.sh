#!/usr/bin/env bash
set -euo pipefail

[[ $# == 1 ]] || { printf 'Использование: validate-terraform.sh TF_ROOT\n' >&2; exit 2; }
tf_root=$1
if [[ ! -d "$tf_root" ]] || ! compgen -G "$tf_root/*.tf" > /dev/null; then
  printf 'Нет root-конфигурации Terraform: %s\n' "$tf_root" >&2
  exit 2
fi
for tool in terraform tflint; do
  command -v "$tool" > /dev/null || { printf 'Не найден %s.\n' "$tool" >&2; exit 2; }
done
terraform -chdir="$tf_root" fmt -check -recursive
terraform -chdir="$tf_root" init -backend=false -input=false -lockfile=readonly -no-color
terraform -chdir="$tf_root" validate -no-color
tflint --chdir="$tf_root" --recursive
