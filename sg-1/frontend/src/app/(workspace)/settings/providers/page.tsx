import type { Metadata } from "next";
import { ProviderSettings } from "@/components/provider-settings";
import { APIError, apiGet } from "@/lib/api";
import type { LLMSettings } from "@/lib/types";

export const metadata: Metadata = { title: "AI providers" };

export default async function ProvidersPage() {
  let settings: LLMSettings;
  try {
    settings = await apiGet<LLMSettings>("/v1/settings/llm-providers");
  } catch (error) {
    if (error instanceof APIError && error.status === 503)
      return (
        <main className="standalone-state">
          <span className="tiny-label">SECURE STORAGE REQUIRED</span>
          <h1>Credential storage is not configured.</h1>
          <p>
            An administrator must configure the backend encryption key before API keys can be saved.
            For local setup, run <code>node scripts/setup.mjs</code> and recreate the API container.
            Do not replace an existing encryption key.
          </p>
        </main>
      );
    throw error;
  }
  return <ProviderSettings settings={settings} />;
}
