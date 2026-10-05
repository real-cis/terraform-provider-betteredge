resource "betteredge_ssh_proxy" "example" {
  vm_id = betteredge_ted.example.id
}

output "ssh_command" {
  value = betteredge_ssh_proxy.example.ssh_command
}
