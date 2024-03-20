resource "cloudsqlmysql_database_grant" "default" {
  database          = "database"
  user              = "user"
  privileges        = ["SELECT", "UPDATE", "DELETE"]
  with_grant_option = true
}