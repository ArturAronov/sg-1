# Offline provider mocks: no credentials, secret retrieval or AWS resources.
mock_provider "aws" {
  mock_resource "aws_acm_certificate_validation" {
    defaults = { certificate_arn = "arn:aws:acm:ap-northeast-3:123456789012:certificate/00000000-0000-4000-8000-000000000001" }
  }
  mock_resource "aws_lb" {
    defaults = { arn = "arn:aws:elasticloadbalancing:ap-northeast-3:123456789012:loadbalancer/app/fixture/0000000000000001" }
  }
  mock_resource "aws_lb_listener" {
    defaults = { arn = "arn:aws:elasticloadbalancing:ap-northeast-3:123456789012:listener/app/fixture/0000000000000001/0000000000000002" }
  }
  mock_resource "aws_lb_target_group" {
    defaults = { arn = "arn:aws:elasticloadbalancing:ap-northeast-3:123456789012:targetgroup/fixture/0000000000000001" }
  }
  mock_resource "aws_cognito_user_pool" {
    defaults = { arn = "arn:aws:cognito-idp:ap-northeast-3:123456789012:userpool/ap-northeast-3_fixture" }
  }
  mock_data "aws_caller_identity" {
    defaults = { account_id = "123456789012" }
  }
  mock_data "aws_vpc" {
    defaults = { enable_dns_support = true, enable_dns_hostnames = true }
  }
  mock_data "aws_subnet" {
    defaults = { vpc_id = "vpc-00000000000000001", availability_zone = "ap-northeast-3a" }
  }
  mock_data "aws_ssm_parameter" {
    defaults = { value = "ami-00000000000000001" }
  }
  mock_data "aws_secretsmanager_secret_version" {
    defaults = { secret_string = "{\"issuer\":\"https://identity.example.test\",\"client_id\":\"fixture-client\",\"client_secret\":\"fixture-not-a-real-secret\"}" }
  }
}

variables {
  vpc_id                  = "vpc-00000000000000001"
  public_subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_subnet_ids      = ["subnet-00000000000000003", "subnet-00000000000000004"]
  private_route_table_ids = ["rtb-00000000000000001"]
  hosted_zone_id          = "ZFIXTURE"
  entra_secret_arn        = "arn:aws:secretsmanager:ap-northeast-3:123456789012:secret:fixture/entra-abcdef"
}

override_data {
  target = data.aws_subnet.private["subnet-00000000000000004"]
  values = { vpc_id = "vpc-00000000000000001", availability_zone = "ap-northeast-3b" }
}

override_resource {
  target          = aws_acm_certificate.ingress
  override_during = plan
  values = {
    arn = "arn:aws:acm:ap-northeast-3:123456789012:certificate/00000000-0000-4000-8000-000000000001"
    domain_validation_options = [
      { domain_name = "sg1.chickensoup.dev", resource_record_name = "_platform.sg1.chickensoup.dev", resource_record_type = "CNAME", resource_record_value = "platform.fixture.acm-validations.aws" },
      { domain_name = "*.apps.chickensoup.dev", resource_record_name = "_app.apps.chickensoup.dev", resource_record_type = "CNAME", resource_record_value = "app.fixture.acm-validations.aws" }
    ]
  }
}

run "entra_only_shared_domain" {
  command = plan
  assert {
    condition     = length(data.aws_secretsmanager_secret_version.onelogin) == 0 && length(aws_cognito_identity_provider.company) == 1
    error_message = "Default deployment must not retrieve or configure OneLogin."
  }
  assert {
    condition     = output.deployment.SG1_APP_ORIGIN == "https://{slug}.apps.chickensoup.dev" && output.deployment.SG1_IDENTITY_PROVIDERS == "Entra"
    error_message = "Shared app origins and Entra-only output must match runtime configuration."
  }
  assert {
    condition     = aws_route53_record.app["*.apps.chickensoup.dev"].zone_id == "ZFIXTURE"
    error_message = "App DNS must use the platform zone by default."
  }
  assert {
    condition     = one([for action in aws_lb_listener_rule.employees["pages"].action : one(action.authenticate_cognito).session_cookie_name if action.type == "authenticate-cognito"]) == "__Host-SG1AuthSession"
    error_message = "The ALB authentication cookie must retain its host prefix."
  }
  assert {
    condition     = aws_vpc_security_group_egress_rule.alb_auth_https.from_port == 443 && aws_vpc_security_group_egress_rule.alb_auth_https.to_port == 443
    error_message = "ALB authentication requires outbound HTTPS."
  }
}

run "onelogin_opt_in" {
  command = plan
  variables {
    enable_onelogin     = true
    onelogin_secret_arn = "arn:aws:secretsmanager:ap-northeast-3:123456789012:secret:fixture/onelogin-abcdef"
  }
  assert {
    condition     = length(data.aws_secretsmanager_secret_version.onelogin) == 1 && length(aws_cognito_identity_provider.company) == 2 && output.deployment.SG1_IDENTITY_PROVIDERS == "Entra,OneLogin"
    error_message = "Explicit opt-in must configure both providers."
  }
}

run "onelogin_requires_secret" {
  command = plan
  variables { enable_onelogin = true }
  expect_failures = [var.onelogin_secret_arn]
}

run "reject_overlapping_app_namespace" {
  command = plan
  variables { apps_domain = "sg1.chickensoup.dev" }
  expect_failures = [var.apps_domain]
}
