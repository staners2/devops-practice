#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf '%s\n' "$1" >&2; exit 2; }
[[ $# == 1 ]] || fail 'Использование: ci.sh validate|plan|apply|destroy-plan|destroy-apply'
action=$1
tf_root=${TF_ROOT:-reference/module-08-terraform}
if [[ $action == validate ]]; then
  exec bash scripts/ci/validate-terraform.sh "$tf_root"
fi
case "$action" in plan|apply|destroy-plan|destroy-apply) ;; *) fail 'Неизвестная операция.' ;; esac
[[ ${CI_DEBUG_TRACE:-false} != true ]] || fail 'CI_DEBUG_TRACE запрещён для cloud jobs.'
[[ ${CI_COMMIT_REF_PROTECTED:-false} == true && ${CI_COMMIT_BRANCH:-} == "${CI_DEFAULT_BRANCH:?}" ]] || fail 'Нужен protected default branch.'
[[ ${CI_PIPELINE_SOURCE:-} == push || ${CI_PIPELINE_SOURCE:-} == web ]] || fail 'Недопустимый источник cloud pipeline.'
[[ ${LAB_FOLDER_ID:-} =~ ^[a-z0-9]{20}$ ]] || fail 'Неверный LAB_FOLDER_ID.'
[[ ${TF_STATE_NAME:-} =~ ^[a-z0-9][a-z0-9_-]{2,63}$ ]] || fail 'Неверный TF_STATE_NAME.'
[[ ${CI_COMMIT_SHA:-} =~ ^[0-9a-f]{40}$ && ${CI_PIPELINE_ID:-} =~ ^[0-9]+$ && ${CI_PROJECT_ID:-} =~ ^[0-9]+$ ]] || fail 'Неверная идентичность pipeline.'
[[ ${CI_API_V4_URL:-} == https://gitlab.com/api/v4 ]] || fail 'Референс рассчитан на GitLab.com.'
[[ -n ${CI_JOB_TOKEN:-} && -f ${YC_SERVICE_ACCOUNT_KEY_FILE:-} ]] || fail 'Нет backend token или file credential provider.'
export TF_VAR_folder_id=$LAB_FOLDER_ID
: "${TF_VAR_egress_mode:?}" "${TF_VAR_operator_cidr:?}" "${TF_VAR_image_id:?}" "${TF_VAR_ssh_user:?}" "${TF_VAR_ssh_public_key:?}" "${TF_VAR_owner:?}"
for name in egress_mode operator_cidr image_id ssh_user ssh_public_key owner; do
  key="TF_VAR_$name"
  [[ -n ${!key:-} ]] || fail "Не задан $key."
done
[[ $TF_VAR_egress_mode == bootstrap || $TF_VAR_egress_mode == restricted ]] || fail 'Неверный режим.'
export TF_IN_AUTOMATION=1 CHECKPOINT_DISABLE=1
export TF_HTTP_ADDRESS="$CI_API_V4_URL/projects/$CI_PROJECT_ID/terraform/state/$TF_STATE_NAME"
export TF_HTTP_LOCK_ADDRESS="$TF_HTTP_ADDRESS/lock" TF_HTTP_UNLOCK_ADDRESS="$TF_HTTP_ADDRESS/lock"
export TF_HTTP_LOCK_METHOD=POST TF_HTTP_UNLOCK_METHOD=DELETE
export TF_HTTP_USERNAME=gitlab-ci-token TF_HTTP_PASSWORD=$CI_JOB_TOKEN
case "$action" in destroy-*) operation=destroy ;; *) operation=apply ;; esac
plan="$tf_root/plan.tfplan"
manifest="$tf_root/plan-manifest.json"
inputs_sha=$(python3 -c 'import os,json,hashlib; data={k:v for k,v in os.environ.items() if k.startswith("TF_VAR_")}; print(hashlib.sha256(json.dumps(data,sort_keys=True,separators=(",",":")).encode()).hexdigest())')

# Проверки artifact выполняются до init/show/apply, чтобы отказ не трогал backend.
if [[ $action == apply || $action == destroy-apply ]]; then
  [[ -f $plan && -f $manifest ]] || fail 'Нет saved plan или manifest; запустите новый pipeline.'
  jq -e --arg commit "$CI_COMMIT_SHA" --arg pipeline "$CI_PIPELINE_ID" \
    --arg project "$CI_PROJECT_ID" --arg folder "$LAB_FOLDER_ID" --arg state "$TF_STATE_NAME" \
    --arg mode "$TF_VAR_egress_mode" --arg operation "$operation" --arg inputs "$inputs_sha" \
    '.commit == $commit and .pipeline == $pipeline and .project == $project and .folder == $folder and .state == $state and .mode == $mode and .operation == $operation and .inputs_sha256 == $inputs' \
    "$manifest" > /dev/null || fail 'Manifest не соответствует текущему pipeline и входам.'
  expected_sha=$(jq -er '.plan_sha256 | select(test("^[a-f0-9]{64}$"))' "$manifest")
actual_sha=$(shasum -a 256 "$plan" | awk '{print $1}')
  [[ $expected_sha == "$actual_sha" ]] || fail 'Checksum plan не совпадает.'
  # CI_JOB_TOKEN допускает list branches, но не GET отдельной ветки.
  branch_filter=$(python3 -c 'import os,re,urllib.parse; print(urllib.parse.quote("^"+re.escape(os.environ["CI_DEFAULT_BRANCH"])+"$",safe=""))')
  current_sha=$(curl --fail --silent --show-error --max-time 15 \
    --header "JOB-TOKEN: $CI_JOB_TOKEN" "$CI_API_V4_URL/projects/$CI_PROJECT_ID/repository/branches?regex=$branch_filter&per_page=1" |
    jq -er --arg branch "$CI_DEFAULT_BRANCH" 'if length == 1 then .[0] | select(.name == $branch and .default == true and .protected == true) | .commit.id else error("ветка не найдена однозначно") end')
  [[ $current_sha == "$CI_COMMIT_SHA" ]] || fail 'Default branch уже изменился; нужен новый pipeline.'
fi

terraform -chdir="$tf_root" init -input=false -lockfile=readonly -no-color
if [[ $action == plan || $action == destroy-plan ]]; then
  plan_args=()
  [[ $operation != destroy ]] || plan_args+=(-destroy)
  # Опыт replacement разрешён только для proxy, не для VM с данными.
  if [[ -n ${TF_REPLACE_PROXY:-} ]]; then
    [[ $TF_REPLACE_PROXY == true && $operation == apply && $TF_VAR_egress_mode == bootstrap ]] || fail 'Replacement proxy требует bootstrap apply plan.'
    plan_args+=('-replace=module.compute.yandex_compute_instance.vm["proxy-01"]')
  fi
  terraform -chdir="$tf_root" plan -input=false -lock-timeout=60s -no-color "${plan_args[@]}" -out=plan.tfplan
fi

# JSON plan не публикуется и не печатается; проверяем обе стороны, включая удаления.
terraform -chdir="$tf_root" show -json plan.tfplan | jq -e \
  --arg folder "$LAB_FOLDER_ID" --arg mode "$TF_VAR_egress_mode" \
  --arg operator "$TF_VAR_operator_cidr" --arg image "$TF_VAR_image_id" \
  --arg user "$TF_VAR_ssh_user" --arg public_key "$TF_VAR_ssh_public_key" --arg owner "$TF_VAR_owner" '
  def unindexed: gsub("\\[[^]]*\\]"; "");
  def configurations($prefix):
    (.resources[]? | . + {full_address: ($prefix + .address)}),
    ((.module_calls // {}) | to_entries[] |
      .key as $key | .value.module | configurations($prefix + "module." + $key + "."));
  [.resource_changes[]? | select(.type == "yandex_vpc_security_group") |
    [.change.before, .change.after][] | select(. != null and .folder_id == $folder) |
    .id | select(type == "string" and length > 0)] as $sg_ids |
  [.resource_changes[]? | select(.type == "yandex_vpc_security_group" and
    .change.after.folder_id == $folder and (.change.actions | index("create") != null)) |
    .address | unindexed] as $new_sgs |
  [.configuration.root_module | configurations("")] as $config |
  def owned_binding($rule; $values):
    if $values.security_group_binding != null then
      $values.security_group_binding as $binding | ($sg_ids | index($binding) != null)
    else
      ($rule.address | unindexed) as $address |
      $rule.change.before == null and $rule.change.after_unknown.security_group_binding == true and
      any($config[];
        (.full_address | sub("yandex_vpc_security_group_rule\\.[^.]+$"; "")) as $prefix |
        [.expressions.security_group_binding.references[]? |
          select(startswith("yandex_vpc_security_group.")) | ($prefix + . | unindexed)] as $refs |
        .full_address == $address and ($refs | length > 0) and all($refs[]; . as $ref | $new_sgs | index($ref) != null) and
        all(.expressions.security_group_binding.references[]?;
          startswith("yandex_vpc_security_group.") or startswith("each.")))
    end;
  .variables.folder_id.value == $folder and .variables.egress_mode.value == $mode and
  .variables.operator_cidr.value == $operator and .variables.image_id.value == $image and
  .variables.ssh_user.value == $user and .variables.ssh_public_key.value == $public_key and
  .variables.owner.value == $owner and
  all(.resource_changes[]?;
    .mode == "managed" and
    (.type as $type | ["yandex_vpc_network","yandex_vpc_subnet","yandex_vpc_gateway","yandex_vpc_route_table","yandex_vpc_security_group","yandex_vpc_security_group_rule","yandex_compute_instance"] | index($type) != null) and
    (if .type == "yandex_vpc_security_group_rule" then
      . as $rule | all([.change.before, .change.after][] | select(. != null); owned_binding($rule; .))
    else
      all([.change.before, .change.after][] | select(. != null); .folder_id == $folder)
    end)) and
  all([.output_changes.lab.before?, .output_changes.lab.after?][] | select(. != null); .folder_id == $folder)
' > /dev/null || fail 'Plan выходит за folder/тип ресурсов или содержит другие inputs.'

if [[ $action == plan || $action == destroy-plan ]]; then
  plan_sha=$(shasum -a 256 "$plan" | awk '{print $1}')
  jq -n --arg commit "$CI_COMMIT_SHA" --arg pipeline "$CI_PIPELINE_ID" --arg project "$CI_PROJECT_ID" \
    --arg folder "$LAB_FOLDER_ID" --arg state "$TF_STATE_NAME" --arg mode "$TF_VAR_egress_mode" \
    --arg operation "$operation" --arg inputs "$inputs_sha" --arg sha "$plan_sha" \
    '{commit:$commit,pipeline:$pipeline,project:$project,folder:$folder,state:$state,mode:$mode,operation:$operation,inputs_sha256:$inputs,plan_sha256:$sha}' > "$manifest"
else
  terraform -chdir="$tf_root" apply -input=false -lock-timeout=60s -no-color plan.tfplan
fi
