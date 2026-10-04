#!/usr/bin/env bash
set -euo pipefail

compose() {
  docker compose --env-file deploy/runner/.env -f deploy/runner/compose.yaml "$@"
}

gitlab_url=${GITLAB_URL:-https://gitlab.com}
runner_description=${RUNNER_DESCRIPTION:-devops-practice-local}

if [ -z "${GITLAB_RUNNER_TOKEN:-}" ]; then
  if [ ! -t 0 ]; then
    echo "GITLAB_RUNNER_TOKEN is required in non-interactive mode" >&2
    exit 1
  fi
  read -r -s -p 'GitLab runner authentication token: ' GITLAB_RUNNER_TOKEN
  printf '\n' >&2
fi

case "$GITLAB_RUNNER_TOKEN" in
  glrt-*) ;;
  *)
    echo "runner authentication token must start with glrt-" >&2
    exit 1
    ;;
esac

export CI_SERVER_URL=$gitlab_url
export RUNNER_TOKEN=$GITLAB_RUNNER_TOKEN

compose exec -T \
  -e CI_SERVER_URL \
  -e RUNNER_TOKEN \
  runner gitlab-runner register \
  --non-interactive \
  --executor docker \
  --description "$runner_description" \
  --docker-image "golang:1.26.4-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648" \
  --docker-privileged=false \
  --docker-volumes "devops-practice-runner-cache:/cache" \
  --request-concurrency 1

compose exec -T runner sed -i 's/^concurrent = .*/concurrent = 1/' /etc/gitlab-runner/config.toml

config=$(compose exec -T runner cat /etc/gitlab-runner/config.toml)
printf '%s\n' "$config" | grep -Fq 'executor = "docker"'
printf '%s\n' "$config" | grep -Fq 'privileged = false'
printf '%s\n' "$config" | grep -Fq 'request_concurrency = 1'
if printf '%s\n' "$config" | grep -Fq '/var/run/docker.sock'; then
  echo "job configuration must not mount Docker socket" >&2
  exit 1
fi

unset GITLAB_RUNNER_TOKEN RUNNER_TOKEN
echo "Runner registered with Docker executor and concurrent=1"
