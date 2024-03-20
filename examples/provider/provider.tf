terraform {
  required_providers {
    cloudsqlmysql = {
      source = "devoteamgcloud/cloudsqlmysql"
    }
  }
}

provider "cloudsqlmysql" {
  connection_name = "project:region:instance"
  username        = "root"
  password        = "password"
}
