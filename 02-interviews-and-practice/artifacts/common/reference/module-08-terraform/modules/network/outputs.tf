output "network_id" {
  description = "VPC стенда."
  value       = yandex_vpc_network.lab.id
}
output "subnet_ids" {
  description = "Подсети по роли VM."
  value       = { for role, subnet in yandex_vpc_subnet.lab : role => subnet.id }
}
output "security_group_ids" {
  description = "Ровно одна custom SG на роль."
  value       = { for role, sg in yandex_vpc_security_group.lab : role => sg.id }
}
output "gateway_id" {
  description = "NAT для приёмки и cleanup."
  value       = yandex_vpc_gateway.egress.id
}
output "route_table_id" {
  description = "Таблица маршрутов private subnet."
  value       = yandex_vpc_route_table.private.id
}
