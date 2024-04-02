package provider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_                resource.Resource              = &auditRuleResource{}
	_                resource.ResourceWithConfigure = &auditRuleResource{}
	auditRuleDbMutex sync.Mutex                     // Need this because the results of the stored procedures we need to get from a new select query (needs to be global too)
)

type auditRuleResource struct {
	db *sql.DB
}

type auditRuleResourceModel struct {
	Id        types.Int64  `tfsdk:"id"`
	User      types.String `tfsdk:"user"`
	Database  types.String `tfsdk:"database"`
	Object    types.String `tfsdk:"object"`
	Operation types.String `tfsdk:"operation"`
	OpsResult types.String `tfsdk:"ops_result"`
}

func newAuditRuleResource() resource.Resource {
	return &auditRuleResource{}
}

func (r *auditRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_audit_rule"
}

func (r *auditRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed: true,
			},
			"user": schema.StringAttribute{
				Required: true,
			},
			"database": schema.StringAttribute{
				Required: true,
			},
			"object": schema.StringAttribute{
				Required: true,
			},
			"operation": schema.StringAttribute{
				Required: true,
			},
			"ops_result": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func (r *auditRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	auditRuleDbMutex.Lock()
	defer auditRuleDbMutex.Unlock()

	var plan auditRuleResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.db.ExecContext(ctx, "CALL mysql.cloudsql_create_audit_rule(?,?,?,?,?,1, @outval,@outmsg);",
		plan.User.ValueString(),
		plan.Database.ValueString(),
		plan.Object.ValueString(),
		plan.Operation.ValueString(),
		plan.OpsResult.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create the audit rule",
			"An unexpected error occured while creating the audit rule: "+err.Error(),
		)
		return
	}

	err = r.auditRuleStoredProcedureResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create the audit rule",
			"An unexpected error occured while creating the audit rule: "+err.Error(),
		)
		return
	}

	rows, err := r.db.QueryContext(ctx, "CALL mysql.cloudsql_list_audit_rule('*',@outval,@outmsg);")
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create the audit rule",
			"An unexpected error occured while creating the audit rule: "+err.Error(),
		)
		return
	}
	defer rows.Close()

	err = r.auditRuleStoredProcedureResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create the audit rule",
			"An unexpected error occured while creating the audit rule: "+err.Error(),
		)
		return
	}

	id := int64(-1)
	for rows.Next() {
		var row auditRuleRow
		err = rows.Scan(&row.Id, &row.User, &row.Dbname, &row.Object, &row.Operation, &row.OpResult)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to create the audit rule",
				"An unexpected error occured while creating the audit rule: "+err.Error(),
			)
			return
		}

		if row.equalsModel(&plan) {
			id = row.Id
			break
		}
	}

	if id == -1 {
		resp.Diagnostics.AddError(
			"Unable to create the audit rule",
			"An unexpected error occured while creating the audit rule: the audit rule is not found after creation",
		)
		return
	}

	plan.Id = types.Int64Value(id)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *auditRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	auditRuleDbMutex.Lock()
	defer auditRuleDbMutex.Unlock()

	var state auditRuleResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.Id.ValueInt64()

	var row auditRuleRow

	err := r.db.QueryRowContext(ctx, "CALL mysql.cloudsql_list_audit_rule(?,@outval,@outmsg);", id).Scan(&row.Id, &row.User, &row.Dbname, &row.Object, &row.Operation, &row.OpResult)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read audit rule",
			fmt.Sprintf("An unexpected error occured while fetching the audit rule with id %d, error: %s", id, err.Error()),
		)
		return
	}

	err = r.auditRuleStoredProcedureResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update the audit rule",
			fmt.Sprintf("An unexpected error occured while fetching the audit rule with id %d, error: %s", id, err.Error()),
		)
		return
	}

	state.Id = types.Int64Value(row.Id)
	state.User = types.StringValue(row.User)
	state.Database = types.StringValue(row.Dbname)
	state.Object = types.StringValue(row.Object)
	state.Operation = types.StringValue(row.Operation)
	state.OpsResult = types.StringValue(row.OpResult)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *auditRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	auditRuleDbMutex.Lock()
	defer auditRuleDbMutex.Unlock()

	var plan auditRuleResourceModel
	diags := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.db.ExecContext(ctx, "CALL mysql.cloudsql_update_audit_rule(?,?,?,?,?,?,1, @outval,@outmsg);",
		plan.Id.ValueInt64(),
		plan.User.ValueString(),
		plan.Database.ValueString(),
		plan.Object.ValueString(),
		plan.Operation.ValueString(),
		plan.OpsResult.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update the audit rule",
			"An unexpected error occured while updating the audit rule: "+err.Error(),
		)
		return
	}

	err = r.auditRuleStoredProcedureResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update the audit rule",
			"An unexpected error occured while updating the audit rule: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *auditRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	auditRuleDbMutex.Lock()
	defer auditRuleDbMutex.Unlock()

	var state auditRuleResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.Id.ValueInt64()

	_, err := r.db.ExecContext(ctx, "CALL mysql.cloudsql_delete_audit_rule(?,1,@outval,@outmsg);", id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete the audit rule",
			fmt.Sprintf("An unexpected error occured while deleting the audit rule with id %d, error: %s", id, err.Error()),
		)
		return
	}
	err = r.auditRuleStoredProcedureResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete the audit rule",
			fmt.Sprintf("An unexpected error occured while deleting the audit rule with id %d, error: %s", id, err.Error()),
		)
		return
	}
}

func (r *auditRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	db, err := config.connectToMySQLDb("") // Not connecting to a specific database
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to connect to the Cloud SQL MySQL instance",
			err.Error(),
		)
		return
	}

	r.db = db
}

func (r *auditRuleResource) auditRuleStoredProcedureResponse(ctx context.Context) error {
	var outval sql.NullInt16
	var outmsg sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT @outval, @outmsg;").Scan(&outval, &outmsg)
	if err != nil {
		return err
	}

	if outval.Int16 > 0 { // outval == 1 means the stored procedure failed
		return errors.New(outmsg.String)
	}

	return nil
}

type auditRuleRow struct {
	Id        int64
	User      string
	Dbname    string
	Object    string
	Operation string
	OpResult  string
}

func (row *auditRuleRow) equalsModel(model *auditRuleResourceModel) bool {
	if !strings.EqualFold(row.User, model.User.ValueString()) {
		return false
	}
	if !strings.EqualFold(row.Dbname, model.Database.ValueString()) {
		return false
	}
	if !strings.EqualFold(row.Object, model.Object.ValueString()) {
		return false
	}
	if !strings.EqualFold(row.Operation, model.Operation.ValueString()) {
		return false
	}
	if !strings.EqualFold(row.OpResult, model.OpsResult.ValueString()) {
		return false
	}
	return true
}
