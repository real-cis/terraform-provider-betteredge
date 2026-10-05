// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccSSHProxyResource(t *testing.T) {
	setup := testAccTEDsConfig(t, "tf-acc-ssh-proxy", 2)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: setup + testAccSSHProxyResourceConfig(0),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("betteredge_ssh_proxy.test", plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("betteredge_ssh_proxy.test", tfjsonpath.New("frontend_port"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("betteredge_ssh_proxy.test", tfjsonpath.New("load_balancer_ipv4"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("betteredge_ssh_proxy.test", tfjsonpath.New("ssh_command"), knownvalue.StringRegexp(regexp.MustCompile(`^ssh -p \d+ linux@\S+$`))),
				},
			},
			{
				ResourceName:            "betteredge_ssh_proxy.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"status"},
			},
			{
				// vm_id forces replacement: the proxy is disabled on the first TED and enabled on the second.
				Config: setup + testAccSSHProxyResourceConfig(1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("betteredge_ssh_proxy.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
		},
	})
}

func testAccSSHProxyResourceConfig(vmIndex int) string {
	return fmt.Sprintf(`
resource "betteredge_ssh_proxy" "test" {
  vm_id = local.vm_ids[%d]
}
`, vmIndex)
}
