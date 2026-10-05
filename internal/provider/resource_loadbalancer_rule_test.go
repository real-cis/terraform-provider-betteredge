// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccLoadBalancerRuleResource(t *testing.T) {
	setup := testAccTEDsConfig(t, "tf-acc-lb-test", 2)
	name := "betteredge_loadbalancer_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: setup + testAccLoadBalancerRuleResourceConfig("[local.vm_ids[0]]", 8443, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(name, tfjsonpath.New("type"), knownvalue.StringExact("TCP")),
					statecheck.ExpectKnownValue(name, tfjsonpath.New("backend_port"), knownvalue.Int64Exact(8443)),
					statecheck.ExpectKnownValue(name, tfjsonpath.New("backend_vm_ids"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(name, tfjsonpath.New("frontend_port"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(name, tfjsonpath.New("proxy_protocol"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(name, tfjsonpath.New("sni_key"), knownvalue.Null()),
				},
			},
			{
				ResourceName:            name,
				ImportState:             true,
				ImportStateIdFunc:       testAccLoadBalancerRuleImportID(name),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"status"},
			},
			{
				// Only backend_vm_ids changes in place.
				Config: setup + testAccLoadBalancerRuleResourceConfig("local.vm_ids", 8443, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(name, tfjsonpath.New("backend_vm_ids"), knownvalue.SetSizeExact(2)),
				},
			},
			{
				Config: setup + testAccLoadBalancerRuleResourceConfig("local.vm_ids", 9443, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(name, tfjsonpath.New("backend_port"), knownvalue.Int64Exact(9443)),
				},
			},
			{
				Config: setup + testAccLoadBalancerRuleResourceConfig("local.vm_ids", 9443, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(name, tfjsonpath.New("proxy_protocol"), knownvalue.Bool(true)),
				},
			},
		},
	})
}

func TestAccLoadBalancerRuleResource_VMPortConflict(t *testing.T) {
	setup := testAccTEDsConfig(t, "tf-acc-lb-conflict", 2)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: setup + `
resource "betteredge_loadbalancer_rule" "first" {
  project_id     = local.project_id
  backend_port   = 9080
  backend_vm_ids = [local.vm_ids[0]]
}

resource "betteredge_loadbalancer_rule" "second" {
  project_id     = local.project_id
  backend_port   = 9080
  backend_vm_ids = local.vm_ids
  depends_on     = [betteredge_loadbalancer_rule.first]
}
`,
				ExpectError: regexp.MustCompile(`already use backend port 9080`),
			},
		},
	})
}

func testAccLoadBalancerRuleImportID(name string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", name)
		}
		return fmt.Sprintf("%s/%s", rs.Primary.Attributes["project_id"], rs.Primary.ID), nil
	}
}

func testAccLoadBalancerRuleResourceConfig(backendVMIDs string, backendPort int, proxyProtocol bool) string {
	return fmt.Sprintf(`
resource "betteredge_loadbalancer_rule" "test" {
  project_id     = local.project_id
  type           = "TCP"
  backend_port   = %[2]d
  backend_vm_ids = %[1]s
  proxy_protocol = %[3]t
}
`, backendVMIDs, backendPort, proxyProtocol)
}

// TestLoadBalancerRuleResource_Validation only plans, so it needs the Terraform CLI but not the platform.
func TestLoadBalancerRuleResource_Validation(t *testing.T) {
	cases := map[string]struct {
		attrs string
		err   string
	}{
		"tcp with sni_key":          {`backend_vm_ids = ["vm-a"]` + "\n" + `sni_key = "app.example.com"`, `sni_key is only valid when type is SNI`},
		"tcp with frontend_port":    {`backend_vm_ids = ["vm-a"]` + "\n" + `frontend_port = 443`, `frontend_port is assigned by the platform`},
		"sni without frontend_port": {`type = "SNI"` + "\n" + `backend_vm_ids = ["vm-a"]` + "\n" + `sni_key = "app.example.com"`, `frontend_port is required when type is SNI`},
		"sni without sni_key":       {`type = "SNI"` + "\n" + `backend_vm_ids = ["vm-a"]` + "\n" + `frontend_port = 443`, `sni_key is required when type is SNI`},
		"invalid type":              {`type = "UDP"` + "\n" + `backend_vm_ids = ["vm-a"]`, `value must be one of`},
		"no backends":               {`backend_vm_ids = []`, `set must contain at least 1 elements`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: fmt.Sprintf(`
provider "betteredge" {
  platform_url = "http://127.0.0.1:1"
  api_token    = "unused"
}

resource "betteredge_loadbalancer_rule" "test" {
  project_id   = "00000000-0000-0000-0000-000000000000"
  backend_port = 8443
  %s
}
`, tc.attrs),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.err),
					},
				},
			})
		})
	}
}

func strPtr(s string) *string { return &s }

func TestCheckVMPortConflict(t *testing.T) {
	rules := []lbRule{
		{ID: "rule-1", BackendPort: 8443, Backends: []lbBackend{{VMID: "vm-a"}, {VMID: "vm-b"}}},
		{ID: "rule-2", BackendPort: 9443, Backends: []lbBackend{{VMID: "vm-c"}}},
	}

	if err := checkVMPortConflict(rules, 8443, []string{"vm-c"}, ""); err != nil {
		t.Errorf("expected no conflict for a TED on a different port, got: %s", err)
	}
	if err := checkVMPortConflict(rules, 8443, []string{"vm-a"}, "rule-1"); err != nil {
		t.Errorf("expected no conflict with the excluded rule, got: %s", err)
	}

	err := checkVMPortConflict(rules, 8443, []string{"vm-b", "vm-a", "vm-d"}, "")
	if err == nil {
		t.Fatal("expected a conflict, got nil")
	}
	if !strings.Contains(err.Error(), "vm-a, vm-b") || !strings.Contains(err.Error(), "rule-1") {
		t.Errorf("unexpected error message: %s", err)
	}
}

func TestFindEquivalentRule(t *testing.T) {
	rules := []lbRule{
		{ID: "tcp-1", Type: "TCP", BackendPort: 8443, Backends: []lbBackend{{VMID: "vm-a"}, {VMID: "vm-b"}}},
		{ID: "sni-1", Type: "SNI", FrontendPort: 443, BackendPort: 8443, SNIKey: strPtr("app.example.com"), ProxyProtocol: strPtr(lbProxyProtocolV2), Backends: []lbBackend{{VMID: "vm-c"}}},
	}

	tcp := LoadBalancerRuleResourceModel{
		Type:          types.StringValue("TCP"),
		BackendPort:   types.Int64Value(8443),
		ProxyProtocol: types.BoolValue(false),
	}

	rule, err := findEquivalentRule(rules, tcp, []string{"vm-b", "vm-a"})
	if err != nil || rule == nil || rule.ID != "tcp-1" {
		t.Errorf("expected tcp-1 regardless of backend order, got rule=%v err=%v", rule, err)
	}

	rule, err = findEquivalentRule(rules, tcp, []string{"vm-a"})
	if err != nil || rule != nil {
		t.Errorf("expected no match for a different backend set, got rule=%v err=%v", rule, err)
	}

	sni := LoadBalancerRuleResourceModel{
		Type:          types.StringValue("SNI"),
		FrontendPort:  types.Int64Value(443),
		BackendPort:   types.Int64Value(8443),
		SNIKey:        types.StringValue("app.example.com"),
		ProxyProtocol: types.BoolValue(true),
	}

	rule, err = findEquivalentRule(rules, sni, []string{"vm-c"})
	if err != nil || rule == nil || rule.ID != "sni-1" {
		t.Errorf("expected sni-1, got rule=%v err=%v", rule, err)
	}

	sni.ProxyProtocol = types.BoolValue(false)
	if _, err := findEquivalentRule(rules, sni, []string{"vm-c"}); err == nil {
		t.Error("expected an error for an SNI rule on the same domain/port with a different configuration")
	}
}
