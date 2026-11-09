resource "aws_security_group" "alb" {
  name_prefix = "${var.name}-alb-"
  vpc_id      = var.vpc_id
}
resource "aws_security_group" "control" {
  name_prefix = "${var.name}-control-"
  vpc_id      = var.vpc_id
}
resource "aws_security_group" "apps" {
  name_prefix = "${var.name}-apps-"
  vpc_id      = var.vpc_id
}
resource "aws_security_group" "control_db" {
  name_prefix = "${var.name}-control-db-"
  vpc_id      = var.vpc_id
}
resource "aws_security_group" "apps_db" {
  name_prefix = "${var.name}-apps-db-"
  vpc_id      = var.vpc_id
}
resource "aws_security_group" "endpoints" {
  name_prefix = "${var.name}-endpoints-"
  vpc_id      = var.vpc_id
}
resource "aws_vpc_security_group_ingress_rule" "public" {
  for_each          = toset(["80", "443"])
  security_group_id = aws_security_group.alb.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = tonumber(each.value)
  to_port           = tonumber(each.value)
}
resource "aws_vpc_security_group_ingress_rule" "alb_to_control" {
  for_each                     = toset(["42069", "8081"])
  security_group_id            = aws_security_group.control.id
  referenced_security_group_id = aws_security_group.alb.id
  ip_protocol                  = "tcp"
  from_port                    = tonumber(each.value)
  to_port                      = tonumber(each.value)
}
resource "aws_vpc_security_group_egress_rule" "alb_to_control" {
  for_each                     = toset(["42069", "8081"])
  security_group_id            = aws_security_group.alb.id
  referenced_security_group_id = aws_security_group.control.id
  ip_protocol                  = "tcp"
  from_port                    = tonumber(each.value)
  to_port                      = tonumber(each.value)
}
resource "aws_vpc_security_group_egress_rule" "control_https" {
  security_group_id = aws_security_group.control.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
}
resource "aws_vpc_security_group_ingress_rule" "apps" {
  security_group_id            = aws_security_group.apps.id
  referenced_security_group_id = aws_security_group.control.id
  ip_protocol                  = "tcp"
  from_port                    = 8080
  to_port                      = 8080
}
resource "aws_vpc_security_group_egress_rule" "control_apps" {
  security_group_id            = aws_security_group.control.id
  referenced_security_group_id = aws_security_group.apps.id
  ip_protocol                  = "tcp"
  from_port                    = 8080
  to_port                      = 8080
}
locals {
  db_rules = {
    control     = { source = aws_security_group.control.id, target = aws_security_group.control_db.id }
    provisioner = { source = aws_security_group.control.id, target = aws_security_group.apps_db.id }
    apps        = { source = aws_security_group.apps.id, target = aws_security_group.apps_db.id }
  }
}
resource "aws_vpc_security_group_ingress_rule" "database" {
  for_each                     = local.db_rules
  security_group_id            = each.value.target
  referenced_security_group_id = each.value.source
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
}
resource "aws_vpc_security_group_egress_rule" "database" {
  for_each                     = local.db_rules
  security_group_id            = each.value.source
  referenced_security_group_id = each.value.target
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
}
resource "aws_vpc_security_group_ingress_rule" "endpoint" {
  for_each                     = { control = aws_security_group.control.id, apps = aws_security_group.apps.id }
  security_group_id            = aws_security_group.endpoints.id
  referenced_security_group_id = each.value
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
}
resource "aws_vpc_security_group_egress_rule" "apps_endpoints" {
  security_group_id            = aws_security_group.apps.id
  referenced_security_group_id = aws_security_group.endpoints.id
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
}
resource "aws_vpc_endpoint" "interface" {
  for_each            = toset(["ecr.api", "ecr.dkr", "logs", "secretsmanager", "ssm", "ssmmessages"])
  vpc_id              = var.vpc_id
  service_name        = "com.amazonaws.${var.region}.${each.value}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = var.private_subnet_ids
  security_group_ids  = [aws_security_group.endpoints.id]
  private_dns_enabled = true
}
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = var.vpc_id
  service_name      = "com.amazonaws.${var.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = var.private_route_table_ids
  # Do not leave a public-S3 exfiltration route available to generated tasks.
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Principal = "*", Action = ["s3:GetObject"], Resource = ["arn:aws:s3:::prod-${var.region}-starport-layer-bucket/*", "arn:aws:s3:::al2023-repos-${var.region}-*/*", "arn:aws:s3:::amazon-ssm-${var.region}/*"] },
    { Effect = "Allow", Principal = "*", Action = ["s3:GetObject", "s3:GetObjectTagging", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket", "s3:GetBucketLocation"], Resource = [aws_s3_bucket.files.arn, "${aws_s3_bucket.files.arn}/*"], Condition = { ArnEquals = { "aws:PrincipalArn" = aws_iam_role.control.arn } } }
  ] })
}
resource "aws_vpc_security_group_egress_rule" "apps_s3_layers" {
  security_group_id = aws_security_group.apps.id
  prefix_list_id    = aws_vpc_endpoint.s3.prefix_list_id
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
}
