// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccTEDResource(t *testing.T) {
	password := os.Getenv("TED_PASSWORD")
	if password == "" {
		t.Skip("TED_PASSWORD must be set to run this acceptance test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTEDResourceConfig(password),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("betteredge_ted.test", tfjsonpath.New("os_name"), knownvalue.StringExact("Ubuntu-26")),
					statecheck.ExpectKnownValue("betteredge_ted.test", tfjsonpath.New("vcpu"), knownvalue.Int64Exact(2)),
					statecheck.ExpectKnownValue("betteredge_ted.test", tfjsonpath.New("memory_gb"), knownvalue.Int64Exact(2)),
					statecheck.ExpectKnownValue("betteredge_ted.test", tfjsonpath.New("storage_gb"), knownvalue.Int64Exact(50)),
				},
			},
			{
				ResourceName:            "betteredge_ted.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password", "include_ssh_keys", "public_ssh_keys"},
			},
		},
	})
}

func testAccTEDResourceConfig(password string) string {
	return fmt.Sprintf(`
resource "betteredge_project" "test" {
  name        = "tf-acc-ted-test"
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
`, password)
}

func TestAccTEDResource_InvalidVCPU(t *testing.T) {
	password := os.Getenv("TED_PASSWORD")
	if password == "" {
		t.Skip("TED_PASSWORD must be set to run this acceptance test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccTEDResourceConfigWithVCPU(password, 999999),
				ExpectError: regexp.MustCompile(`Invalid vCPU`),
			},
		},
	})
}

func testAccTEDResourceConfigWithVCPU(password string, vcpu int) string {
	return fmt.Sprintf(`
resource "betteredge_project" "test" {
  name        = "tf-acc-ted-vcpu-test"
  description = "created by acceptance test"
}

resource "betteredge_ted" "test" {
  project_id  = betteredge_project.test.id
  os_name     = "Ubuntu-26"
  vcpu        = %[2]d
  memory_gb   = 2
  storage_gb  = 50
  description = "created by acceptance test"
  password    = %[1]q
}
`, password, vcpu)
}

func TestAccTEDResource_WithSSHKey(t *testing.T) {
	password := os.Getenv("TED_PASSWORD")
	if password == "" {
		t.Skip("TED_PASSWORD must be set to run this acceptance test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTEDResourceConfigWithSSHKey(password),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("betteredge_ted.test", tfjsonpath.New("include_ssh_keys"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("betteredge_ted.test", tfjsonpath.New("public_ssh_keys"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl tf-acc-test"),
						}),
					),
				},
			},
		},
	})
}

func testAccTEDResourceConfigWithSSHKey(password string) string {
	return fmt.Sprintf(`
resource "betteredge_project" "test" {
  name        = "tf-acc-ted-ssh-test"
  description = "created by acceptance test"
}

resource "betteredge_ted" "test" {
  project_id       = betteredge_project.test.id
  os_name          = "Ubuntu-26"
  vcpu             = 2
  memory_gb        = 2
  storage_gb       = 50
  description      = "created by acceptance test"
  password         = %[1]q
  include_ssh_keys = true
  public_ssh_keys  = ["ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl tf-acc-test"]
}
`, password)
}

func TestAccTEDResource_ManualPlacement(t *testing.T) {
	password := os.Getenv("TED_PASSWORD")
	moduleID := os.Getenv("BETTEREDGE_TEST_MODULE_ID")
	serverID := os.Getenv("BETTEREDGE_TEST_SERVER_ID")
	if password == "" || moduleID == "" || serverID == "" {
		t.Skip("TED_PASSWORD, BETTEREDGE_TEST_MODULE_ID and BETTEREDGE_TEST_SERVER_ID must be set to run this acceptance test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTEDResourceConfigWithPlacement(password, moduleID, serverID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("betteredge_ted.test", tfjsonpath.New("module_id"), knownvalue.StringExact(moduleID)),
					statecheck.ExpectKnownValue("betteredge_ted.test", tfjsonpath.New("server_id"), knownvalue.StringExact(serverID)),
				},
			},
		},
	})
}

func testAccTEDResourceConfigWithPlacement(password, moduleID, serverID string) string {
	return fmt.Sprintf(`
resource "betteredge_project" "test" {
  name        = "tf-acc-ted-placement-test"
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
  module_id   = %[2]q
  server_id   = %[3]q
}
`, password, moduleID, serverID)
}
