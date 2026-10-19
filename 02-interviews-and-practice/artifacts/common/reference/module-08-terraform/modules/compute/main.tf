resource "yandex_compute_instance" "vm" {
  for_each    = var.machines
  folder_id   = var.folder_id
  name        = "${var.name_prefix}-${each.key}"
  zone        = var.zone
  platform_id = "standard-v3"
  labels      = { owner = var.owner, project = "devops-practice", module = "08" }

  resources {
    cores         = each.value.cores
    memory        = each.value.memory
    core_fraction = 100
  }
  boot_disk {
    auto_delete = true
    initialize_params {
      name     = "${var.name_prefix}-${each.key}-boot"
      image_id = var.image_id
      type     = "network-hdd"
      size     = each.value.disk_size
    }
  }
  network_interface {
    subnet_id          = var.subnet_ids[each.value.role]
    security_group_ids = [var.security_group_ids[each.value.role]]
    nat                = each.value.role == "proxy"
    ipv4               = true
    ipv6               = false
  }
  metadata = {
    user-data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
      ssh_user       = var.ssh_user
      ssh_public_key = var.ssh_public_key
    })
  }
  scheduling_policy {
    preemptible = false
  }
}
