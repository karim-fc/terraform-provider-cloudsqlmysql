package provider

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &roleResource{}
	_ resource.ResourceWithConfigure = &roleResource{}
)

type roleResource struct {
	db *sql.DB
}

func NewRoleResource() resource.Resource {
	return &roleResource{}
}

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	roleName := plan.Name.ValueString()

	_, err := r.db.ExecContext(ctx, fmt.Sprintf("CREATE ROLE '%s'", roleName)) // Fix this when CREATE ROLE is supported in prepared statements
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating role",
			"Could not create role '"+roleName+"', unexpected error: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	role := state.Name.ValueString()

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("SHOW GRANTS FOR '%s'", role))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading role",
			"Could not read role "+role+", unexpected error: "+err.Error(),
		)
		return
	}
	defer rows.Close()

	if !rows.Next() {
		resp.Diagnostics.AddError(
			"Role not found",
			"Could not read role "+role,
		)
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

}

func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No updates possible, needs to recreate
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleName := state.Name.ValueString()
	_, err := r.db.ExecContext(ctx, fmt.Sprintf("DROP ROLE '%s'", roleName))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting role",
			"Could not delete role "+roleName+", unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	db, err := config.connectToMySQLNoDb() // Not connecting to a specific database
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to connect to the Cloud SQL MySQL instance",
			err.Error(),
		)
		return
	}

	r.db = db
}

type roleResourceModel struct {
	Name types.String `tfsdk:"name"`
}
