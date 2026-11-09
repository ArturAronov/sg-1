#!/usr/bin/env python3
"""Run on the private EC2 host. No secrets are printed or put in command arguments."""
import base64
import json
import os
import re
from pathlib import Path
import secrets
import subprocess
import sys
from urllib.parse import quote

root = Path(__file__).resolve().parent
config = json.loads((root / "deployment.json").read_text())
release = json.loads((root / "release.json").read_text())
region = config["AWS_REGION"]

def aws(*args, payload=None):
    command = ["aws", "--region", region, *args, "--output", "json"]
    result = subprocess.run(command, input=payload, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise RuntimeError("AWS request failed; verify the instance role and secret configuration")
    return json.loads(result.stdout)

def secret(arn):
    return json.loads(aws("secretsmanager", "get-secret-value", "--secret-id", arn)["SecretString"])

settings = secret(config["SG1_CONFIG_SECRET_ARN"])
key = settings.get("SG1_CREDENTIALS_KEY", "")
if len(base64.b64decode(key, validate=True)) != 32:
    raise RuntimeError("Set SG1_CREDENTIALS_KEY to a separately generated base64-encoded 32-byte key")
if not settings.get("SG1_BOOTSTRAP_SUBJECTS"):
    raise RuntimeError("Set the authorized Cognito bootstrap subject(s); never use email for identity linking")
if not settings.get("SG1_DATABASE_PASSWORD"):
    settings["SG1_DATABASE_PASSWORD"] = secrets.token_urlsafe(40)
    # Keep the existing encryption key and all existing settings, never rotate them.
    aws("secretsmanager", "put-secret-value", "--cli-input-json", "file:///dev/stdin", payload=json.dumps({"SecretId": config["SG1_CONFIG_SECRET_ARN"], "SecretString": json.dumps(settings)}))
if not re.fullmatch(r"[A-Za-z0-9_-]{32,128}", settings["SG1_DATABASE_PASSWORD"]):
    raise RuntimeError("Runtime database password must be a strong, generated URL-safe credential")

control = secret(config["SG1_CONTROL_RDS_SECRET_ARN"])
projects = secret(config["SG1_PROJECT_RDS_SECRET_ARN"])

def connection(user, password, host, database):
    return f"postgres://{quote(user, safe='')}:{quote(password, safe='')}@{host}:5432/{database}?sslmode=verify-full&sslrootcert=/etc/ssl/certs/sg1-rds-ca.pem"

def env_file(name, values):
    # Docker env files are not shell scripts. Refuse multiline/interpolation input.
    for value in values.values():
        if any(char in str(value) for char in ["\n", "\r", "\x00", "'", "\\"]):
            raise RuntimeError("Unsafe environment value; use generated URL-safe credentials")
    path = root / name
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as target:
        for key, value in values.items():
            target.write(f"{key}='{value}'\n")
    os.chmod(path, 0o600)

def psql(credentials, host, database, statement):
    env = {**os.environ, "PGPASSWORD": credentials["password"], "PGSSLMODE": "verify-full", "PGSSLROOTCERT": str(root / "rds-ca.pem")}
    result = subprocess.run(["psql", "-X", "-h", host, "-U", credentials["username"], "-d", database, "-v", "ON_ERROR_STOP=1"], input=statement, text=True, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    if result.returncode:
        raise RuntimeError("RDS setup failed; verify TLS, database permissions, and migration status")

if sys.argv[1:] == ["grant"]:
    password = settings["SG1_DATABASE_PASSWORD"].replace("'", "''")
    psql(control, config["SG1_CONTROL_RDS_HOST"], "sg1", f"""
DO $body$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='sg1_api') THEN CREATE ROLE sg1_api LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION; END IF; END $body$;
ALTER ROLE sg1_api PASSWORD '{password}';
REVOKE CONNECT ON DATABASE sg1 FROM PUBLIC;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE sg1 TO sg1_api;
GRANT USAGE ON SCHEMA public TO sg1_api;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO sg1_api;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO sg1_api;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO sg1_api;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE,SELECT ON SEQUENCES TO sg1_api;
""")
    psql(projects, config["SG1_PROJECT_RDS_HOST"], "sg1_apps", "REVOKE CONNECT ON DATABASE sg1_apps FROM PUBLIC;")
    print("RDS application grants applied without exposing credentials.")
else:
    common = {k: v for k, v in config.items() if k.startswith("SG1_") or k == "AWS_REGION"}
    common.update({"SG1_ENV": "production", "SG1_RUNTIME": "aws", "SG1_LISTEN_ADDR": "0.0.0.0:8080", "SG1_CREDENTIALS_KEY": key,
        "SG1_INIT_IMAGE": release["SG1_API_IMAGE"],
        "SG1_BOOTSTRAP_SUBJECTS": settings["SG1_BOOTSTRAP_SUBJECTS"],
        "SG1_DATABASE_URL": connection("sg1_api", settings["SG1_DATABASE_PASSWORD"], config["SG1_CONTROL_RDS_HOST"], "sg1"),
        # Non-workers need the hostname for validation, not the provisioner credential.
        "SG1_PROJECT_DATABASE_URL": connection("unused", "unused", config["SG1_PROJECT_RDS_HOST"], "sg1_apps")})
    env_file(".env.production", common)
    env_file(".env.worker", {"SG1_PROJECT_DATABASE_URL": connection(projects["username"], projects["password"], config["SG1_PROJECT_RDS_HOST"], "sg1_apps")})
    env_file(".env.migrate", {"SG1_DATABASE_URL": connection(control["username"], control["password"], config["SG1_CONTROL_RDS_HOST"], "sg1")})
    env_file(".env.web", {key: common[key] for key in ["SG1_ENV", "SG1_RUNTIME", "SG1_PUBLIC_URL", "SG1_APP_ORIGIN", "SG1_COGNITO_CLIENT_ID", "SG1_COGNITO_DOMAIN"]} | {"SG1_API_URL": "http://api:8080"})
    env_file(".release.env", {**release, "AWS_REGION": region, "SG1_CONTROL_LOG_GROUP": config["SG1_CONTROL_LOG_GROUP"]})
    print("Production environment files written with mode 0600. Existing encryption key preserved.")
