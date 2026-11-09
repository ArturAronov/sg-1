terraform {
  required_version = ">= 1.10, < 2.0"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 6.0" }
  }
  # Use an existing encrypted, versioned state bucket with native S3 locking.
  backend "s3" {}
}
provider "aws" {
  region = var.region
  default_tags { tags = { Application = "SG-1", Environment = "production", ManagedBy = "Terraform" } }
}
data "aws_caller_identity" "current" {}
data "aws_vpc" "existing" { id = var.vpc_id }
data "aws_subnet" "private" {
  for_each = toset(var.private_subnet_ids)
  id       = each.value
}
data "aws_ssm_parameter" "ami" { name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64" }
locals {
  account             = data.aws_caller_identity.current.account_id
  registry            = "${local.account}.dkr.ecr.${var.region}.amazonaws.com"
  project_repo_arn    = "arn:aws:ecr:${var.region}:${local.account}:repository/${var.name}-project-*"
  build_role_arn      = "arn:aws:iam::${local.account}:role/${var.name}-build-*"
  workload_secret_arn = "arn:aws:secretsmanager:${var.region}:${local.account}:secret:${var.name}/workloads/*"
}
