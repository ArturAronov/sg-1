import { NextResponse } from "next/server";
import { ALB_SESSION_COOKIES } from "@/lib/auth-cookies";
import { publicURL, SESSION_COOKIE } from "@/lib/auth";
import { isSameOrigin } from "@/lib/security";

export async function POST(request: Request) {
  const base = publicURL();
  if (!isSameOrigin(request.headers.get("origin"), base))
    return new Response(null, { status: 403 });
  if (process.env.SG1_ENV === "production") {
    const domain = process.env.SG1_COGNITO_DOMAIN;
    const client = process.env.SG1_COGNITO_CLIENT_ID;
    if (!domain || !client || new URL(domain).protocol !== "https:")
      return new Response("Sign-out is not configured", { status: 503 });
    const target = new URL("/logout", domain);
    target.searchParams.set("client_id", client);
    target.searchParams.set("logout_uri", new URL("/signed-out", base).href);
    const response = NextResponse.redirect(target, 303);
    // Clear every known shard, even when missing/duplicated in the request.
    // Legacy names are cleared for migration, never accepted for authentication.
    for (const name of [
      ...ALB_SESSION_COOKIES,
      "SG1AuthSession",
      ...[0, 1, 2, 3].map((i) => `SG1AuthSession-${i}`),
    ]) {
      response.cookies.set(name, "", {
        path: "/",
        secure: true,
        httpOnly: true,
        maxAge: 0,
      });
    }
    response.headers.set("Cache-Control", "no-store");
    return response;
  }
  const response = NextResponse.redirect(new URL("/login", base), 303);
  response.cookies.set(SESSION_COOKIE, "", {
    httpOnly: true,
    sameSite: "lax",
    secure: new URL(base).protocol === "https:",
    path: "/",
    maxAge: 0,
  });
  response.headers.set("Cache-Control", "no-store");
  return response;
}
