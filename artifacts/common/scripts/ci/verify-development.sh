#!/bin/sh
set -eu

for command in jq kubectl; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "$command is required" >&2
    exit 1
  fi
done

if ! printf '%s\n' "${CI_COMMIT_SHA:-}" | grep -Eq '^[0-9a-f]{40}$'; then
  echo "CI_COMMIT_SHA must contain 40 lowercase hexadecimal characters" >&2
  exit 1
fi

max_attempts=${VERIFY_MAX_ATTEMPTS:-120}
interval_seconds=${VERIFY_INTERVAL_SECONDS:-5}
case "$max_attempts" in
  ''|*[!0-9]*|0) echo "VERIFY_MAX_ATTEMPTS must be a positive integer" >&2; exit 1 ;;
esac
case "$interval_seconds" in
  ''|*[!0-9]*) echo "VERIFY_INTERVAL_SECONDS must be a non-negative integer" >&2; exit 1 ;;
esac

values_path=deploy/environments/development/values.yaml
if [ ! -f "$values_path" ]; then
  echo "$values_path is missing" >&2
  exit 1
fi

image_value() {
  key=$1
  awk -v key="$key" '
    $0 == "image:" {in_image=1; next}
    in_image && /^[^[:space:]]/ {in_image=0}
    in_image && $1 == key ":" {print $2; found++}
    END {if (found != 1) exit 1}
  ' "$values_path"
}

if ! repository=$(image_value repository) || ! tag=$(image_value tag); then
  echo "development values must contain one image repository and tag" >&2
  exit 1
fi
if [ -z "$repository" ] || [ -z "$tag" ]; then
  echo "development image repository and tag must not be empty" >&2
  exit 1
fi
if [ "$tag" = latest ]; then
  echo "development image tag must be immutable, not latest" >&2
  exit 1
fi
expected_image=$repository:$tag

attempt=1
sync_status=Unknown
health_status=Unknown
operation_phase=Unknown
operation_message='status is not available'
while [ "$attempt" -le "$max_attempts" ]; do
  application=$(kubectl -n argocd get application devops-practice -o json)
  if ! printf '%s' "$application" | jq -e 'type == "object"' >/dev/null; then
    echo "Argo CD application response is not a JSON object" >&2
    exit 1
  fi

  sync_status=$(printf '%s' "$application" | jq -r '.status.sync.status // "Unknown"')
  health_status=$(printf '%s' "$application" | jq -r '.status.health.status // "Unknown"')
  operation_phase=$(printf '%s' "$application" | jq -r '.status.operationState.phase // "Unknown"')
  operation_message=$(printf '%s' "$application" | jq -r '.status.operationState.message // "status is not available"')

  case "$operation_phase" in
    Failed|Error)
      echo "Argo CD operation failed: phase=$operation_phase message=$operation_message" >&2
      exit 1
      ;;
  esac

  revision_matches=false
  if printf '%s' "$application" | jq -e --arg revision "$CI_COMMIT_SHA" '
    ((.status.sync.revisions? // []) | index($revision)) != null
      or .status.sync.revision == $revision
  ' >/dev/null; then
    revision_matches=true
  fi

  if [ "$sync_status" = Synced ] \
    && [ "$health_status" = Healthy ] \
    && [ "$revision_matches" = true ]; then
    echo "Argo CD application is Synced/Healthy at $CI_COMMIT_SHA"
    break
  fi

  if [ "$attempt" -eq "$max_attempts" ]; then
    echo "timed out waiting for Argo CD application: sync=$sync_status health=$health_status operation=$operation_phase message=$operation_message" >&2
    exit 1
  fi
  sleep "$interval_seconds"
  attempt=$((attempt + 1))
done

kubectl -n devops-practice rollout status \
  deployment/devops-practice-api --timeout=3m
kubectl -n devops-practice rollout status \
  deployment/devops-practice-worker --timeout=3m
deployments=$(kubectl -n devops-practice get deployment \
  devops-practice-api devops-practice-worker -o json)

for deployment in devops-practice-api devops-practice-worker; do
  actual_image=$(printf '%s' "$deployments" | jq -r --arg name "$deployment" '
    [.items[] | select(.metadata.name == $name)] as $matches
    | if ($matches | length) == 1
        and ($matches[0].spec.template.spec.containers | length) == 1
      then $matches[0].spec.template.spec.containers[0].image
      else ""
      end
  ')
  if [ "$actual_image" != "$expected_image" ]; then
    echo "expected image: $expected_image" >&2
    echo "actual image for $deployment: ${actual_image:-missing or ambiguous}" >&2
    exit 1
  fi
done
echo "API and worker use the expected image: $expected_image"

kubectl -n devops-practice wait --for=condition=complete \
  job/devops-practice-smoke --timeout=3m
smoke_output=$(kubectl -n devops-practice logs job/devops-practice-smoke)
printf '%s\n' "$smoke_output" | tee smoke.log
echo "Smoke job completed"

./scripts/ci/check-ingress.sh
echo "Ingress health check passed"
