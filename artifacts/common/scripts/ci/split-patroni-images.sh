#!/bin/sh
set -eu
test -s patroni-image.oci.tar || { echo 'Patroni OCI index artifact is required' >&2; exit 1; }
# Trivy archive input does not select a child manifest using --platform.
for architecture in amd64 arm64; do
  archive="patroni-$architecture.tar"
  skopeo copy --override-os linux --override-arch "$architecture" \
    oci-archive:patroni-image.oci.tar "docker-archive:$archive"
  actual=$(skopeo inspect --format '{{.Architecture}}' "docker-archive:$archive")
  [ "$actual" = "$architecture" ] || { echo 'Selected image architecture does not match' >&2; exit 1; }
done
