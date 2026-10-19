variable "folder_id" {
  description = "Отдельный учебный folder, не production."
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9]{20}$", var.folder_id))
    error_message = "Укажите точный 20-символьный folder ID."
  }
}
variable "zone" {
  description = "Одна зона для обеих подсетей и VM."
  type        = string
  default     = "ru-central1-a"
  validation {
    condition     = contains(["ru-central1-a", "ru-central1-b", "ru-central1-d"], var.zone)
    error_message = "Используйте доступную зону региона Россия."
  }
}
variable "name_prefix" {
  description = "Уникальный префикс ресурсов ученика."
  type        = string
  default     = "devops-08"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,30}$", var.name_prefix))
    error_message = "Префикс: 3–31 символ, строчная латиница, цифры, дефис."
  }
}
variable "owner" {
  description = "Идентификатор ученика для labels."
  type        = string
  validation {
    condition     = can(regex("^[a-z][a-z0-9_-]{1,30}$", var.owner))
    error_message = "owner: 2–31 символ, строчная латиница, цифры, дефис/подчёркивание."
  }
}
variable "public_cidr" {
  description = "Каноническая IPv4 /24 подсеть proxy; исключите конфликт с VPN."
  type        = string
  default     = "10.80.10.0/24"
  validation {
    condition     = can(cidrnetmask(var.public_cidr)) && can(regex("/24$", var.public_cidr)) && try(cidrhost(var.public_cidr, 0) == split("/", var.public_cidr)[0], false)
    error_message = "public_cidr должен быть канонической IPv4 /24 сетью."
  }
}
variable "private_cidr" {
  description = "Каноническая IPv4 /24 подсеть app, отличная от public."
  type        = string
  default     = "10.80.20.0/24"
  validation {
    condition     = can(cidrnetmask(var.private_cidr)) && can(regex("/24$", var.private_cidr)) && try(cidrhost(var.private_cidr, 0) == split("/", var.private_cidr)[0], false) && var.private_cidr != var.public_cidr
    error_message = "private_cidr должен быть отдельной канонической IPv4 /24 сетью."
  }
}
variable "operator_cidr" {
  description = "Текущий публичный IPv4 инженера /32."
  type        = string
  validation {
    condition     = can(cidrnetmask(var.operator_cidr)) && can(regex("/32$", var.operator_cidr))
    error_message = "operator_cidr допускает только один IPv4 /32, не /0."
  }
}
variable "egress_mode" {
  description = "bootstrap разрешает интернет TCP 80/443; restricted закрывает новые соединения."
  type        = string
  default     = "restricted"
  validation {
    condition     = contains(["bootstrap", "restricted"], var.egress_mode)
    error_message = "egress_mode должен быть bootstrap либо restricted."
  }
}
variable "nat_route_enabled" {
  description = "Отдельный routing-опыт; при restricted маршрут обычно остаётся."
  type        = bool
  default     = true
}
variable "image_id" {
  description = "Проверенный ID Ubuntu 24.04 amd64; не family."
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9]{20}$", var.image_id))
    error_message = "Укажите конкретный 20-символьный image ID."
  }
}
variable "ssh_user" {
  description = "Непривилегированный пользователь cloud-init с sudo."
  type        = string
  default     = "devops"
  validation {
    condition     = can(regex("^[a-z][a-z0-9_-]{0,30}$", var.ssh_user)) && var.ssh_user != "root"
    error_message = "Укажите безопасное имя Linux-пользователя, не root."
  }
}
variable "ssh_public_key" {
  description = "Публичный Ed25519 SSH-ключ без комментария и переносов."
  type        = string
  validation {
    condition     = can(regex("^ssh-ed25519 [A-Za-z0-9+/]+={0,2}$", var.ssh_public_key))
    error_message = "Требуется одна строка публичного ssh-ed25519 ключа без комментария."
  }
}
variable "disk_sizes" {
  description = "Размеры HDD в ГБ; изменение требует пересчёта бюджета."
  type        = object({ app = number, proxy = number })
  default     = { app = 30, proxy = 10 }
  validation {
    condition     = var.disk_sizes.app >= 30 && var.disk_sizes.proxy >= 10 && floor(var.disk_sizes.app) == var.disk_sizes.app && floor(var.disk_sizes.proxy) == var.disk_sizes.proxy
    error_message = "app HDD минимум 30 ГБ, proxy минимум 10 ГБ, целые значения."
  }
}
