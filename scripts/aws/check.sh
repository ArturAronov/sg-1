#!/bin/bash
set -euo pipefail
# Read-only AWS checks; uses the deploying operator's identity, not a generated app.
config=${1:-infrastructure/aws/deployment.json}
region=$(jq -r .AWS_REGION "$config")
account=$(jq -r .SG1_AWS_ACCOUNT_ID "$config")
test "$(aws sts get-caller-identity --query Account --output text)" = "$account" || { echo "Wrong AWS account" >&2; exit 1; }
bucket=$(jq -r .SG1_S3_BUCKET "$config")
aws s3api get-public-access-block --region "$region" --bucket "$bucket" | jq -e '.PublicAccessBlockConfiguration | .BlockPublicAcls and .IgnorePublicAcls and .BlockPublicPolicy and .RestrictPublicBuckets' >/dev/null
aws s3api get-bucket-encryption --region "$region" --bucket "$bucket" | jq -e '.ServerSideEncryptionConfiguration.Rules | length > 0' >/dev/null
aws s3api get-bucket-versioning --region "$region" --bucket "$bucket" | jq -e '.Status == "Enabled"' >/dev/null
prefix=$(jq -r .SG1_AWS_PREFIX "$config")
for suffix in control apps; do
  aws rds describe-db-instances --region "$region" --db-instance-identifier "$prefix-$suffix" | jq -e '.DBInstances[0] | .DBInstanceStatus == "available" and .PubliclyAccessible == false and .StorageEncrypted and .MultiAZ and .DeletionProtection and .BackupRetentionPeriod >= 7' >/dev/null
done
pool=$(jq -r '.SG1_COGNITO_ISSUER | split("/")[-1]' "$config")
client=$(jq -r .SG1_COGNITO_CLIENT_ID "$config")
providers=$(jq -ce '.SG1_IDENTITY_PROVIDERS | split(",") | select(index("Entra") != null and all(.[]; . == "Entra" or . == "OneLogin"))' "$config")
aws cognito-idp describe-user-pool-client --region "$region" --user-pool-id "$pool" --client-id "$client" | jq -e --argjson expected "$providers" '(.UserPoolClient.SupportedIdentityProviders | sort) == ($expected | sort)' >/dev/null
instance=$(jq -r .SG1_INSTANCE_ID "$config")
aws ec2 describe-instances --region "$region" --instance-ids "$instance" | jq -e '.Reservations[0].Instances[0] | .MetadataOptions.HttpTokens == "required" and .PublicIpAddress == null' >/dev/null
printf 'AWS account, private encrypted S3/RDS, configured identity providers, and private IMDSv2 EC2 checks passed.\n'
printf 'Still required: live sign-in, upload/scan/download, isolation, build/deploy/cancel/rollback, and recovery checks in infrastructure/aws/README.md.\n'
