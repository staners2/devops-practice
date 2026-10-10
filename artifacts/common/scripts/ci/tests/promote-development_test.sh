#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../../.." && pwd)
promotion_script=$repo_root/scripts/ci/promote-development.sh
if [ ! -x "$promotion_script" ]; then
  echo "promote-development.sh is missing" >&2
  exit 1
fi

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
remote=$test_dir/remote.git
seed=$test_dir/seed
runner=$test_dir/runner
fake_bin=$test_dir/bin
mr_response=$test_dir/mr-response.json
trigger_log=$test_dir/triggers.log
push_options_log=$test_dir/push-options.log
mkdir -p "$fake_bin"
: > "$trigger_log"
: > "$push_options_log"

cat > "$fake_bin/skopeo" <<'SCRIPT'
#!/bin/sh
set -eu
case "$1" in
  login)
    cat >/dev/null
    ;;
  inspect)
    printf '%s\n' "$IMAGE_DIGEST"
    ;;
  *)
    echo "unexpected skopeo command: $1" >&2
    exit 1
    ;;
esac
SCRIPT

cat > "$fake_bin/curl" <<'SCRIPT'
#!/bin/sh
set -eu
case "$*" in
  *'/merge_requests'*)
    cat "$MR_RESPONSE_FILE"
    ;;
  *'/trigger/pipeline'*)
    printf 'trigger\n' >> "$TRIGGER_LOG"
    printf '{"id":1}\n'
    ;;
  *)
    echo "unexpected curl request" >&2
    exit 1
    ;;
esac
SCRIPT
chmod +x "$fake_bin/skopeo" "$fake_bin/curl"

git init --bare "$remote" >/dev/null
git --git-dir="$remote" config receive.advertisePushOptions true
cat > "$remote/hooks/pre-receive" <<'SCRIPT'
#!/bin/sh
set -eu
i=0
while [ "$i" -lt "${GIT_PUSH_OPTION_COUNT:-0}" ]; do
  eval "option=\${GIT_PUSH_OPTION_$i}"
  printf '%s\n' "$option" >> "$PUSH_OPTIONS_LOG"
  i=$((i + 1))
done
cat >/dev/null
SCRIPT
chmod +x "$remote/hooks/pre-receive"

git init "$seed" >/dev/null
git -C "$seed" switch -c main >/dev/null
git -C "$seed" config user.name Test
git -C "$seed" config user.email test@example.invalid
mkdir -p "$seed/deploy/environments/development" "$seed/scripts/ci"
cp "$repo_root/deploy/environments/development/values.yaml" \
  "$seed/deploy/environments/development/values.yaml"
cp "$promotion_script" "$seed/scripts/ci/promote-development.sh"
git -C "$seed" add .
git -C "$seed" commit -m seed >/dev/null
git -C "$seed" remote add origin "$remote"
PUSH_OPTIONS_LOG=$push_options_log git -C "$seed" push -u origin main >/dev/null
git --git-dir="$remote" symbolic-ref HEAD refs/heads/main
git clone "$remote" "$runner" >/dev/null
git -C "$runner" config user.name Test
git -C "$runner" config user.email test@example.invalid

export PATH="$fake_bin:$PATH"
export CI_API_V4_URL=https://gitlab.example/api/v4
export CI_PROJECT_ID=42
export CI_PROJECT_URL=https://gitlab.example/training/devops-practice
export CI_REPOSITORY_URL=$remote
export CI_REGISTRY=registry.gitlab.example
export CI_REGISTRY_IMAGE=registry.gitlab.example/training/devops-practice
export CI_REGISTRY_USER=gitlab-ci-token
export CI_REGISTRY_PASSWORD=registry-secret
export CI_DEFAULT_BRANCH=main
read -r CI_JOB_TOKEN <<'VALUE'
job-secret
VALUE
export CI_JOB_TOKEN
export MR_RESPONSE_FILE=$mr_response
export TRIGGER_LOG=$trigger_log
export PUSH_OPTIONS_LOG=$push_options_log

set_image() {
  CI_COMMIT_SHA=$1
  IMAGE_REFERENCE=$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
  IMAGE_DIGEST=sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
  export CI_COMMIT_SHA IMAGE_REFERENCE IMAGE_DIGEST
}

set_image aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
printf '[]\n' > "$mr_response"
(cd "$runner" && ./scripts/ci/promote-development.sh) > "$test_dir/first.log" 2>&1
promotion_branch=gitops/development-$CI_COMMIT_SHA

git --git-dir="$remote" show \
  "refs/heads/$promotion_branch:deploy/environments/development/values.yaml" \
  | grep -F "repository: $CI_REGISTRY_IMAGE"
git --git-dir="$remote" show \
  "refs/heads/$promotion_branch:deploy/environments/development/values.yaml" \
  | grep -F "tag: $CI_COMMIT_SHA"
grep -Fx 'merge_request.create' "$push_options_log"
grep -Fx "merge_request.target=$CI_DEFAULT_BRANCH" "$push_options_log"
test "$(wc -l < "$trigger_log" | tr -d ' ')" -eq 1
! grep -F "$CI_JOB_TOKEN" "$test_dir/first.log"
! grep -F "$CI_REGISTRY_PASSWORD" "$test_dir/first.log"

before=$(git --git-dir="$remote" rev-parse "refs/heads/$promotion_branch")
printf '[{"iid":1,"web_url":"https://gitlab.example/merge_requests/1"}]\n' \
  > "$mr_response"
(cd "$runner" && ./scripts/ci/promote-development.sh) > "$test_dir/retry.log" 2>&1
after=$(git --git-dir="$remote" rev-parse "refs/heads/$promotion_branch")
test "$before" = "$after"
test "$(grep -c '^merge_request.create$' "$push_options_log")" -eq 1
test "$(wc -l < "$trigger_log" | tr -d ' ')" -eq 2

set_image bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
foreign_branch=gitops/development-$CI_COMMIT_SHA
git -C "$seed" switch -C "$foreign_branch" origin/main >/dev/null
sed 's/tag: module-02/tag: foreign/' \
  "$seed/deploy/environments/development/values.yaml" \
  > "$test_dir/foreign-values.yaml"
mv "$test_dir/foreign-values.yaml" \
  "$seed/deploy/environments/development/values.yaml"
git -C "$seed" add deploy/environments/development/values.yaml
git -C "$seed" commit -m foreign >/dev/null
PUSH_OPTIONS_LOG=$push_options_log git -C "$seed" push origin \
  "HEAD:refs/heads/$foreign_branch" >/dev/null
printf '[]\n' > "$mr_response"
if (cd "$runner" && ./scripts/ci/promote-development.sh) \
  > "$test_dir/foreign.log" 2>&1; then
  echo "promotion overwrote a branch without an open MR" >&2
  exit 1
fi
grep -F 'promotion branch exists without an open merge request' \
  "$test_dir/foreign.log"

set_image cccccccccccccccccccccccccccccccccccccccc
git -C "$seed" switch -C main origin/main >/dev/null
sed -e "s|repository: devops-practice|repository: $CI_REGISTRY_IMAGE|" \
  -e "s|tag: module-02|tag: $CI_COMMIT_SHA|" \
  "$seed/deploy/environments/development/values.yaml" \
  > "$test_dir/current-values.yaml"
mv "$test_dir/current-values.yaml" \
  "$seed/deploy/environments/development/values.yaml"
git -C "$seed" add deploy/environments/development/values.yaml
git -C "$seed" commit -m current >/dev/null
PUSH_OPTIONS_LOG=$push_options_log git -C "$seed" push origin main >/dev/null
triggers_before=$(wc -l < "$trigger_log" | tr -d ' ')
(cd "$runner" && ./scripts/ci/promote-development.sh) > "$test_dir/current.log" 2>&1
test "$(wc -l < "$trigger_log" | tr -d ' ')" -eq "$triggers_before"
grep -F 'development already references the published image' \
  "$test_dir/current.log"

echo "promotion tests passed"
