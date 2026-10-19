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
variable "public_cidr" {
  description = "Проверенная proxy /24 сеть."
  type        = string
}
variable "private_cidr" {
  description = "Проверенная app /24 сеть."
  type        = string
}
variable "operator_cidr" {
  description = "Проверенный IPv4 инженера /32."
  type        = string
}
variable "egress_mode" {
  description = "Проверенный режим bootstrap/restricted."
  type        = string
}
variable "nat_route_enabled" {
  description = "Независимый NAT routing-переключатель."
  type        = bool
}
