import { NextResponse } from "next/server";
import { sessionToken } from "@/lib/auth";
import { backendFetch } from "@/lib/api";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ provider: string }> },
) {
  const token = await sessionToken();
  if (!token)
    return NextResponse.json({ error: { message: "Sign in to continue" } }, { status: 401 });
  const { provider } = await params;
  if (!["openai", "anthropic", "glm", "kimi"].includes(provider))
    return new Response(null, { status: 400 });
  try {
    const r = await backendFetch(`/v1/settings/llm-providers/${provider}/models`, token);
    return new Response(await r.text(), {
      status: r.status,
      headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
    });
  } catch {
    return NextResponse.json(
      { error: { message: "Could not load models; enter the model ID manually" } },
      { status: 503 },
    );
  }
}
