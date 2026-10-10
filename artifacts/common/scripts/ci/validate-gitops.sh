#!/bin/sh
set -eu

for command in helm kubeconform kube-linter; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "$command is required" >&2
    exit 1
  fi
done

repo_url=${GITOPS_REPO_URL:-}
case "$repo_url" in
  https://gitlab.com/*.git) ;;
  *)
    echo "GITOPS_REPO_URL must be a public GitLab HTTPS URL ending in .git" >&2
    exit 1
    ;;
esac
if printf '%s' "$repo_url" | grep -q '[[:space:]]'; then
  echo "GITOPS_REPO_URL must not contain whitespace" >&2
  exit 1
fi

argocd_dir=.cache/rendered/argocd
application_rendered=.cache/rendered/devops-practice-gitops.yaml
mkdir -p "$argocd_dir" "$(dirname "$application_rendered")"

escaped_repo_url=$(printf '%s' "$repo_url" | sed 's/[&|]/\\&/g')
for manifest in project application restore-application; do
  source_file="deploy/argocd/$manifest.yaml"
  rendered_file="$argocd_dir/$manifest.yaml"
  sed "s|__GITOPS_REPO_URL__|$escaped_repo_url|g" \
    "$source_file" > "$rendered_file"
done

if grep -R -q '__GITOPS_REPO_URL__' "$argocd_dir"; then
  echo "repository token remains in rendered Argo CD manifests" >&2
  exit 1
fi
repo_url_count=$(grep -R -F "$repo_url" "$argocd_dir" | wc -l | tr -d ' ')
if [ "$repo_url_count" -ne 5 ]; then
  echo "Argo CD manifests must contain the same repository URL five times" >&2
  exit 1
fi

application=$argocd_dir/application.yaml
for policy in 'enabled: true' 'prune: true' 'selfHeal: true' 'allowEmpty: false'; do
  if ! grep -Fq "$policy" "$application"; then
    echo "Application is missing sync policy: $policy" >&2
    exit 1
  fi
done
if grep -Eq '^[[:space:]]+parameters:' "$application"; then
  echo "Argo CD Helm parameter overrides are forbidden" >&2
  exit 1
fi
if grep -Eq "^[[:space:]]+tag:[[:space:]]*['\"]?latest['\"]?[[:space:]]*$" \
  deploy/environments/development/values.yaml; then
  echo "latest image tag is forbidden" >&2
  exit 1
fi
if grep -R -q 'argocd.argoproj.io/hook' deploy/helm/devops-practice/templates; then
  echo "Argo CD hooks must not be mixed with Helm hooks" >&2
  exit 1
fi
grep -Fq 'helm.sh/hook: pre-install,pre-upgrade' \
  deploy/helm/devops-practice/templates/migration-job.yaml
grep -Fq 'helm.sh/hook: post-install,post-upgrade' \
  deploy/helm/devops-practice/templates/smoke-job.yaml

helm lint deploy/helm/devops-practice \
  -f deploy/environments/development/values.yaml
helm template devops-practice deploy/helm/devops-practice \
  --namespace devops-practice \
  -f deploy/environments/development/values.yaml \
  > "$application_rendered"
kubeconform -strict -summary -kubernetes-version 1.35.0 \
  "$application_rendered"
kube-linter lint "$application_rendered"
helm template devops-practice-restore deploy/helm/devops-practice \
  --namespace devops-practice -f deploy/environments/development/values.yaml \
  -f deploy/environments/restore/values.yaml > .cache/rendered/devops-practice-restore.yaml
kubeconform -strict -summary -kubernetes-version 1.35.0 .cache/rendered/devops-practice-restore.yaml
kube-linter lint .cache/rendered/devops-practice-restore.yaml
if grep -q 'kind: Ingress' .cache/rendered/devops-practice-restore.yaml; then
  echo 'restore release must have no Ingress' >&2; exit 1
fi
helm template devops-practice deploy/helm/devops-practice \
  --namespace devops-practice -f deploy/environments/development/values.yaml \
  -f deploy/environments/maintenance/values.yaml > .cache/rendered/devops-practice-maintenance.yaml
if grep -q 'kind: Job' .cache/rendered/devops-practice-maintenance.yaml; then
  echo 'maintenance must disable migration and smoke hooks' >&2; exit 1
fi
kubeconform -strict -summary -kubernetes-version 1.35.0 .cache/rendered/devops-practice-maintenance.yaml
kubeconform -strict -summary -ignore-missing-schemas \
  "$argocd_dir/project.yaml" \
  "$argocd_dir/application.yaml" \
  "$argocd_dir/restore-application.yaml"

echo "GitOps manifests are valid"
