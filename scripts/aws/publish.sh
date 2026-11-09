#!/bin/bash
set -euo pipefail
# Run at repository root after Terraform apply. No cloud resources are created here.
config=${1:-infrastructure/aws/deployment.json}
test -f "$config" || { echo "Export: terraform -chdir=infrastructure/aws output -json deployment > infrastructure/aws/deployment.json" >&2; exit 1; }
test -z "$(git status --porcelain)" || { echo "Commit/review the working tree before publishing immutable production artifacts." >&2; exit 1; }
region=$(jq -r .AWS_REGION "$config")
account=$(jq -r .SG1_AWS_ACCOUNT_ID "$config")
test "$(aws sts get-caller-identity --query Account --output text)" = "$account" || { echo "Wrong AWS account" >&2; exit 1; }
revision=$(git rev-parse HEAD)
api=$(jq -r .SG1_API_REPOSITORY "$config")
web=$(jq -r .SG1_WEB_REPOSITORY "$config")
registry=${api%%/*}
aws ecr get-login-password --region "$region" | docker login --username AWS --password-stdin "$registry"
if ! aws ecr describe-images --region "$region" --repository-name "${api#*/}" --image-ids "imageTag=$revision" >/dev/null 2>&1; then
  docker buildx build --platform linux/amd64 --target production --push --tag "$api:$revision" sg-1/backend
fi
if ! aws ecr describe-images --region "$region" --repository-name "${web#*/}" --image-ids "imageTag=$revision" >/dev/null 2>&1; then
  docker buildx build --platform linux/amd64 --push --tag "$web:$revision" sg-1/frontend
fi
for repository in "${api#*/}" "${web#*/}"; do
  aws ecr wait image-scan-complete --region "$region" --repository-name "$repository" --image-id "imageTag=$revision"
  aws ecr describe-image-scan-findings --region "$region" --repository-name "$repository" --image-id "imageTag=$revision" | jq -e '.imageScanStatus.status == "COMPLETE" and ((.imageScanFindings.findingSeverityCounts.HIGH // 0) == 0) and ((.imageScanFindings.findingSeverityCounts.CRITICAL // 0) == 0)' >/dev/null
done
api_digest=$(aws ecr describe-images --region "$region" --repository-name "${api#*/}" --image-ids "imageTag=$revision" --query 'imageDetails[0].imageDigest' --output text)
web_digest=$(aws ecr describe-images --region "$region" --repository-name "${web#*/}" --image-ids "imageTag=$revision" --query 'imageDetails[0].imageDigest' --output text)
stage=$(mktemp -d)
trap 'rm -f "$stage/compose.aws.yaml" "$stage/configure.py" "$stage/boot.sh" "$stage/rds-ca.pem" "$stage/deployment.json" "$stage/release.json" "$stage/release.tar.gz"; rmdir "$stage"' EXIT
cp compose.aws.yaml scripts/aws/configure.py scripts/aws/boot.sh "$stage/"
cp sg-1/backend/internal/platform/rds-ca.pem "$stage/rds-ca.pem"
cp "$config" "$stage/deployment.json"
jq -n --arg api "$api@$api_digest" --arg web "$web@$web_digest" --arg revision "$revision" '{SG1_API_IMAGE:$api,SG1_WEB_IMAGE:$web,revision:$revision}' > "$stage/release.json"
tar -czf "$stage/release.tar.gz" -C "$stage" compose.aws.yaml configure.py boot.sh rds-ca.pem deployment.json release.json
bucket=$(jq -r .SG1_S3_BUCKET "$config")
aws s3 cp --region "$region" "$stage/release.tar.gz" "s3://$bucket/releases/$revision/release.tar.gz" --sse AES256
printf '\nPublished revision %s. Deploy this release through SSM after the production checklist passes.\n' "$revision"
printf 'Release: s3://%s/releases/%s/release.tar.gz\n' "$bucket" "$revision"
python3 -c 'import hashlib,sys; print("Release SHA-256: " + hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$stage/release.tar.gz"
