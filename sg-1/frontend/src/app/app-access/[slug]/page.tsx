import { notFound } from "next/navigation";
import { backendFetch } from "@/lib/api";
import { requireSession } from "@/lib/auth";

export const dynamic = "force-dynamic";

export default async function MemberApplicationViewer({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  if (!/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(slug) || slug.length > 48) notFound();
  const token = await requireSession();
  const response = await backendFetch(
    `/v1/projects/${encodeURIComponent(slug)}/member-session`,
    token,
  );
  if (response.status === 401 || response.status === 403 || response.status === 404) {
    return (
      <main className="standalone-state">
        <h1>Project membership required.</h1>
        <p>
          Only active members of this project can open its application. Ask a project administrator
          for access.
        </p>
      </main>
    );
  }
  if (!response.ok) throw new Error("Application access is temporarily unavailable");
  const { url } = await response.json();
  return (
    <main className="published-viewer">
      <iframe
        title="Published application"
        src={url}
        sandbox="allow-scripts allow-forms allow-same-origin allow-downloads"
        referrerPolicy="no-referrer"
      />
    </main>
  );
}
