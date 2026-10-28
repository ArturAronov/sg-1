import { NextResponse } from "next/server";
import { developmentEnabled, publicURL, SESSION_COOKIE, validDevToken } from "@/lib/auth";
import { isSameOrigin, readLimitedBody } from "@/lib/security";

export async function POST(request: Request) {
  if (!developmentEnabled())
    return NextResponse.json({ error: { message: "SSO is not configured" } }, { status: 503 });
  const base = publicURL();
  if (!isSameOrigin(request.headers.get("origin"), base))
    return NextResponse.json({ error: { message: "Origin is not allowed" } }, { status: 403 });
  const contentType = request.headers.get("content-type")?.split(";")[0];
  if (contentType !== "application/x-www-form-urlencoded")
    return new Response(null, { status: 415 });
  let token: string;
  try {
    token = new URLSearchParams(await readLimitedBody(request, 4096)).get("token") || "";
  } catch {
    return new Response(null, { status: 413 });
  }
  if (!validDevToken(token)) return NextResponse.redirect(new URL("/login?error=1", base), 303);
  const response = NextResponse.redirect(new URL("/", base), 303);
  response.cookies.set(SESSION_COOKIE, token, {
    httpOnly: true,
    secure: new URL(base).protocol === "https:",
    sameSite: "lax",
    path: "/",
    maxAge: 8 * 60 * 60,
  });
  response.headers.set("Cache-Control", "no-store");
  return response;
}
