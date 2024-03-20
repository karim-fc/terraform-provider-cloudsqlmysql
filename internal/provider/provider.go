package provider

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"

	"cloud.google.com/go/cloudsqlconn"
	"cloud.google.com/go/cloudsqlconn/mysql/mysql"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/net/proxy"
)

var (
	_ provider.Provider = &CloudSqlMysqlProvider{}
)

type CloudSqlMysqlProvider struct {
	version string
}

type CloudSqlMysqlProviderModel struct {
	ConnectionName types.String `tfsdk:"connection_name"`
	Username       types.String `tfsdk:"username"`
	Password       types.String `tfsdk:"password"`
	Proxy          types.String `tfsdk:"proxy"`
	PrivateIP      types.Bool   `tfsdk:"private_ip"`
	PSC            types.Bool   `tfsdk:"psc"`
	// IAMAuthentication types.Bool   `tfsdk:"iam_authentication"` # Not supporting IAM authentication for now.
}

func (p *CloudSqlMysqlProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cloudsqlmysql"
	resp.Version = p.version
}

func (p *CloudSqlMysqlProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "The cloudsqlmysql provider makes it possible to grant permissions on MySQL databases and add rules for MySQL Audit Plugin. More info: https://cloud.google.com/sql/docs/mysql/db-audit",
		MarkdownDescription: "The `cloudsqlmysql` provider makes it possible to grant permissions on MySQL databases and add rules for MySQL Audit Plugin. More info in the [Google documentation](https://cloud.google.com/sql/docs/mysql/db-audit).",
		Attributes: map[string]schema.Attribute{
			"connection_name": schema.StringAttribute{
				Description:         "The connection name of the Google Cloud SQL MySQL instance",
				MarkdownDescription: "The connection name of the Google Cloud SQL MySQL instance",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9\-]+\:[a-z0-9\-]+\:[a-z0-9\-]+$`),
						"`connection_name` must have the format of `<project>:<region>:<instance>`"),
				},
			},
			"username": schema.StringAttribute{
				Description:         "The username to use to authenticate with the Cloud SQL MySQL instance",
				MarkdownDescription: "The username to use to authenticate with the Cloud SQL MySQL instance",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				Description:         "The password to use to authenticate using the built-in database authentication",
				MarkdownDescription: "The password to use to authenticate using the built-in database authentication",
				Optional:            true,
				Sensitive:           true,
			},
			"proxy": schema.StringAttribute{
				Description:         "Proxy socks url if used. Format needs to be `socks5://<ip>:<port>`",
				MarkdownDescription: "Proxy socks url if used. Format needs to be `socks5://<ip>:<port>`",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^socks5:\/\/.*:\d+$`),
						"`proxy` must have the format of `socks5://<ip>:<port>`"),
				},
			},
			// "iam_authentication": schema.BoolAttribute{
			// 	MarkdownDescription: "Enables the use of IAM authentication. The `password` field needs to be used to fill in the access token",
			// 	Optional:            true,
			// },
			"private_ip": schema.BoolAttribute{
				Description:         "Use the private IP address of the Cloud SQL MySQL instance to connect to",
				MarkdownDescription: "Use the private IP address of the Cloud SQL MySQL instance to connect to",
				Optional:            true,
			},
			"psc": schema.BoolAttribute{
				Description:         "Use the Private Service Connect endpoint of the Cloud SQL MySQL instance to connect to",
				MarkdownDescription: "Use the Private Service Connect endpoint of the Cloud SQL MySQL instance to connect to",
				Optional:            true,
			},
		},
	}
}

func (p *CloudSqlMysqlProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config CloudSqlMysqlProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.ConnectionName.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("connection_name"),
			"Unknown Cloud SQL MySQL connection name",
			"The provider cannot create the Cloud SQL Mysql client as there is an unknown configuration value for the `connection_name`")
	}

	// username and password are required for now as long IAM authentication is not supported.
	if config.Username.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("username"),
			"Unknown Cloud SQL MySQL username",
			"The provider cannot create the Cloud SQL Mysql client as there is an unknown configuration value for the `username`")
	}

	if config.Password.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("password"),
			"Unknown Cloud SQL MySQL username",
			"The provider cannot create the Cloud SQL Mysql client as there is an unknown configuration value for the `password`")
	}

	if resp.Diagnostics.HasError() {
		return
	}

	connectionName := os.Getenv("CLOUDSQL_MYSQL_CONNECTION_NAME")
	username := os.Getenv("CLOUDSQL_MYSQL_USERNAME")
	password := os.Getenv("CLOUDSQL_MYSQL_PASSWORD")

	if !config.ConnectionName.IsNull() {
		connectionName = config.ConnectionName.ValueString()
	}

	if !config.Username.IsNull() {
		username = config.Username.ValueString()
	}

	if !config.Password.IsNull() {
		password = config.Password.ValueString()
	}

	if connectionName == "" {
		resp.Diagnostics.AddAttributeError(path.Root("connection_name"),
			"Missing Cloud SQL MySQL connection name",
			"The provider cannot create the Cloud SQL MySQL connection as there is a missing or empty value for the Cloud SQL MySQL connection name. "+
				"Set the connection name value in the configuration or use the CLOUDSQL_MYSQL_CONNECTION_NAME environment variable. ")
	}

	if username == "" {
		resp.Diagnostics.AddAttributeError(path.Root("username"),
			"Missing Cloud SQL MySQL username",
			"The provider cannot create the Cloud SQL MySQL connection as there is a missing or empty value for the Cloud SQL MySQL username. "+
				"Set the username value in the configuration or use the CLOUDSQL_MYSQL_USERNAME environment variable.")
	}

	if password == "" {
		resp.Diagnostics.AddAttributeError(path.Root("password"),
			"Missing Cloud SQL MySQL password",
			"The provider cannot create the Cloud SQL MySQL connection as there is a missing or empty value for the Cloud SQL MySQL password. "+
				"Set the password value in the configuration or use the CLOUDSQL_MYSQL_PASSWORD environment variable.")
	}

	if resp.Diagnostics.HasError() {
		return
	}

	var dialOptions []cloudsqlconn.DialOption
	// dialOptions = append(dialOptions, cloudsqlconn.WithDialIAMAuthN(username == "")) // enable IAM authentication when username is not set

	if config.PrivateIP.ValueBool() {
		dialOptions = append(dialOptions, cloudsqlconn.WithPrivateIP())
	}

	if config.PSC.ValueBool() {
		dialOptions = append(dialOptions, cloudsqlconn.WithPSC())
	}

	var options []cloudsqlconn.Option

	options = append(options, cloudsqlconn.WithDefaultDialOptions(dialOptions...))

	if !config.Proxy.IsNull() {
		tflog.Debug(ctx, "`proxy` is not null")
		options = append(options, cloudsqlconn.WithDialFunc(createDialer(config.Proxy.ValueString(), ctx)))
	}

	_, err := mysql.RegisterDriver("cloudsql-mysql", options...)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cloud SQL MySQL connection",
			"An unexpected error occured when creating the Cloud SQL connection.\n\n"+
				"Error: "+err.Error(),
		)
	}

	dataSourceNameTemplate := fmt.Sprintf("%s:%s@cloudsql-mysql(%s)/%%s?parseTime=true", username, password, connectionName)

	dbConfig := newConfig(dataSourceNameTemplate)

	resp.ResourceData = dbConfig
	resp.DataSourceData = dbConfig
}

func (p *CloudSqlMysqlProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewRoleResource,
		newDatabaseGrantResource,
		newAuditRuleResource,
	}
}

func (p *CloudSqlMysqlProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDatabaseDataSource,
	}
}

func (p *CloudSqlMysqlProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CloudSqlMysqlProvider{
			version: version,
		}
	}
}

func createDialer(proxyInput string, ctxProvider context.Context) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		tflog.Info(ctxProvider, "Creating Dialer with proxy: "+proxyInput)
		if len(proxyInput) == 0 {
			return nil, fmt.Errorf("proxy is empty")
		}

		proxyURL, err := url.Parse(proxyInput)
		if err != nil {
			return nil, err
		}
		d, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, err
		}

		if xd, ok := d.(proxy.ContextDialer); ok {
			return xd.DialContext(ctx, network, address)
		}

		tflog.Warn(ctxProvider, "net.Conn created without context.Context")
		return d.Dial(network, address) // TODO: force use of context?
	}
}
