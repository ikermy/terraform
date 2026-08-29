package delivery

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-ai/internal/entity"
)

type clusterResource struct {
	service ClusterService
}

type clusterModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Replicas types.Int64  `tfsdk:"replicas"`
	Model    types.String `tfsdk:"model"`
}

func NewClusterResource() resource.Resource {
	return &clusterResource{}
}

func (r *clusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	services, ok := req.ProviderData.(*ProviderServices)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			"Expected *ProviderServices",
		)
		return
	}
	r.service = services.Clusters
}

func (r *clusterResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "aiprovider_cluster"
}

func (r *clusterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Server-generated unique identifier of the cluster.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the cluster. Must be unique.",
			},
			"replicas": schema.Int64Attribute{
				Optional:    true,
				Computed:    true, // required when using a Default
				Default:     int64default.StaticInt64(1),
				Description: "Number of replicas for the cluster. Defaults to 1. Must be >= 0.",
			},
			"model": schema.StringAttribute{
				Optional:    true,
				Description: "AI model used by the cluster (case-insensitive).",
				PlanModifiers: []planmodifier.String{
					caseInsensitiveString(),
				},
			},
		},
	}
}

func (r *clusterResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg clusterModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if cfg.Name.ValueString() == "" {
		resp.Diagnostics.AddError("Invalid name", "name must not be empty")
	}
	if !cfg.Replicas.IsNull() && cfg.Replicas.ValueInt64() < 0 {
		resp.Diagnostics.AddError("Invalid replicas", "replicas must be >= 0")
	}
}

func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "creating cluster", map[string]any{"name": plan.Name.ValueString()})
	created, err := r.service.CreateCluster(ctx, clusterEntityFromModel(plan))
	if err != nil {
		resp.Diagnostics.AddError("Create failed", err.Error())
		return
	}

	state := clusterModelFromEntity(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "reading cluster", map[string]any{"id": state.ID.ValueString()})
	got, err := r.service.GetCluster(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, entity.ErrClusterNotFound) {
			tflog.Debug(ctx, "cluster not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read failed", err.Error())
		return
	}

	state = clusterModelFromEntity(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "updating cluster", map[string]any{"id": plan.ID.ValueString()})
	updated, err := r.service.UpdateCluster(ctx, clusterEntityFromModel(plan))
	if err != nil {
		resp.Diagnostics.AddError("Update failed", err.Error())
		return
	}

	state := clusterModelFromEntity(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "deleting cluster", map[string]any{"id": state.ID.ValueString()})
	if err := r.service.DeleteCluster(ctx, state.ID.ValueString()); err != nil {
		// A resource already removed out-of-band should not fail the delete.
		if errors.Is(err, entity.ErrClusterNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Delete failed", err.Error())
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// clusterEntityFromModel / clusterModelFromEntity map between the Terraform
// model and the domain entity, avoiding duplication across CRUD methods.
func clusterEntityFromModel(m clusterModel) entity.Cluster {
	c := entity.Cluster{
		Name:     m.Name.ValueString(),
		Replicas: int(m.Replicas.ValueInt64()),
		Model:    m.Model.ValueString(),
	}
	// ID is Computed, so it may be Unknown ("" via ValueString) before it is
	// assigned by the server. Only carry it when it is actually known.
	if !m.ID.IsNull() && !m.ID.IsUnknown() {
		c.ID = m.ID.ValueString()
	}
	return c
}

func clusterModelFromEntity(e *entity.Cluster) clusterModel {
	return clusterModel{
		ID:       types.StringValue(e.ID),
		Name:     types.StringValue(e.Name),
		Replicas: types.Int64Value(int64(e.Replicas)),
		Model:    types.StringValue(e.Model),
	}
}

// caseInsensitiveString is a plan modifier that suppresses diffs for model.
type ciStringModifier struct{}

func (m ciStringModifier) Description(_ context.Context) string {
	return "Suppresses diff when the value differs only by case."
}

func (m ciStringModifier) MarkdownDescription(_ context.Context) string {
	return "Suppresses diff when the value differs only by case."
}

func (m ciStringModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() ||
		req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if strings.EqualFold(req.StateValue.ValueString(), req.ConfigValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}

func caseInsensitiveString() planmodifier.String {
	return ciStringModifier{}
}
