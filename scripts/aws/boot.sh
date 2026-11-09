#!/bin/bash
set -euo pipefail
cd /opt/sg1
umask 077
test "$(id -u)" = 0 || { echo "Run through an administrator SSM session" >&2; exit 1; }
region=$(jq -r .AWS_REGION deployment.json)
repository=$(jq -r .SG1_API_REPOSITORY deployment.json)
aws ecr get-login-password --region "$region" | docker login --username AWS --password-stdin "${repository%%/*}"
python3 configure.py
trap 'rm -f .env.migrate' EXIT
compose() { docker compose --env-file .release.env -f compose.aws.yaml "$@"; }
compose pull api gateway worker web migrate
compose run --rm migrate
python3 configure.py grant
compose up -d --wait api gateway worker web
url=$(jq -r .SG1_PUBLIC_URL deployment.json)
printf '\nControl plane is healthy. Verify both company sign-in providers at %s before opening access to employees.\n' "$url"
