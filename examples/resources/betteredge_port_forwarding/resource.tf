resource "betteredge_port_forwarding" "example" {
  vm_id    = betteredge_ted.example.id
  port     = "8443"
  protocol = "tcp"
}
