import { NextResponse } from "next/server";
import { publicURL, sessionToken } from "@/lib/auth";
import { backendFetch } from "@/lib/api";
import { isSameOrigin, readLimitedBody } from "@/lib/security";

type Context = { params: Promise<{ slug: string; action: string }> };
async function proxy(request: Request, context: Context) {
  if (request.method !== "GET" && !isSameOrigin(request.headers.get("origin"), publicURL()))
    return NextResponse.json({ error: { message: "Origin is not allowed" } }, { status: 403 });
  const token = await sessionToken();
  if (!token)
    return NextResponse.json({ error: { message: "Sign in to continue" } }, { status: 401 });
  const { slug, action } = await context.params;
  const methods: Record<string, string> = {
    runtime: "GET",
    source: "GET",
    builds: "POST",
    preview: "POST",
    access: "PUT",
    rollback: "POST",
    retry: "POST",
  };
  if (methods[action] !== request.method) return new Response(null, { status: 405 });
  let body: string | undefined;
  if (["builds", "access", "rollback"].includes(action)) {
    if (request.headers.get("content-type")?.split(";")[0] !== "application/json")
      return new Response(null, { status: 415 });
    try {
      body = await readLimitedBody(request, 20 * 1024);
    } catch {
      return NextResponse.json({ error: { message: "Request is too large" } }, { status: 413 });
    }
  }
  try {
    const response = await backendFetch(
      `/v1/projects/${encodeURIComponent(slug)}/${action}`,
      token,
      {
        method: request.method,
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": request.headers.get("Idempotency-Key") || "",
        },
        body,
      },
    );
    return new Response(
      action === "source" && response.ok ? await response.arrayBuffer() : await response.text(),
      {
        status: response.status,
        headers: {
          "Content-Type":
            action === "source" && response.ok ? "application/zip" : "application/json",
          "Cache-Control": "no-store",
          ...(action === "source" && response.ok
            ? { "Content-Disposition": response.headers.get("Content-Disposition") || "attachment" }
            : {}),
        },
      },
    );
  } catch {
    return NextResponse.json(
      { error: { message: "The API is unavailable. Retry safely." } },
      { status: 503 },
    );
  }
}
export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
