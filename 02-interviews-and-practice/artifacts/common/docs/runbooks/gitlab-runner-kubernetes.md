# GitLab Runner в Kubernetes

## Принятая модель безопасности

Лаборатория использует один project runner для MR, promotion и verification
jobs. Job pods получают ServiceAccount `gitlab-runner`. После перехода на
GitOps он только читает workloads в namespace `devops-practice` и status
одного `Application/devops-practice` в namespace `argocd`.

У Runner нет доступа к Secrets и verbs `create`, `update`, `patch` или
`delete`. Недоверенный код Merge Request всё ещё выполняется в общем кластере,
поэтому это учебное упрощение, а не production-изоляция.

## Установка

В GitLab открой `Settings → CI/CD → Runners`, создай project runner с tag
`k8s`, отключи `Run untagged jobs` и не включай protected-only mode. Скопируй
authentication token с префиксом `glrt-`.

```bash
kubectl apply -f deploy/platform/rbac/gitlab-runner-observer.yaml

printf 'GitLab runner authentication token: ' >&2
IFS= read -r -s RUNNER_TOKEN
printf '\n' >&2
kubectl -n gitlab-runner create secret generic gitlab-runner-auth \
  --from-literal=runner-token="$RUNNER_TOKEN" \
  --from-literal=runner-registration-token='' \
  --dry-run=client -o yaml | kubectl apply -f -
unset RUNNER_TOKEN

helm repo add gitlab https://charts.gitlab.io
helm repo update gitlab
helm pull gitlab/gitlab-runner --version 0.93.0 \
  --untar --untardir .cache/charts
helm upgrade --install gitlab-runner .cache/charts/gitlab-runner \
  --namespace gitlab-runner \
  -f deploy/platform/values/gitlab-runner.yaml \
  --wait --timeout 10m

kubectl -n gitlab-runner get deployment,pod,serviceaccount,role,rolebinding
```

Отдельный `KUBECONFIG` Runner'у не нужен. Не сохраняй ServiceAccount token или
kubeconfig в repository, cache и 02-interviews-and-practice/artifacts. После установки запусти MR
pipeline и убедись, что все jobs выполняются Kubernetes executor с tag `k8s`.
После перехода на GitOps проверь `publish-image`, `promote-development`,
`verify-development`, environment `development`, deployment и artifact
`smoke.log`. Полный flow настройки Job Token, promotion MR и Argo CD описан в
[`argocd-gitops.md`](argocd-gitops.md).

Проверь границы прав:

```bash
runner=system:serviceaccount:gitlab-runner:gitlab-runner
kubectl auth can-i get deployments -n devops-practice --as="$runner"
kubectl auth can-i get applications.argoproj.io/devops-practice \
  -n argocd --as="$runner"
kubectl auth can-i list applications.argoproj.io -n argocd --as="$runner"
kubectl auth can-i create deployments -n devops-practice --as="$runner"
kubectl auth can-i patch deployments -n devops-practice --as="$runner"
kubectl auth can-i get secrets -n devops-practice --as="$runner"
```

Ожидаемые ответы: `yes`, `yes`, затем четыре `no`.

При переходе с Module 02 сначала примени observer RBAC, проверь pipeline и
только затем удали прежнюю роль:

```bash
kubectl apply -f deploy/platform/rbac/gitlab-runner-observer.yaml
kubectl delete rolebinding gitlab-runner-deployer \
  -n devops-practice --ignore-not-found
kubectl delete role gitlab-runner-deployer \
  -n devops-practice --ignore-not-found
```

Runtime Secret при этом не удаляется.

## Переключение со старого runner

1. Установи Kubernetes Runner и дождись статуса online в GitLab.
2. Переведи все jobs на tag `k8s`.
3. Проверь MR pipeline, main pipeline и deployment.
4. Перенеси локальный `deploy/runner/.env` в `deploy/notifier/.env`.
5. Останови старый Compose stack.
6. Удали старую регистрацию runner в GitLab и отзови token.
7. Удали только volumes `devops-practice-runner-config` и
   `devops-practice-runner-cache`.

Подтверди отсутствие старой реализации в checkout:

```bash
test ! -e deploy/runner
! git grep -n 'devops-practice-runner-config\|devops-practice-runner-cache' \
  -- .gitlab-ci.yml Makefile deploy
git grep -n 'devops-practice-notifier-state\|deploy/notifier'
```

Последняя команда должна найти notifier: он остаётся отдельным Compose
компонентом и продолжает отправлять одно уведомление о новом deployment.

## Notifier

Volume `devops-practice-notifier-state` сохраняется, поэтому подтверждённые
события не отправляются повторно. При независимом старте создай его:

```bash
docker volume inspect devops-practice-notifier-state >/dev/null 2>&1 || \
  docker volume create devops-practice-notifier-state
cp deploy/notifier/env.example deploy/notifier/.env
make notifier-up
make notifier-logs
```

Не удаляй notifier volume при очистке старого runner.
