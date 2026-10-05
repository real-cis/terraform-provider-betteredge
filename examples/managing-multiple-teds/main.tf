terraform {
  # Validations that compare variables with each other need Terraform 1.9.
  required_version = ">= 1.9"

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
  description = "Which of the names in `teds` or `existing_teds` should have the port forwarding rule. Defaults to all of `teds` (never to `existing_teds`). Remove a name here to drop just its port rule, keeping the TED."
  default     = null

  validation {
    condition     = var.port_forwarded_teds == null || length(setsubtract(var.port_forwarded_teds, setunion(var.teds, keys(var.existing_teds)))) == 0
    error_message = "Every name in `port_forwarded_teds` must be in `teds` or `existing_teds`."
  }
}

variable "ssh_proxy_teds" {
  type        = set(string)
  description = "Which of the names in `teds` or `existing_teds` should have SSH Proxy Access enabled. Remove a name to disable just its proxy, keeping the TED."
  default     = []

  validation {
    condition     = length(setsubtract(var.ssh_proxy_teds, setunion(var.teds, keys(var.existing_teds)))) == 0
    error_message = "Every name in `ssh_proxy_teds` must be in `teds` or `existing_teds`."
  }
}

variable "loadbalancer_rules" {
  type = map(object({
    type           = optional(string, "TCP")
    frontend_port  = optional(number)
    backend_port   = number
    teds           = set(string)
    sni_key        = optional(string)
    proxy_protocol = optional(bool, false)
  }))
  description = "Load balancing rules keyed by a name of your choice. `teds` are names from `teds` or `existing_teds` used as backends; changing it updates the rule in place. `frontend_port` and `sni_key` are only for `type = \"SNI\"`."
  default     = {}

  validation {
    condition     = alltrue([for rule in values(var.loadbalancer_rules) : length(setsubtract(rule.teds, setunion(var.teds, keys(var.existing_teds)))) == 0])
    error_message = "Every name in a rule's `teds` must be in `teds` or `existing_teds`."
  }
}

variable "existing_teds" {
  type        = map(string)
  description = "TEDs that already exist and are not managed by this configuration, as name -> vm_id. Their names can be used in `port_forwarded_teds`, `ssh_proxy_teds` and `loadbalancer_rules` like the ones in `teds`; Terraform never creates or destroys these TEDs."
  default     = {}

  validation {
    condition     = length(setintersection(keys(var.existing_teds), var.teds)) == 0
    error_message = "A name can't be in both `teds` and `existing_teds`."
  }
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

locals {
  ted_ids = merge(var.existing_teds, { for k, v in betteredge_ted.example : k => v.id })
}

resource "betteredge_port_forwarding" "example" {
  for_each = local.port_forwarded_teds
  vm_id    = local.ted_ids[each.key]
  port     = var.port
  protocol = var.protocol
}

resource "betteredge_ssh_proxy" "example" {
  for_each = var.ssh_proxy_teds
  vm_id    = local.ted_ids[each.key]
}

resource "betteredge_loadbalancer_rule" "example" {
  for_each       = var.loadbalancer_rules
  project_id     = local.project_id
  type           = each.value.type
  frontend_port  = each.value.frontend_port
  backend_port   = each.value.backend_port
  sni_key        = each.value.sni_key
  proxy_protocol = each.value.proxy_protocol
  backend_vm_ids = [for name in each.value.teds : local.ted_ids[name]]
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

output "ssh_commands" {
  value = { for k, v in betteredge_ssh_proxy.example : k => v.ssh_command }
}

output "loadbalancer_rules" {
  value = {
    for k, v in betteredge_loadbalancer_rule.example : k => {
      id                 = v.id
      frontend_port      = v.frontend_port
      load_balancer_ipv4 = v.load_balancer_ipv4
      load_balancer_ipv6 = v.load_balancer_ipv6
      status             = v.status
    }
  }
}
