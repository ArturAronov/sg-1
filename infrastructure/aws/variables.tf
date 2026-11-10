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
variable "apps_hosted_zone_id" {
  type        = string
  default     = ""
  description = "Optional separate app zone; defaults to the platform zone."
}
variable "domain" {
  type    = string
  default = "sg1.chickensoup.dev"
}
variable "apps_domain" {
  type    = string
  default = "apps.chickensoup.dev"
  validation {
    condition     = var.domain != var.apps_domain && !endswith(var.domain, ".${var.apps_domain}") && !endswith(var.apps_domain, ".${var.domain}")
    error_message = "The app wildcard namespace and platform hostname must not overlap."
  }
}
variable "entra_secret_arn" { type = string }
variable "enable_onelogin" {
  type    = bool
  default = false
}
variable "onelogin_secret_arn" {
  type    = string
  default = ""
  validation {
    condition     = !var.enable_onelogin || length(var.onelogin_secret_arn) > 0
    error_message = "Set onelogin_secret_arn when enable_onelogin is true."
  }
}
variable "instance_type" {
  type    = string
  default = "t3.medium"
}
variable "db_instance_class" {
  type    = string
  default = "db.t4g.medium"
}
