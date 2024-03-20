resource "cloudsqlmysql_audit_rule" "default" {
  username   = "*"
  database   = "*"
  object     = "*"
  operation  = "*"
  ops_result = "B"
}