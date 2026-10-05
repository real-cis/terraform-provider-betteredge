// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/real-cis/terraform-provider-betteredge/internal/client"
)

var (
	_ resource.Resource                = &SSHProxyResource{}
	_ resource.ResourceWithImportState = &SSHProxyResource{}
)

func NewSSHProxyResource() resource.Resource {
	return &SSHProxyResource{}
}

type SSHProxyResource struct {
	client *client.Client
}

type SSHProxyResourceModel struct {
	Id               types.String `tfsdk:"id"`
	VMID             types.String `tfsdk:"vm_id"`
	FrontendPort     types.Int64  `tfsdk:"frontend_port"`
	LoadBalancerIPv4 types.String `tfsdk:"load_balancer_ipv4"`
	LoadBalancerIPv6 types.String `tfsdk:"load_balancer_ipv6"`
	Status           types.String `tfsdk:"status"`
	SSHCommand       types.String `tfsdk:"ssh_command"`
}

type sshProxy struct {
	FrontendPort     int64  `json:"frontendPort"`
	LoadBalancerIPv4 string `json:"loadBalancerIpv4"`
	LoadBalancerIPv6 string `json:"loadBalancerIpv6"`
	Status           string `json:"status"`
}

type vmSSHProxyLookupResponse struct {
	SSHProxy *sshProxy `json:"sshProxy"`
}

func (r *SSHProxyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_proxy"
}

func (r *SSHProxyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "SSH Proxy Access on a BetterEdge TED. While it exists, the platform allocates a load balancer endpoint that forwards to port 22 of the TED, for clients without direct IPv6 connectivity.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as `vm_id` - a TED has at most one SSH proxy.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vm_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the TED to enable SSH proxy access on.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"frontend_port": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Port on the load balancer's public endpoints that forwards to the TED's port 22.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"load_balancer_ipv4": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IPv4 address of the load balancer.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"load_balancer_ipv6": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IPv6 address of the load balancer.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Provisioning status of the SSH proxy rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ssh_command": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SSH command to connect through the proxy.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *SSHProxyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (m *SSHProxyResourceModel) setProxy(proxy sshProxy) {
	m.Id = m.VMID
	m.FrontendPort = types.Int64Value(proxy.FrontendPort)
	m.LoadBalancerIPv4 = types.StringValue(proxy.LoadBalancerIPv4)
	m.LoadBalancerIPv6 = types.StringValue(proxy.LoadBalancerIPv6)
	m.Status = types.StringValue(proxy.Status)
	m.SSHCommand = types.StringValue(fmt.Sprintf("ssh -p %d linux@%s", proxy.FrontendPort, proxy.LoadBalancerIPv4))
}

func (r *SSHProxyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SSHProxyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vmID := data.VMID.ValueString()

	var vm vmSSHProxyLookupResponse
	if err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/lookup/vms/%s", vmID), nil, &vm); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to read TED %s: %s", vmID, err))
		return
	}

	proxy := vm.SSHProxy
	if proxy == nil {
		proxy = &sshProxy{}
		if err := r.client.Do(ctx, http.MethodPost, fmt.Sprintf("/api/vms/%s/ssh-proxy", vmID), nil, proxy); err != nil {
			resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to enable SSH proxy access: %s", err))
			return
		}
	}

	data.setProxy(*proxy)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SSHProxyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SSHProxyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var vm vmSSHProxyLookupResponse
	err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/lookup/vms/%s", data.VMID.ValueString()), nil, &vm)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to read SSH proxy for TED %s: %s", data.VMID.ValueString(), err))
		return
	}

	if vm.SSHProxy == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	data.setProxy(*vm.SSHProxy)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is never called with a real change: vm_id forces replacement and every other attribute is computed.
func (r *SSHProxyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SSHProxyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SSHProxyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SSHProxyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/vms/%s/ssh-proxy", data.VMID.ValueString()), nil, nil)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return
		}
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to disable SSH proxy access: %s", err))
		return
	}
}

func (r *SSHProxyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
