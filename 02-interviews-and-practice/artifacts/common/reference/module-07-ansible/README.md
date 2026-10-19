# Референс ментора: модуль 07 Ansible

Этот каталог — решение для проверки, не старт ученика.
Задание: [07-ansible](../../docs/modules/07-ansible.md).
Предпосылки: [bootstrap двух VM](../../docs/runbooks/07-ansible-bootstrap.md).
Не публикуйте этот каталог в tree будущего `module-07-start`.
Git history и доступ к другим refs не обеспечивают секретность решения.

## Подготовка

Все команды выполняются на `app-01`, в checkout учебного репозитория.
Используйте инструменты из закреплённого virtual environment по bootstrap.
Нужны два проверенных SSH host keys и ключ в SSH agent.

```bash
cd "$HOME/devops-practice/reference/module-07-ansible" || exit 1
cp inventories/lab/hosts.example.yml inventories/lab/hosts.local.yml
```

В `hosts.local.yml` замените пример IP и пользователя реальными значениями.
Пароль PostgreSQL создайте один раз: `openssl rand -hex 24`. Создайте Vault:

```bash
ansible-vault create inventories/lab/group_vars/all/vault.yml
```

В открывшемся редакторе задайте `vault_postgres_password` полученным
48-символьным hex-значением. Закройте редактор, файл сохранится зашифрованным.
Пароль Vault не совпадает с паролем PostgreSQL и не хранится в репозитории.
Локальный inventory и Vault исключены из Git этого референса.

```bash
ansible-inventory --graph --ask-vault-pass
ansible all -m ansible.builtin.ping --ask-vault-pass
ansible all -m ansible.builtin.command -a 'id -u' --become --ask-become-pass --ask-vault-pass
ansible-playbook site.yml --syntax-check --ask-vault-pass
ansible-lint --offline site.yml
```

Для явно выбранного ключа добавляйте `--private-key "$HOME/.ssh/devops-ansible-07"`.
Не применяйте inventory до проверки, что он содержит именно две учебные VM.

## Установка предпосылок и первый deployment

```bash
ansible-playbook site.yml --tags base,docker --limit app --ask-vault-pass --ask-become-pass
cd "$HOME/devops-practice" || exit 1
sudo bash scripts/ansible-lab/prepare-images.sh
cd reference/module-07-ansible || exit 1
ansible-playbook site.yml --ask-vault-pass --ask-become-pass
```

Role Nginx применяется до application. Unit сначала готовит зависимости,
Kafka topic и миграции, затем API/worker. Application role проверяет health
и запускает Go smoke через Nginx после изменения deployment.

Runtime: `/opt/devops-practice-ansible-07`, unit:
`devops-practice-ansible-07.service`. Принятая конфигурация хранится
в отдельном root-only каталоге `.accepted-*`: Compose, helper-файлы, unit и env.
Только после успешной проверки и полной записи снимка указатель `.accepted`
атомарно переключается на него. Предыдущие снимки не перезаписываются;
автоматическая очистка в этой практике не реализуется.
Не запускайте два Ansible deployment одновременно.

## Повторное применение, check и limit

```bash
ansible-playbook site.yml --ask-vault-pass --ask-become-pass
ansible-playbook site.yml --check --diff --ask-vault-pass --ask-become-pass \
  -e nginx_response_label=preview-07
ansible-playbook site.yml --limit proxy --ask-vault-pass --ask-become-pass \
  -e nginx_response_label=preview-07
```

На установившемся стенде обычный повтор не вызывает Compose recreate,
Nginx reload или smoke с новыми заявками. Периодический apt cache refresh
может изменить recap, но не означает restart приложения.
Check mode прогнозирует файлы, не запускает deployment и smoke.
Проверка на холодных VM имеет ограничения отсутствующих пакетов/каталогов.

Полезные read-only снимки на `app-01`:

```bash
sudo systemctl is-active docker devops-practice-ansible-07
sudo stat -c '%a %U:%G' /opt/devops-practice-ansible-07/.env
sudo docker ps --filter label=com.docker.compose.project=devops-practice-ansible-07 \
  --format '{{.ID}} {{.Image}} {{.Status}}'
```

На `proxy-01` сравните `sha256sum /etc/nginx/nginx.conf`,
`systemctl is-active nginx` и `ps -C nginx -o pid,lstart,args`.
Изменение template с ошибкой должно отклоняться `template.validate` до
замены live config, сохраняя checksum и действующие worker processes.

## Релиз и rollback

```bash
ansible-playbook site.yml --ask-vault-pass --ask-become-pass \
  -e application_image=devops-practice:lab-07-v2
ansible-playbook site.yml --ask-vault-pass --ask-become-pass \
  -e application_image=devops-practice:lab-07-v1
```

Для каждого принятого релиза указатель на snapshot меняется только после успешного
потока. Образы и runtime env не обновляют major-версии зависимостей, схемы
или данные. PostgreSQL password неизменен; ротация не является частью опыта.

Проверка image, которого нет, завершается до записи env и изменения runtime:

```bash
ansible-playbook site.yml --ask-vault-pass --ask-become-pass \
  -e application_image=devops-practice:lab-07-not-found
```

Доступный, но неработающий image проверяет `rescue`:

```bash
ansible-playbook site.yml --ask-vault-pass --ask-become-pass \
  -e application_image=devops-practice:lab-07-failed
```

Ожидается ненулевой exit code и сообщение, что новый deployment не принят,
а предыдущий восстановлен и проверен. Если recovery тоже не проходит,
playbook прекращается с ошибкой на соответствующем шаге. Secret tasks
скрывают детали; вывод полного env/inspect не является способом диагностики.
Если snapshot ещё нет, первичная установка завершается ошибкой без rollback.

## Данные и reboot

Создайте контрольную заявку до обновления, сохраните UUID и дождитесь
`processed`. Для проверки непосредственно PostgreSQL и нового сквозного
потока используйте helper на `app-01`:

```bash
cd "$HOME/devops-practice" || exit 1
read -r -p 'Частный IP proxy-01: ' PROXY_IP
read -r -p 'UUID контрольной заявки: ' REQUEST_ID
sudo bash scripts/ansible-lab/verify.sh /opt/devops-practice-ansible-07 "http://$PROXY_IP" "$REQUEST_ID"
```

Перезагрузите VM из их отдельных SSH-сеансов через `sudo reboot`.
Соединение с control node прервётся. После возвращения выполните read-only
service checks и helper; вручную `compose up` не запускайте.
Unit не гарантирует health только фактом `active`; проверяются данные и поток.
Helper ограничивает все Docker-операции общим бюджетом 130 секунд
и дополнительными 5 секундами на принудительное завершение.
Нельзя применять `down -v`.

## Приёмка и границы проверки

Проверяйте все 12 опытов из ученического задания, не только recap и health.
Отдельно оцените inventory precedence, `become`, понимание handlers,
Vault at rest, check mode, rollback vs restore и ограничения одной VM.
Стартовый tag должен содержать готовые inputs и задание, но не этот reference.
До будущей публикации проверьте отсутствие reference в его tree:

```bash
git ls-tree -r --name-only module-07-start reference/module-07-ansible
```

Ожидается пустой вывод; команда применима только после создания тега.
На этапе подготовки выполнены статические проверки конфигурации и scripts.
Runtime чистых VM, reboot, сохранность данных и гипервизорный networking
ещё должны пройти отдельную проверку; этот файл не утверждает её успех.
