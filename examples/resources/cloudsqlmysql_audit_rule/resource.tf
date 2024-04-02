resource "cloudsqlmysql_audit_rule" "default" {
  user       = "*"
  database   = "*"
  object     = "*"
  operation  = "*"
  ops_result = "B"
}