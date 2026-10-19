#!/bin/sh
set -eu
cd "$(dirname "$0")/../../.."
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
for invalid in 'replicaCount=2' 'patroni.ttl=15' 'image.tag=latest' 'auth.existingSecret=BAD_NAME'; do
  case "$invalid" in image.*|auth.*) set_flag=--set-string ;; *) set_flag=--set ;; esac
  if helm template postgresql-patroni deploy/helm/postgresql-patroni --namespace platform \
    --set-string image.tag=0000000000000000000000000000000000000000 \
    "$set_flag" "$invalid" > "$temporary/rendered" 2> "$temporary/error"; then
    echo "Chart accepted invalid values: $invalid" >&2; exit 1
  fi
  test -s "$temporary/error"
done
if helm template postgresql-patroni deploy/helm/postgresql-patroni --namespace platform \
  > "$temporary/rendered" 2> "$temporary/error"; then
  echo 'Chart accepted unpublished empty image tag' >&2; exit 1
fi
printf 'Invalid Patroni values rejected\n'
