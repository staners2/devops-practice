#!/usr/bin/env bash
set -euo pipefail

[[ $# == 2 ]] || { printf 'Использование: install-terraform-tools.sh VERSIONS_FILE BIN_DIR\n' >&2; exit 2; }
# shellcheck source=/dev/null
source "$1"
bin_dir=$2
case "$(uname -m)" in
  x86_64) arch=amd64; tf_sha=$TERRAFORM_SHA256_LINUX_AMD64; lint_sha=$TFLINT_SHA256_LINUX_AMD64 ;;
  aarch64|arm64) arch=arm64; tf_sha=$TERRAFORM_SHA256_LINUX_ARM64; lint_sha=$TFLINT_SHA256_LINUX_ARM64 ;;
  *) printf 'Поддерживаются Linux amd64/arm64.\n' >&2; exit 2 ;;
esac
[[ $(uname -s) == Linux ]] || { printf 'Installer предназначен для Linux.\n' >&2; exit 2; }
task_tmp=$(mktemp -d)
trap 'rm -f "$task_tmp/terraform.zip" "$task_tmp/tflint.zip"; rmdir "$task_tmp"' EXIT
mkdir -p "$bin_dir"
curl --fail --silent --show-error --location --retry 3 --max-time 120 \
  "https://releases.hashicorp.com/terraform/$TERRAFORM_VERSION/terraform_${TERRAFORM_VERSION}_linux_${arch}.zip" -o "$task_tmp/terraform.zip"
curl --fail --silent --show-error --location --retry 3 --max-time 120 \
  "https://github.com/terraform-linters/tflint/releases/download/v$TFLINT_VERSION/tflint_linux_${arch}.zip" -o "$task_tmp/tflint.zip"
printf '%s  %s\n%s  %s\n' "$tf_sha" "$task_tmp/terraform.zip" "$lint_sha" "$task_tmp/tflint.zip" | sha256sum --check --status
unzip -o -j "$task_tmp/terraform.zip" terraform -d "$bin_dir"
unzip -o -j "$task_tmp/tflint.zip" tflint -d "$bin_dir"
