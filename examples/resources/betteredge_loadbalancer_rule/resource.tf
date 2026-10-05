# TCP rule: the platform assigns the frontend port.
resource "betteredge_loadbalancer_rule" "tcp" {
  project_id   = betteredge_project.example.id
  type         = "TCP"
  backend_port = 8443
  backend_vm_ids = [
    betteredge_ted.a.id,
    betteredge_ted.b.id,
  ]
}

# SNI rule: TLS routing by domain on a shared frontend port.
resource "betteredge_loadbalancer_rule" "sni" {
  project_id     = betteredge_project.example.id
  type           = "SNI"
  frontend_port  = 443
  backend_port   = 8443
  sni_key        = "app.example.com"
  proxy_protocol = true
  backend_vm_ids = [betteredge_ted.a.id]
}
