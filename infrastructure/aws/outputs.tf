output "deployment" {
  value = {
    AWS_REGION                  = var.region
    SG1_AWS_ACCOUNT_ID          = local.account
    SG1_AWS_PREFIX              = var.name
    SG1_PUBLIC_URL              = "https://${var.domain}"
    SG1_APP_ORIGIN              = "https://{slug}.${var.apps_domain}"
    SG1_ALB_ARN                 = aws_lb.ingress.arn
    SG1_COGNITO_ISSUER          = "https://cognito-idp.${var.region}.amazonaws.com/${aws_cognito_user_pool.employees.id}"
    SG1_COGNITO_CLIENT_ID       = aws_cognito_user_pool_client.alb.id
    SG1_IDENTITY_PROVIDERS      = join(",", sort(tolist(local.enabled_identity_providers)))
    SG1_COGNITO_DOMAIN          = "https://${aws_cognito_user_pool_domain.employees.domain}.auth.${var.region}.amazoncognito.com"
    SG1_S3_BUCKET               = aws_s3_bucket.files.id
    SG1_ECS_CLUSTER_ARN         = aws_ecs_cluster.apps.arn
    SG1_DISCOVERY_NAMESPACE_ID  = aws_service_discovery_private_dns_namespace.apps.id
    SG1_DISCOVERY_NAMESPACE     = aws_service_discovery_private_dns_namespace.apps.name
    SG1_TASK_SECURITY_GROUP     = aws_security_group.apps.id
    SG1_TASK_SUBNETS            = join(",", var.private_subnet_ids)
    SG1_TASK_EXECUTION_ROLE_ARN = aws_iam_role.execution.arn
    SG1_BUILD_BOUNDARY_ARN      = aws_iam_policy.build_boundary.arn
    SG1_APP_LOG_GROUP           = aws_cloudwatch_log_group.apps.name
    SG1_CONTROL_LOG_GROUP       = aws_cloudwatch_log_group.control.name
    SG1_CONFIG_SECRET_ARN       = aws_secretsmanager_secret.settings.arn
    SG1_CONTROL_RDS_SECRET_ARN  = aws_db_instance.postgres["control"].master_user_secret[0].secret_arn
    SG1_PROJECT_RDS_SECRET_ARN  = aws_db_instance.postgres["apps"].master_user_secret[0].secret_arn
    SG1_CONTROL_RDS_HOST        = aws_db_instance.postgres["control"].address
    SG1_PROJECT_RDS_HOST        = aws_db_instance.postgres["apps"].address
    SG1_API_REPOSITORY          = aws_ecr_repository.platform["api"].repository_url
    SG1_WEB_REPOSITORY          = aws_ecr_repository.platform["web"].repository_url
    SG1_INSTANCE_ID             = aws_instance.control.id
  }
}
