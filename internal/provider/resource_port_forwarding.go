// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/real-cis/terraform-provider-betteredge/internal/client"
)

var (
	_ resource.Resource                = &PortForwardingResource{}
	_ resource.ResourceWithImportState = &PortForwardingResource{}
)

func NewPortForwardingResource() resource.Resource {
	return &PortForwardingResource{}
}

type PortForwardingResource struct {
	client *client.Client
}

type PortForwardingResourceModel struct {
	Id       types.String `tfsdk:"id"`
	VMID     types.String `tfsdk:"vm_id"`
	Port     types.String `tfsdk:"port"`
	Protocol types.String `tfsdk:"protocol"`
}

type portForwardingRule struct {
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
}

type portForwardingIDPlanModifier struct{}

func (m portForwardingIDPlanModifier) Description(ctx context.Context) string {
	return "Keeps id stable when port is unchanged; lets it be recomputed when port changes, since id is derived from vm_id/port."
}

func (m portForwardingIDPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m portForwardingIDPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() {
		return
	}

	var statePort, planPort types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("port"), &statePort)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("port"), &planPort)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if statePort.Equal(planPort) {
		resp.PlanValue = req.StateValue
	}
}

func (r *PortForwardingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_forwarding"
}

func (r *PortForwardingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A single port forwarding rule on a BetterEdge TED.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Synthetic identifier (`<vm_id>/<port>`) - the platform API has no per-rule ID.",
				PlanModifiers: []planmodifier.String{
					portForwardingIDPlanModifier{},
				},
			},
			"vm_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the VM to update.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"port": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Port to forward.",
			},
			"protocol": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "tcp or udp.",
				Default:             stringdefault.StaticString("tcp"),
				Validators: []validator.String{
					stringvalidator.OneOf("tcp", "udp"),
				},
			},
		},
	}
}

func (r *PortForwardingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

type vmLookupResponse struct {
	Data struct {
		PortForwardingRules []portForwardingRule `json:"portForwardingRules"`
	} `json:"data"`
}

func portForwardingRulesEqual(a, b []portForwardingRule) bool {
	if len(a) != len(b) {
		return false
	}

	toSet := func(rules []portForwardingRule) map[portForwardingRule]struct{} {
		set := make(map[portForwardingRule]struct{}, len(rules))
		for _, rule := range rules {
			set[rule] = struct{}{}
		}
		return set
	}

	setA, setB := toSet(a), toSet(b)
	if len(setA) != len(setB) {
		return false
	}
	for rule := range setA {
		if _, ok := setB[rule]; !ok {
			return false
		}
	}
	return true
}

func (r *PortForwardingResource) applyRule(ctx context.Context, vmID, portToRemove string, newRule *portForwardingRule) error {
	var current vmLookupResponse
	if err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/lookup/vms/%s", vmID), nil, &current); err != nil {
		return fmt.Errorf("reading current port forwarding rules: %w", err)
	}

	desired := make([]portForwardingRule, 0, len(current.Data.PortForwardingRules)+1)
	for _, rule := range current.Data.PortForwardingRules {
		if portToRemove != "" && rule.Port == portToRemove {
			continue
		}
		desired = append(desired, rule)
	}
	if newRule != nil {
		desired = append(desired, *newRule)
	}

	if portForwardingRulesEqual(current.Data.PortForwardingRules, desired) {
		return nil
	}

	body := struct {
		Rules []portForwardingRule `json:"rules"`
	}{Rules: desired}

	var jobResp struct {
		JobID string `json:"jobId"`
	}
	if err := r.client.Do(ctx, http.MethodPost, fmt.Sprintf("/api/vms/%s/port-forwarding", vmID), body, &jobResp); err != nil {
		return fmt.Errorf("updating port forwarding rules: %w", err)
	}

	if _, err := r.client.PollJob(ctx, jobResp.JobID); err != nil {
		return fmt.Errorf("port forwarding update failed: %w", err)
	}

	return nil
}

func (r *PortForwardingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PortForwardingResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vmID := data.VMID.ValueString()
	port := data.Port.ValueString()
	newRule := portForwardingRule{Port: port, Protocol: data.Protocol.ValueString()}

	if err := r.applyRule(ctx, vmID, port, &newRule); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to create port forwarding rule: %s", err))
		return
	}

	data.Id = types.StringValue(fmt.Sprintf("%s/%s", vmID, port))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PortForwardingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PortForwardingResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var vm vmLookupResponse
	err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/lookup/vms/%s", data.VMID.ValueString()), nil, &vm)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to read port forwarding rules for TED %s: %s", data.VMID.ValueString(), err))
		return
	}

	found := false
	for _, rule := range vm.Data.PortForwardingRules {
		if rule.Port == data.Port.ValueString() {
			data.Protocol = types.StringValue(rule.Protocol)
			found = true
			break
		}
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PortForwardingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PortForwardingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	var state PortForwardingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	vmID := plan.VMID.ValueString()
	newRule := portForwardingRule{Port: plan.Port.ValueString(), Protocol: plan.Protocol.ValueString()}

	if err := r.applyRule(ctx, vmID, state.Port.ValueString(), &newRule); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to update port forwarding rule: %s", err))
		return
	}

	plan.Id = types.StringValue(fmt.Sprintf("%s/%s", vmID, plan.Port.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PortForwardingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PortForwardingResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.applyRule(ctx, data.VMID.ValueString(), data.Port.ValueString(), nil); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to remove port forwarding rule: %s", err))
		return
	}
}

func (r *PortForwardingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier in the format <vm_id>/<port>, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("port"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
