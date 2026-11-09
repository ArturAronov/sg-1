resource "aws_s3_bucket" "files" {
  bucket        = "${var.name}-${local.account}-${var.region}"
  force_destroy = false
  lifecycle { prevent_destroy = true }
}
resource "aws_s3_bucket_public_access_block" "files" {
  bucket                  = aws_s3_bucket.files.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_ownership_controls" "files" {
  bucket = aws_s3_bucket.files.id
  rule { object_ownership = "BucketOwnerEnforced" }
}
resource "aws_s3_bucket_server_side_encryption_configuration" "files" {
  bucket = aws_s3_bucket.files.id
  rule {
    apply_server_side_encryption_by_default { sse_algorithm = "AES256" }
  }
}
resource "aws_s3_bucket_versioning" "files" {
  bucket = aws_s3_bucket.files.id
  versioning_configuration { status = "Enabled" }
}
resource "aws_s3_bucket_policy" "tls" {
  bucket = aws_s3_bucket.files.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Deny", Principal = "*", Action = "s3:*", Resource = [aws_s3_bucket.files.arn, "${aws_s3_bucket.files.arn}/*"], Condition = { Bool = { "aws:SecureTransport" = "false" } } },
    { Effect = "Deny", Principal = "*", Action = ["s3:GetObject", "s3:GetObjectVersion"], Resource = "${aws_s3_bucket.files.arn}/uploads/*", Condition = {
      StringNotEquals = { "s3:ExistingObjectTag/GuardDutyMalwareScanStatus" = "NO_THREATS_FOUND" },
      ArnNotEquals    = { "aws:PrincipalArn" = aws_iam_role.malware.arn }
    } }
  ] })
}
resource "aws_db_subnet_group" "private" {
  name       = var.name
  subnet_ids = var.private_subnet_ids
}
resource "aws_db_parameter_group" "postgres" {
  name_prefix = "${var.name}-"
  family      = "postgres17"
  parameter {
    name         = "rds.force_ssl"
    value        = "1"
    apply_method = "pending-reboot"
  }
}
resource "aws_db_instance" "postgres" {
  for_each                        = { control = aws_security_group.control_db.id, apps = aws_security_group.apps_db.id }
  identifier                      = "${var.name}-${each.key}"
  engine                          = "postgres"
  engine_version                  = "17"
  instance_class                  = var.db_instance_class
  allocated_storage               = 30
  max_allocated_storage           = 200
  storage_type                    = "gp3"
  storage_encrypted               = true
  username                        = "sg1admin"
  manage_master_user_password     = true
  db_name                         = each.key == "control" ? "sg1" : "sg1_apps"
  db_subnet_group_name            = aws_db_subnet_group.private.name
  parameter_group_name            = aws_db_parameter_group.postgres.name
  vpc_security_group_ids          = [each.value]
  publicly_accessible             = false
  multi_az                        = true
  backup_retention_period         = 35
  copy_tags_to_snapshot           = true
  deletion_protection             = true
  skip_final_snapshot             = false
  final_snapshot_identifier       = "${var.name}-${each.key}-final"
  auto_minor_version_upgrade      = true
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]
  depends_on                      = [aws_cloudwatch_log_group.rds]
  lifecycle { prevent_destroy = true }
}
resource "aws_cloudwatch_log_group" "rds" {
  for_each          = toset(["control-postgresql", "control-upgrade", "apps-postgresql", "apps-upgrade"])
  name              = "/aws/rds/instance/${var.name}-${split("-", each.value)[0]}/${split("-", each.value)[1]}"
  retention_in_days = 30
}
resource "aws_ecr_repository" "platform" {
  for_each             = toset(["api", "web"])
  name                 = "${var.name}-${each.value}"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration { encryption_type = "AES256" }
}
resource "aws_cloudwatch_log_group" "apps" {
  name              = "/aws/ecs/${var.name}"
  retention_in_days = 30
}
resource "aws_cloudwatch_log_group" "builds" {
  name              = "/aws/codebuild/${var.name}"
  retention_in_days = 30
}
resource "aws_cloudwatch_log_group" "control" {
  name              = "/sg1/${var.name}/control"
  retention_in_days = 30
}
resource "aws_secretsmanager_secret" "settings" {
  name                    = "${var.name}/config/application"
  recovery_window_in_days = 30
  lifecycle { prevent_destroy = true }
}
resource "aws_ecs_cluster" "apps" {
  name = "${var.name}-apps"
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}
resource "aws_service_discovery_private_dns_namespace" "apps" {
  name = "${var.name}.internal"
  vpc  = var.vpc_id
}
