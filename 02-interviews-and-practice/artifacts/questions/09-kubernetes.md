# Блок 9. Kubernetes

Цель блока — проверить понимание декларативного orchestration, reconciliation, сетевой модели, probes, ресурсов и базовой диагностики.

## 1. Блок вопросов

1. Что такое Kubernetes и чем он принципиально отличается от Docker Compose?

   **Ответ (пояснение):** Kubernetes — кластерный control plane, постоянно согласующий желаемое и фактическое состояние workloads. Compose в основном описывает запуск связанных контейнеров на одном Docker-окружении и не предоставляет такой модели контроллеров и кластера.

2. Какие компоненты входят в control plane и worker node?

   **Ответ (пояснение):** Control plane включает API server, etcd, scheduler и controller manager. Worker обычно запускает kubelet, container runtime и сетевые компоненты; конкретная дистрибуция может упаковывать их иначе.

3. Что такое controller loop и self-healing?

   **Ответ (пояснение):** Контроллер наблюдает фактическое состояние через API, сравнивает его с spec и повторно выполняет согласование. Self-healing — результат этого цикла: замена Pod или восстановление replicas, но не автоматическое исправление любой ошибки приложения.

4. Как устроен manifest: `apiVersion`, `kind`, `metadata`, `spec`, `status`?

   **Ответ (пояснение):** `apiVersion` выбирает API, `kind` — тип ресурса, `metadata` — имя, namespace и метки, `spec` — желаемое состояние, `status` — наблюдаемое состояние, которое обычно обновляет контроллер.

5. Как связаны labels, selectors и Service?

   **Ответ (пояснение):** Labels помечают объекты, selector выбирает набор по этим меткам. Service создаёт стабильную точку доступа и направляет трафик на Ready Pods, чьи labels соответствуют его selector.

6. Почему Pod не равен контейнеру?

   **Ответ (пояснение):** Pod — минимальная единица планирования Kubernetes, содержащая один или несколько контейнеров с общей сетью, localhost, volumes и жизненным циклом. Контейнер — лишь один процессуальный runtime-компонент Pod.

7. Зачем Deployment и ReplicaSet? Как работают rollout и rollback?

   **Ответ (пояснение):** Deployment управляет версиями Pod template и ReplicaSets, ReplicaSet поддерживает число реплик. Rollout постепенно переводит replicas на новый template, rollback создаёт новое согласование к ранее сохранённой revision.

8. Когда нужны StatefulSet, DaemonSet, Job и CronJob?

   **Ответ (пояснение):** StatefulSet даёт стабильные идентичности и storage, DaemonSet запускает Pod на подходящих nodes, Job выполняет конечную работу до успеха, CronJob создаёт Jobs по расписанию.

9. Чем ConfigMap отличается от Secret и почему Secret не полностью безопасен?

   **Ответ (пояснение):** ConfigMap хранит несекретную конфигурацию, Secret — чувствительные bytes и имеет отдельные механизмы доступа. Base64 не является шифрованием; безопасность требует RBAC, encryption at rest, ограничения выдачи и внешнего secret management при необходимости.

10. Чем ClusterIP, NodePort и LoadBalancer отличаются?

    **Ответ (пояснение):** ClusterIP доступен внутри кластера, NodePort открывает порт на nodes, LoadBalancer запрашивает внешний балансировщик у поддерживаемой инфраструктуры. Все типы обычно используют Service selector и endpoints.

11. Почему Ingress без Ingress Controller не работает?

    **Ответ (пояснение):** Ingress — декларативный объект правил, а не data plane. Controller должен наблюдать эти объекты и настраивать реальный proxy/load balancer; дополнительно важны IngressClass и поддерживаемые annotations.

12. Чем readiness, liveness и startup probes отличаются?

    **Ответ (пояснение):** Readiness управляет включением Pod в трафик, liveness инициирует перезапуск зависшего контейнера, startup даёт медленному приложению время стартовать и до успеха подавляет остальные проверки.

13. Чем requests отличаются от limits?

    **Ответ (пояснение):** Requests используются scheduler и гарантированным выделением ресурсов; limits ограничивают максимальное потребление. CPU сверх limit throttling, превышение memory limit может привести к OOM kill.

14. Для чего нужны namespace, affinity, taints и tolerations?

    **Ответ (пояснение):** Namespace логически группирует namespaced resources, affinity влияет на размещение относительно nodes и Pods, taint отталкивает Pods от node, toleration разрешает планирование на node с соответствующим taint, но не гарантирует его.

15. Как связаны ServiceAccount, Role и RoleBinding?

    **Ответ (пояснение):** ServiceAccount задаёт identity workload, Role перечисляет разрешённые действия внутри namespace, RoleBinding выдаёт эту Role субъекту. Для кластерной области применяют ClusterRole и ClusterRoleBinding.

## 2. Объяснение тем и вопросов

Kubernetes принимает желаемое состояние через API и постоянно сверяет его с наблюдаемым. API server — входная точка, etcd хранит состояние, scheduler выбирает node, controller-manager запускает reconciliation loops. На worker kubelet управляет Pod через container runtime, сетевые компоненты реализуют доступ, CoreDNS — service discovery.

### Объекты и workloads

Manifest содержит желаемый `spec`; контроллер обновляет наблюдаемый `status`. Labels идентифицируют объекты, selectors выбирают их. Ошибка selector часто создаёт Service без endpoints.

Pod — минимальная единица размещения: один или несколько контейнеров с общей сетевой средой и volumes. Deployment управляет ReplicaSet и stateless Pods, поддерживает масштабирование и rolling update. StatefulSet добавляет стабильные идентичности и storage-contract, но не делает приложение автоматически отказоустойчивым. DaemonSet размещает Pod на подходящих nodes, Job выполняет конечную работу, CronJob создаёт Jobs по расписанию.

`Pending` означает, что Pod ещё не запущен; `Running` не равен `Ready`. `CrashLoopBackOff` — повторные падения с backoff. `ImagePullBackOff` — повторные неудачи загрузки image; причину ищут в events.

### Конфигурация, сеть и probes

ConfigMap хранит несекретную конфигурацию, Secret — чувствительные данные как объект API. Base64 не является шифрованием; нужны RBAC, encryption at rest и безопасная доставка.

Service даёт стабильный виртуальный адрес и выбирает Pods. `port` — порт Service, `targetPort` — порт Pod, `nodePort` — порт node. ClusterIP доступен внутри кластера, NodePort публикуется на nodes, LoadBalancer просит внешнюю интеграцию создать балансировщик. Ingress — правила HTTP(S), а Controller реализует их.

Readiness исключает Pod из трафика, liveness инициирует перезапуск зависшего контейнера, startup защищает медленный старт от преждевременной liveness. Liveness не должна зависеть от временной недоступности внешней БД.

### Ресурсы, размещение и доступ

Requests используются scheduler-ом и для гарантируемой доли ресурса; limits ограничивают потребление. Memory limit может привести к OOMKill, CPU limit — к throttling. Affinity и nodeSelector выбирают размещение, taint отталкивает Pod, toleration разрешает размещение, но не гарантирует его.

ServiceAccount задаёт идентичность workload. Role описывает permissions в namespace, RoleBinding выдаёт их субъекту. Следует придерживаться least privilege.

### Диагностика

Базовая цепочка: `kubectl get` → `describe` → events → `logs` текущего/предыдущего контейнера → проверка manifest/config/endpoints → `exec` или `port-forward` для локализации. Удаление Pod до сбора фактов уничтожает часть доказательств.

## 3. Вопросы для проверки знаний (с ответами)

| Вопрос | Эталонный ответ |
|---|---|
| Зачем Deployment, если можно создать Pod? | Deployment поддерживает число replicas, rollout, rollback и восстановление через ReplicaSet; одиночный Pod этого контракта не даёт. |
| Service есть, но endpoints пусты. Что проверить? | Совпадение selector с labels, Ready-state Pods, namespace и port mapping. |
| Pod Running, но не Ready. Возможна ли недоступность? | Да. Процесс запущен, но readiness не прошла, поэтому Service не должен направлять трафик. |
| Почему liveness не должна проверять внешнюю БД? | Временный сбой БД вызовет бессмысленные рестарты всех приложений и усилит инцидент. |
| Чем requests отличаются от limits? | Requests участвуют в scheduling/гарантиях, limits ограничивают максимальное потребление. |
| Что означает OOMKilled? | Контейнер превысил доступную память cgroup либо был выбран при нехватке; проверяют limits и реальное потребление. |
| Почему Secret не безопасен только из-за base64? | Base64 легко декодируется; защиту дают доступ, шифрование хранения и безопасная доставка. |
| Toleration заставляет Pod попасть на tainted node? | Нет. Она лишь разрешает размещение; выбор всё равно делает scheduler по остальным условиям. |
| Что проверять при ImagePullBackOff? | Имя/tag/digest, registry, imagePullSecrets, права, сеть/DNS и events. |
| Чем Kubernetes отличается от Compose? | Kubernetes — распределённый control plane с reconciliation, scheduling и cluster abstractions; Compose управляет приложением на Docker-окружении без такого control plane. |

Практика: дайте симптомы `Pending`, `CrashLoopBackOff`, `Running 0/1` и Service без endpoints. Менти должен выбрать команды и назвать гипотезы до изменения ресурсов.

Пошаговые задания для менти: [CNI, Service и MetalLB, хранение данных, RBAC, NetworkPolicy и DaemonSet](../../../03-advanced/06-kubernetes/03-network-storage-access/README.md).
