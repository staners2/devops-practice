locals {
  labels  = { owner = var.owner, project = "devops-practice", module = "08" }
  subnets = { proxy = var.public_cidr, app = var.private_cidr }
  internal_rules = {
    proxy-ssh-out = { binding = "proxy", target = "app", direction = "egress", port = 22 }
    app-ssh-in    = { binding = "app", target = "proxy", direction = "ingress", port = 22 }
    proxy-api-out = { binding = "proxy", target = "app", direction = "egress", port = 8080 }
    app-api-in    = { binding = "app", target = "proxy", direction = "ingress", port = 8080 }
    app-http-out  = { binding = "app", target = "proxy", direction = "egress", port = 80 }
    proxy-http-in = { binding = "proxy", target = "app", direction = "ingress", port = 80 }
  }
  service_rules = {
    for rule in flatten([
      for role, cidr in local.subnets : [
        { key = "${role}-dns-udp", role = role, protocol = "UDP", port = 53, cidr = "${cidrhost(cidr, 2)}/32" },
        { key = "${role}-dns-tcp", role = role, protocol = "TCP", port = 53, cidr = "${cidrhost(cidr, 2)}/32" },
        { key = "${role}-metadata", role = role, protocol = "TCP", port = 80, cidr = "169.254.169.254/32" }
      ]
    ]) : rule.key => rule
  }
  bootstrap_rules = var.egress_mode == "bootstrap" ? {
    app-http    = { role = "app", port = 80 }
    app-https   = { role = "app", port = 443 }
    proxy-http  = { role = "proxy", port = 80 }
    proxy-https = { role = "proxy", port = 443 }
  } : {}
}

resource "yandex_vpc_network" "lab" {
  folder_id = var.folder_id
  name      = "${var.name_prefix}-vpc"
  labels    = local.labels
}
resource "yandex_vpc_gateway" "egress" {
  folder_id = var.folder_id
  name      = "${var.name_prefix}-egress"
  labels    = local.labels
  shared_egress_gateway {}
}
resource "yandex_vpc_route_table" "private" {
  folder_id  = var.folder_id
  name       = "${var.name_prefix}-private"
  network_id = yandex_vpc_network.lab.id
  labels     = local.labels
  dynamic "static_route" {
    for_each = var.nat_route_enabled ? [true] : []
    content {
      destination_prefix = "0.0.0.0/0"
      gateway_id         = yandex_vpc_gateway.egress.id
    }
  }
}
resource "yandex_vpc_subnet" "lab" {
  for_each       = local.subnets
  folder_id      = var.folder_id
  name           = "${var.name_prefix}-${each.key}"
  network_id     = yandex_vpc_network.lab.id
  zone           = var.zone
  v4_cidr_blocks = [each.value]
  route_table_id = each.key == "app" ? yandex_vpc_route_table.private.id : null
  labels         = local.labels
}
resource "yandex_vpc_security_group" "lab" {
  for_each   = local.subnets
  folder_id  = var.folder_id
  name       = "${var.name_prefix}-${each.key}"
  network_id = yandex_vpc_network.lab.id
  labels     = local.labels
}
resource "yandex_vpc_security_group_rule" "operator" {
  for_each               = { ssh = 22, http = 80 }
  security_group_binding = yandex_vpc_security_group.lab["proxy"].id
  direction              = "ingress"
  protocol               = "TCP"
  port                   = each.value
  v4_cidr_blocks         = [var.operator_cidr]
  description            = "operator ${each.key}"
}
resource "yandex_vpc_security_group_rule" "internal" {
  for_each               = local.internal_rules
  security_group_binding = yandex_vpc_security_group.lab[each.value.binding].id
  security_group_id      = yandex_vpc_security_group.lab[each.value.target].id
  direction              = each.value.direction
  protocol               = "TCP"
  port                   = each.value.port
  description            = each.key
}
resource "yandex_vpc_security_group_rule" "service" {
  for_each               = local.service_rules
  security_group_binding = yandex_vpc_security_group.lab[each.value.role].id
  direction              = "egress"
  protocol               = each.value.protocol
  port                   = each.value.port
  v4_cidr_blocks         = [each.value.cidr]
  description            = each.key
}
resource "yandex_vpc_security_group_rule" "bootstrap" {
  for_each               = local.bootstrap_rules
  security_group_binding = yandex_vpc_security_group.lab[each.value.role].id
  direction              = "egress"
  protocol               = "TCP"
  port                   = each.value.port
  v4_cidr_blocks         = ["0.0.0.0/0"]
  description            = "Временный bootstrap ${each.key}"
}
