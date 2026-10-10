# Модуль 07: подготовка двух Linux VM

Это подготовка входной среды, а не решение [задания Ansible](../modules/07-ansible.md).
Docker, Nginx и приложение устанавливаются позже вашим playbook.
Нужен интернет для ISO, Git, пакетов и образов; платные сервисы не используются.

## 1. Ресурсы и архитектура

| VM | ОС | vCPU | RAM | Диск |
| --- | --- | --- | --- | --- |
| `app-01` | Ubuntu Server 24.04 LTS | 2 | 4 ГиБ | 25 ГБ |
| `proxy-01` | Ubuntu Server 24.04 LTS | 1 | 1 ГиБ | 10 ГБ |

Установите гостевую ОС той же архитектуры, что и ПК: amd64 для Intel/AMD,
arm64 для Apple Silicon. Графическое окружение и параллельный Kubernetes
не нужны. Выделенные 5 ГиБ — бюджет VM, не замер пикового потребления.
На время сборки образов закройте другие тяжёлые стенды.

ISO: [Ubuntu Server](https://ubuntu.com/download/server),
[архив Ubuntu 24.04 amd64](https://releases.ubuntu.com/24.04/),
[Ubuntu 24.04 arm64](https://cdimage.ubuntu.com/releases/24.04/release/).
Проверьте SHA256 скачанного ISO по странице дистрибутива.

## 2. Бесплатный гипервизор и общая сеть

### Windows/Linux x86_64: VirtualBox

Используйте [базовый VirtualBox](https://www.oracle.com/virtualization/technologies/vm/downloads/virtualbox-downloads.html).
Extension Pack для этой практики не нужен.

1. В Network Manager создайте host-only сеть, например `192.168.56.0/24`,
   с адресом host `192.168.56.1`. Выберите подсеть, не занятую вашей LAN/VPN.
2. У каждой VM настройте адаптер 1 как NAT для интернета.
3. Адаптер 2 обеих VM подключите к одной созданной host-only сети.
4. В установщике Ubuntu оставьте первый интерфейс на DHCP. Второму назначьте
   свободные постоянные адреса, например `192.168.56.10/24` и `.11/24`.
   Gateway на втором интерфейсе не задавайте: default route идёт через NAT.
5. Не выдавайте обеим VM один IP. Если включён host-only DHCP, статические
   адреса должны находиться вне его диапазона.

Это два интерфейса, а не два независимых NAT, изолирующих VM друг от друга.
[Документация сетевых режимов VirtualBox](https://docs.oracle.com/en/virtualization/virtualbox/7.2/user/networkingdetails.html).

### macOS: UTM

Скачайте бесплатную версию [UTM с сайта](https://mac.getutm.app/), не требуется
покупка в App Store. Создайте две VM через Virtualize → Linux, с ISO своей
архитектуры. Для QEMU-настроек выберите `macOS Shared Network` на обеих VM,
не `Emulated VLAN`: последний создаёт отдельную сеть для каждой VM.
Режим Shared позволяет host и guest обращаться друг к другу.
[Сетевые настройки UTM](https://docs.getutm.app/settings-qemu/devices/network/network/).

Получите реальные адреса через `ip -br address`; не копируйте VirtualBox IP
в UTM автоматически. Закрепите свободные IP в фактической shared-подсети
через сетевые настройки установщика/Netplan, сохранив её gateway и DNS.
Перед изменением проверьте диапазон DHCP и отсутствие конфликта адресов.
Настройку Netplan применяйте из консоли VM через `sudo netplan try`:
так потеря SSH не оставит вас без доступа.

Для Apple Virtualization backend названия настроек отличаются; не смешивайте
с ним шаги QEMU. Windows ARM и другие сочетания не входят в базовый маршрут.
Настоящий runtime на перечисленных гипервизорах должен быть проверен перед
публикацией практики; эта инструкция не является протоколом такого прогона.

## 3. Подготовка гостевой ОС

При установке задайте имена `app-01` и `proxy-01`, создайте обычного
пользователя с `sudo`, включите OpenSSH Server. Root SSH не нужен.
Если SSH/Python не установлены, выполните из консоли каждой VM:

```bash
sudo apt-get update
sudo apt-get install -y openssh-server python3
sudo systemctl enable --now ssh
ip -br address
ip route
sudo -v
```

Проверка на обеих VM:

```bash
python3 --version
systemctl is-active ssh
curl --head --max-time 10 https://download.docker.com
```

Если `curl` ещё отсутствует, для последней команды установите его как
инструмент проверки связи: `sudo apt-get install -y curl`.
Итог этого этапа: две VM с SSH/Python, работающим `sudo`, общей сетью
и интернетом. Docker/Nginx по-прежнему отсутствуют.

## 4. Control node на app-01

На `app-01` установите только инструменты разработки и Ansible:

```bash
sudo apt-get install -y git python3-venv openssh-client openssl
read -r -p 'URL учебного репозитория: ' PRACTICE_REPO_URL
git clone "$PRACTICE_REPO_URL" devops-practice
cd devops-practice || exit 1
```

Используйте текущую версию учебного репозитория без переключения веток.
Готовые файлы стенда находятся в `deploy/ansible-lab` и `scripts/ansible-lab`.
Собственные inventory и roles создайте по условию модуля; файлы `reference/`
являются примерами решений.

```bash
set -a
source deploy/ansible-lab/versions.env
set +a
python3 -m venv "$HOME/.venvs/devops-ansible-07"
source "$HOME/.venvs/devops-ansible-07/bin/activate"
python -m pip install "ansible-core==$ANSIBLE_CORE_VERSION" "ansible-lint==$ANSIBLE_LINT_VERSION"
ansible-galaxy collection install "community.docker:==$COMMUNITY_DOCKER_VERSION"
ansible --version
ansible-lint --version
ansible-galaxy collection list community.docker
```

Ubuntu 24.04 предоставляет подходящий Python 3.12; ansible-core 2.21.1
требует Python 3.12+. [Метаданные релиза](https://pypi.org/project/ansible-core/2.21.1/).
При новой SSH-сессии повторяйте активацию virtual environment.
Не устанавливайте Ansible через `sudo pip` в системный Python.

## 5. SSH от app-01 к обеим VM

Создайте отдельный SSH-ключ на `app-01`. Команда запрашивает passphrase:

```bash
ssh-keygen -t ed25519 -f "$HOME/.ssh/devops-ansible-07"
eval "$(ssh-agent -s)"
ssh-add "$HOME/.ssh/devops-ansible-07"
read -r -p 'SSH-пользователь VM: ' LAB_USER
read -r -p 'Частный IP app-01: ' APP_IP
read -r -p 'Частный IP proxy-01: ' PROXY_IP
ssh-copy-id -i "$HOME/.ssh/devops-ansible-07.pub" "$LAB_USER@$APP_IP"
ssh-copy-id -i "$HOME/.ssh/devops-ansible-07.pub" "$LAB_USER@$PROXY_IP"
```

До подтверждения нового host key сравните fingerprint из SSH с выводом
команды в консоли соответствующей VM:

```bash
sudo ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

Не обходите проверку через `StrictHostKeyChecking=no`.
Проверки запускаются с `app-01`, включая SSH к её собственному IP:

```bash
ssh "$LAB_USER@$APP_IP" 'hostname; python3 --version'
ssh "$LAB_USER@$PROXY_IP" 'hostname; python3 --version'
ssh -t "$LAB_USER@$PROXY_IP" sudo -v
```

С ПК проверьте SSH к обеим VM. HTTP `proxy-01:80` появится только после
вашего Nginx role; порт `app-01:8080` — после приложения.

## 6. Дальше работает ваш Ansible

Создайте inventory с группами `app` и `proxy`, реальными IP и пользователем.
Используйте `--private-key "$HOME/.ssh/devops-ansible-07"`, если ключ
не выбран SSH-конфигурацией или агентом. Пароль `sudo` передаётся через
`--ask-become-pass`; не выдавайте passwordless `sudo ALL` ради удобства.

Сначала реализуйте и примените `base` / `docker` только к `app-01`.
Затем подготовьте выдаваемые образы — это не установка приложения:

```bash
cd "$HOME/devops-practice" || exit 1
sudo bash scripts/ansible-lab/prepare-images.sh
```

Получаются `devops-practice:lab-07-v1`, `lab-07-v2`, `lab-07-failed`.
Первые два используют тот же код с разными OCI release labels и IDs.
Третий намеренно не содержит API/worker. Повторная команда не перезаписывает
существующие tags. Сборка может потребовать нескольких минут и свободного диска.

Теперь продолжайте [практику Ansible](../modules/07-ansible.md): Nginx,
закрытый env, приложение, systemd и эксплуатационные эксперименты.
Не используйте `make up`: он относится к исходному локальному стенду,
а не к VM-конфигурации модуля 07.
