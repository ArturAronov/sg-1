import { notFound } from "next/navigation";
import { LiveBuilder, type RuntimeState } from "@/components/live-builder";
import { APIError, apiGet } from "@/lib/api";
import type { LLMSettings } from "@/lib/types";

export default async function BuilderPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  let initial: RuntimeState;
  let settings: LLMSettings;
  try {
    [initial, settings] = await Promise.all([
      apiGet<RuntimeState>(`/v1/projects/${encodeURIComponent(slug)}/runtime`),
      apiGet<LLMSettings>("/v1/settings/llm-providers"),
    ]);
  } catch (error) {
    if (error instanceof APIError && error.status === 404) notFound();
    throw error;
  }
  return <LiveBuilder initial={initial} settings={settings} />;
}
