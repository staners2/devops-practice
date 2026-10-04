#!/bin/sh
set -eu

for variable in CI_REGISTRY CI_REGISTRY_IMAGE CI_REGISTRY_USER CI_REGISTRY_PASSWORD CI_COMMIT_SHA; do
  eval "value=\${$variable:-}"
  if [ -z "$value" ]; then
    echo "$variable is required" >&2
    exit 1
  fi
done
if [ ! -s image.oci.tar ]; then
  echo "image.oci.tar is required" >&2
  exit 1
fi

printf '%s' "$CI_REGISTRY_PASSWORD" | skopeo login \
  --username "$CI_REGISTRY_USER" \
  --password-stdin \
  "$CI_REGISTRY"

sha_reference="$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA"
skopeo copy --all "oci-archive:image.oci.tar" "docker://$sha_reference"
skopeo copy --all "docker://$sha_reference" "docker://$CI_REGISTRY_IMAGE:main"

digest=$(skopeo inspect --format '{{.Digest}}' "docker://$sha_reference")
if ! printf '%s\n' "$digest" | grep -Eq '^sha256:[0-9a-f]{64}$'; then
  echo "published image digest is missing or invalid" >&2
  exit 1
fi

printf 'IMAGE_REFERENCE=%s\nIMAGE_DIGEST=%s\n' "$sha_reference" "$digest" > image.env
