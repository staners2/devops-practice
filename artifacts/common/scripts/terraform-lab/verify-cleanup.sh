#!/usr/bin/env bash
set -euo pipefail
umask 077

[[ $# == 2 && $1 == --outputs && -f $2 ]] || { printf 'Использование: verify-cleanup.sh --outputs LAB_BEFORE_DESTROY.json\n' >&2; exit 2; }
outputs=$2
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
python3 "$repo_root/scripts/terraform-lab/inventory.py" --user devops < "$outputs" > /dev/null
folder=$(jq -er '.folder_id' "$outputs")
if command -v timeout > /dev/null; then
  bounded=(timeout --signal=TERM --kill-after=5s 60s)
elif command -v gtimeout > /dev/null; then
  bounded=(gtimeout --signal=TERM --kill-after=5s 60s)
else
  printf 'Нужен GNU timeout (coreutils).\n' >&2; exit 2
fi
command -v yc > /dev/null || { printf 'Установите Yandex Cloud CLI.\n' >&2; exit 2; }
remaining=0
for kind in 'compute instance' 'compute disk' 'compute snapshot' 'vpc address' 'vpc gateway' 'vpc route-table' 'vpc security-group' 'vpc subnet' 'vpc network'; do
  read -r service resource <<< "$kind"
  # Folder выделен только этому стенду: незнакомые/непомеченные остатки тоже отклоняются.
  count=$("${bounded[@]}" yc "$service" "$resource" list --folder-id "$folder" --format json | jq -er 'if type == "array" then length else error("ожидался список") end')
  printf '%s: осталось %s\n' "$kind" "$count"
  remaining=$((remaining + count))
done
if (( remaining != 0 )); then
  printf 'Cleanup не завершён. Разберите точные остатки folder; helper ничего не удаляет.\n' >&2
  exit 1
fi
printf 'VM, диски, snapshots, IP, NAT и сеть выделенного lab folder удалены.\n'
