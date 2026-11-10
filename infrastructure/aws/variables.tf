variable "region" {
  type    = string
  default = "ap-northeast-3"
}
variable "name" {
  type    = string
  default = "sg1-prod"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,16}$", var.name))
    error_message = "Name must be 3–17 lowercase letters, numbers, or hyphens."
  }
}
variable "vpc_id" { type = string }
variable "public_subnet_ids" {
  type = list(string)
  validation {
    condition     = length(var.public_subnet_ids) >= 2
    error_message = "ALB requires public subnets in at least two AZs."
  }
}
variable "private_subnet_ids" {
  type = list(string)
  validation {
    condition     = length(var.private_subnet_ids) >= 2
    error_message = "RDS/Fargate require private subnets in at least two AZs."
  }
}
variable "private_route_table_ids" {
  type = list(string)
  validation {
    condition     = length(var.private_route_table_ids) > 0
    error_message = "Provide private route tables for the restricted S3 gateway endpoint."
  }
}
variable "hosted_zone_id" { type = string }
variable "apps_hosted_zone_id" { type = string }
variable "domain" { type = string }
variable "apps_domain" { type = string }
variable "entra_secret_arn" { type = string }
variable "onelogin_secret_arn" { type = string }
variable "instance_type" {
  type    = string
  default = "t3.medium"
}
variable "db_instance_class" {
  type    = string
  default = "db.t4g.medium"
}
