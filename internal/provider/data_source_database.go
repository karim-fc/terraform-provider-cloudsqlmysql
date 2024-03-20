package provider

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &databaseDataSource{}
	_ datasource.DataSourceWithConfigure = &databaseDataSource{}
)

func NewDatabaseDataSource() datasource.DataSource {
	return &databaseDataSource{}
}

type databaseDataSourceModel struct {
	Name                types.String `tfsdk:"name"`
	DefaultCharacterSet types.String `tfsdk:"default_character_set"`
	DefaultCollation    types.String `tfsdk:"default_collation"`
}

type databaseDataSource struct {
	db *sql.DB
}

func (d *databaseDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	log.Println(req.ProviderTypeName)
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (d *databaseDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
			},
			"default_character_set": schema.StringAttribute{
				Computed: true,
			},
			"default_collation": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *databaseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state databaseDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)

	database := state.Name.ValueString()
	row := d.db.QueryRowContext(ctx, "SELECT SCHEMA_NAME, DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME "+
		"FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = ?", database)

	var (
		name                string
		defaultCharacterSet string
		defaultCollation    string
	)
	if err := row.Scan(&name, &defaultCharacterSet, &defaultCollation); err != nil {
		if err == sql.ErrNoRows {
			resp.Diagnostics.AddError(
				"Database not found",
				"Database '"+database+"' not found")
			tflog.Debug(ctx, "Database '"+database+"' not found, error: "+err.Error())
			return
		}
		resp.Diagnostics.AddError(
			"Error reading the the database information",
			"Could not read the database information of '"+database+"', unexpected error: "+err.Error())
		return
	}

	state.Name = types.StringValue(name)
	state.DefaultCharacterSet = types.StringValue(defaultCharacterSet)
	state.DefaultCollation = types.StringValue(defaultCollation)

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (d *databaseDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.db = db
}
