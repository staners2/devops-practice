#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
revision=$(git -C "$repo_root" rev-parse HEAD)

for release in v1 v2 failed; do
  image="devops-practice:lab-07-$release"
  if docker image inspect "$image" >/dev/null 2>&1; then
    label=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' "$image")
    if [[ "$label" != "lab-07-$release" ]]; then
      printf 'Образ %s уже существует с другим release label; не перезаписываем его.\n' "$image" >&2
      exit 1
    fi
    continue
  fi
  if [[ "$release" == failed ]]; then
    docker build --file "$repo_root/deploy/ansible-lab/Dockerfile.failed" \
      --build-arg BASE_IMAGE=devops-practice:lab-07-v1 \
      --label "org.opencontainers.image.version=lab-07-$release" \
      --tag "$image" "$repo_root"
  else
    docker build --file "$repo_root/Dockerfile" --target runtime \
      --build-arg "VCS_REF=$revision" \
      --label "org.opencontainers.image.version=lab-07-$release" \
      --tag "$image" "$repo_root"
  fi
done

v1=$(docker image inspect --format '{{.Id}}' devops-practice:lab-07-v1)
v2=$(docker image inspect --format '{{.Id}}' devops-practice:lab-07-v2)
[[ "$v1" != "$v2" ]] || { printf 'IDs релизов совпадают.\n' >&2; exit 1; }
printf 'v1: %s\nv2: %s\nГотовы два релиза и образ failed без API/worker.\n' "$v1" "$v2"
