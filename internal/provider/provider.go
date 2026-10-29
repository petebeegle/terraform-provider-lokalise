// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
)

const defaultAPIURL = "https://api.lokalise.com/api2"

// Ensure LokaliseProvider satisfies the provider.Provider interface.
var _ provider.Provider = &LokaliseProvider{}

// LokaliseProvider defines the provider implementation.
type LokaliseProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// LokaliseProviderModel describes the provider data model.
type LokaliseProviderModel struct {
	Token  types.String `tfsdk:"token"`
	APIURL types.String `tfsdk:"api_url"`
}

func (p *LokaliseProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "lokalise"
	resp.Version = p.version
}

func (p *LokaliseProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"token": schema.StringAttribute{
				MarkdownDescription: "Lokalise API token. May also be set via the LOKALISE_API_TOKEN environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"api_url": schema.StringAttribute{
				MarkdownDescription: "Lokalise API base URL. Defaults to " + defaultAPIURL + ".",
				Optional:            true,
			},
		},
	}
}

func (p *LokaliseProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data LokaliseProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	token := data.Token.ValueString()
	if token == "" {
		token = os.Getenv("LOKALISE_API_TOKEN")
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing Lokalise API token",
			"The provider requires a Lokalise API token. Set the token attribute or the LOKALISE_API_TOKEN environment variable.",
		)
		return
	}

	apiURL := data.APIURL.ValueString()
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	c := client.New(apiURL, token)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *LokaliseProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewProjectLanguageResource,
		NewTeamUserGroupResource,
		NewTeamUserGroupMemberResource,
		NewTeamUserGroupProjectResource,
	}
}

func (p *LokaliseProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPermissionTemplateDataSource,
		NewTeamUserDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &LokaliseProvider{
			version: version,
		}
	}
}

// configureClient extracts a client.API from providerData, as set by
// LokaliseProvider.Configure on resp.ResourceData / resp.DataSourceData.
// It returns nil if providerData is nil (Configure has not yet run), and
// appends an error diagnostic if providerData is not a client.API.
func configureClient(providerData any, diags *diag.Diagnostics) client.API {
	if providerData == nil {
		return nil
	}

	c, ok := providerData.(client.API)
	if !ok {
		diags.AddError(
			"Unexpected Configure Type",
			"Expected client.API, got a different type. This is a bug in the lokalise provider; please report it.",
		)
		return nil
	}

	return c
}
