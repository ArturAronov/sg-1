import { notFound } from "next/navigation";

export const dynamic = "force-dynamic";
export default async function ApplicationViewer({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  if (!/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(slug) || slug.length > 48) notFound();
  const base = process.env.SG1_API_URL;
  if (!base) notFound();
  const response = await fetch(new URL(`/public/projects/${encodeURIComponent(slug)}`, base), {
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) notFound();
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
