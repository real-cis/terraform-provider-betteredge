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
| `name` | yes | 4-32 characters. Forces replacement (there's no update endpoint). |
| `description` | no, default `""` | Free text. Forces replacement. |
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
| `description` | yes | Free-text description for the TED. |
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

Set `password` with `TF_VAR_ted_password` instead of typing it in each time. If the value changes between runs, Terraform reads that as a real diff and replaces the TED.

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

## Example: managing multiple TEDs in a project

[`examples/managing-multiple-teds/main.tf`](examples/managing-multiple-teds/main.tf) covers everything below. Run it from that directory with `dev_overrides` pointed at your local build.

Variables: `project_id` (leave empty to create a new project), `project_name`, `teds` (set of names), `ted_placement` (map of name -> `{module_id, server_id}`, only for names you list), `include_ssh_keys`/`public_ssh_keys`, `port`/`protocol`, `port_forwarded_teds` (subset of `teds` that gets the port rule, defaults to all of them).

**Important**: `teds` and `port_forwarded_teds` are the full set every time, not a diff. Always list every name that should still be there.

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
