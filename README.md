# Terraform Provider - real-cis/betteredge

Manages projects and Trusted Execution Domains (TEDs) on the BetterEdge platform.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.25, only needed if you're building the provider yourself

## Provider configuration

| Argument | Required | Description |
|---|---|---|
| `platform_url` | yes (or via env var) | Base URL of the BetterEdge platform API. Falls back to the `BETTEREDGE_PLATFORM_URL` environment variable. |
| `api_token` | yes (or via env var) | API token (`X-Token` header). Falls back to the `BETTEREDGE_API_TOKEN` environment variable. |

```hcl
terraform {
  required_providers {
    betteredge = {
      source = "real-cis/betteredge"
    }
  }
}

provider "betteredge" {}
```

## Resources

### `betteredge_project`

A project on the BetterEdge platform.

| Attribute | Required | Description |
|---|---|---|
| `name` | yes | 4-32 characters. Can be changed in place. |
| `description` | no | Free text. Can be changed in place. If omitted, the current description is kept (empty for a new project); set `description = ""` to clear it. |
| `id` | computed | Project UUID. |

```hcl
resource "betteredge_project" "example" {
  name        = "example-project"
  description = "Created by Terraform"
}
```

### `betteredge_ted`

A Trusted Execution Domain (TED).

| Attribute | Required | Description |
|---|---|---|
| `project_id` | yes | UUID of the project the TED will be created in. |
| `os_name` | yes | OS image. Must match one of the platform's valid options. |
| `vcpu` | yes | Number of vCPUs. Must match one of the platform's valid options. |
| `memory_gb` | yes | Memory in GB. Must match one of the platform's valid options. |
| `storage_gb` | yes | Storage in GB. Must match one of the platform's valid options. |
| `description` | yes | Free-text description for the TED. The only argument that can be changed in place. |
| `password` | yes, sensitive | Password for the default user. Required regardless of `include_ssh_keys`. |
| `include_ssh_keys` | no (default `false`) | Also grant SSH access via `public_ssh_keys`. |
| `public_ssh_keys` | no (default `[]`) | List of SSH public keys. Required when `include_ssh_keys` is `true`. |
| `module_id` | no, computed | UUID of the module to place the TED on. Omit (along with `server_id`) for automatic placement. |
| `server_id` | no, computed | UUID of the server to place the TED on. Omit (along with `module_id`) for automatic placement. |
| `id` | computed | UUID of the TED. |

```hcl
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
  description = "Create Project"
  password    = var.ted_password
}
```

Set `password` with `TF_VAR_ted_password` instead of typing it in each time. If the value changes between runs, the plan fails until you set it back.

### `betteredge_port_forwarding`

A single port forwarding rule on a TED.

| Attribute | Required | Description |
|---|---|---|
| `vm_id` | yes | UUID of the TED to update. Forces replacement on change. |
| `port` | yes | Port to forward. Changing it updates the rule in place (removes the old port, adds the new one). |
| `protocol` | no (default `tcp`) | `tcp` or `udp`. |
| `id` | computed | Synthetic identifier (`<vm_id>/<port>`) - the platform has no per-rule ID. |

```hcl
resource "betteredge_port_forwarding" "example" {
  vm_id    = betteredge_ted.example.id
  port     = "8443"
  protocol = "tcp"
}
```

### `betteredge_ssh_proxy`

SSH Proxy Access on a TED. While it exists, the platform allocates a load balancer endpoint that forwards to port 22 of the TED, for clients without direct IPv6 connectivity. Destroying it disables the proxy.

| Attribute | Required | Description |
|---|---|---|
| `vm_id` | yes | UUID of the TED. Forces replacement on change. |
| `frontend_port` | computed | Port on the load balancer that forwards to the TED's port 22. |
| `load_balancer_ipv4` | computed | Public IPv4 address of the load balancer. |
| `load_balancer_ipv6` | computed | Public IPv6 address of the load balancer. |
| `status` | computed | Provisioning status of the SSH proxy rule. |
| `ssh_command` | computed | SSH command to connect through the proxy (`ssh -p <port> linux@<ipv4>`). |
| `id` | computed | Same as `vm_id`. |

```hcl
resource "betteredge_ssh_proxy" "example" {
  vm_id = betteredge_ted.example.id
}

output "ssh_command" {
  value = betteredge_ssh_proxy.example.ssh_command
}
```

### `betteredge_loadbalancer_rule`

A load balancing rule across one or more TED backends. A TED can only be used once per backend port. A conflicting rule fails with the ID of the rule that already uses it. On the platform, a TED's SSH proxy access is a TCP rule to backend port 22 with that TED as its only backend. Such a rule can be managed either with `betteredge_ssh_proxy` or with `betteredge_loadbalancer_rule`, but not with both at once: deleting one also removes the other.

| Attribute | Required | Description |
|---|---|---|
| `project_id` | yes | UUID of the project. Forces replacement. |
| `type` | no (default `TCP`) | `TCP` or `SNI`. Forces replacement. |
| `frontend_port` | only when `type=SNI` | Public-facing port, one of the platform's SNI ports. Must be omitted for `TCP`. Forces replacement. |
| `backend_port` | yes | Port on each backend TED. Forces replacement. |
| `backend_vm_ids` | yes | Set of TED UUIDs (at least one). Updated in place. |
| `sni_key` | only when `type=SNI` | Domain for TLS routing. Forces replacement. |
| `proxy_protocol` | no (default `false`) | Send a PROXY v2 header to the backends. Forces replacement. |
| `status` | computed | Provisioning status of the rule. |
| `load_balancer_ipv4` | computed | Public IPv4 frontend address. |
| `load_balancer_ipv6` | computed | Public IPv6 frontend address. |
| `id` | computed | UUID of the rule. |

```hcl
resource "betteredge_loadbalancer_rule" "tcp" {
  project_id     = betteredge_project.example.id
  type           = "TCP"
  backend_port   = 8443
  backend_vm_ids = [betteredge_ted.a.id, betteredge_ted.b.id]
}

resource "betteredge_loadbalancer_rule" "sni" {
  project_id     = betteredge_project.example.id
  type           = "SNI"
  frontend_port  = 443
  backend_port   = 8443
  sni_key        = "app.example.com"
  backend_vm_ids = [betteredge_ted.a.id]
}
```

For SNI rules, the domain must have a CNAME record to `<project_id>.lb.betteredge.cloud` before the rule is created. Otherwise the platform rejects the rule with `LB_SNI_DOMAIN_NOT_VERIFIED`.

## Example: managing multiple TEDs in a project

[`examples/managing-multiple-teds/main.tf`](examples/managing-multiple-teds/main.tf) covers everything below. Run it from that directory with `dev_overrides` pointed at your local build.

Variables: `project_id` (leave empty to create a new project), `project_name`, `teds` (set of names), `ted_placement` (map of TED names to `{module_id, server_id}`, only for names you list), `include_ssh_keys`/`public_ssh_keys`, `port`/`protocol`, `port_forwarded_teds` (names from `teds` or `existing_teds` that get the port rule, defaults to all of `teds`), `ssh_proxy_teds` (names from `teds` or `existing_teds` with SSH proxy access, defaults to none), `loadbalancer_rules` (map of rule names to rule settings, defaults to none), `existing_teds` (map of names to the `vm_id` of TEDs created outside this configuration).

**Important**: `teds`, `port_forwarded_teds`, `ssh_proxy_teds` and `loadbalancer_rules` are the full set every time, not a diff. Always list every name that should still be there.

### Setup (once per session)

```shell
export BETTEREDGE_PLATFORM_URL="https://..."
export BETTEREDGE_API_TOKEN="..."
export TF_VAR_ted_password="..."
```

### Project

New project:

```shell
terraform apply
```

Existing project:

```shell
terraform apply -var="project_id=<uuid>"
```

Delete project:

```shell
terraform apply -var='teds=[]'
terraform destroy
```

### TED

Password only:

```shell
terraform apply -var='teds=["ted-1"]'
```

With an SSH key:

```shell
terraform apply -var='teds=["ted-1"]' -var="include_ssh_keys=true" -var='public_ssh_keys=["ssh-ed25519 AAAA... user@host"]'
```

On a specific module/server (only names listed in `ted_placement` get manual placement; everything else keeps auto placement):

```shell
terraform apply -var='teds=["ted-1"]' -var='ted_placement={"ted-1"={module_id="<uuid>",server_id="<uuid>"}}'
```

Add another one:

```shell
terraform apply -var='teds=["ted-1","ted-2"]'
```

Delete a specific TED. Find which name owns the id, then drop that name:

```shell
terraform output ted_ids
terraform apply -var='teds=["ted-1"]'
```

### Port forwarding

Create or update the rule on every TED (default when `port_forwarded_teds` is omitted):

```shell
terraform apply -var='teds=["ted-1","ted-2"]' -var="port=9443" -var="protocol=tcp"
```

Remove a port-forwarding rule from a specific TED. List just the names that should still have one:

```shell
terraform apply -var='teds=["ted-1","ted-2"]' -var='port_forwarded_teds=["ted-1"]'
```

### Using TEDs that already exist

To add SSH proxy, port forwarding or load balancing to TEDs created outside this configuration, list them in `existing_teds` and use those names like any other. Terraform never creates or destroys them. Don't import them as `betteredge_ted` unless Terraform should own them.

```shell
terraform apply -var="project_id=<uuid>" -var='teds=[]' \
  -var='existing_teds={vm-1="<vm_id>",vm-2="<vm_id>"}' \
  -var='ssh_proxy_teds=["vm-1"]' \
  -var='loadbalancer_rules={web={backend_port=8443,teds=["vm-1","vm-2"]}}'
```

### SSH proxy access

Enable it on specific TEDs, then read the SSH command:

```shell
terraform apply -var='teds=["ted-1","ted-2"]' -var='ssh_proxy_teds=["ted-1"]'
terraform output ssh_commands
```

Disable it on a TED by removing its name:

```shell
terraform apply -var='teds=["ted-1","ted-2"]' -var='ssh_proxy_teds=[]'
```

### Load balancing rules

Create a TCP rule across two TEDs (the platform assigns the frontend port):

```shell
terraform apply -var='teds=["ted-1","ted-2"]' \
  -var='loadbalancer_rules={web={backend_port=8443,teds=["ted-1","ted-2"]}}'
```

Change the backends of a rule in place by editing its `teds`:

```shell
terraform apply -var='teds=["ted-1","ted-2"]' \
  -var='loadbalancer_rules={web={backend_port=8443,teds=["ted-1"]}}'
```

Create an SNI rule:

```shell
terraform apply -var='teds=["ted-1","ted-2"]' \
  -var='loadbalancer_rules={app={type="SNI",frontend_port=443,backend_port=8443,sni_key="app.example.com",teds=["ted-1"]}}'
```

Delete a rule by removing its key:

```shell
terraform apply -var='teds=["ted-1","ted-2"]' -var='loadbalancer_rules={}'
```

### Inspecting state

```shell
terraform state list
terraform output
terraform output ted_ids
```

## Importing existing resources

```shell
terraform import betteredge_project.example <project_id>
terraform import betteredge_ted.example <vm_id>
terraform import betteredge_port_forwarding.example <vm_id>/<port>
terraform import betteredge_ssh_proxy.example <vm_id>
terraform import betteredge_loadbalancer_rule.example <project_id>/<rule_id>
```

## Developing the Provider

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources on the BetterEdge platform.

```shell
export BETTEREDGE_PLATFORM_URL="https://..."
export BETTEREDGE_API_TOKEN="..."
export TED_PASSWORD="..."
export BETTEREDGE_TEST_MODULE_ID="..."         # optional, enables the manual placement test
export BETTEREDGE_TEST_SERVER_ID="..."         # optional, enables the manual placement test
make testacc
```

The SSH proxy and load balancer tests create their own project and two TEDs by default. To run them against TEDs that already exist instead, set:

```shell
export BETTEREDGE_PROJECT_ID="..."         # project that owns the TEDs
export BETTEREDGE_VM_IDS="<vm_id>,<vm_id>" # at least two, comma-separated
TF_ACC=1 go test -v -timeout 60m -run 'TestAccSSHProxyResource|TestAccLoadBalancerRuleResource' ./internal/provider/
```

These tests enable and then disable SSH proxy access on those TEDs, so don't point them at TEDs whose proxy you want to keep.

### Pointing Terraform at your local build

Point `~/.terraformrc` at your local build:

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/real-cis/betteredge" = "/path/to/terraform-provider-betteredge"
  }

  direct {}
}
```

Skip `terraform init` while `dev_overrides` is active.
