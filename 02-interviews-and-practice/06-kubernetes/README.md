# Kubernetes: углубление и базовая практика

Уровень 02. Изучай тему параллельно собеседованиям, после обзорной недели.

## Что изучить

- Control plane и worker: API server, etcd, scheduler, controller manager, kubelet — роли компонентов.
- Manifest, labels/selectors; Pod, Deployment/ReplicaSet, StatefulSet, DaemonSet, Job/CronJob.
- Service ClusterIP/NodePort/LoadBalancer; назначение Ingress и внутрикластерного DNS.
- ConfigMap, Secret; requests/limits; readiness/liveness; namespace, ServiceAccount и простое правило RBAC.
- Volume, PV, PVC, StorageClass; rollout, rollback и типовые состояния ошибок.
- Kubectl get/describe/logs/exec/apply/delete/port-forward; Helm chart, release, values и установка готового chart.

## Разбор типового вопроса

Running показывает состояние запуска, но не заменяет readiness. Service выбирает Pod’ы через labels, однако ошибочный selector даст отсутствие нужных backend-адресов. Сбой зависимой БД не стоит автоматически превращать в причину перезапуска всех контейнеров через liveness.

### Чекпоинт перед практикой

Как связаны Deployment, ReplicaSet, Pod и Service? Что проверять при Pending, ImagePullBackOff, CrashLoopBackOff и Running без Ready?

## Материалы

- [Kubernetes](../artifacts/questions/09-kubernetes.md).
- [Helm: chart, release и values](../artifacts/questions/10-helm.md).

Читай только перечисленные темы: полный справочник содержит и материал третьего уровня.

## Практика второго уровня

[01-basic](01-basic/README.md). Приложение в готовом кластере, Service, конфигурация, probes, ресурсы, rollout/rollback, PVC, диагностика и готовый Helm chart. Установка кластера не требуется.

Внутри задания отмечай отдельно наблюдаемый результат и понимание причины.

## Самопроверка после практики

1. Зачем requests, если есть limits?
2. Почему base64 не защищает Secret?
3. Почему port-forward не доказывает внешний доступ через Service?
4. Чем chart отличается от release?

## Необязательное продолжение

[Задания третьего уровня](../../03-advanced/06-kubernetes/README.md). Не являются условием начала собеседований или перехода к следующей теме.
