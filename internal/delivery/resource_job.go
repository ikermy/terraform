package delivery

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-ai/internal/entity"
)

type jobResource struct {
	service JobService
}

type jobModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Command  types.String `tfsdk:"command"`
	Priority types.Int64  `tfsdk:"priority"`
	Status   types.String `tfsdk:"status"`
}

func NewJobResource() resource.Resource {
	return &jobResource{}
}

func (r *jobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.service = services.Jobs
}

func (r *jobResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "aiprovider_job"
}

func (r *jobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Server-generated unique identifier of the job.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the job. Must be unique.",
			},
			"command": schema.StringAttribute{
				Required:    true,
				Description: "Command executed by the job.",
			},
			"priority": schema.Int64Attribute{
				Optional:      true,
				Computed:      true,
				Description:   "Job priority. Must be >= 0. Defaults to 0.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				Computed:      true,
				Description:   "Server-computed job status (e.g. queued).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *jobResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg jobModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Name.ValueString() == "" {
		resp.Diagnostics.AddError("Invalid name", "name must not be empty")
	}
	if cfg.Command.ValueString() == "" {
		resp.Diagnostics.AddError("Invalid command", "command must not be empty")
	}
	if !cfg.Priority.IsNull() && cfg.Priority.ValueInt64() < 0 {
		resp.Diagnostics.AddError("Invalid priority", "priority must be >= 0")
	}
}

func (r *jobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan jobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "creating job", map[string]any{"name": plan.Name.ValueString()})
	created, err := r.service.CreateJob(ctx, jobEntityFromModel(plan))
	if err != nil {
		resp.Diagnostics.AddError("Create failed", err.Error())
		return
	}

	state := jobModelFromEntity(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *jobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state jobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "reading job", map[string]any{"id": state.ID.ValueString()})
	got, err := r.service.GetJob(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, entity.ErrJobNotFound) {
			tflog.Debug(ctx, "job not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read failed", err.Error())
		return
	}

	state = jobModelFromEntity(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *jobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan jobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "updating job", map[string]any{"id": plan.ID.ValueString()})
	updated, err := r.service.UpdateJob(ctx, jobEntityFromModel(plan))
	if err != nil {
		resp.Diagnostics.AddError("Update failed", err.Error())
		return
	}

	state := jobModelFromEntity(updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *jobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state jobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "deleting job", map[string]any{"id": state.ID.ValueString()})
	if err := r.service.DeleteJob(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete failed", err.Error())
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *jobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func jobEntityFromModel(m jobModel) entity.Job {
	return entity.Job{
		ID:       m.ID.ValueString(),
		Name:     m.Name.ValueString(),
		Command:  m.Command.ValueString(),
		Priority: int(m.Priority.ValueInt64()),
		Status:   m.Status.ValueString(),
	}
}

func jobModelFromEntity(e *entity.Job) jobModel {
	return jobModel{
		ID:       types.StringValue(e.ID),
		Name:     types.StringValue(e.Name),
		Command:  types.StringValue(e.Command),
		Priority: types.Int64Value(int64(e.Priority)),
		Status:   types.StringValue(e.Status),
	}
}
