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

func TestAccProjectResource(t *testing.T) {
	if os.Getenv("BETTEREDGE_PLATFORM_URL") == "" || os.Getenv("BETTEREDGE_API_TOKEN") == "" {
		t.Skip("BETTEREDGE_PLATFORM_URL and BETTEREDGE_API_TOKEN must be set to run this acceptance test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectResourceConfig("tf-acc-test", "created by acceptance test"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("betteredge_project.test", tfjsonpath.New("name"), knownvalue.StringExact("tf-acc-test")),
					statecheck.ExpectKnownValue("betteredge_project.test", tfjsonpath.New("description"), knownvalue.StringExact("created by acceptance test")),
				},
			},
			{
				ResourceName:      "betteredge_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccProjectResourceConfig("tf-acc-test-renamed", "created by acceptance test"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("betteredge_project.test", tfjsonpath.New("name"), knownvalue.StringExact("tf-acc-test-renamed")),
				},
			},
		},
	})
}

func testAccProjectResourceConfig(name, description string) string {
	return fmt.Sprintf(`
resource "betteredge_project" "test" {
  name        = %[1]q
  description = %[2]q
}
`, name, description)
}
