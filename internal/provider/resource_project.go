// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/real-cis/terraform-provider-betteredge/internal/client"
)

var (
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

type ProjectResource struct {
	client *client.Client
}

type ProjectResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (r *ProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A project on the BetterEdge platform. TEDs are created inside a project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "UUID of the project.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the project. Must be 4-32 characters long.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(4, 32),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Free-text description for the project. If omitted, the current description is kept (empty for a new project); set it to `\"\"` to clear it.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ProjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ProjectResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// An omitted description is unknown at this point; a new project starts with an empty one.
	if data.Description.IsUnknown() {
		data.Description = types.StringValue("")
	}

	body := createProjectRequest{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
	}

	var projectID string
	if err := r.client.Do(ctx, http.MethodPost, "/api/project", body, &projectID); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to create project: %s", err))
		return
	}

	data.Id = types.StringValue(projectID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ProjectResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var project projectDetailsResponse
	err := r.client.Do(ctx, http.MethodGet, fmt.Sprintf("/api/lookup/projects/%s", data.Id.ValueString()), nil, &project)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to read project %s: %s", data.Id.ValueString(), err))
		return
	}

	data.Name = types.StringValue(project.Name)
	data.Description = types.StringValue(project.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type projectDetailsResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateProjectRequest struct {
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ProjectResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := updateProjectRequest{
		ProjectID:   data.Id.ValueString(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
	}

	if err := r.client.Do(ctx, http.MethodPatch, "/api/project", body, nil); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to update project %s: %s", data.Id.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type deleteProjectRequest struct {
	ProjectID string `json:"projectId"`
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ProjectResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	exists, err := r.client.Exists(ctx, fmt.Sprintf("/api/lookup/projects/%s", data.Id.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to check project %s before deletion: %s", data.Id.ValueString(), err))
		return
	}
	if !exists {
		return
	}

	body := deleteProjectRequest{ProjectID: data.Id.ValueString()}
	if err := r.client.Do(ctx, http.MethodDelete, "/api/project", body, nil); err != nil {
		resp.Diagnostics.AddError("BetterEdge API Error", fmt.Sprintf("Unable to delete project %s: %s", data.Id.ValueString(), err))
		return
	}
}

func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
