import "server-only";
import { timingSafeEqual } from "node:crypto";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

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
