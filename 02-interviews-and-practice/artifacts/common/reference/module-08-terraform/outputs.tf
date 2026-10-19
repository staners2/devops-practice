output "lab" {
  description = "Единственный контракт inventory и приёмки; не весь state."
  value = {
    folder_id      = var.folder_id
    name_prefix    = var.name_prefix
    owner          = var.owner
    egress_mode    = var.egress_mode
    network_id     = module.network.network_id
    gateway_id     = module.network.gateway_id
    route_table_id = module.network.route_table_id
    app            = module.compute.app
    proxy          = module.compute.proxy
  }
}
