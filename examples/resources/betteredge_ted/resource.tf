variable "ted_password" {
  type      = string
  sensitive = true
}

resource "betteredge_ted" "example" {
  project_id  = betteredge_project.example.id
  os_name     = "Ubuntu-26"
  vcpu        = 2
  memory_gb   = 2
  storage_gb  = 50
  description = "Created by Terraform"
  password    = var.ted_password
}
