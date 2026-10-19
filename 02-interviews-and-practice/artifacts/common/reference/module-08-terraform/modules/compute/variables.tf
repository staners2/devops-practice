variable "folder_id" {
  description = "Учебный folder."
  type        = string
}
variable "zone" {
  description = "Зона."
  type        = string
}
variable "name_prefix" {
  description = "Префикс."
  type        = string
}
variable "owner" {
  description = "Владелец."
  type        = string
}
variable "image_id" {
  description = "Проверенный Ubuntu image ID."
  type        = string
}
variable "ssh_user" {
  description = "Проверенный SSH-пользователь."
  type        = string
}
variable "ssh_public_key" {
  description = "Публичный SSH-ключ."
  type        = string
}
variable "subnet_ids" {
  description = "Выход network."
  type        = object({ app = string, proxy = string })
}
variable "security_group_ids" {
  description = "Выход network."
  type        = object({ app = string, proxy = string })
}
variable "machines" {
  description = "Стабильные VM ключи app-01/proxy-01."
  type        = map(object({ role = string, cores = number, memory = number, disk_size = number }))
}
