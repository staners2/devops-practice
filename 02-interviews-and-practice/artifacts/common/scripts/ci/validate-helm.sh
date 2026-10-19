#!/bin/sh
set -eu

for command in helm kubeconform kube-linter; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "$command is required" >&2
    exit 1
  fi
done

chart=deploy/helm/devops-practice
rendered=.cache/rendered/devops-practice.yaml
mkdir -p "$(dirname "$rendered")"

helm lint "$chart" --strict
helm template devops-practice "$chart" --namespace devops-practice > "$rendered"
kubeconform -strict -summary -kubernetes-version 1.35.0 "$rendered"
kube-linter lint "$rendered"

if grep -Eq 'namespace:[[:space:]]+(default|devops-practice)' "$rendered"; then
  echo "chart must not hardcode a namespace" >&2
  exit 1
fi
if grep -Eq 'image:[[:space:]].*:latest([[:space:]]|$)' "$rendered"; then
  echo "chart must not use latest image tag" >&2
  exit 1
fi
