import { NextResponse } from "next/server";
import { publicURL, sessionToken } from "@/lib/auth";
import { backendFetch } from "@/lib/api";
import { isSameOrigin, readLimitedBody } from "@/lib/security";

type RouteContext = { params: Promise<{ provider: string }> };

async function update(request: Request, context: RouteContext) {
  if (!isSameOrigin(request.headers.get("origin"), publicURL()))
    return NextResponse.json({ error: { message: "Origin is not allowed" } }, { status: 403 });
  const token = await sessionToken();
  if (!token)
    return NextResponse.json({ error: { message: "Sign in to continue" } }, { status: 401 });
  const { provider } = await context.params;
  if (!["openai", "anthropic", "glm", "kimi"].includes(provider))
    return NextResponse.json({ error: { message: "Unknown AI provider" } }, { status: 400 });
  let body: string | undefined;
  if (request.method === "PUT") {
    if (request.headers.get("content-type")?.split(";")[0] !== "application/json")
      return NextResponse.json({ error: { message: "Use application/json" } }, { status: 415 });
    try {
      body = await readLimitedBody(request, 8192);
    } catch {
      return NextResponse.json(
        { error: { message: "Request is too large or invalid" } },
        { status: 413 },
      );
    }
  }
  try {
    const response = await backendFetch(`/v1/settings/llm-providers/${provider}`, token, {
      method: request.method,
      headers: { "Content-Type": "application/json" },
      body,
    });
    return new Response(response.status === 204 ? null : await response.text(), {
      status: response.status,
      headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
    });
  } catch {
    return NextResponse.json(
      { error: { message: "Could not reach secure storage. Try again." } },
      { status: 503 },
    );
  }
}

export const PUT = update;
export const DELETE = update;
