// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/real-cis/terraform-provider-betteredge/internal/client"
)

var (
	_ resource.Resource                = &TEDResource{}
	_ resource.ResourceWithImportState = &TEDResource{}
)

func NewTEDResource() resource.Resource {
	return &TEDResource{}
}

type TEDResource struct {
	client *client.Client
}

type TEDResourceModel struct {
	Id             types.String `tfsdk:"id"`
	ProjectID      types.String `tfsdk:"project_id"`
	OSName         types.String `tfsdk:"os_name"`
	VCPU           types.Int64  `tfsdk:"vcpu"`
	MemoryGB       types.Int64  `tfsdk:"memory_gb"`
	StorageGB      types.Int64  `tfsdk:"storage_gb"`
	Description    types.String `tfsdk:"description"`
	Password       types.String `tfsdk:"password"`
	IncludeSSHKeys types.Bool   `tfsdk:"include_ssh_keys"`
	PublicSSHKeys  types.List   `tfsdk:"public_ssh_keys"`
	ModuleID       types.String `tfsdk:"module_id"`
	ServerID       types.String `tfsdk:"server_id"`
}

var immutableTED = immutableAfterCreate{resourceName: "TED"}

func (r *TEDResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ted"
}

func (r *TEDResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Trusted Execution Domain (TED) on the BetterEdge platform. Only `description` can be changed after creation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "UUID of the TED.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project the TED will be created in.",
				PlanModifiers: []planmodifier.String{
					immutableTED,
				},
			},
			"os_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Operating system image to use. Must be one of the values returned by the platform's config endpoint.",
				PlanModifiers: []planmodifier.String{
					immutableTED,
				},
			},
			"vcpu": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Number of vCPUs. Must be one of the values returned by the platform's config endpoint.",
				PlanModifiers: []planmodifier.Int64{
					immutableTED,
				},
			},
			"memory_gb": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Memory size in GB. Must be one of the values returned by the platform's config endpoint.",
				PlanModifiers: []planmodifier.Int64{
					immutableTED,
				},
			},
			"storage_gb": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Storage size in GB. Must be one of the values returned by the platform's config endpoint.",
				PlanModifiers: []planmodifier.Int64{
					immutableTED,
				},
			},
			"description": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Free-text description for the TED.",
			},
			"password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Password for the default user. Required by the platform API regardless of include_ssh_keys.",
				PlanModifiers: []planmodifier.String{
					immutableTED,
				},
			},
			"include_ssh_keys": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether to also grant SSH access via public_ssh_keys, in addition to the password.",
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					immutableTED,
				},
			},
			"public_ssh_keys": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "List of SSH public keys to authorize. Required when include_ssh_keys is true.",
				Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})),
				PlanModifiers: []planmodifier.List{
					immutableTED,
				},
			},
			"module_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "UUID of the module to place the TED on. If omitted, the platform chooses placement automatically.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					immutableTED,
				},
			},
			"server_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "UUID of the server to place the TED on. If omitted, the platform chooses placement automatically.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					immutableTED,
				},
			},
		},
	}
}

func (r *TEDResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

type vmsConfigResponse struct {
	CPUOptions           []int64  `json:"cpuOptions"`
	MemorySizeOptionsGB  []int64  `json:"memorySizeOptionsGB"`
	StorageSizeOptionsGB []int64  `json:"storageSizeOptionsGB"`
	OSOptions            []string `json:"osOptions"`
}

func containsInt64(haystack []int64, needle int64) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func containsString(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

type createTEDRequest struct {
	ProjectID      string   `json:"projectId"`
	OSName         string   `json:"osName"`
	Description    string   `json:"description"`
	VCPU           int64    `json:"vcpu"`
	MemoryMB       int64    `json:"memoryMb"`
	StorageMB      int64    `json:"storageMb"`
	IncludeSSHKeys bool     `json:"includeSshKeys"`
	PublicSSHKeys  []string `json:"publicSshKeys"`
	Password       string   `json:"password"`
	ModuleID       string   `json:"moduleId,omitempty"`
	ServerID       string   `json:"serverId,omitempty"`
}

type createTEDResponse struct {
	JobID string `json:"jobId"`
}

func (r *TEDResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TEDResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var publicSSHKeys []string
	resp.Diagnostics.Append(data.PublicSSHKeys.ElementsAs(ctx, &publicSSHKeys, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.IncludeSSHKeys.ValueBool() && len(publicSSHKeys) == 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("public_ssh_keys"),
			"Missing SSH Keys",
			"public_ssh_keys must be set when include_ssh_keys is true.",
		)
		return
	}

	var config vmsConfigResponse
	if err := r.client.Do(ctx, http.MethodGet, "/api/vms/config", nil, &config); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to read VM config options: %s", err))
		return
	}

	if !containsInt64(config.CPUOptions, data.VCPU.ValueInt64()) {
		resp.Diagnostics.AddAttributeError(path.Root("vcpu"), "Invalid vCPU", fmt.Sprintf("Must be one of: %v", config.CPUOptions))
	}
	if !containsInt64(config.MemorySizeOptionsGB, data.MemoryGB.ValueInt64()) {
		resp.Diagnostics.AddAttributeError(path.Root("memory_gb"), "Invalid Memory Size", fmt.Sprintf("Must be one of: %v", config.MemorySizeOptionsGB))
	}
	if !containsInt64(config.StorageSizeOptionsGB, data.StorageGB.ValueInt64()) {
		resp.Diagnostics.AddAttributeError(path.Root("storage_gb"), "Invalid Storage Size", fmt.Sprintf("Must be one of: %v", config.StorageSizeOptionsGB))
	}
	if !containsString(config.OSOptions, data.OSName.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("os_name"), "Invalid OS", fmt.Sprintf("Must be one of: %v", config.OSOptions))
	}
	if resp.Diagnostics.HasError() {
		return
	}

	body := createTEDRequest{
		ProjectID:      data.ProjectID.ValueString(),
		OSName:         data.OSName.ValueString(),
		Description:    data.Description.ValueString(),
		VCPU:           data.VCPU.ValueInt64(),
		MemoryMB:       data.MemoryGB.ValueInt64() * 1024,
		StorageMB:      data.StorageGB.ValueInt64() * 1024,
		IncludeSSHKeys: data.IncludeSSHKeys.ValueBool(),
		PublicSSHKeys:  publicSSHKeys,
		Password:       data.Password.ValueString(),
		ModuleID:       data.ModuleID.ValueString(),
		ServerID:       data.ServerID.ValueString(),
	}

	var createResp createTEDResponse
	if err := r.client.Do(ctx, http.MethodPost, "/api/vms", body, &createResp); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to create TED: %s", err))
		return
	}

	journal, err := r.client.PollJob(ctx, createResp.JobID)
	if err != nil {
		resp.Diagnostics.AddError("BetterEdge Job Error", fmt.Sprintf("TED creation failed: %s", err))
		return
	}

	vmID := journal.ResourceID()
	data.Id = types.StringValue(vmID)

	var vmDetails vmDetailsResponse
	if err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/lookup/vms/%s", vmID), nil, &vmDetails); err != nil {
		if data.ModuleID.IsUnknown() {
			data.ModuleID = types.StringValue("")
		}
		if data.ServerID.IsUnknown() {
			data.ServerID = types.StringValue("")
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("TED %s was created but its details could not be read: %s", vmID, err))
		return
	}

	data.ModuleID = resolveComputedString(data.ModuleID, vmDetails.ModuleID)
	data.ServerID = resolveComputedString(data.ServerID, vmDetails.ServerID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type vmDetailsResponse struct {
	ProjectID   string `json:"projectId"`
	OSName      string `json:"osName"`
	ModuleID    string `json:"moduleId"`
	ServerID    string `json:"serverId"`
	Description string `json:"description"`
	Specs       struct {
		VCPU      int64 `json:"vcpu"`
		MemoryMB  int64 `json:"memoryMb"`
		StorageMB int64 `json:"storageMb"`
	} `json:"specs"`
}

func resolveComputedString(planned types.String, apiValue string) types.String {
	if !planned.IsUnknown() {
		return planned
	}
	if apiValue != "" {
		return types.StringValue(apiValue)
	}
	return types.StringValue("")
}

func (r *TEDResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TEDResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var vmDetails vmDetailsResponse
	err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/lookup/vms/%s", data.Id.ValueString()), nil, &vmDetails)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to read TED %s: %s", data.Id.ValueString(), err))
		return
	}

	data.ProjectID = types.StringValue(vmDetails.ProjectID)
	data.OSName = types.StringValue(vmDetails.OSName)
	data.Description = types.StringValue(vmDetails.Description)
	data.VCPU = types.Int64Value(vmDetails.Specs.VCPU)
	data.MemoryGB = types.Int64Value(vmDetails.Specs.MemoryMB / 1024)
	data.StorageGB = types.Int64Value(vmDetails.Specs.StorageMB / 1024)
	data.ModuleID = types.StringValue(vmDetails.ModuleID)
	data.ServerID = types.StringValue(vmDetails.ServerID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type updateTEDRequest struct {
	Description string `json:"description"`
}

// Update only calls the API when description changes.
func (r *TEDResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state TEDResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Description.Equal(state.Description) {
		body := updateTEDRequest{Description: data.Description.ValueString()}

		if err := r.client.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/vms/%s", data.Id.ValueString()), body, nil); err != nil {
			resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to update description of TED %s: %s", data.Id.ValueString(), err))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type deleteTEDResponse struct {
	JobID string `json:"jobId"`
}

func (r *TEDResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TEDResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vmID := data.Id.ValueString()
	lookupPath := fmt.Sprintf("/api/lookup/vms/%s", vmID)

	exists, err := r.client.Exists(ctx, lookupPath)
	if err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to check TED %s before deletion: %s", vmID, err))
		return
	}
	if !exists {
		return
	}

	var deleteResp deleteTEDResponse
	if err := r.client.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/vms/%s", vmID), nil, &deleteResp); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to delete TED %s: %s", vmID, err))
		return
	}

	_, err = r.client.PollJob(ctx, deleteResp.JobID, client.WithSuccessCheck(func(ctx context.Context) (bool, error) {
		stillExists, err := r.client.Exists(ctx, lookupPath)
		if err != nil {
			return false, err
		}
		return !stillExists, nil
	}))
	if err != nil {
		resp.Diagnostics.AddError("BetterEdge Job Error", fmt.Sprintf("TED deletion failed: %s", err))
		return
	}
}

func (r *TEDResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
