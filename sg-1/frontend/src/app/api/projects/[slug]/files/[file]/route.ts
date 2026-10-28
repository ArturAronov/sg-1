import { backendFetch } from "@/lib/api";
import { sessionToken } from "@/lib/auth";

export async function GET(
  _r: Request,
  { params }: { params: Promise<{ slug: string; file: string }> },
) {
  const token = await sessionToken();
  if (!token) return new Response(null, { status: 401 });
  const { slug, file } = await params;
  const r = await backendFetch(
    `/v1/projects/${encodeURIComponent(slug)}/files/${encodeURIComponent(file)}`,
    token,
  );
  return new Response(r.body, {
    status: r.status,
    headers: {
      "Content-Type": r.ok ? "application/octet-stream" : "application/json",
      "Cache-Control": "no-store",
      "Content-Disposition": r.headers.get("Content-Disposition") || "attachment",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
