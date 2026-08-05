// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/real-cis/terraform-provider-betteredge/internal/client"
)

var _ provider.Provider = &BetterEdgeProvider{}

type BetterEdgeProvider struct {
	version string
}

type BetterEdgeProviderModel struct {
	PlatformURL types.String `tfsdk:"platform_url"`
	APIToken    types.String `tfsdk:"api_token"`
}

func (p *BetterEdgeProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "betteredge"
	resp.Version = p.version
}

func (p *BetterEdgeProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages projects and Trusted Execution Domains (TEDs) on the BetterEdge platform.",
		Attributes: map[string]schema.Attribute{
			"platform_url": schema.StringAttribute{
				MarkdownDescription: "Base URL of the BetterEdge platform API. Can also be set via the `BETTEREDGE_PLATFORM_URL` environment variable.",
				Optional:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "API token used to authenticate with the BetterEdge platform (sent as the `X-Token` header). Can also be set via the `BETTEREDGE_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *BetterEdgeProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data BetterEdgeProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	platformURL := data.PlatformURL.ValueString()
	if platformURL == "" {
		platformURL = os.Getenv("BETTEREDGE_PLATFORM_URL")
	}
	if platformURL == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("platform_url"),
			"Missing BetterEdge Platform URL",
			"Set platform_url in the provider configuration block or the BETTEREDGE_PLATFORM_URL environment variable.",
		)
	}

	apiToken := data.APIToken.ValueString()
	if apiToken == "" {
		apiToken = os.Getenv("BETTEREDGE_API_TOKEN")
	}
	if apiToken == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Missing BetterEdge API Token",
			"Set api_token in the provider configuration block or the BETTEREDGE_API_TOKEN environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	betterEdgeClient := client.New(platformURL, apiToken)
	resp.DataSourceData = betterEdgeClient
	resp.ResourceData = betterEdgeClient
}

func (p *BetterEdgeProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewTEDResource,
		NewPortForwardingResource,
	}
}

func (p *BetterEdgeProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &BetterEdgeProvider{
			version: version,
		}
	}
}
