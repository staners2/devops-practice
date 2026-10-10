#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../../.." && pwd)
verification_script=$repo_root/scripts/ci/verify-development.sh
if [ ! -x "$verification_script" ]; then
  echo "verify-development.sh is missing" >&2
  exit 1
fi

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
fake_bin=$test_dir/bin
sandbox=$test_dir/repository
commands_log=$test_dir/kubectl.log
application_attempt=$test_dir/application-attempt
mkdir -p "$fake_bin" "$sandbox/scripts/ci" \
  "$sandbox/deploy/environments/development"

cp "$verification_script" "$sandbox/scripts/ci/verify-development.sh"
cp "$repo_root/scripts/ci/check-ingress.sh" "$sandbox/scripts/ci/check-ingress.sh"
cp "$repo_root/deploy/environments/development/values.yaml" \
  "$sandbox/deploy/environments/development/values.yaml"

cat > "$fake_bin/sleep" <<'SCRIPT'
#!/bin/sh
exit 0
SCRIPT

cat > "$fake_bin/curl" <<'SCRIPT'
#!/bin/sh
printf '{"status":"ok"}\n'
SCRIPT

cat > "$fake_bin/kubectl" <<'SCRIPT'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$COMMANDS_LOG"
case " $* " in
  *' apply '*|*' create '*|*' delete '*|*' patch '*|*' replace '*|*' rollout\ restart '*|*' scale '*)
    echo "mutating kubectl command is forbidden: $*" >&2
    exit 97
    ;;
esac

case "$*" in
  '-n argocd get application devops-practice -o json')
    attempt=0
    if [ -f "$APPLICATION_ATTEMPT" ]; then
      attempt=$(cat "$APPLICATION_ATTEMPT")
    fi
    attempt=$((attempt + 1))
    printf '%s\n' "$attempt" > "$APPLICATION_ATTEMPT"
    if [ "$SCENARIO" = timeout ] || { [ "$SCENARIO" = success ] && [ "$attempt" -eq 1 ]; }; then
      printf '{"status":{"sync":{"status":"OutOfSync","revision":"old"},"health":{"status":"Progressing"},"operationState":{"phase":"Running","message":"syncing"}}}\n'
    else
      printf '{"status":{"sync":{"status":"Synced","revision":"%s"},"health":{"status":"Healthy"},"operationState":{"phase":"Succeeded","message":"success"}}}\n' "$CI_COMMIT_SHA"
    fi
    ;;
  '-n devops-practice rollout status deployment/devops-practice-api --timeout=3m'|\
  '-n devops-practice rollout status deployment/devops-practice-worker --timeout=3m')
    printf 'successfully rolled out\n'
    ;;
  '-n devops-practice get deployment devops-practice-api devops-practice-worker -o json')
    worker_image=$EXPECTED_IMAGE
    if [ "$SCENARIO" = mismatch ]; then
      worker_image=registry.gitlab.example/training/devops-practice:previous
    fi
    printf '{"items":[{"metadata":{"name":"devops-practice-api"},"spec":{"template":{"spec":{"containers":[{"name":"api","image":"%s"}]}}}},{"metadata":{"name":"devops-practice-worker"},"spec":{"template":{"spec":{"containers":[{"name":"worker","image":"%s"}]}}}}]}\n' \
      "$EXPECTED_IMAGE" "$worker_image"
    ;;
  '-n devops-practice wait --for=condition=complete job/devops-practice-smoke --timeout=3m')
    printf 'job.batch/devops-practice-smoke condition met\n'
    ;;
  '-n devops-practice logs job/devops-practice-smoke')
    printf 'smoke checks passed\n'
    ;;
  *)
    echo "unexpected kubectl command: $*" >&2
    exit 98
    ;;
esac
SCRIPT
chmod +x "$fake_bin/kubectl" "$fake_bin/curl" "$fake_bin/sleep"

export PATH="$fake_bin:$PATH"
export CI_COMMIT_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export EXPECTED_IMAGE=devops-practice:module-02
export COMMANDS_LOG=$commands_log
export APPLICATION_ATTEMPT=$application_attempt
export VERIFY_INTERVAL_SECONDS=0

run_scenario() {
  SCENARIO=$1
  VERIFY_MAX_ATTEMPTS=$2
  export SCENARIO VERIFY_MAX_ATTEMPTS
  : > "$commands_log"
  rm -f "$application_attempt" "$sandbox/smoke.log"
  (cd "$sandbox" && ./scripts/ci/verify-development.sh)
}

run_scenario success 3 > "$test_dir/success.log" 2>&1
grep -F 'Argo CD application is Synced/Healthy' "$test_dir/success.log"
grep -F 'API and worker use the expected image' "$test_dir/success.log"
grep -F 'Smoke job completed' "$test_dir/success.log"
grep -F 'Ingress health check passed' "$test_dir/success.log"
grep -Fx 'smoke checks passed' "$sandbox/smoke.log"

if run_scenario mismatch 2 > "$test_dir/mismatch.log" 2>&1; then
  echo "verification accepted a mismatched worker image" >&2
  exit 1
fi
grep -F "expected image: $EXPECTED_IMAGE" "$test_dir/mismatch.log"
grep -F 'actual image for devops-practice-worker: registry.gitlab.example/training/devops-practice:previous' \
  "$test_dir/mismatch.log"

if run_scenario timeout 2 > "$test_dir/timeout.log" 2>&1; then
  echo "verification did not stop after the configured attempts" >&2
  exit 1
fi
grep -F 'timed out waiting for Argo CD application' "$test_dir/timeout.log"
test "$(grep -c 'get application' "$commands_log")" -eq 2
if grep -Eq '(^| )(apply|create|delete|patch|replace|scale)( |$)|rollout restart' \
  "$commands_log"; then
  echo "verification used a mutating kubectl command" >&2
  exit 1
fi

echo "development verification tests passed"
