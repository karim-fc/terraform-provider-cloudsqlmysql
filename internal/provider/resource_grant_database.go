package provider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                     = &databaseGrantResource{}
	_ resource.ResourceWithConfigure        = &databaseGrantResource{}
	_ resource.ResourceWithConfigValidators = &databaseGrantResource{}
)

type databaseGrantResource struct {
	db *sql.DB
}

func newDatabaseGrantResource() resource.Resource {
	return &databaseGrantResource{}
}

func (r *databaseGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_grant_database"
}

func (r *databaseGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"database": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_\-]*$`),
						"`database` must be a correct name of a database"),
				},
			},
			"user": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"host": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("%"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"with_grant_option": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"privileges": schema.SetAttribute{
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

func (r *databaseGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseGrantResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	userOrRole, err := plan.userOrRole() //TODO fix this
	if err != nil {
		resp.Diagnostics.AddError(
			"Error in input values",
			"No value for user nor role, unexpected error: "+err.Error(),
		)
		return
	}
	sqlStatement := fmt.Sprintf("GRANT %s ON %s.* TO %s@'%s'", strings.Join(plan.privilegesAsString(), ", "),
		plan.databaseAsString(), userOrRole, plan.hostAsString())
	if plan.withGrantOption() {
		sqlStatement = sqlStatement + " WITH GRANT OPTION"
	}
	tflog.Debug(ctx, fmt.Sprintf("SQL Statement: \"%s\"", sqlStatement))

	_, err = r.db.ExecContext(ctx, sqlStatement)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error granting database permissions",
			"Unable to grant permissions to "+userOrRole+", unexpected error: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

}

func (r *databaseGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseGrantResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	userOrRole, err := state.userOrRole()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error in input values",
			"No value for user nor role, unexpected error: "+err.Error(),
		)
		return
	}
	var row dbRow
	err = r.db.QueryRowContext(ctx, "SELECT "+
		"Host,Db,User,Select_priv,Insert_priv,Update_priv,Delete_priv,Create_priv,Drop_priv,Grant_priv,References_priv,"+
		"Index_priv,Alter_priv,Create_tmp_table_priv,Lock_tables_priv,Create_view_priv,Show_view_priv,Create_routine_priv,"+
		"Alter_routine_priv,Execute_priv,Event_priv,Trigger_priv"+
		" FROM mysql.db WHERE Host = ? AND User = ? AND Db = ?",
		state.hostAsString(),
		userOrRole,
		state.databaseAsString()).Scan(&row.Host,
		&row.Db, &row.User, &row.SelectPriv, &row.InsertPriv, &row.UpdatePriv, &row.DeletePriv,
		&row.CreatePriv, &row.DropPriv, &row.GrantPriv, &row.ReferencesPriv, &row.IndexPriv, &row.AlterPriv,
		&row.CreateTmpTablePriv, &row.LockTablesPriv, &row.CreateViewPriv, &row.ShowViewPriv, &row.CreateRoutinePriv,
		&row.AlterRoutinePriv, &row.ExecutePriv, &row.EventPriv, &row.TriggerPriv)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading database privileges data",
			"Unable to read data from the database privileges table, unexpected error: "+err.Error(),
		)
		return
	}
	var privileges []types.String
	for _, rowPermission := range row.allPrivilegesStringValues() {
		found := false
		for _, statePermission := range state.Privileges {
			if strings.EqualFold(statePermission.ValueString(), rowPermission.ValueString()) {
				privileges = append(privileges, statePermission)
				found = true
				break
			}
		}
		if !found {
			privileges = append(privileges, rowPermission)
		}
	}
	state.Privileges = privileges
	state.WithGrantOption = row.withGrantOption()
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *databaseGrantResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// No updates possible, needs to recreate
}

func (r *databaseGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseGrantResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	userOrRole, err := state.userOrRole()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error in input values",
			"No value for user nor role, unexpected error: "+err.Error(),
		)
		return
	}
	sqlStatement := fmt.Sprintf("REVOKE %s ON %s.* FROM %s@'%s'", strings.Join(state.privilegesAsString(), ", "), state.databaseAsString(), userOrRole, state.hostAsString())
	_, err = r.db.ExecContext(ctx, sqlStatement)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error removing grant database permissions",
			"Unable to remove grant permissions from "+userOrRole+", unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *databaseGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *databaseGrantResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("user"),
			path.MatchRoot("role"),
		),
		resourcevalidator.AtLeastOneOf(
			path.MatchRoot("user"),
			path.MatchRoot("role"),
		),
	}
}

type databaseGrantResourceModel struct {
	Database        types.String   `tfsdk:"database"`
	User            types.String   `tfsdk:"user"`
	Role            types.String   `tfsdk:"role"`
	Host            types.String   `tfsdk:"host"`
	Privileges      []types.String `tfsdk:"privileges"`
	WithGrantOption types.Bool     `tfsdk:"with_grant_option"`
}

func (m *databaseGrantResourceModel) privilegesAsString() []string {
	var privileges []string
	for _, priv := range m.Privileges {
		privileges = append(privileges, priv.ValueString())
	}
	return privileges
}

func (m *databaseGrantResourceModel) databaseAsString() string {
	return m.Database.ValueString()
}

func (m *databaseGrantResourceModel) hostAsString() string {
	return m.Host.ValueString()
}

func (m *databaseGrantResourceModel) userOrRole() (string, error) {
	if m.User.IsNull() && m.Role.IsNull() {
		return "", errors.New("user nor role are not filled in")
	}
	if !m.User.IsNull() {
		return m.User.ValueString(), nil
	}
	return m.Role.ValueString(), nil
}

func (m *databaseGrantResourceModel) withGrantOption() bool {
	return m.WithGrantOption.ValueBool()
}

type dbRow struct {
	Host               string
	Db                 string
	User               string
	SelectPriv         string
	InsertPriv         string
	UpdatePriv         string
	DeletePriv         string
	CreatePriv         string
	DropPriv           string
	GrantPriv          string
	ReferencesPriv     string
	IndexPriv          string
	AlterPriv          string
	CreateTmpTablePriv string
	LockTablesPriv     string
	CreateViewPriv     string
	ShowViewPriv       string
	CreateRoutinePriv  string
	AlterRoutinePriv   string
	ExecutePriv        string
	EventPriv          string
	TriggerPriv        string
}

func (dbRow *dbRow) allPrivilegesStringValues() []types.String {
	var privileges []types.String
	for _, priv := range dbRow.allPrivileges() {
		privileges = append(privileges, types.StringValue(priv))
	}
	return privileges
}

func (dbRow *dbRow) allPrivileges() []string {
	var privileges []string
	if dbRow.selectPrivBool() {
		privileges = append(privileges, "SELECT")
	}
	if dbRow.insertPrivBool() {
		privileges = append(privileges, "INSERT")
	}
	if dbRow.updatePrivBool() {
		privileges = append(privileges, "UPDATE")
	}
	if dbRow.deletePrivBool() {
		privileges = append(privileges, "DELETE")
	}
	if dbRow.createPrivBool() {
		privileges = append(privileges, "CREATE")
	}
	if dbRow.dropPrivBool() {
		privileges = append(privileges, "DROP")
	}
	if dbRow.referencesPrivBool() {
		privileges = append(privileges, "REFERENCES")
	}
	if dbRow.indexPrivBool() {
		privileges = append(privileges, "INDEX")
	}
	if dbRow.alterPrivBool() {
		privileges = append(privileges, "ALTER")
	}
	if dbRow.createTmpTablePrivBool() {
		privileges = append(privileges, "CREATE TEMPORARY TABLES")
	}
	if dbRow.lockTablesPrivBool() {
		privileges = append(privileges, "LOCK TABLES")
	}
	if dbRow.createViewPrivBool() {
		privileges = append(privileges, "CREATE VIEW")
	}
	if dbRow.showViewPrivBool() {
		privileges = append(privileges, "SHOW VIEW")
	}
	if dbRow.createRoutinePrivBool() {
		privileges = append(privileges, "CREATE ROUTINE")
	}
	if dbRow.alterRoutinePrivBool() {
		privileges = append(privileges, "ALTER ROUTINE")
	}
	if dbRow.executePrivBool() {
		privileges = append(privileges, "EXECUTE")
	}
	if dbRow.eventPrivBool() {
		privileges = append(privileges, "EVENT")
	}
	if dbRow.triggerPrivBool() {
		privileges = append(privileges, "TRIGGER")
	}
	return privileges
}

func (dbRow *dbRow) withGrantOption() types.Bool {
	return types.BoolValue(dbRow.grantPrivBool())
}

func (dbRow *dbRow) selectPrivBool() bool {
	return dbRow.SelectPriv == "Y"
}

func (dbRow *dbRow) insertPrivBool() bool {
	return dbRow.InsertPriv == "Y"
}

func (dbRow *dbRow) updatePrivBool() bool {
	return dbRow.UpdatePriv == "Y"
}

func (dbRow *dbRow) deletePrivBool() bool {
	return dbRow.DeletePriv == "Y"
}

func (dbRow *dbRow) createPrivBool() bool {
	return dbRow.CreatePriv == "Y"
}

func (dbRow *dbRow) dropPrivBool() bool {
	return dbRow.DropPriv == "Y"
}

func (dbRow *dbRow) grantPrivBool() bool {
	return dbRow.GrantPriv == "Y"
}

func (dbRow *dbRow) referencesPrivBool() bool {
	return dbRow.ReferencesPriv == "Y"
}

func (dbRow *dbRow) indexPrivBool() bool {
	return dbRow.IndexPriv == "Y"
}

func (dbRow *dbRow) alterPrivBool() bool {
	return dbRow.AlterPriv == "Y"
}

func (dbRow *dbRow) createTmpTablePrivBool() bool {
	return dbRow.CreateTmpTablePriv == "Y"
}

func (dbRow *dbRow) lockTablesPrivBool() bool {
	return dbRow.LockTablesPriv == "Y"
}

func (dbRow *dbRow) createViewPrivBool() bool {
	return dbRow.CreateViewPriv == "Y"
}

func (dbRow *dbRow) showViewPrivBool() bool {
	return dbRow.ShowViewPriv == "Y"
}

func (dbRow *dbRow) createRoutinePrivBool() bool {
	return dbRow.CreateRoutinePriv == "Y"
}

func (dbRow *dbRow) alterRoutinePrivBool() bool {
	return dbRow.AlterRoutinePriv == "Y"
}

func (dbRow *dbRow) executePrivBool() bool {
	return dbRow.ExecutePriv == "Y"
}

func (dbRow *dbRow) eventPrivBool() bool {
	return dbRow.EventPriv == "Y"
}

func (dbRow *dbRow) triggerPrivBool() bool {
	return dbRow.TriggerPriv == "Y"
}
