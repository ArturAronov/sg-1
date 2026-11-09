#!/bin/bash
set -euo pipefail
# No application secrets, SSH keys, or source credentials in EC2 user data.
dnf install -y docker jq postgresql17
systemctl enable --now docker
install -d -m 0700 /opt/sg1
install -d -m 0755 /usr/local/lib/docker/cli-plugins
COMPOSE_VERSION=v2.31.0
curl --fail --silent --show-error --location "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-x86_64" -o /usr/local/lib/docker/cli-plugins/docker-compose
curl --fail --silent --show-error --location "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/checksums.txt" -o /opt/sg1/compose-checksums.txt
cd /usr/local/lib/docker/cli-plugins
grep ' docker-compose-linux-x86_64$' /opt/sg1/compose-checksums.txt | sed 's/docker-compose-linux-x86_64$/docker-compose/' | sha256sum --check --status
chmod 0755 docker-compose
