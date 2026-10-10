output "app" {
  description = "Private VM без public IP."
  value = {
    id           = yandex_compute_instance.vm["app-01"].id
    boot_disk_id = yandex_compute_instance.vm["app-01"].boot_disk[0].disk_id
    private_ip   = yandex_compute_instance.vm["app-01"].network_interface[0].ip_address
  }
}
output "proxy" {
  description = "Входная VM; public IP может измениться при replacement."
  value = {
    id           = yandex_compute_instance.vm["proxy-01"].id
    boot_disk_id = yandex_compute_instance.vm["proxy-01"].boot_disk[0].disk_id
    private_ip   = yandex_compute_instance.vm["proxy-01"].network_interface[0].ip_address
    public_ip    = yandex_compute_instance.vm["proxy-01"].network_interface[0].nat_ip_address
  }
}
