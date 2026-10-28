import { NextResponse } from "next/server";
import { publicURL, sessionToken } from "@/lib/auth";
import { backendFetch } from "@/lib/api";
import { isSameOrigin, readLimitedBody } from "@/lib/security";

export async function POST(request: Request) {
  if (!isSameOrigin(request.headers.get("origin"), publicURL()))
    return NextResponse.json({ error: { message: "Origin is not allowed" } }, { status: 403 });
  const token = await sessionToken();
  if (!token)
    return NextResponse.json({ error: { message: "Sign in to continue" } }, { status: 401 });
  if (request.headers.get("content-type")?.split(";")[0] !== "application/json")
    return NextResponse.json({ error: { message: "Use application/json" } }, { status: 415 });
  let body: string;
  try {
    body = await readLimitedBody(request);
  } catch {
    return NextResponse.json(
      { error: { message: "Request is too large or invalid" } },
      { status: 413 },
    );
  }
  try {
    const response = await backendFetch("/v1/projects", token, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Idempotency-Key": request.headers.get("Idempotency-Key") || "",
      },
      body,
    });
    return new Response(await response.text(), {
      status: response.status,
      headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
    });
  } catch {
    return NextResponse.json(
      { error: { message: "The API is unavailable. You can safely retry." } },
      { status: 503 },
    );
  }
}
