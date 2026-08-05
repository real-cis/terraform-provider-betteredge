// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccPortForwardingResource(t *testing.T) {
	password := os.Getenv("TED_PASSWORD")
	if password == "" {
		t.Skip("TED_PASSWORD must be set to run this acceptance test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPortForwardingResourceConfig(password, "8443"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("betteredge_port_forwarding.test", tfjsonpath.New("port"), knownvalue.StringExact("8443")),
					statecheck.ExpectKnownValue("betteredge_port_forwarding.test", tfjsonpath.New("protocol"), knownvalue.StringExact("tcp")),
				},
			},
			{
				ResourceName:      "betteredge_port_forwarding.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccPortForwardingResourceConfig(password, "9443"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("betteredge_port_forwarding.test", tfjsonpath.New("port"), knownvalue.StringExact("9443")),
				},
			},
		},
	})
}

func testAccPortForwardingResourceConfig(password, port string) string {
	return fmt.Sprintf(`
resource "betteredge_project" "test" {
  name        = "tf-acc-pf-test"
  description = "created by acceptance test"
}

resource "betteredge_ted" "test" {
  project_id  = betteredge_project.test.id
  os_name     = "Ubuntu-26"
  vcpu        = 2
  memory_gb   = 2
  storage_gb  = 50
  description = "created by acceptance test"
  password    = %[1]q
}

resource "betteredge_port_forwarding" "test" {
  vm_id    = betteredge_ted.test.id
  port     = %[2]q
  protocol = "tcp"
}
`, password, port)
}
