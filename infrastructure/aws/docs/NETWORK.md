# SG-1 AWS network and security-group reference

Read with [docs/SETUP.md](../docs/SETUP.md) and [IAM.md](IAM.md). The tables describe **current Terraform rules**, including authentication HTTPS egress. Do not confuse rules defined in source with rules already applied in AWS. See [shared-domain browser security](COOKIE_SECURITY.md) for cookie and origin requirements.

Sources: [`network.tf`](../network.tf), [`compute.tf`](../compute.tf), [`dns-firewall.tf`](../dns-firewall.tf), [`data.tf`](../data.tf), [`compose.aws.yaml`](../../../compose.aws.yaml).

## 1. Placement and default posture

| Component           | Placement                                                               | Exposure                                                                                       |
| ------------------- | ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Internet-facing ALB | At least two public subnets/AZs                                         | IPv4 TCP 80/443; HTTP redirects to HTTPS                                                       |
| Control EC2         | First supplied private subnet                                           | No public IP, no SSH, only ALB-to-web/gateway ingress; SSM administration                      |
| RDS control/apps    | Supplied private subnet group, Multi-AZ                                 | Two instances/SGs; not publicly accessible                                                     |
| Fargate apps        | Supplied private subnets                                                | `AssignPublicIp=DISABLED`; no public ALB registration of app tasks; gateway is the entry point |
| Interface endpoints | Supplied private subnets                                                | Private DNS, dedicated shared endpoint SG                                                      |
| S3 endpoint         | Gateway endpoint on **every supplied private route table**              | Restricted bucket/principal policy                                                             |
| CodeBuild           | AWS-managed build environment, **no VPC config in this implementation** | Fargate SG/route/DNS restrictions do not apply to generated builds                             |

Use a **dedicated existing VPC**. This module does not create the VPC, NAT, IGW, subnet routes, NACLs, or hosted zones. Terraform validates private subnet VPC membership, distinct AZs, and VPC DNS features; that is not a complete route/public-subnet/ownership audit.

No IPv6 rules/AAAA records/egress-only IGW are configured by this module. Adding IPv6 or another SG can change the isolation boundary and requires a separate review. Multiple SGs combine their Allows; never attach a permissive extra SG to a task/host to troubleshoot.

## 2. Exact security-group matrices

SG names have a generated suffix because Terraform uses `name_prefix`. Refer to SG **IDs**, not guessed full names. `P` below is the deployment name. All listed ports are **TCP**; ranges are single destination ports.

### Inbound rules currently declared

| Terraform SG                    | Name prefix     | Source      | Destination port | Reason                                      |
| ------------------------------- | --------------- | ----------- | ---------------- | ------------------------------------------- |
| `aws_security_group.alb`        | `P-alb-`        | `0.0.0.0/0` | 80               | Public HTTP redirect only                   |
| `aws_security_group.alb`        | `P-alb-`        | `0.0.0.0/0` | 443              | Public HTTPS + employee authentication      |
| `aws_security_group.control`    | `P-control-`    | ALB SG      | 42069            | Frontend target                             |
| `aws_security_group.control`    | `P-control-`    | ALB SG      | 8081             | Gateway target                              |
| `aws_security_group.apps`       | `P-apps-`       | Control SG  | 8080             | Gateway/controller to generated application |
| `aws_security_group.control_db` | `P-control-db-` | Control SG  | 5432             | Control DB access                           |
| `aws_security_group.apps_db`    | `P-apps-db-`    | Control SG  | 5432             | Project DB provisioner                      |
| `aws_security_group.apps_db`    | `P-apps-db-`    | Apps SG     | 5432             | Project application SQL                     |
| `aws_security_group.endpoints`  | `P-endpoints-`  | Control SG  | 443              | AWS interface endpoints                     |
| `aws_security_group.endpoints`  | `P-endpoints-`  | Apps SG     | 443              | ECS execution path via interface endpoints  |

No SSH 22, direct API 8080 on EC2, Docker 2375/2376, public PostgreSQL, task self-ingress, or public task port is allowed. The `apps` ingress rule is not permission for arbitrary task-to-task initiation.

### Outbound rules currently declared

| Source SG | Destination                                      | Port  | Reason                                                                                            |
| --------- | ------------------------------------------------ | ----- | ------------------------------------------------------------------------------------------------- |
| ALB       | Control SG                                       | 42069 | Frontend requests/health                                                                          |
| ALB       | Control SG                                       | 8081  | Gateway requests/health                                                                           |
| ALB       | `0.0.0.0/0`                                      | 443   | Cognito token/user-info HTTPS; does not grant task internet egress |
| Control   | `0.0.0.0/0`                                      | 443   | AWS API/SSM, provider HTTPS, key retrieval, package/release tooling through applicable routes     |
| Control   | Apps SG                                          | 8080  | Private HTTP app access/probes                                                                    |
| Control   | Control DB SG                                    | 5432  | Control SQL                                                                                       |
| Control   | Apps DB SG                                       | 5432  | Provisioner SQL                                                                                   |
| Apps      | Apps DB SG                                       | 5432  | Project SQL                                                                                       |
| Apps      | Endpoints SG                                     | 443   | Private ECR/logs/Secrets Manager service endpoints                                                |
| Apps      | Regional S3 managed prefix list from S3 endpoint | 443   | Image-layer and explicitly allowed AWS bootstrap-object downloads; endpoint policy narrows access |

There are no explicit outbound rules for the DB or endpoint SGs. SGs are **stateful**: replies to allowed connections do not require matching outbound rules. AWS initially creates default egress on a new SG; Terraform's SG resource removes that default and manages the standalone rules. Inspect the deployed SGs to detect drift; do not assume the default still exists or add it back.

SGs cannot authorize an FQDN or an S3 bucket. The S3 managed prefix list alone covers regional S3 IPs, **not just one bucket**. The endpoint/bucket/IAM policies enforce the storage boundary. SGs do not filter AmazonProvidedDNS; see the resolver section.

### Required correction: ALB-to-Cognito HTTPS

`network.tf` includes `alb_auth_https` for outbound IPv4 TCP 443. Authentication needs access from the ALB to token/user-info endpoints. An employee's browser reaching Cognito is not sufficient: the ALB must complete its own HTTPS exchange. Verify the rule in the approved plan and deployed SG.

The implemented rule is shown below. It is not applied to AWS merely because it exists in Terraform or documentation:

```hcl
resource "aws_vpc_security_group_egress_rule" "alb_auth_https" {
  security_group_id = aws_security_group.alb.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  description       = "ALB HTTPS to Cognito authentication endpoints"
}
```

Obtain security approval for the destination scope. If narrower destinations are required, use an approved maintained IPv4 destination strategy compatible with Cognito changes; don't hardcode one DNS answer or invent a Cognito SG/FQDN rule. This rule allows only ALB-initiated TCP 443, not arbitrary ports or task internet egress. Verify public subnet IGW routing, NACLs and every enabled provider's full callback flow after an approved apply.

Source: [AWS ALB authentication requirements](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html#configure-user-authentication). Do not remove authentication while testing reachability.

## 3. Routing and NACLs

The network owner must supply and validate:

| Route table                          | Required path                                                                                                                                      |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Public ALB subnets, at least two AZs | VPC-local routes plus `0.0.0.0/0 -> IGW`; verify ALB subnet size/free IPs against AWS guidance                                                     |
| Each private control/task/DB subnet  | VPC-local routes; private endpoint reachability; control-plane public HTTPS through `0.0.0.0/0 -> NAT gateway` (or separately approved equivalent) |
| Every supplied private route table   | Regional S3 managed-prefix route through the restricted S3 gateway endpoint                                                                        |

Fargate uses the same private subnets here; a NAT route does **not** mean task internet access is authorized. Task SG egress does not allow arbitrary public TCP 443. Preserve that restriction. For resilience, review NAT per AZ/route ownership with the network owner; this module does not create NAT redundancy.

NACLs are **stateless**. A restrictive NACL needs the necessary request ports **and return ephemeral destination ports** in the opposite direction. Review the actual client/platform ranges (commonly TCP 1024–65535) for ALB-target, EC2/task-DB, HTTPS endpoint, NAT/IGW, and cross-subnet traffic. A return-packet failure can look like an IAM or TLS timeout. Do not introduce blanket public task/DB SG ingress to fix a NACL.

VPC DNS support and DNS hostnames must both be enabled. Instances/tasks must use the intended VPC resolver/private DNS configuration. If custom resolvers, proxies, inspection appliances, or transit/peering exist, assess DNS and bypass routes explicitly. Custom DNS traffic is not automatically allowed by the SG matrices above. Inspect more-specific routes as well as default routes.

Before creating endpoints, look for existing endpoints/private DNS and route-table associations. An S3 gateway endpoint policy or DNS firewall applied in a shared VPC can break unrelated systems. Do not proceed on a shared VPC without an approved redesign.

## 4. VPC endpoints and resource policies

### Interface endpoints

All use `com.amazonaws.R.<service>`, private DNS, private subnet ENIs and the endpoint SG:

| Service          | Purpose                         |
| ---------------- | ------------------------------- |
| `ecr.api`        | ECR API/authorization           |
| `ecr.dkr`        | ECR Docker registry             |
| `logs`           | Container log delivery          |
| `secretsmanager` | Execution/control secret access |
| `ssm`            | Instance management             |
| `ssmmessages`    | SSM session/data channels       |

The current Terraform does **not** declare restrictive policies on these interface endpoints. Do not claim that endpoint policy isolates projects: service IAM and absence of an app task role enforce authorization. A service execution role is shared across task definitions. Additional endpoint policies are a separate reviewed hardening change.

CodeBuild, ECS control APIs, IAM, Cloud Map APIs, ALB key retrieval and AI provider endpoints are not all covered by these six endpoints. The trusted EC2 control plane still needs approved NAT HTTPS egress. Verify installed SSM agent/region requirements; if a legacy agent needs `ec2messages`, upgrade/review the exact requirement instead of opening arbitrary ingress.

### S3 gateway endpoint

The endpoint is associated with `private_route_table_ids`. Its policy allows:

| Principal                                             | Actions                                                                                            | Resource                                                                                     |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `*`                                                   | `s3:GetObject`                                                                                     | `arn:aws:s3:::prod-R-starport-layer-bucket/*` (regional ECR layers)                          |
| `*`                                                   | `s3:GetObject`                                                                                     | `arn:aws:s3:::al2023-repos-R-*/*` and `arn:aws:s3:::amazon-ssm-R/*` (AWS bootstrap packages) |
| Exact EC2 `P-control` role ARN via `aws:PrincipalArn` | `s3:GetObject`, `GetObjectTagging`, `PutObject`, `DeleteObject`, `ListBucket`, `GetBucketLocation` | Application bucket `B` and `B/*`                                                             |

Everything else through this endpoint is implicitly denied, including arbitrary third-party buckets and app-role access to uploads/build sources. AWS repository exceptions are broader than ECR layers alone; verify required bucket naming and supply-chain assumptions in the chosen region. These endpoint Allows do not override a bucket Deny or grant access to private AWS objects by themselves.

An external publisher uploads through the public S3 service using its own IAM policy; it is not the EC2 role. A publisher, Terraform runner, or state backend **inside these routed private subnets** may fail because the endpoint policy does not allow their principal/bucket. Prefer the approved external operator/CI placement; any state/publisher endpoint exception needs a narrowly scoped reviewed IaC change. Do not grant `Principal: "*"` access to all application/state objects.

### Application bucket policy

- All actions require `aws:SecureTransport=true`.
- `GetObject`/`GetObjectVersion` on `uploads/*` are denied unless `GuardDutyMalwareScanStatus=NO_THREATS_FOUND`.
- The exact GuardDuty scanner role is exempt from that quarantine Deny to scan new input. No other role is exempt.
- Public access is blocked on all four S3 controls; ACLs are disabled (`BucketOwnerEnforced`), versioning is enabled and default SSE-S3/AES256 encryption is configured.
- Temporary `builds/*` current/noncurrent objects expire after one day. This is not an approved uploads/releases/history deletion policy.

The API independently checks the clean tag before download, serves attachments and enforces project authorization. Missing, unsafe, unsupported or failed scan results must fail closed. Never work around the policy with manual clean tags or public object URLs.

## 5. DNS Firewall and egress limitations

`dns-firewall.tf` associates a mutation-protected group with the **entire VPC**, association priority 100, fail-open **DISABLED**:

1. Rule priority 100 ALLOW: `*.amazonaws.com`, `*.amazoncognito.com`, `amazonlinux.com`, `*.amazonlinux.com`, `github.com`, `*.githubusercontent.com`, the four configured provider API hostnames, `P.internal` and `*.P.internal`.
2. Rule priority 200 BLOCK: `*`, response `NODATA`.

Provider names currently allowed: `api.openai.com`, `api.anthropic.com`, `api.z.ai`, `api.moonshot.ai`. The allow rule uses `TRUST_REDIRECTION_DOMAIN`; review that CNAME/redirection behavior and required package/release hosts when testing bootstrap. Do not add `*` to the allowlist, remove the block rule, or switch fail-open to fix an unresolved name.

This is not complete data-loss prevention. The AWS/GitHub allowlist is intentionally broad and shared by trusted EC2 and app tasks; DNS queries to allowed names remain possible. Fargate SGs separately block general HTTPS, but shared application DB endpoints and allowed DNS remain attack surfaces. CodeBuild is outside this VPC entirely. Document these limitations, test negative paths and assess stricter per-workload networking if the security requirement demands it. Don't claim DNS-only policy proves arbitrary untrusted code cannot exfiltrate data.

## 6. Listener rules and transport

### HTTPS listener routing (lower priority number wins)

| Priority | Host/path                                            | Action                                                            |
| -------- | ---------------------------------------------------- | ----------------------------------------------------------------- |
| 5        | Platform host, `/signed-out`                         | Public frontend                                                   |
| 10       | Platform host, `/`, `/build/*`, `/settings/*`, `/app-access/*` | Cognito authentication, then frontend |
| 11       | Platform host, `/api/*`, `/login`                    | Cognito authentication, then frontend                             |
| 15       | Remaining platform-host paths (viewer/static assets) | Public frontend; private APIs still verify identity independently |
| 20       | `*.apps_domain`                                      | Gateway; app access enforced there                                |
| Default  | Unmatched host                                       | Fixed 404                                                         |

Do not protect every platform path with SSO: visitor-facing `/projects/{app-slug-id}` URLs must remain usable according to app access settings. These paths are handled by the priority-15 public frontend rule; legacy root-level viewer URLs redirect to the new path. The trusted viewer embeds generated code on the separate gateway origin rather than serving it from SG-1's origin. Conversely, a public viewer route is not permission to bypass backend identity checks. Explicitly test path variations and the callback handling. Identity verification checks ES256 signature, signer ALB ARN, Cognito issuer, client, expiry and immutable subject; unsigned identity/email headers are not trusted.

| Connection                             | Current transport                                                                         |
| -------------------------------------- | ----------------------------------------------------------------------------------------- |
| Browser -> ALB                         | HTTPS 443, ACM certificate, `ELBSecurityPolicy-TLS13-1-2-2021-06`; HTTP 80 only redirects |
| ALB -> EC2 frontend/gateway            | **HTTP**, constrained by SGs; ALB claims signed, not encrypted on this hop                |
| Frontend -> API (host Compose network) | HTTP, no API host port                                                                    |
| Gateway -> Fargate                     | **HTTP** 8080 on private network                                                          |
| EC2/Fargate -> RDS                     | TLS, `sslmode=verify-full`, official RDS CA; `rds.force_ssl=1`                            |
| AWS API/provider access                | HTTPS with certificate verification                                                       |

Do not call this end-to-end TLS. If the company's policy requires encrypted ALB-to-target or gateway-to-app traffic, stop for an approved implementation change before acceptance. The present private HTTP design does not meet that requirement automatically.

Frontend health: HTTP `/signed-out` -> 200 on 42069. Gateway health: `/__sg1_health` -> 200 on 8081. API readiness includes database migrations and S3 bucket access; it is not proof of an entire upload/scan/download or SSO flow.

## 7. Read-only verification commands

Run only in an approved account/region with the [validator permissions](IAM.md#read-only-validation-permissions). Replace placeholders with verified IDs; do not dump Terraform state or secret-bearing API responses. These examples are inspection, not instructions to mutate resources.

```sh
export AWS_REGION='ap-northeast-3'
export VPC_ID='<approved-vpc-id>'
aws ec2 describe-vpc-attribute --vpc-id "$VPC_ID" --attribute enableDnsSupport
aws ec2 describe-vpc-attribute --vpc-id "$VPC_ID" --attribute enableDnsHostnames
aws ec2 describe-subnets --filters "Name=vpc-id,Values=$VPC_ID" \
  --query 'Subnets[].{Id:SubnetId,AZ:AvailabilityZone,CIDR:CidrBlock,PublicIP:MapPublicIpOnLaunch,FreeIPs:AvailableIpAddressCount}'
aws ec2 describe-route-tables --filters "Name=vpc-id,Values=$VPC_ID" \
  --query 'RouteTables[].{Id:RouteTableId,Associations:Associations,Routes:Routes}'
aws ec2 describe-security-groups --filters "Name=vpc-id,Values=$VPC_ID" \
  --query 'SecurityGroups[].{Id:GroupId,Name:GroupName,Ingress:IpPermissions,Egress:IpPermissionsEgress}'
aws ec2 describe-vpc-endpoints --filters "Name=vpc-id,Values=$VPC_ID" \
  --query 'VpcEndpoints[].{Id:VpcEndpointId,Type:VpcEndpointType,Service:ServiceName,State:State,PrivateDNS:PrivateDnsEnabled,Routes:RouteTableIds,Subnets:SubnetIds,Policy:PolicyDocument}'
aws ec2 describe-network-acls --filters "Name=vpc-id,Values=$VPC_ID" \
  --query 'NetworkAcls[].{Id:NetworkAclId,Associations:Associations,Entries:Entries}'
aws ecr get-registry-scanning-configuration --query 'scanningConfiguration'
aws elbv2 describe-target-health --target-group-arn '<WEB_TARGET_GROUP_ARN>' \
  --query 'TargetHealthDescriptions[].{Id:Target.Id,Port:Target.Port,State:TargetHealth.State,Reason:TargetHealth.Reason}'
aws guardduty get-malware-protection-plan --malware-protection-plan-id '<PLAN_ID>' \
  --query '{Status:Status,Reasons:StatusReasons,Resource:ProtectedResource,Actions:Actions}'
```

Inspect resolver association/rules with scoped `route53resolver:Get*`/`List*` permissions if needed; the commands above do not verify them. Verify the deployed rules against the matrices plus the **approved** ALB HTTPS correction. Unexplained extra rules/routes/SGs/endpoints are drift to investigate, not an acceptable substitute for the declared design.

Negative connectivity tests execute code/requests and may create artifacts: obtain separate test approval, run only from the intended workload boundary and use disposable data. Validate blocked control/IMDS/other-task paths, unauthorized database login, cross-project ECR/source access, third-party S3 and unapproved DNS **as well as** permitted startup, providers and SQL. Record actual observations and remaining limitations, not just that a security-group table looked correct.
