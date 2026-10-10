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

BUILD_CONTEXT=${BUILD_CONTEXT:-.}
DOCKERFILE_DIR=${DOCKERFILE_DIR:-.}
OCI_ARTIFACT=${OCI_ARTIFACT:-image.oci.tar}
TARGET_IMAGE=${TARGET_IMAGE:-$CI_REGISTRY_IMAGE}
case "$OCI_ARTIFACT" in ''|*[!a-zA-Z0-9_.-]*|.*) echo 'unsafe OCI_ARTIFACT' >&2; exit 1 ;; esac
[ -d "$BUILD_CONTEXT" ] && [ -f "$DOCKERFILE_DIR/Dockerfile" ] || { echo 'build context/Dockerfile missing' >&2; exit 1; }
set --
if [ -n "${BUILD_PLATFORMS:-}" ]; then
  set -- --opt "platform=$BUILD_PLATFORMS"
fi

buildctl-daemonless.sh build \
  --frontend dockerfile.v0 \
  --local "context=$BUILD_CONTEXT" \
  --local "dockerfile=$DOCKERFILE_DIR" \
  --opt "build-arg:VCS_REF=$CI_COMMIT_SHA" \
  --opt "build-arg:SOURCE_URL=$CI_PROJECT_URL" \
  --output "type=oci,dest=$OCI_ARTIFACT,name=$TARGET_IMAGE:$CI_COMMIT_SHA" \
  "$@" \
  --progress plain
