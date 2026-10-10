#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../../.." && pwd)
validator=$repo_root/scripts/ci/validate-gitops.sh
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

if GITOPS_REPO_URL=ssh://gitlab.com/training/devops-practice.git \
  "$validator" >"$test_dir/invalid.log" 2>&1; then
  echo "validator accepted a non-HTTPS GitLab repository" >&2
  exit 1
fi
grep -F 'GITOPS_REPO_URL must be a public GitLab HTTPS URL ending in .git' \
  "$test_dir/invalid.log"

cd "$repo_root"
GITOPS_REPO_URL=https://gitlab.com/training/devops-practice.git \
  "$validator" >"$test_dir/valid.log" 2>&1

test -s .cache/rendered/argocd/project.yaml
test -s .cache/rendered/argocd/application.yaml
! grep -R '__GITOPS_REPO_URL__' .cache/rendered/argocd
test "$(grep -R -F 'https://gitlab.com/training/devops-practice.git' \
  .cache/rendered/argocd | wc -l | tr -d ' ')" -eq 5
grep -F 'GitOps manifests are valid' "$test_dir/valid.log"
