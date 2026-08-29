package delivery

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-ai/internal/entity"
)

type clusterDataSource struct {
	service ClusterService
}

func NewClusterDataSource() datasource.DataSource {
	return &clusterDataSource{}
}

func (d *clusterDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	service, ok := req.ProviderData.(ClusterService)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected DataSource Configure Type",
			"Expected ClusterService",
		)
		return
	}
	d.service = service
}

func (d *clusterDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "aiprovider_cluster"
}

func (d *clusterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "Server-generated unique identifier of the cluster.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the cluster.",
			},
			"replicas": schema.Int64Attribute{
				Computed:    true,
				Description: "Number of replicas for the cluster.",
			},
			"model": schema.StringAttribute{
				Computed:    true,
				Description: "AI model used by the cluster.",
			},
		},
	}
}

func (d *clusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.service.GetCluster(ctx, config.ID.ValueString())
	if err != nil {
		if errors.Is(err, entity.ErrClusterNotFound) {
			resp.Diagnostics.AddError("Cluster not found", "no cluster with id "+config.ID.ValueString())
			return
		}
		resp.Diagnostics.AddError("Read failed", err.Error())
		return
	}

	state := clusterModelFromEntity(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
