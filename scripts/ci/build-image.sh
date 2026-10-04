#!/bin/sh
set -eu

for variable in CI_COMMIT_SHA CI_PROJECT_URL CI_REGISTRY_IMAGE; do
  eval "value=\${$variable:-}"
  if [ -z "$value" ]; then
    echo "$variable is required" >&2
    exit 1
  fi
done

if [ -n "${DOCKER_HOST:-}" ]; then
  echo "DOCKER_HOST must not be set for rootless BuildKit" >&2
  exit 1
fi
if [ -e /var/run/docker.sock ]; then
  echo "Docker socket must not be available to build job" >&2
  exit 1
fi

buildctl-daemonless.sh build \
  --frontend dockerfile.v0 \
  --local context=. \
  --local dockerfile=. \
  --opt "build-arg:VCS_REF=$CI_COMMIT_SHA" \
  --opt "build-arg:SOURCE_URL=$CI_PROJECT_URL" \
  --output "type=oci,dest=image.oci.tar,name=$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA" \
  --progress plain
