#!/bin/sh
set -eu
repo_root=$(CDPATH='' cd -- "$(dirname "$0")/../../.." && pwd)
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
mkdir "$temporary/bin"
ln -s "$repo_root/scripts/ci/tests/image-client-stub.sh" "$temporary/bin/buildctl-daemonless.sh"
ln -s "$repo_root/scripts/ci/tests/image-client-stub.sh" "$temporary/bin/skopeo"
export PATH="$temporary/bin:$PATH"
export IMAGE_TEST_LOG="$temporary/tools.log"
export CI_COMMIT_SHA=1111111111111111111111111111111111111111
export CI_PROJECT_URL=https://gitlab.com/training/devops-practice
export CI_REGISTRY=registry.gitlab.com CI_REGISTRY_IMAGE=registry.gitlab.com/training/devops-practice
export CI_REGISTRY_USER=ci-test CI_REGISTRY_PASSWORD=image_fixture_secret
cd "$repo_root"
sh scripts/ci/build-image.sh > "$temporary/build.log" 2>&1
grep -F 'type=oci,dest=image.oci.tar,name=registry.gitlab.com/training/devops-practice:' "$IMAGE_TEST_LOG" >/dev/null
BUILD_CONTEXT=containers/patroni DOCKERFILE_DIR=containers/patroni \
  OCI_ARTIFACT=patroni-image.oci.tar TARGET_IMAGE="$CI_REGISTRY_IMAGE/patroni" \
  BUILD_PLATFORMS=linux/amd64,linux/arm64 sh scripts/ci/build-image.sh > "$temporary/build-patroni.log" 2>&1
grep -F 'context=containers/patroni' "$IMAGE_TEST_LOG" >/dev/null
grep -F 'platform=linux/amd64,linux/arm64' "$IMAGE_TEST_LOG" >/dev/null
if DOCKER_HOST=tcp://unexpected:2375 sh scripts/ci/build-image.sh > "$temporary/rejected.log" 2>&1; then
  echo 'Build accepted Docker daemon' >&2; exit 1
fi
if OCI_ARTIFACT=../bad.tar sh scripts/ci/build-image.sh > "$temporary/rejected.log" 2>&1; then
  echo 'Build accepted unsafe artifact path' >&2; exit 1
fi
cd "$temporary"
printf 'oci fixture\n' > image.oci.tar
printf 'patroni oci fixture\n' > patroni-image.oci.tar
sh "$repo_root/scripts/ci/split-patroni-images.sh" > split.log 2>&1
test -s patroni-amd64.tar && test -s patroni-arm64.tar
if IMAGE_TEST_ARCH=unexpected sh "$repo_root/scripts/ci/split-patroni-images.sh" > rejected.log 2>&1; then
  echo 'Split accepted wrong image architecture' >&2; exit 1
fi
sh "$repo_root/scripts/ci/publish-image.sh" > publish.log 2>&1
OCI_ARTIFACT=patroni-image.oci.tar TARGET_IMAGE="$CI_REGISTRY_IMAGE/patroni" \
  IMAGE_ENV_FILE=patroni-image.env IMAGE_ENV_PREFIX=PATRONI_IMAGE \
  sh "$repo_root/scripts/ci/publish-image.sh" > publish-patroni.log 2>&1
grep -F "IMAGE_REFERENCE=$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA" image.env >/dev/null
grep -F "PATRONI_IMAGE_REFERENCE=$CI_REGISTRY_IMAGE/patroni:$CI_COMMIT_SHA" patroni-image.env >/dev/null
grep -F 'PATRONI_IMAGE_DIGEST=sha256:' patroni-image.env >/dev/null
test "$(wc -l < image.env | tr -d ' ')" = 2
if IMAGE_TEST_DIGEST=invalid IMAGE_ENV_FILE=bad.env sh "$repo_root/scripts/ci/publish-image.sh" > rejected.log 2>&1; then
  echo 'Publish accepted invalid digest' >&2; exit 1
fi
test ! -e bad.env
if IMAGE_TEST_FAIL_COPY=1 IMAGE_ENV_FILE=failed.env sh "$repo_root/scripts/ci/publish-image.sh" > rejected.log 2>&1; then
  echo 'Publish ignored failed copy' >&2; exit 1
fi
test ! -e failed.env
if grep -Fq "$CI_REGISTRY_PASSWORD" "$IMAGE_TEST_LOG" ./*.log ./*.env; then
  echo 'Image scripts exposed a credential' >&2; exit 1
fi
printf 'Application and Patroni image artifact contracts passed\n'
