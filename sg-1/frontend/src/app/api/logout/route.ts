import { NextResponse } from "next/server";
import { publicURL, SESSION_COOKIE } from "@/lib/auth";
import { isSameOrigin } from "@/lib/security";

export async function POST(request: Request) {
  const base = publicURL();
  if (!isSameOrigin(request.headers.get("origin"), base))
    return new Response(null, { status: 403 });
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
