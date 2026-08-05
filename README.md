# Terraform Provider - real-cis/betteredge

Terraform provider to manage projects and Trusted Execution Domains (TEDs) on the BetterEdge platform.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.25 (only if building the provider yourself)

## Provider Configuration

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

A project on the BetterEdge platform. TEDs are created inside a project.

| Attribute | Required | Description |
|---|---|---|
| `name` | yes | Name of the project. Must be 4-32 characters long. Forces replacement on change (no update endpoint). |
| `description` | no (default `""`) | Free-text description. Forces replacement on change. |
| `id` | computed | UUID of the project. |

```hcl
resource "betteredge_project" "example" {
  name        = "example-project"
  description = "Created by Terraform"
}
```

### `betteredge_ted`

A Trusted Execution Domain (TED) on the BetterEdge platform. Every attribute forces replacement on change - the platform API has no update endpoint for an existing TED.

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

Creation blocks until the platform's provisioning job completes, so `terraform apply` can take a few minutes for this resource.

### `betteredge_port_forwarding`

A single port forwarding rule on a TED. The platform only exposes a full-list replace endpoint, so this resource reads the TED's current rules, replaces the one it owns, and writes the whole list back - each `betteredge_port_forwarding` block manages exactly one rule.

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

*Note:* Acceptance tests create real resources on the BetterEdge platform, and may cost money to run.

```shell
export BETTEREDGE_PLATFORM_URL="https://..."
export BETTEREDGE_API_TOKEN="..."
export TED_PASSWORD="..."                     # required for TED/port forwarding tests
export BETTEREDGE_TEST_MODULE_ID="..."         # optional, enables the manual placement test
export BETTEREDGE_TEST_SERVER_ID="..."         # optional, enables the manual placement test
make testacc
```

### Testing locally without publishing

Point `~/.terraformrc` at your local build so Terraform uses it instead of the registry:

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/real-cis/betteredge" = "/path/to/terraform-provider-betteredge"
  }

  direct {}
}
```

With `dev_overrides` active, skip `terraform init` - it isn't needed and Terraform will warn that the override is in effect.
