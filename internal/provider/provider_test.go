// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"betteredge": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv("BETTEREDGE_PLATFORM_URL") == "" {
		t.Fatal("BETTEREDGE_PLATFORM_URL must be set for acceptance tests")
	}
	if os.Getenv("BETTEREDGE_API_TOKEN") == "" {
		t.Fatal("BETTEREDGE_API_TOKEN must be set for acceptance tests")
	}
}

// testAccTEDsConfig defines local.project_id and local.vm_ids, reusing BETTEREDGE_PROJECT_ID and
// BETTEREDGE_VM_IDS when set, or creating a project and count TEDs otherwise.
func testAccTEDsConfig(t *testing.T, name string, count int) string {
	t.Helper()

	projectID := os.Getenv("BETTEREDGE_PROJECT_ID")
	vmIDs := os.Getenv("BETTEREDGE_VM_IDS")
	if projectID != "" && vmIDs != "" {
		var ids []string
		for _, id := range strings.Split(vmIDs, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, fmt.Sprintf("%q", id))
			}
		}
		if len(ids) < count {
			t.Skipf("BETTEREDGE_VM_IDS must list at least %d TEDs to run this acceptance test", count)
		}
		return fmt.Sprintf(`
locals {
  project_id = %q
  vm_ids     = [%s]
}
`, projectID, strings.Join(ids, ", "))
	}

	password := os.Getenv("TED_PASSWORD")
	if password == "" {
		t.Skip("TED_PASSWORD, or BETTEREDGE_PROJECT_ID and BETTEREDGE_VM_IDS, must be set to run this acceptance test")
	}

	return fmt.Sprintf(`
resource "betteredge_project" "test" {
  name        = %[1]q
  description = "created by acceptance test"
}

resource "betteredge_ted" "test" {
  count       = %[2]d
  project_id  = betteredge_project.test.id
  os_name     = "Ubuntu-26"
  vcpu        = 2
  memory_gb   = 2
  storage_gb  = 50
  description = "created by acceptance test"
  password    = %[3]q
}

locals {
  project_id = betteredge_project.test.id
  vm_ids     = betteredge_ted.test[*].id
}
`, name, count, password)
}
