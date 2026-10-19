#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf '%s\n' "$1" >&2; exit 2; }
outputs='' inventory='' mode='' request=''
while [[ $# -gt 0 ]]; do
  [[ $# -ge 2 ]] || fail 'У каждого аргумента должно быть значение.'
  case "$1" in
    --outputs) outputs=$2 ;; --inventory) inventory=$2 ;; --mode) mode=$2 ;; --request) request=$2 ;;
    *) fail 'Использование: verify-access.sh --outputs FILE --inventory FILE --mode bootstrap|restricted [--request UUID]' ;;
  esac
  shift 2
done
[[ -f $outputs && -f $inventory && ( $mode == bootstrap || $mode == restricted ) ]] || fail 'Нужны outputs, inventory и режим.'
[[ -z $request || $request =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]] || fail 'Неверный UUID.'
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
if command -v timeout > /dev/null; then
  bounded=(timeout --signal=TERM --kill-after=5s 200s)
elif command -v gtimeout > /dev/null; then
  bounded=(gtimeout --signal=TERM --kill-after=5s 200s)
else
  fail 'Нужен GNU timeout (coreutils).'
fi
task_tmp=$(mktemp -d)
trap 'rm -f "$task_tmp/expected.json" "$task_tmp/actual.json"; rmdir "$task_tmp"' EXIT
ssh_user=$(jq -er '.all.vars.ansible_user' "$inventory")
python3 "$repo_root/scripts/terraform-lab/inventory.py" --user "$ssh_user" < "$outputs" | jq -S . > "$task_tmp/expected.json"
jq -S . "$inventory" > "$task_tmp/actual.json"
cmp -s "$task_tmp/expected.json" "$task_tmp/actual.json" || fail 'Inventory не соответствует сохранённому lab output.'
[[ $(jq -er '.egress_mode' "$outputs") == "$mode" ]] || fail 'Обновите output после изменения режима.'
proxy_public=$(jq -er '.proxy.public_ip' "$outputs")
proxy_private=$(jq -er '.proxy.private_ip' "$outputs")
probe_ip=$(python3 -c 'import socket; print(socket.getaddrinfo("example.com",443,socket.AF_INET,socket.SOCK_STREAM)[0][4][0])')
curl -4 --noproxy '*' --fail --silent --show-error --connect-timeout 4 --max-time 8 \
  --resolve "example.com:443:$probe_ip" https://example.com/ > /dev/null
curl -4 --noproxy '*' --fail --silent --show-error --connect-timeout 4 --max-time 8 \
  "http://$proxy_public/api/health-check" > /dev/null

export ANSIBLE_CONFIG="$repo_root/deploy/terraform-lab/ansible/ansible.cfg"
export ANSIBLE_SSH_ARGS='-C -o ControlMaster=no -o ControlPath=none'
"${bounded[@]}" ansible all -i "$inventory" -m ansible.builtin.ping
"${bounded[@]}" ansible all -i "$inventory" -m ansible.builtin.command -a 'timeout 15s getent ahostsv4 example.com'
# Результат SSH/DNS проверен отдельно; HTTP ошибка не считается доказательством фильтрации.
probe_command="set -eu; for probe in hostname ipv4; do
  result=0
  if [ \"\$probe\" = ipv4 ]; then
    curl -4 --noproxy '*' --silent --show-error --connect-timeout 4 --max-time 8 --resolve example.com:443:$probe_ip https://example.com/ >/dev/null 2>&1 || result=\$?
  else
    curl -4 --noproxy '*' --silent --show-error --connect-timeout 4 --max-time 8 https://example.com/ >/dev/null 2>&1 || result=\$?
  fi
  if [ $mode = bootstrap ]; then
    [ \"\$result\" -eq 0 ]
  else
    [ \"\$result\" -eq 7 ] || [ \"\$result\" -eq 28 ]
  fi
  printf '%s: curl exit=%s ($mode)\\n' \"\$probe\" \"\$result\"
done"
"${bounded[@]}" ansible all -i "$inventory" -m ansible.builtin.shell -a "$probe_command"
smoke_args=$(jq -cn --arg url "http://$proxy_private" --arg request "$request" \
  '{argv:(["bash","/opt/devops-practice-ansible-07/verify.sh","/opt/devops-practice-ansible-07",$url] + (if $request == "" then [] else [$request] end))}')
"${bounded[@]}" ansible app -i "$inventory" --become -m ansible.builtin.command -a "$smoke_args"
curl -4 --noproxy '*' --fail --silent --show-error --connect-timeout 4 --max-time 8 \
  --resolve "example.com:443:$probe_ip" https://example.com/ > /dev/null
printf 'SSH, DNS, HTTP и сквозной поток работают; внешний control endpoint доступен с ПК.\n'
