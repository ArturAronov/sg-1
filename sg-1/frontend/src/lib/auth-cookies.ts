// Must match the ALB listener in infrastructure/aws/compute.tf. The __Host-
// prefix forbids Domain cookies and non-root paths in supporting browsers.
export const ALB_SESSION_COOKIE = "__Host-SG1AuthSession";
export const ALB_SESSION_COOKIES = [
  ALB_SESSION_COOKIE,
  ...[0, 1, 2, 3].map((shard) => `${ALB_SESSION_COOKIE}-${shard}`),
];

export function hasAmbiguousALBSessionCookies(header: string | null): boolean {
  const seen = new Set<string>();
  for (const part of (header ?? "").split(";")) {
    const name = part.split("=", 1)[0].trim();
    if (!ALB_SESSION_COOKIES.includes(name)) continue;
    if (seen.has(name)) return true;
    seen.add(name);
  }
  // A session is either unsharded or sharded, never both.
  return seen.has(ALB_SESSION_COOKIE) && seen.size > 1;
}
