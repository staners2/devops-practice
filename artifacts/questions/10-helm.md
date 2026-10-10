# Блок 10. Helm

Цель блока — проверить понимание шаблонизации Kubernetes-ресурсов и жизненного цикла Helm release.

## 1. Блок вопросов

1. Какую проблему решает Helm?
   **Ответ (пояснение):** Helm шаблонизирует и версионирует набор Kubernetes-ресурсов как chart, объединяет их в release и хранит revisions. Он уменьшает дублирование конфигурации, но не заменяет понимание отрендеренных manifests.
2. Чем chart, release и revision отличаются?
   **Ответ (пояснение):** Chart — пакет шаблонов и defaults, release — установленный экземпляр chart в кластере с именем и values, revision — конкретная версия состояния release после install, upgrade или rollback.
3. Чем chart отличается от набора готовых Kubernetes manifests?
   **Ответ (пояснение):** Готовые manifests уже содержат конкретные ресурсы. Chart содержит templates, values и метаданные, из которых Helm сначала рендерит manifests для выбранной конфигурации.
4. Для чего нужны `Chart.yaml`, `values.yaml`, `templates/` и `.helmignore`?
   **Ответ (пояснение):** `Chart.yaml` описывает chart, `values.yaml` задаёт defaults, `templates/` содержит шаблоны ресурсов, `.helmignore` исключает файлы из упакованного chart.
5. Для чего применяют `_helpers.tpl` и `NOTES.txt`?
   **Ответ (пояснение):** `_helpers.tpl` хранит именованные переиспользуемые templates, например labels и names. `NOTES.txt` рендерит подсказку пользователю после install/upgrade и не создаёт Kubernetes-ресурс.
6. Как значения из `values.yaml` попадают в template?
   **Ответ (пояснение):** Helm объединяет defaults chart с переданными values по правилам приоритета и предоставляет результат через `.Values`. Template-функции преобразуют значения, но обязательность и типы нужно проверять явно.
7. Что делают `helm create`, `lint` и `template`?
   **Ответ (пояснение):** `create` генерирует стартовый chart, `lint` проверяет структуру и типовые ошибки, `template` локально рендерит YAML. Серверную совместимость дополнительно проверяют schema validation или dry-run против нужного кластера.
8. Чем `install` отличается от `upgrade`?
   **Ответ (пояснение):** `install` создаёт новый release, `upgrade` изменяет существующий и создаёт следующую revision. `upgrade --install` выбирает действие по наличию release и удобен для идемпотентной доставки.
9. Что делает `rollback` и создаёт ли он новую revision?
   **Ответ (пояснение):** Rollback берёт manifest и values выбранной прошлой revision, применяет их как текущее состояние и записывает новую revision. История не перематывается и не удаляется.
10. Как использовать `list`, `status`, `history` и `get` при диагностике?
    **Ответ (пояснение):** `list` находит release, `status` показывает текущее состояние и ресурсы, `history` — revisions, `get values/manifest/hooks` — фактическую сохранённую конфигурацию и рендеринг конкретного release.
11. Почему отрендеренный manifest нужно проверять до применения?
    **Ответ (пояснение):** Ошибка values или шаблона может создать валидный YAML с опасной семантикой: неверным namespace, selector, правами или удалением ресурса. Проверка render/diff показывает реальные объекты до изменения кластера.

## 2. Объяснение тем и вопросов

Chart — версионируемый пакет templates, default values и metadata. Release — установленный экземпляр chart в Kubernetes с именем и namespace. Revision — номер изменения release при install, upgrade или rollback.

Обычный manifest содержит готовые значения. Helm template генерирует manifests из chart и values, поэтому один chart можно применять к разным окружениям. Это повышает переиспользование, но добавляет слой вычисления: ошибку нужно искать и в values, и в шаблоне, и в итоговом YAML.

### Структура chart

- `Chart.yaml` — metadata и зависимости chart.
- `values.yaml` — значения по умолчанию.
- `templates/` — шаблоны Kubernetes-объектов.
- `_helpers.tpl` — переиспользуемые именованные templates.
- `NOTES.txt` — сообщение после install/upgrade.
- `.helmignore` — исключения из пакета chart.

Значения доступны через `.Values`. Обязательные значения полезно проверять функцией `required`; для YAML-фрагментов применяют корректный отступ через `toYaml` и `nindent`.

### Команды и release lifecycle

`helm lint` выполняет статические проверки chart. `helm template` рендерит локально. Перед deployment полезно сочетать их с валидацией итоговых Kubernetes manifests.

`helm install` создаёт release. `helm upgrade` изменяет существующий release на основе нового chart/values; `--install` позволяет создать его при отсутствии. `helm rollback` возвращает содержимое к выбранной revision и фиксирует новое изменение истории. `uninstall` удаляет release-ресурсы, но поведение persistent data зависит от объектов и retention annotations/policies.

`helm get values` показывает значения release, `helm get manifest` — сохранённый manifest, `history` — revisions, `status` — состояние. Проверять нужно правильные namespace и release name.

## 3. Вопросы для проверки знаний (с ответами)

| Вопрос | Эталонный ответ |
|---|---|
| Можно ли установить один chart несколько раз? | Да, как разные releases с разными именами/namespaces/values. |
| Почему изменение `values.yaml` само не меняет кластер? | Нужно выполнить upgrade или GitOps-синхронизацию; файл лишь вход для rendering. |
| Что показывает `helm template`? | Итоговые manifests после локального rendering; он не доказывает успешность admission/runtime в кластере. |
| Чем install отличается от upgrade? | Install создаёт новый release, upgrade создаёт новую revision существующего. |
| Rollback стирает историю? | Нет. Он применяет состояние выбранной revision как новое изменение истории release. |
| Почему нельзя смотреть только исходный template? | Реальная ошибка может появиться после подстановки values; проверяют отрендеренный YAML. |
| Где найти values работающего release? | `helm get values RELEASE -n NAMESPACE`, при необходимости с полными/вычисленными значениями. |
| Удалит ли uninstall данные PVC гарантированно? | Не всегда; это зависит от шаблонов, reclaim policy, hooks и keep-аннотаций. Нужно проверить фактические объекты. |
| Зачем `_helpers.tpl`? | Для повторяемых имен, labels и фрагментов, чтобы контракт chart оставался единым. |
| Что проверить перед upgrade? | Diff/render, изменяемые/immutable поля, values, secrets, hooks, состояние workloads и план rollback. |

Практика: дайте values с неправильным типом и template с неверным отступом. Попросите найти проблему через `lint` и `template`, не применяя chart в кластер.
