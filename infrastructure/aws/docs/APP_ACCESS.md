# Published application access

Project administrators choose **Application access** in the builder's **Sharing & passwords** section. This controls the actual `<slug>.apps.chickensoup.dev` gateway, not just the public viewer link.

| Mode | Who may open the generated app |
| --- | --- |
| Project members only (`members`) | Signed-in, active employees with an explicit `project_memberships` row for this project (read, write or admin) |
| Public (`public`) | Anyone, without employee sign-in or an app password |
| Password protected (`password`) | Visitors with the project's password; retained for existing workflows |

New projects default to `members`. Migration `0008_member_app_access.sql` adds the mode/default without altering existing public/password choices. Run migrations before starting the updated services; readiness requires the new migration.

## Authorization and sign-in flow

1. `/projects/<slug>` remains the shared viewer link. For member-only apps it redirects to `/app-access/<slug>`, which is an employee-authenticated ALB route. Public/password viewers remain available without employee SSO.
2. After employee sign-in, the trusted frontend requests `/v1/projects/<slug>/member-session`. The API checks active status and explicit project membership. Department or platform admin status alone does **not** grant member-only app access, although existing administrator permissions still govern project/settings administration.
3. The API issues a short-lived signed gateway handoff URL, embedded in the isolated iframe. The handoff expires in one minute. The gateway rechecks membership and exchanges it for a host-only, HttpOnly access cookie lasting up to 30 minutes. These URLs/cookies are bearer credentials: never log or share them, and never forward employee ALB identity into generated code.
4. Every restricted HTTP request and WebSocket upgrade validates the signed cookie, project ID, credential version, expiry, active account and current membership. Direct app URLs do not bypass the gate: browser visits go to the employee entry route; unauthenticated API calls receive 401.
5. Changing the access mode increments the credential version, invalidating previous gateway sessions. Removing project membership or disabling an employee denies subsequent member-app requests immediately. Existing in-flight streams/WebSocket connections are not forcibly terminated; establish an operational disconnect/restart policy if that stronger revocation is required.

An employee's builder preview cannot bypass member-only restrictions: previews also require explicit membership when this mode is selected. Password login cannot unlock a member-only app. Account removal/membership checks fail closed on database errors.

The creator is assigned project admin membership during project creation. This feature uses existing explicit membership records; it does **not** add an employee directory/invite/member-management UI. Provision additional memberships through the approved administrator process until that management interface exists. Do not add every department member or every platform administrator automatically.

Logging out of the platform does not automatically clear cookies on app hostnames; app sessions have their own bounded expiry, and account/membership revocation is checked separately. Review this behavior during live SSO acceptance. A signed-in user can still deliberately share secrets/screenshots; membership gating is not DRM.

## Acceptance checks

- Verify the creator and an explicitly assigned read/write member can open both shared and direct app URLs.
- Verify a signed-in non-member, including a department/platform administrator without membership, cannot receive a member session or use preview to bypass the gate.
- Verify anonymous direct API requests fail; direct browser visits lead to employee sign-in without exposing generated responses.
- Remove membership/disable the account, reuse its existing gateway cookie, and confirm the next request fails. Document the existing-stream limitation.
- Switch public -> members -> public and check anonymous access and old-cookie invalidation. Retest legacy password mode and version revocation.
- Only project administrators may change access. Switching to public requires explicit confirmation in the UI. Audit records track access-mode changes.
- Verify `/app-access/*` is included in the authenticated ALB listener paths when applying this update. Old ALB rules without that path cannot provide the required employee identity.

Backend regression tests use isolated PostgreSQL schemas and an in-memory upstream (`member_access_integration_test.go`), not real projects or AWS resources. Frontend presentation tests exercise the member-only selector, signed-in viewer and non-member denial with fixtures; live Entra/ALB/gateway acceptance is still required.
