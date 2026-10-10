#!/bin/sh
set -eu

for variable in CI_REGISTRY CI_REGISTRY_IMAGE CI_REGISTRY_USER CI_REGISTRY_PASSWORD CI_COMMIT_SHA; do
  eval "value=\${$variable:-}"
  if [ -z "$value" ]; then
    echo "$variable is required" >&2
    exit 1
  fi
done
OCI_ARTIFACT=${OCI_ARTIFACT:-image.oci.tar}
TARGET_IMAGE=${TARGET_IMAGE:-$CI_REGISTRY_IMAGE}
IMAGE_ENV_FILE=${IMAGE_ENV_FILE:-image.env}
IMAGE_ENV_PREFIX=${IMAGE_ENV_PREFIX:-IMAGE}
case "$OCI_ARTIFACT:$IMAGE_ENV_FILE" in *[!a-zA-Z0-9_.:-]*|.*|*:.*) echo 'unsafe artifact path' >&2; exit 1 ;; esac
case "$IMAGE_ENV_PREFIX" in ''|*[!A-Z_]*) echo 'unsafe IMAGE_ENV_PREFIX' >&2; exit 1 ;; esac
if [ ! -s "$OCI_ARTIFACT" ]; then
  echo "OCI artifact is required" >&2
  exit 1
fi

printf '%s' "$CI_REGISTRY_PASSWORD" | skopeo login \
  --username "$CI_REGISTRY_USER" \
  --password-stdin \
  "$CI_REGISTRY"

sha_reference="$TARGET_IMAGE:$CI_COMMIT_SHA"
skopeo copy --all "oci-archive:$OCI_ARTIFACT" "docker://$sha_reference"

digest=$(skopeo inspect --format '{{.Digest}}' "docker://$sha_reference")
if ! printf '%s\n' "$digest" | grep -Eq '^sha256:[0-9a-f]{64}$'; then
  echo "published image digest is missing or invalid" >&2
  exit 1
fi

printf '%s_REFERENCE=%s\n%s_DIGEST=%s\n' "$IMAGE_ENV_PREFIX" "$sha_reference" "$IMAGE_ENV_PREFIX" "$digest" > "$IMAGE_ENV_FILE"
