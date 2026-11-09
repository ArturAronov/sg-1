# Security groups do not filter AmazonProvidedDNS. Restrict resolver egress too,
# otherwise a generated application could exfiltrate data using DNS queries.
# This module must run in a dedicated SG-1 VPC, not a shared application VPC.
resource "aws_route53_resolver_firewall_domain_list" "allowed" {
  name = "${var.name}-allowed"
  domains = [
    "*.amazonaws.com", "*.amazoncognito.com",
    "amazonlinux.com", "*.amazonlinux.com", "github.com", "*.githubusercontent.com",
    "api.openai.com", "api.anthropic.com", "api.z.ai", "api.moonshot.ai",
    aws_service_discovery_private_dns_namespace.apps.name,
    "*.${aws_service_discovery_private_dns_namespace.apps.name}"
  ]
}
resource "aws_route53_resolver_firewall_domain_list" "blocked" {
  name    = "${var.name}-default-deny"
  domains = ["*"]
}
resource "aws_route53_resolver_firewall_rule_group" "egress" {
  name = "${var.name}-egress"
}
resource "aws_route53_resolver_firewall_rule" "allow" {
  name                               = "approved-destinations"
  action                             = "ALLOW"
  firewall_domain_list_id            = aws_route53_resolver_firewall_domain_list.allowed.id
  firewall_rule_group_id             = aws_route53_resolver_firewall_rule_group.egress.id
  firewall_domain_redirection_action = "TRUST_REDIRECTION_DOMAIN"
  priority                           = 100
}
resource "aws_route53_resolver_firewall_rule" "block" {
  name                    = "default-deny"
  action                  = "BLOCK"
  block_response          = "NODATA"
  firewall_domain_list_id = aws_route53_resolver_firewall_domain_list.blocked.id
  firewall_rule_group_id  = aws_route53_resolver_firewall_rule_group.egress.id
  priority                = 200
}
resource "aws_route53_resolver_firewall_rule_group_association" "egress" {
  name                   = "${var.name}-egress"
  firewall_rule_group_id = aws_route53_resolver_firewall_rule_group.egress.id
  vpc_id                 = var.vpc_id
  priority               = 100
  mutation_protection    = "ENABLED"
}
resource "aws_route53_resolver_firewall_config" "closed" {
  resource_id        = var.vpc_id
  firewall_fail_open = "DISABLED"
}
