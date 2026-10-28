import { NextResponse } from "next/server";
import { backendFetch } from "@/lib/api";
import { publicURL, sessionToken } from "@/lib/auth";
import { isSameOrigin } from "@/lib/security";

type Context = { params: Promise<{ slug: string }> };
export async function GET(_r: Request, { params }: Context) {
  const token = await sessionToken();
  if (!token) return new Response(null, { status: 401 });
  const { slug } = await params;
  try {
    const r = await backendFetch(`/v1/projects/${encodeURIComponent(slug)}/files`, token);
    return new Response(await r.text(), {
      status: r.status,
      headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
    });
  } catch {
    return NextResponse.json(
      { error: { message: "File storage is unavailable" } },
      { status: 503 },
    );
  }
}
export async function POST(request: Request, { params }: Context) {
  if (!isSameOrigin(request.headers.get("origin"), publicURL()))
    return new Response(null, { status: 403 });
  const token = await sessionToken();
  if (!token) return new Response(null, { status: 401 });
  const { slug } = await params;
  const type = request.headers.get("content-type") || "";
  if (!type.startsWith("multipart/form-data;")) return new Response(null, { status: 415 });
  const reader = request.body?.getReader();
  if (!reader) return new Response(null, { status: 400 });
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      total += value.byteLength;
      if (total > 10 * 1024 * 1024 + 64 * 1024) {
        await reader.cancel();
        return NextResponse.json(
          { error: { message: "Files must be at most 10 MiB" } },
          { status: 413 },
        );
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  try {
    const r = await backendFetch(`/v1/projects/${encodeURIComponent(slug)}/files`, token, {
      method: "POST",
      headers: { "Content-Type": type },
      body: Buffer.concat(chunks),
    });
    return new Response(await r.text(), {
      status: r.status,
      headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
    });
  } catch {
    return NextResponse.json({ error: { message: "File upload failed" } }, { status: 503 });
  }
}
