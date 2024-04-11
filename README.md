# Terraform Provider Google Cloud SQL Mysql

Terraform provider for Google Cloud SQL Mysql. This Terraform provider supports public IPs, private IPs, Private Service Connect and the use of socks5 proxy.

_This Cloud SQL Postgresql repository is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)._

## Implementation

- The provider implements the audit rule feature on Cloud SQL: https://cloud.google.com/sql/docs/mysql/use-db-audit