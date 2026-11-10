# Shared-domain browser isolation

Approved layout (Osaka, `ap-northeast-3`):

| Purpose | URL |
| --- | --- |
| Trusted dashboard | `https://sg1.chickensoup.dev` |
| Shared viewer | `https://sg1.chickensoup.dev/projects/maintenance-tracker` |
| Generated app in the iframe | `https://maintenance-tracker.apps.chickensoup.dev` |

One registered domain is sufficient. Use its authoritative public Route 53 zone for both the platform A alias and `*.apps.chickensoup.dev` A alias, plus ACM validation CNAMEs. The certificate contains `sg1.chickensoup.dev` and `*.apps.chickensoup.dev`. Do not register a second domain or change registrar delegation without approval.

## Security contract

- Origins are separated by hostname. Generated HTML/JS must never be proxied onto the platform origin, even under `/projects/*`. That path serves only the trusted iframe viewer. Application hostnames must occupy a disjoint namespace: the platform may not equal, contain or sit underneath the wildcard app namespace.
- Siblings share a registrable domain and are **same-site**, not same-origin. `SameSite` cookies are not a CSRF boundary here. All platform mutation handlers must continue to require an exact platform `Origin`; no wildcard sibling CORS or credential forwarding.
- The ALB listener uses **`__Host-SG1AuthSession`**, with any shard suffix appended after that prefix. Supporting browsers require Secure, Path=/, and no Domain for `__Host-` cookies. An app cannot manufacture a Domain=chickensoup.dev cookie with that name in such browsers.
- The frontend rejects duplicate session cookie names and mixed unsharded/sharded session cookies before forwarding signed identity to the API. Logout clears every configured base/shard name, independent of which names the request contains. Legacy unprefixed names are cleared during migration but are not the configured authentication session.
- Gateway preview/password cookies already use **`__Host-sg1_app`**, Secure, HttpOnly, Path=/, no Domain, SameSite=None and Partitioned. The gateway removes reserved platform/access cookies and identity/Authorization headers before proxying to generated code, strips Domain from upstream Set-Cookie, and blocks reserved cookie names in upstream responses.
- ALB identity claims are still cryptographically verified in Go. Neither cookie presence, unsigned identity headers, nor matching email grants an employee identity.
- The iframe sandbox continues to prohibit top-level navigation/popups and the platform denies cross-origin framing of its dashboard. Do not relax the sandbox or add `document.domain`/wildcard CORS to make app interactions work.

The frontend cookie constants and Terraform `session_cookie_name` must remain synchronized. Do not revert to unprefixed authentication cookie names as an AWS troubleshooting workaround. The cookie-name migration invalidates existing ALB sessions and requires employees to sign in again; schedule it explicitly if a deployment already exists.

## Residual risks and mandatory live verification

This is not equivalent to a separate registered domain. Generated JS can still set arbitrary **unprotected** parent-domain cookie names; cookie/header flooding can disrupt access. Any future security-sensitive platform cookie must use the correct host prefix/options and be included in the threat review. AWS-managed auxiliary cookies (for example authentication nonce behavior), legacy browsers, reverse-proxy cookie parsing, CSP, and tenant-controlled content need real-account security testing.

Before admitting employees or sensitive data:

1. Verify Entra sign-in, refresh and logout against the deployed ALB. Inspect cookies privately: the base and every shard must have the exact protected name, Secure, HttpOnly, Path=/ and no Domain. A local browser test with synthetic cookies does **not** prove AWS issued them correctly. If ALB rejects the prefix or changes its cookie attributes, stop and reassess the architecture; do not silently weaken it.
2. In each supported browser, use an approved disposable app to attempt parent-domain and sibling-domain overwrites of all employee session shards and the gateway access cookie. Confirm the legitimate host-only cookies remain unchanged and invisible to app JS.
3. Confirm sibling app requests cannot mutate platform APIs even though the sites share `chickensoup.dev`; reject missing, null and sibling Origins. Check no employee identity/cookies reach generated handlers. The viewer remains public according to the selected app access mode, while the builder/API require identity and project authorization.
4. Test duplicate/mixed cookies, old session cookies, session expiry, app password changes, logout, and immediate SG-1 user deactivation. Never include actual cookie/JWT values in logs or chat.
5. Review cookie-flooding/oversized-header behavior and recovery, plus AWS-managed auth nonce handling. Record any denial-of-service exposure rather than claiming that host prefixes eliminate every shared-domain risk.

Local regression coverage lives in frontend `tests/auth-cookies.test.ts`, `tests/security.test.ts`, and the synthetic HTTPS sibling-cookie test in `tests/ui/theme.spec.ts`; backend `internal/platform/cookie_test.go` checks reserved names and access-cookie attributes. These tests create no real projects or AWS resources. Live acceptance and security sign-off are still required.
