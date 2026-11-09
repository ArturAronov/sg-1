resource "aws_iam_role" "execution" {
  name               = "${var.name}-task-execution"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = "ecs-tasks.amazonaws.com" }, Action = "sts:AssumeRole", Condition = { StringEquals = { "aws:SourceAccount" = local.account } } }] })
}
resource "aws_iam_role_policy" "execution" {
  role = aws_iam_role.execution.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Action = ["ecr:GetAuthorizationToken"], Resource = "*" },
    { Effect = "Allow", Action = ["ecr:BatchCheckLayerAvailability", "ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage"], Resource = [local.project_repo_arn, aws_ecr_repository.platform["api"].arn] },
    { Effect = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"], Resource = "${aws_cloudwatch_log_group.apps.arn}:*" },
    { Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = local.workload_secret_arn }
  ] })
}
resource "aws_iam_policy" "build_boundary" {
  name = "${var.name}-build-boundary"
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Action = ["s3:GetObject"], Resource = "${aws_s3_bucket.files.arn}/builds/*" },
    { Effect = "Allow", Action = ["ecr:GetAuthorizationToken"], Resource = "*" },
    { Effect = "Allow", Action = ["ecr:BatchCheckLayerAvailability", "ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage", "ecr:InitiateLayerUpload", "ecr:UploadLayerPart", "ecr:CompleteLayerUpload", "ecr:PutImage"], Resource = local.project_repo_arn },
    { Effect = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"], Resource = "${aws_cloudwatch_log_group.builds.arn}:*" }
  ] })
}
resource "aws_iam_role" "control" {
  name               = "${var.name}-control"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole" }] })
}
resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.control.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}
resource "aws_iam_role_policy" "control" {
  role = aws_iam_role.control.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Action = ["s3:ListBucket", "s3:GetBucketLocation"], Resource = aws_s3_bucket.files.arn },
    { Effect = "Allow", Action = ["s3:GetObject", "s3:GetObjectTagging", "s3:PutObject", "s3:DeleteObject"], Resource = "${aws_s3_bucket.files.arn}/*" },
    { Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = concat([aws_secretsmanager_secret.settings.arn], [for db in aws_db_instance.postgres : db.master_user_secret[0].secret_arn]) },
    { Effect = "Allow", Action = ["secretsmanager:PutSecretValue"], Resource = aws_secretsmanager_secret.settings.arn },
    { Effect = "Allow", Action = ["secretsmanager:CreateSecret", "secretsmanager:DescribeSecret", "secretsmanager:PutSecretValue", "secretsmanager:DeleteSecret"], Resource = local.workload_secret_arn },
    { Effect = "Allow", Action = ["ecr:GetAuthorizationToken"], Resource = "*" },
    { Effect = "Allow", Action = ["ecr:BatchCheckLayerAvailability", "ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage", "ecr:DescribeImages", "ecr:DescribeRepositories", "ecr:DescribeImageScanFindings"], Resource = concat([local.project_repo_arn], [for repo in aws_ecr_repository.platform : repo.arn]) },
    { Effect = "Allow", Action = ["ecr:CreateRepository", "ecr:TagResource"], Resource = local.project_repo_arn },
    { Effect = "Allow", Action = ["iam:CreateRole"], Resource = local.build_role_arn, Condition = { StringEquals = { "iam:PermissionsBoundary" = aws_iam_policy.build_boundary.arn } } },
    { Effect = "Allow", Action = ["iam:GetRole", "iam:PutRolePolicy", "iam:TagRole"], Resource = local.build_role_arn },
    { Effect = "Allow", Action = ["iam:PassRole"], Resource = local.build_role_arn, Condition = { StringEquals = { "iam:PassedToService" = "codebuild.amazonaws.com" } } },
    { Effect = "Allow", Action = ["iam:PassRole"], Resource = aws_iam_role.execution.arn, Condition = { StringEquals = { "iam:PassedToService" = "ecs-tasks.amazonaws.com" } } },
    { Effect = "Allow", Action = ["codebuild:CreateProject", "codebuild:StartBuild", "codebuild:StopBuild", "codebuild:BatchGetBuilds"], Resource = ["arn:aws:codebuild:${var.region}:${local.account}:project/${var.name}-*", "arn:aws:codebuild:${var.region}:${local.account}:build/${var.name}-*:*"] },
    { Effect = "Allow", Action = ["ecs:RegisterTaskDefinition", "ecs:DeregisterTaskDefinition"], Resource = "*" },
    { Effect = "Allow", Action = ["iam:CreateServiceLinkedRole"], Resource = "arn:aws:iam::${local.account}:role/aws-service-role/ecs.amazonaws.com/AWSServiceRoleForECS", Condition = { StringEquals = { "iam:AWSServiceName" = "ecs.amazonaws.com" } } },
    { Effect = "Allow", Action = ["ecs:CreateService", "ecs:UpdateService", "ecs:DeleteService", "ecs:DescribeServices"], Resource = "arn:aws:ecs:${var.region}:${local.account}:service/${aws_ecs_cluster.apps.name}/sg1-app-*" },
    { Effect = "Allow", Action = ["servicediscovery:CreateService", "servicediscovery:DeleteService", "servicediscovery:GetService", "servicediscovery:GetNamespace", "servicediscovery:RegisterInstance", "servicediscovery:DeregisterInstance", "servicediscovery:GetOperation"], Resource = [aws_service_discovery_private_dns_namespace.apps.arn, "arn:aws:servicediscovery:${var.region}:${local.account}:service/*", "arn:aws:servicediscovery:${var.region}:${local.account}:operation/*"] },
    { Effect = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"], Resource = "${aws_cloudwatch_log_group.control.arn}:*" }
  ] })
}
resource "aws_iam_instance_profile" "control" {
  name = "${var.name}-control"
  role = aws_iam_role.control.name
}
