# Identity-provider client secrets live in Secrets Manager, never tfvars.
# Terraform still stores provider configuration in state: encrypt and restrict it.
data "aws_secretsmanager_secret_version" "entra" { secret_id = var.entra_secret_arn }
data "aws_secretsmanager_secret_version" "onelogin" { secret_id = var.onelogin_secret_arn }
locals {
  idps = {
    Entra    = jsondecode(data.aws_secretsmanager_secret_version.entra.secret_string)
    OneLogin = jsondecode(data.aws_secretsmanager_secret_version.onelogin.secret_string)
  }
}
resource "aws_cognito_user_pool" "employees" {
  name                = "${var.name}-employees"
  deletion_protection = "ACTIVE"
  user_pool_tier      = "PLUS"
  admin_create_user_config { allow_admin_create_user_only = true }
  user_pool_add_ons { advanced_security_mode = "ENFORCED" }
  lifecycle { prevent_destroy = true }
}
resource "aws_cognito_identity_provider" "company" {
  for_each      = toset(["Entra", "OneLogin"])
  user_pool_id  = aws_cognito_user_pool.employees.id
  provider_name = each.value
  provider_type = "OIDC"
  provider_details = {
    oidc_issuer               = local.idps[each.value].issuer
    client_id                 = local.idps[each.value].client_id
    client_secret             = local.idps[each.value].client_secret
    authorize_scopes          = "openid email profile"
    attributes_request_method = "GET"
  }
  attribute_mapping = { email = "email", name = "name" }
}
resource "aws_cognito_user_pool_client" "alb" {
  name                                 = "${var.name}-alb"
  user_pool_id                         = aws_cognito_user_pool.employees.id
  generate_secret                      = true
  supported_identity_providers         = [for provider in aws_cognito_identity_provider.company : provider.provider_name]
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes                 = ["openid", "email", "profile"]
  callback_urls                        = ["https://${var.domain}/oauth2/idpresponse"]
  logout_urls                          = ["https://${var.domain}/signed-out"]
  prevent_user_existence_errors        = "ENABLED"
}
resource "aws_cognito_user_pool_domain" "employees" {
  domain       = "${var.name}-${local.account}"
  user_pool_id = aws_cognito_user_pool.employees.id
}
resource "aws_acm_certificate" "ingress" {
  domain_name               = var.domain
  subject_alternative_names = ["*.${var.apps_domain}"]
  validation_method         = "DNS"
  lifecycle { create_before_destroy = true }
}
resource "aws_route53_record" "validation" {
  for_each = { for record in aws_acm_certificate.ingress.domain_validation_options : record.domain_name => record }
  zone_id  = each.key == var.domain ? var.hosted_zone_id : var.apps_hosted_zone_id
  name     = each.value.resource_record_name
  type     = each.value.resource_record_type
  records  = [each.value.resource_record_value]
  ttl      = 60
}
resource "aws_acm_certificate_validation" "ingress" {
  certificate_arn         = aws_acm_certificate.ingress.arn
  validation_record_fqdns = [for record in aws_route53_record.validation : record.fqdn]
}
