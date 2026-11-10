import "server-only";
import { timingSafeEqual } from "node:crypto";
import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";
import { hasAmbiguousALBSessionCookies } from "@/lib/auth-cookies";

export const SESSION_COOKIE = "sg1_development_session";

export function developmentEnabled(): boolean {
  return process.env.SG1_ENV === "development" && (process.env.SG1_DEV_TOKEN?.length ?? 0) >= 32;
}

export function validDevToken(token: string): boolean {
  if (!developmentEnabled()) return false;
  const expected = Buffer.from(process.env.SG1_DEV_TOKEN!);
  const actual = Buffer.from(token);
  return actual.length === expected.length && timingSafeEqual(actual, expected);
}

export async function sessionToken(): Promise<string | null> {
  if (process.env.SG1_ENV === "production") {
    // The Go API verifies the ES256 signature, signer ALB, issuer, client, and expiry.
    // Never trust x-amzn-oidc-identity or an unsigned email header as authentication.
    const incoming = await headers();
    if (hasAmbiguousALBSessionCookies(incoming.get("cookie"))) return null;
    const token = incoming.get("x-amzn-oidc-data");
    return token && token.length <= 24576 ? token : null;
  }
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  return token && validDevToken(token) ? token : null;
}

export async function requireSession(): Promise<string> {
  const token = await sessionToken();
  if (!token) redirect("/login");
  return token;
}

export function publicURL(): string {
  // Fail closed instead of trusting an arbitrary Host or forwarded-host header.
  const value = process.env.SG1_PUBLIC_URL;
  if (!value) throw new Error("SG1_PUBLIC_URL is required");
  return new URL(value).origin;
}
