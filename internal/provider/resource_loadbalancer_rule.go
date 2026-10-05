// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/real-cis/terraform-provider-betteredge/internal/client"
)

var (
	_ resource.Resource                   = &LoadBalancerRuleResource{}
	_ resource.ResourceWithImportState    = &LoadBalancerRuleResource{}
	_ resource.ResourceWithValidateConfig = &LoadBalancerRuleResource{}
)

func NewLoadBalancerRuleResource() resource.Resource {
	return &LoadBalancerRuleResource{}
}

type LoadBalancerRuleResource struct {
	client *client.Client
}

type LoadBalancerRuleResourceModel struct {
	Id               types.String `tfsdk:"id"`
	ProjectID        types.String `tfsdk:"project_id"`
	Type             types.String `tfsdk:"type"`
	FrontendPort     types.Int64  `tfsdk:"frontend_port"`
	BackendPort      types.Int64  `tfsdk:"backend_port"`
	BackendVMIDs     types.Set    `tfsdk:"backend_vm_ids"`
	SNIKey           types.String `tfsdk:"sni_key"`
	ProxyProtocol    types.Bool   `tfsdk:"proxy_protocol"`
	Status           types.String `tfsdk:"status"`
	LoadBalancerIPv4 types.String `tfsdk:"load_balancer_ipv4"`
	LoadBalancerIPv6 types.String `tfsdk:"load_balancer_ipv6"`
}

const lbProxyProtocolV2 = "PROXY_V2"

type lbBackend struct {
	VMID        string `json:"vmId"`
	IPv6Address string `json:"ipv6Address,omitempty"`
}

type lbRule struct {
	ID               string      `json:"id"`
	Type             string      `json:"type"`
	FrontendPort     int64       `json:"frontendPort"`
	BackendPort      int64       `json:"backendPort"`
	SNIKey           *string     `json:"sniKey"`
	ProxyProtocol    *string     `json:"proxyProtocol"`
	Status           string      `json:"status"`
	Backends         []lbBackend `json:"backends"`
	LoadBalancerIPv4 string      `json:"loadBalancerIpv4"`
	LoadBalancerIPv6 string      `json:"loadBalancerIpv6"`
}

func (r lbRule) backendVMIDs() []string {
	ids := make([]string, 0, len(r.Backends))
	for _, b := range r.Backends {
		ids = append(ids, b.VMID)
	}
	return ids
}

type createLBRuleRequest struct {
	ProjectID     string      `json:"projectId"`
	Type          string      `json:"type"`
	BackendPort   int64       `json:"backendPort"`
	Backends      []lbBackend `json:"backends"`
	ProxyProtocol string      `json:"proxyProtocol,omitempty"`
	FrontendPort  int64       `json:"frontendPort,omitempty"`
	SNIKey        string      `json:"sniKey,omitempty"`
}

type updateLBRuleBackendsRequest struct {
	ProjectID string   `json:"projectId"`
	VMIDs     []string `json:"vmIds"`
}

type lbConfigResponse struct {
	SNIPorts []int64 `json:"sniPorts"`
}

func (r *LoadBalancerRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_loadbalancer_rule"
}

func (r *LoadBalancerRuleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A load balancing rule on the BetterEdge platform, distributing traffic across one or more TED backends. A TED can only be used once per backend port. Only `backend_vm_ids` can be changed in place; every other argument forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "UUID of the rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project the rule is created in.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`TCP` does plain TCP forwarding and has its frontend port assigned automatically by the platform. `SNI` does TLS routing by SNI domain on a shared frontend port and requires `frontend_port` and `sni_key`.",
				Default:             stringdefault.StaticString("TCP"),
				Validators: []validator.String{
					stringvalidator.OneOf("TCP", "SNI"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"frontend_port": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Public-facing port clients connect to. Required when `type` is `SNI`, where it must be one of the platform's SNI ports. Must be omitted for `TCP`, where the platform assigns it.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
					int64planmodifier.RequiresReplace(),
				},
			},
			"backend_port": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Port on each backend TED that traffic is forwarded to.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"backend_vm_ids": schema.SetAttribute{
				ElementType:         types.StringType,
				Required:            true,
				MarkdownDescription: "UUIDs of the TEDs to use as backends. Must contain at least one TED.",
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
			},
			"sni_key": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SNI domain used for TLS routing (e.g. `app.example.com`). Required when `type` is `SNI`. Must have a CNAME record to the project's SNI domain.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"proxy_protocol": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether to send a PROXY v2 header to the backends so they see the real client address.",
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Provisioning status of the rule.",
			},
			"load_balancer_ipv4": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IPv4 frontend address.",
			},
			"load_balancer_ipv6": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IPv6 frontend address.",
			},
		},
	}
}

func (r *LoadBalancerRuleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data LoadBalancerRuleResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data.Type.IsUnknown() {
		return
	}

	if data.Type.ValueString() == "SNI" {
		if data.FrontendPort.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("frontend_port"), "Missing frontend_port", "frontend_port is required when type is SNI.")
		}
		if data.SNIKey.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("sni_key"), "Missing sni_key", "sni_key is required when type is SNI.")
		}
		return
	}

	if !data.FrontendPort.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("frontend_port"), "Unexpected frontend_port", "frontend_port is assigned by the platform for TCP rules and must be omitted.")
	}
	if !data.SNIKey.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("sni_key"), "Unexpected sni_key", "sni_key is only valid when type is SNI.")
	}
}

func (r *LoadBalancerRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = c
}

func (r *LoadBalancerRuleResource) listRules(ctx context.Context, projectID string) ([]lbRule, error) {
	var rules []lbRule
	if err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/loadbalancer/rules?projectId=%s", projectID), nil, &rules); err != nil {
		return nil, fmt.Errorf("listing load balancing rules: %w", err)
	}
	return rules, nil
}

func sameVMIDs(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

// checkVMPortConflict mirrors the platform rule that a TED can only be used once per backend port.
func checkVMPortConflict(rules []lbRule, backendPort int64, vmIDs []string, excludeRuleID string) error {
	for _, rule := range rules {
		if rule.ID == excludeRuleID || rule.BackendPort != backendPort {
			continue
		}

		var overlapping []string
		for _, id := range rule.backendVMIDs() {
			if slices.Contains(vmIDs, id) {
				overlapping = append(overlapping, id)
			}
		}
		if len(overlapping) > 0 {
			sort.Strings(overlapping)
			return fmt.Errorf("TED(s) %s already use backend port %d in rule %s", strings.Join(overlapping, ", "), backendPort, rule.ID)
		}
	}
	return nil
}

// findEquivalentRule returns an existing rule with the same configuration, if any.
func findEquivalentRule(rules []lbRule, data LoadBalancerRuleResourceModel, vmIDs []string) (*lbRule, error) {
	ruleType := data.Type.ValueString()

	for i, rule := range rules {
		if rule.Type != ruleType {
			continue
		}

		sameConfig := rule.BackendPort == data.BackendPort.ValueInt64() &&
			(rule.ProxyProtocol != nil && *rule.ProxyProtocol == lbProxyProtocolV2) == data.ProxyProtocol.ValueBool() &&
			sameVMIDs(rule.backendVMIDs(), vmIDs)

		if ruleType == "TCP" {
			if sameConfig {
				return &rules[i], nil
			}
			continue
		}

		if rule.SNIKey != nil && *rule.SNIKey == data.SNIKey.ValueString() && rule.FrontendPort == data.FrontendPort.ValueInt64() {
			if sameConfig {
				return &rules[i], nil
			}
			return nil, fmt.Errorf("an SNI rule for %s on port %d already exists with a different configuration (rule %s)", data.SNIKey.ValueString(), data.FrontendPort.ValueInt64(), rule.ID)
		}
	}

	return nil, nil
}

func (m *LoadBalancerRuleResourceModel) setRule(ctx context.Context, rule lbRule) error {
	m.Id = types.StringValue(rule.ID)
	m.Type = types.StringValue(rule.Type)
	m.FrontendPort = types.Int64Value(rule.FrontendPort)
	m.BackendPort = types.Int64Value(rule.BackendPort)
	m.ProxyProtocol = types.BoolValue(rule.ProxyProtocol != nil && *rule.ProxyProtocol == lbProxyProtocolV2)
	m.Status = types.StringValue(rule.Status)
	m.LoadBalancerIPv4 = types.StringValue(rule.LoadBalancerIPv4)
	m.LoadBalancerIPv6 = types.StringValue(rule.LoadBalancerIPv6)

	if rule.SNIKey != nil && *rule.SNIKey != "" {
		m.SNIKey = types.StringValue(*rule.SNIKey)
	} else {
		m.SNIKey = types.StringNull()
	}

	vmIDs, diags := types.SetValueFrom(ctx, types.StringType, rule.backendVMIDs())
	if diags.HasError() {
		return fmt.Errorf("converting backend_vm_ids: %v", diags)
	}
	m.BackendVMIDs = vmIDs

	return nil
}

func (r *LoadBalancerRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data LoadBalancerRuleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var vmIDs []string
	resp.Diagnostics.Append(data.BackendVMIDs.ElementsAs(ctx, &vmIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := data.ProjectID.ValueString()

	rules, err := r.listRules(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to create load balancing rule: %s", err))
		return
	}

	rule, err := findEquivalentRule(rules, data, vmIDs)
	if err != nil {
		resp.Diagnostics.AddError("Conflicting Load Balancing Rule", err.Error())
		return
	}

	if rule == nil {
		if err := checkVMPortConflict(rules, data.BackendPort.ValueInt64(), vmIDs, ""); err != nil {
			resp.Diagnostics.AddError("Conflicting Load Balancing Rule", err.Error())
			return
		}

		body := createLBRuleRequest{
			ProjectID:   projectID,
			Type:        data.Type.ValueString(),
			BackendPort: data.BackendPort.ValueInt64(),
			Backends:    make([]lbBackend, 0, len(vmIDs)),
		}
		for _, id := range vmIDs {
			body.Backends = append(body.Backends, lbBackend{VMID: id})
		}
		if data.ProxyProtocol.ValueBool() {
			body.ProxyProtocol = lbProxyProtocolV2
		}

		if body.Type == "SNI" {
			var config lbConfigResponse
			if err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/loadbalancer/config?projectId=%s", projectID), nil, &config); err != nil {
				resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to read load balancer config: %s", err))
				return
			}
			if !slices.Contains(config.SNIPorts, data.FrontendPort.ValueInt64()) {
				resp.Diagnostics.AddAttributeError(path.Root("frontend_port"), "Invalid frontend_port", fmt.Sprintf("frontend_port must be one of %v.", config.SNIPorts))
				return
			}

			body.FrontendPort = data.FrontendPort.ValueInt64()
			body.SNIKey = data.SNIKey.ValueString()
		}

		rule = &lbRule{}
		if err := r.client.Do(ctx, http.MethodPost, "/api/loadbalancer/rules", body, rule); err != nil {
			var apiErr *client.APIError
			if errors.As(err, &apiErr) && strings.Contains(apiErr.Body, "LB_SNI_DOMAIN_NOT_VERIFIED") {
				resp.Diagnostics.AddAttributeError(path.Root("sni_key"), fmt.Sprintf("The SNI domain %s must be valid", body.SNIKey), "")
				return
			}
			resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to create load balancing rule: %s", err))
			return
		}
	}

	if err := data.setRule(ctx, *rule); err != nil {
		resp.Diagnostics.AddError("Provider Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LoadBalancerRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data LoadBalancerRuleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rules, err := r.listRules(ctx, data.ProjectID.ValueString())
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to read load balancing rule %s: %s", data.Id.ValueString(), err))
		return
	}

	idx := slices.IndexFunc(rules, func(rule lbRule) bool { return rule.ID == data.Id.ValueString() })
	if idx < 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	if err := data.setRule(ctx, rules[idx]); err != nil {
		resp.Diagnostics.AddError("Provider Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LoadBalancerRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan LoadBalancerRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var vmIDs []string
	resp.Diagnostics.Append(plan.BackendVMIDs.ElementsAs(ctx, &vmIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ruleID := plan.Id.ValueString()
	projectID := plan.ProjectID.ValueString()

	rules, err := r.listRules(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to update load balancing rule %s: %s", ruleID, err))
		return
	}

	idx := slices.IndexFunc(rules, func(rule lbRule) bool { return rule.ID == ruleID })
	if idx < 0 {
		resp.Diagnostics.AddError("Load Balancing Rule Not Found", fmt.Sprintf("Load balancing rule %s not found in project %s.", ruleID, projectID))
		return
	}

	rule := rules[idx]

	if !sameVMIDs(rule.backendVMIDs(), vmIDs) {
		if err := checkVMPortConflict(rules, rule.BackendPort, vmIDs, ruleID); err != nil {
			resp.Diagnostics.AddError("Conflicting Load Balancing Rule", err.Error())
			return
		}

		body := updateLBRuleBackendsRequest{ProjectID: projectID, VMIDs: vmIDs}
		rule = lbRule{}
		if err := r.client.Do(ctx, http.MethodPut, fmt.Sprintf("/api/loadbalancer/rules/%s/backends", ruleID), body, &rule); err != nil {
			resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to update load balancing rule backends: %s", err))
			return
		}
	}

	if err := plan.setRule(ctx, rule); err != nil {
		resp.Diagnostics.AddError("Provider Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LoadBalancerRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LoadBalancerRuleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/loadbalancer/rules/%s", data.Id.ValueString()), nil, nil)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound && !strings.Contains(apiErr.Body, "PROJECT_NOT_FOUND") {
			return
		}
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to delete load balancing rule %s: %s", data.Id.ValueString(), err))
		return
	}
}

func (r *LoadBalancerRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier in the format <project_id>/<rule_id>, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
