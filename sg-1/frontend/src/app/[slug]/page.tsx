import { notFound, redirect } from "next/navigation";

// Preserve previously shared viewer links without serving apps on SG-1's origin.
export default async function LegacyApplicationViewer({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  if (!/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(slug) || slug.length > 48) notFound();
  redirect(`/projects/${encodeURIComponent(slug)}`);
}
