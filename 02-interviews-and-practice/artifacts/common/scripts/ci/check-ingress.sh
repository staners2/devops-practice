#!/bin/sh
set -eu

url=http://ingress-nginx-controller.ingress-nginx.svc.cluster.local/api/health-check
attempt=1
while [ "$attempt" -le 60 ]; do
  response=$(curl --fail --silent --show-error \
    -H 'Host: devops-practice.localhost' "$url" 2>/dev/null || true)
  if printf '%s' "$response" | grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"'; then
    printf '%s\n' "$response"
    exit 0
  fi
  sleep 1
  attempt=$((attempt + 1))
done

echo "ingress route did not become healthy" >&2
exit 1
