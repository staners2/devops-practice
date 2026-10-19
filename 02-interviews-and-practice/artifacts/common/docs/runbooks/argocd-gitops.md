# Argo CD и GitOps-поставка

## Требования

Используй существующий context `k3d-devops-practice`, Helm `4.3.0` и kubectl
`1.35.8`. Версии Argo CD закреплены в
`deploy/platform/versions.env`: chart `10.9.6`, приложение `v3.5.3`.

Argo CD устанавливается вручную как platform component. Он не управляет своей
установкой в этом модуле.

## Установка Argo CD

```bash
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
mkdir -p .cache/charts
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update argo
helm pull argo/argo-cd --version 10.9.6 \
  --untar --untardir .cache/charts
helm upgrade --install argocd .cache/charts/argo-cd \
  --namespace argocd \
  -f deploy/platform/values/argocd.yaml \
  --wait --timeout 10m
```

Проверь необходимые workloads и отсутствие отключённых компонентов:

```bash
kubectl -n argocd get deploy,statefulset,pod,service
! kubectl -n argocd get deploy -o name | grep -Eq 'dex|notifications'
test "$(kubectl -n argocd get deployment argocd-applicationset-controller \
  -o jsonpath='{.spec.replicas}')" = 0
```

Dex, ApplicationSet и Notifications в этой лаборатории не используются.
High availability также отключена, чтобы стенд помещался на компьютере с
16 ГБ RAM.

## Локальный доступ к UI

Argo CD Server не публикуется через Ingress. Запусти port-forward:

```bash
kubectl -n argocd port-forward service/argocd-server 18081:443
```

Открой `https://localhost:18081`, используй пользователя `admin` и получи
начальный пароль во втором терминале:

```bash
argocd_password=$(kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath='{.data.password}' | base64 --decode)
printf 'Initial Argo CD password is loaded into argocd_password\n'
```

Введи значение в UI, не печатай его в отчёт и не сохраняй в Git. После входа:

```bash
unset argocd_password
```

Для CLI можно выполнить `argocd login localhost:18081 --username admin
--insecure` и ввести пароль интерактивно. Не передавай пароль прямо в
аргументах общей shell history.

## Bootstrap приложения и первый manual sync

До создания `Application` установи инфраструктуру и создай runtime Secret по
[`kubernetes-platform.md`](kubernetes-platform.md). Secret остаётся вне Git и
не управляется Argo CD.

Application принимает только публичный HTTPS repository на GitLab.com.
Отрендери URL в игнорируемую `.cache`, не меняя tracked manifests:

```bash
GITOPS_REPO_URL=$(git remote get-url origin)
case "$GITOPS_REPO_URL" in
  https://gitlab.com/*.git) ;;
  *) echo "public GitLab HTTPS origin is required" >&2; exit 1 ;;
esac
escaped_repo_url=$(printf '%s' "$GITOPS_REPO_URL" | sed 's/[&|]/\\&/g')
mkdir -p .cache/rendered/argocd
for manifest in project application; do
  sed "s|__GITOPS_REPO_URL__|$escaped_repo_url|g" \
    "deploy/argocd/$manifest.yaml" \
    > ".cache/rendered/argocd/$manifest.yaml"
done

kubectl apply -f .cache/rendered/argocd/project.yaml
awk '/^  syncPolicy:/{exit} {print}' \
  .cache/rendered/argocd/application.yaml \
  > .cache/rendered/argocd/application-manual.yaml
kubectl apply -f .cache/rendered/argocd/application-manual.yaml
```

Первый `Application` намеренно не содержит automated policy. Дождись статуса
`OutOfSync`, изучи diff и только затем запусти sync:

```bash
argocd app get devops-practice
argocd app diff devops-practice
argocd app sync devops-practice
argocd app wait devops-practice --sync --health --timeout 600
kubectl -n devops-practice get deploy,job,service,ingress
```

После успешного manual sync замени bootstrap-вариант полным tracked manifest:

```bash
kubectl apply -f .cache/rendered/argocd/application.yaml
kubectl -n argocd get application devops-practice \
  -o jsonpath='{.spec.syncPolicy.automated}{"\n"}'
```

Эта команда изменяет только объект `Application`. Workloads создаёт и
обновляет Argo CD reconciliation. Политика включает `selfHeal`, `prune` и
`allowEmpty: false`.

## GitLab setting и первый promotion

Пользователь с ролью Maintainer или Owner включает в GitLab:
`Settings → CI/CD → Job token permissions → Allow Git push requests to the
repository`. Не включай cross-project push. Постоянный personal/project token
для promotion не нужен.

Проведи первое изменение Go-кода или chart через обычный MR и после merge в
`main` проследи полный путь:

1. `publish-image` публикует image с tag commit SHA.
2. `promote-development` создаёт ветку и MR
   `GitOps: deploy <SHA> to development`.
3. Узкая pipeline с source `pipeline`, а затем MR pipeline валидируют GitOps.
4. Ментор вручную проверяет и merge'ит promotion MR.
5. Argo CD замечает новый desired state и выполняет sync.
6. `verify-development` подтверждает revision, image, rollout, smoke и
   ingress; только после этого GitLab регистрирует deployment.

Push с `CI_JOB_TOKEN` не запускает обычную pipeline, поэтому promotion job
явно вызывает validation pipeline. Повторный запуск для того же SHA использует
существующий открытый MR.

## Эксперимент: Manual sync и automated sync

Для отчёта сохрани вывод до и после первого ручного sync:

```bash
argocd app get devops-practice
argocd app diff devops-practice
argocd app sync devops-practice
argocd app wait devops-practice --sync --health --timeout 600
```

Затем внеси небольшое изменение через обычный MR, пройди promotion flow и не
вызывай `argocd app sync`. Покажи, что после merge Argo CD самостоятельно
перешёл в `Synced/Healthy`. В отчёте ответь:

- кто инициировал изменение в manual и automated режимах;
- где находился approval gate в каждом случае;
- почему CI не требуются права изменять workloads.

## Эксперимент: Push-based CD и pull-based GitOps

В Module 02 GitLab Runner выполнял `helm upgrade` и отправлял изменения в
кластер. В Module 03 Runner только публикует artifact и меняет desired state
через MR, а Argo CD внутри кластера периодически читает Git и делает reconcile.

Сравни в отчёте расположение desired state, инициатора deployment, права CI и
аудит approval. Не называй запуск Helm из CI GitOps только потому, что chart
хранится в Git.

## Эксперимент: Live hotfix и Git desired state

Создай drift и наблюдай self-heal:

```bash
kubectl -n devops-practice scale deployment/devops-practice-api --replicas=1
kubectl -n argocd get application devops-practice \
  -o jsonpath='{.status.sync.status}{" "}{.status.health.status}{"\n"}'
kubectl -n devops-practice wait --for=jsonpath='{.spec.replicas}'=2 \
  deployment/devops-practice-api --timeout=3m
```

Live-изменение исчезает, потому что Git по-прежнему требует две replicas.
Затем отдельным обычным MR измени `api.replicas` в
`deploy/environments/development/values.yaml` на `1`, пройди promotion и
покажи устойчивое состояние. Вторым MR верни `2`, чтобы финальная ветка не
содержала изменение эксперимента.

## Эксперимент: prune

Первым MR добавь в `deploy/helm/devops-practice/templates` безопасный
ConfigMap с именем `devops-practice-prune-demo`. После promotion и sync
проверь его наличие:

```bash
kubectl -n devops-practice get configmap devops-practice-prune-demo
```

Вторым MR удали template. Automated prune обязан удалить live resource:

```bash
kubectl -n devops-practice wait --for=delete \
  configmap/devops-practice-prune-demo --timeout=3m
```

Финальная ветка и итоговый render не должны содержать demo ConfigMap.

## Эксперимент: failed image и migration

В отдельном MR замени image tag в development values на несуществующий SHA.
После merge зафиксируй failed/degraded sync, events migration Job и успешный
health-check предыдущей версии:

```bash
argocd app get devops-practice
kubectl -n devops-practice get pod,job
curl --fail --show-error -H 'Host: devops-practice.localhost' \
  http://127.0.0.1:8080/api/health-check
```

Закрой MR до merge либо revert'ни сломанное изменение до рабочего immutable
tag. Не исправляй его через `kubectl set image`.

Ошибка migration воспроизводится только в учебной ветке: отдельным MR замени
`/usr/local/bin/migrate` на `/usr/local/bin/does-not-exist` в migration hook.
После failed sync покажи, что Deployments всё ещё используют предыдущий image
и health-check доступен. Сразу revert'ни изменение; сломанная команда не
должна остаться в финальной ветке.

## Helm rollback и Git revert

В automated режиме не используй `argocd app rollback`: self-heal снова
применит состояние из Git, а audit trail не объяснит желаемый rollback.
Возвращай предыдущий immutable image новым проверяемым MR:

```bash
git switch -c gitops/revert-development origin/main
git revert --no-edit "$promotion_merge_commit"
git push -u origin gitops/revert-development \
  -o merge_request.create \
  -o "merge_request.target=main" \
  -o "merge_request.title=GitOps: rollback development"
```

После review и merge дождись `Synced/Healthy`, проверь фактический image и
GitLab deployment. Git revert возвращает Kubernetes manifests, но не отменяет
уже зафиксированные изменения схемы PostgreSQL. Миграции должны оставаться
обратно совместимыми: `expand`, совместимый deployment, затем отдельный
`contract` после ухода старой версии.

## Доказательства для отчёта

Сохрани ссылки на application MR, promotion MR, обе pipelines, GitLab
deployment и Telegram-сообщение. Добавь вывод:

```bash
argocd app get devops-practice
argocd app history devops-practice
kubectl -n devops-practice get deployment \
  devops-practice-api devops-practice-worker \
  -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.spec.template.spec.containers[0].image}{"\n"}{end}'
```

Отдельно приложи результаты manual/automated sync, drift/self-heal, prune,
failed image или migration и rollback через Git revert. Секреты, tokens и
полные Kubernetes Secret objects в доказательства не включай.
