import { NextResponse } from "next/server";
import { sessionToken } from "@/lib/auth";
import { backendFetch } from "@/lib/api";

export async function GET() {
  const token = await sessionToken();
  if (!token)
    return NextResponse.json({ error: { message: "Sign in to continue" } }, { status: 401 });
  try {
    const r = await backendFetch("/v1/settings/llm-providers", token);
    return new Response(await r.text(), {
      status: r.status,
      headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
    });
  } catch {
    return NextResponse.json(
      { error: { message: "Credential settings are unavailable" } },
      { status: 503 },
    );
  }
}
