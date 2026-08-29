// Package delivery implements the Terraform provider, resource and data
// source, mapping the Terraform schema to the domain via ClusterService.
package delivery

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name aiprovider

import (
	"context"
	"net/url"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-ai/internal/entity"
	"terraform-provider-ai/internal/repository"
	"terraform-provider-ai/internal/usecase"
)

// ClusterService is the business-logic abstraction used by delivery.
// It lets callers swap implementations (REST/gRPC) and mock in resource tests.
type ClusterService interface {
	CreateCluster(ctx context.Context, c entity.Cluster) (*entity.Cluster, error)
	GetCluster(ctx context.Context, id string) (*entity.Cluster, error)
	UpdateCluster(ctx context.Context, c entity.Cluster) (*entity.Cluster, error)
	DeleteCluster(ctx context.Context, id string) error
}

const defaultEndpoint = "http://localhost:8080"

type aiProvider struct {
	version         string
	defaultEndpoint string
}

type aiProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	ApiToken types.String `tfsdk:"api_token"`
}

type ProviderOption func(*aiProvider)

// WithDefaultEndpoint overrides the default endpoint (for tests).
func WithDefaultEndpoint(endpoint string) ProviderOption {
	return func(p *aiProvider) { p.defaultEndpoint = endpoint }
}

// WithVersion sets the provider version (from ldflags at build time).
func WithVersion(v string) ProviderOption {
	return func(p *aiProvider) { p.version = v }
}

func NewProvider(opts ...ProviderOption) provider.Provider {
	p := &aiProvider{version: "dev", defaultEndpoint: defaultEndpoint}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *aiProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "aiprovider"
	resp.Version = p.version
}

func (p *aiProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				// The Framework does not support Default in the provider schema;
				// the default is applied in Configure below.
				Optional:    true,
				Description: "Base URL of the AI API. Defaults to the local mock API.",
			},
			"api_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Bearer token used to authenticate against the AI API.",
			},
		},
	}
}

func (p *aiProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg aiProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := cfg.Endpoint.ValueString()
	if endpoint == "" {
		endpoint = p.defaultEndpoint
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		resp.Diagnostics.AddError("Invalid endpoint", err.Error())
		return
	}

	// api_token falls back to the AIPROVIDER_API_TOKEN environment variable
	// when not set in the provider configuration.
	apiToken := cfg.ApiToken.ValueString()
	if apiToken == "" {
		apiToken = os.Getenv("AIPROVIDER_API_TOKEN")
	}

	client := repository.NewRestClient(endpoint, apiToken)
	service := usecase.NewClusterInteractor(client)

	resp.ResourceData = service
	resp.DataSourceData = service
}

func (p *aiProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewClusterResource,
	}
}

func (p *aiProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterDataSource,
	}
}
