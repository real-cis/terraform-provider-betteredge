terraform {
  required_providers {
    betteredge = {
      source = "real-cis/betteredge"
    }
  }
}

provider "betteredge" {}

variable "ted_password" {
  type      = string
  sensitive = true
}

variable "project_id" {
  type        = string
  description = "UUID of an existing BetterEdge project to create TEDs in. Leave empty to have Terraform create a new project."
  default     = ""
}

variable "project_name" {
  type        = string
  description = "Name for the new project. Only used when project_id is empty."
  default     = "terraform-managed"
}

variable "teds" {
  type        = set(string)
  description = "Names of the TEDs that should exist. Add a name to create one; remove a name to destroy just that one."
  default     = ["ted-1"]
}

variable "ted_placement" {
  type = map(object({
    module_id = string
    server_id = string
  }))
  description = "Manual placement per TED name (a subset of `teds`). Names not listed here get automatic placement. Keying by name keeps this from affecting TEDs you didn't mean to touch."
  default     = {}
}

variable "include_ssh_keys" {
  type        = bool
  description = "Whether to also grant SSH access via public_ssh_keys, in addition to the password."
  default     = false
}

variable "public_ssh_keys" {
  type        = list(string)
  description = "SSH public keys to authorize. Required when include_ssh_keys is true."
  default     = []
}

variable "port" {
  type        = string
  description = "Port forwarded on TEDs listed in port_forwarded_teds."
  default     = "8443"
}

variable "protocol" {
  type        = string
  description = "Protocol for the forwarded port."
  default     = "tcp"
}

variable "port_forwarded_teds" {
  type        = set(string)
  description = "Which of the names in `teds` should have the port forwarding rule. Defaults to all of them. Remove a name here (without removing it from `teds`) to drop just its port rule, keeping the TED."
  default     = null
}

locals {
  port_forwarded_teds = var.port_forwarded_teds != null ? var.port_forwarded_teds : var.teds
}

resource "betteredge_project" "example" {
  count       = var.project_id == "" ? 1 : 0
  name        = var.project_name
  description = "Created by Terraform"
}

locals {
  project_id = var.project_id != "" ? var.project_id : betteredge_project.example[0].id
}

resource "betteredge_ted" "example" {
  for_each         = var.teds
  project_id       = local.project_id
  os_name          = "Ubuntu-26"
  vcpu             = 2
  memory_gb        = 2
  storage_gb       = 50
  description      = "TED ${each.key} created by Terraform"
  password         = var.ted_password
  module_id        = try(var.ted_placement[each.key].module_id, null)
  server_id        = try(var.ted_placement[each.key].server_id, null)
  include_ssh_keys = var.include_ssh_keys
  public_ssh_keys  = var.public_ssh_keys
}

resource "betteredge_port_forwarding" "example" {
  for_each = local.port_forwarded_teds
  vm_id    = betteredge_ted.example[each.key].id
  port     = var.port
  protocol = var.protocol
}

output "project_id" {
  value = local.project_id
}

output "ted_ids" {
  value = { for k, v in betteredge_ted.example : k => v.id }
}

output "ted_module_ids" {
  value = { for k, v in betteredge_ted.example : k => v.module_id }
}

output "ted_server_ids" {
  value = { for k, v in betteredge_ted.example : k => v.server_id }
}
