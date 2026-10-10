provider "yandex" {
  folder_id = var.folder_id
  zone      = var.zone
}

module "network" {
  source            = "./modules/network"
  folder_id         = var.folder_id
  zone              = var.zone
  name_prefix       = var.name_prefix
  owner             = var.owner
  public_cidr       = var.public_cidr
  private_cidr      = var.private_cidr
  operator_cidr     = var.operator_cidr
  egress_mode       = var.egress_mode
  nat_route_enabled = var.nat_route_enabled
}

module "compute" {
  source             = "./modules/compute"
  folder_id          = var.folder_id
  zone               = var.zone
  name_prefix        = var.name_prefix
  owner              = var.owner
  image_id           = var.image_id
  ssh_user           = var.ssh_user
  ssh_public_key     = var.ssh_public_key
  subnet_ids         = module.network.subnet_ids
  security_group_ids = module.network.security_group_ids
  machines = {
    app-01   = { role = "app", cores = 2, memory = 4, disk_size = var.disk_sizes.app }
    proxy-01 = { role = "proxy", cores = 2, memory = 2, disk_size = var.disk_sizes.proxy }
  }
}
