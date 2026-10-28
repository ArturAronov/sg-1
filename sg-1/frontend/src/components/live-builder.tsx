"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Icon } from "@/components/icon";
import { ProjectFiles } from "@/components/project-files";
import { useHydrated } from "@/lib/hydration";
import type { LLMSettings, Project } from "@/lib/types";

export type RuntimeState = {
  project: Project;
  messages: { id: string; role: string; content: string; created_at: string }[];
  builds: { id: string; status: string; message: string; provider: string; model: string }[];
  deployments: { id: string; summary: string; created_at: string }[];
  current_deployment_id: string | null;
  last_error: string | null;
  password_set: boolean;
};
async function call(path: string, method = "GET", body?: unknown, key?: string) {
  const response = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json", ...(key ? { "Idempotency-Key": key } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
  });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error?.message || "The request failed");
  return result;
}

export function LiveBuilder({
  initial,
  settings,
}: {
  initial: RuntimeState;
  settings: LLMSettings;
}) {
  const [state, setState] = useState(initial);
  const [keySettings, setKeySettings] = useState(settings);
  const hydrated = useHydrated();
  const [error, setError] = useState("");
  const [prompt, setPrompt] = useState("");
  const configured = keySettings.providers.filter((p) =>
    keySettings.credentials.some((c) => c.provider === p.id && c.configured),
  );
  const [provider, setProvider] = useState(configured[0]?.id || "openai");
  const [model, setModel] = useState("");
  const [models, setModels] = useState<string[]>([]);
  const [sending, setSending] = useState(false);
  const [loadingModels, setLoadingModels] = useState(false);
  const [preview, setPreview] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [mode, setMode] = useState(initial.project.access_mode);
  const [password, setPassword] = useState("");
  const [confirmPublic, setConfirmPublic] = useState(false);
  const [sharing, setSharing] = useState(false);
  const [sharingMessage, setSharingMessage] = useState("");
  const request = useRef<{ payload: string; key: string } | null>(null);
  const end = useRef<HTMLDivElement>(null);
  const slug = state.project.slug;
  const base = `/api/projects/${slug}`;
  const active = state.builds.find((b) => !["succeeded", "failed"].includes(b.status));
  const writable = state.project.role !== "read";
  const ready = state.project.status === "ready";

  useEffect(() => {
    let stopped = false;
    const load = async () => {
      try {
        const next: LLMSettings = await call("/api/settings/llm-providers");
        if (stopped) return;
        setKeySettings(next);
        if (!next.credentials.some((c) => c.provider === provider && c.configured)) {
          const first = next.providers.find((p) =>
            next.credentials.some((c) => c.provider === p.id && c.configured),
          );
          setProvider(first?.id || "openai");
          setModel("");
          setModels([]);
        }
      } catch {
        /* Runtime polling reports connectivity failures separately. */
      }
    };
    void load();
    const timer = setInterval(load, 10000);
    window.addEventListener("focus", load);
    return () => {
      stopped = true;
      clearInterval(timer);
      window.removeEventListener("focus", load);
    };
  }, [provider]);
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const next = await call(`${base}/runtime`);
        if (!stopped) setState(next);
      } catch {
        if (!stopped) setError("Connection interrupted. Progress is saved; reconnecting…");
      } finally {
        if (!stopped) timer = setTimeout(poll, 2000);
      }
    };
    timer = setTimeout(poll, 1000);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [base]);
  useEffect(() => {
    if (!ready) return;
    let stopped = false;
    call(`${base}/preview`, "POST")
      .then((r) => {
        if (!stopped) setPreview(r.url);
      })
      .catch((e) => {
        if (!stopped) setError(e.message);
      });
    const timer = setTimeout(() => setRefresh((r) => r + 1), 20 * 60 * 1000);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [base, ready, state.current_deployment_id, refresh]);
  useEffect(() => {
    end.current?.scrollIntoView({ block: "nearest" });
  }, [state.messages.length]);
  async function send(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setSending(true);
    const payload = JSON.stringify({ provider, model, prompt });
    if (request.current?.payload !== payload)
      request.current = { payload, key: crypto.randomUUID() };
    try {
      await call(`${base}/builds`, "POST", { provider, model, prompt }, request.current.key);
      setPrompt("");
      request.current = null;
      setState(await call(`${base}/runtime`));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not submit build");
    } finally {
      setSending(false);
    }
  }
  async function loadModels() {
    setLoadingModels(true);
    setError("");
    try {
      const result = await call(`/api/settings/llm-providers/${provider}/models`);
      const list = (result.models as string[]).filter(
        (id) => provider !== "openai" || /^(gpt-|chatgpt-|o[1-9])/.test(id),
      );
      setModels(list);
      if (list.length) setModel(list[0]);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not load models");
    } finally {
      setLoadingModels(false);
    }
  }
  async function saveAccess(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    setSharing(true);
    setError("");
    setSharingMessage("");
    try {
      await call(`${base}/access`, "PUT", { mode, password });
      setPassword("");
      setConfirmPublic(false);
      setSharingMessage("Access settings saved. Existing access sessions were revoked.");
      setState(await call(`${base}/runtime`));
      setRefresh((r) => r + 1);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not save access");
    } finally {
      setSharing(false);
    }
  }
  async function rollback(id: string) {
    if (
      !window.confirm("Restore this application version? Database records will not be rolled back.")
    )
      return;
    try {
      await call(`${base}/rollback`, "POST", { deployment_id: id }, crypto.randomUUID());
      setState(await call(`${base}/runtime`));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Rollback failed");
    }
  }
  async function retry() {
    try {
      await call(`${base}/retry`, "POST");
      setState(await call(`${base}/runtime`));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Retry failed");
    }
  }
  return (
    <main className="builder">
      <header className="builder-heading">
        <div>
          <Link href="/" className="back-link">
            ← All projects
          </Link>
          <h1>{state.project.name}</h1>
          <span className="department-label">{state.project.department_name}</span>
        </div>
        <div className="provider-actions">
          <span className={`status-badge ${ready ? "ready" : "pending"}`}>
            <span />
            {ready
              ? "Container running"
              : state.project.status === "failed"
                ? "Provisioning failed"
                : "Starting container…"}
          </span>
          <Link className="button secondary" href={`/${slug}`} target="_blank">
            Open app
            <Icon name="external" size={14} />
          </Link>
          {ready && (
            <a className="button secondary" href={`${base}/source`} download>
              Download source
            </a>
          )}
        </div>
      </header>
      {error && (
        <p role="alert" className="form-error">
          {error}
          <button className="icon-button" aria-label="Dismiss error" onClick={() => setError("")}>
            ×
          </button>
        </p>
      )}
      <div className="builder-panels">
        <section className="chat-panel" aria-label="Build conversation">
          <div className="panel-title">
            <Icon name="message" size={16} />
            <strong>Build together</strong>
            <span>LIVE</span>
          </div>
          <div className="conversation-log" aria-live="polite">
            {!state.messages.length && (
              <p className="runtime-note">
                Starting an isolated container and persistent project databases. You do not need to
                send a prompt first.
              </p>
            )}
            {state.messages.map((m) => (
              <div key={m.id} className={`conversation-message message-${m.role}`}>
                <strong>
                  {m.role === "user" ? "You" : m.role === "assistant" ? "SG-1" : "Build status"}
                </strong>
                <p>{m.content}</p>
              </div>
            ))}
            <div ref={end} />
          </div>
          {active && (
            <div className="build-progress" role="status">
              <Icon name="clock" size={16} />
              <span>
                <strong>{active.status}</strong>
                {active.message}
              </span>
            </div>
          )}
          <form className="live-composer" onSubmit={send}>
            <div className="model-controls">
              <label>
                Provider
                <select
                  aria-label="AI provider"
                  value={provider}
                  onChange={(e) => {
                    setProvider(e.target.value as typeof provider);
                    setModels([]);
                    setModel("");
                  }}
                  disabled={!!active || !configured.length}
                >
                  {configured.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                  {!configured.length && <option value="openai">No saved API keys</option>}
                </select>
              </label>
              <button
                type="button"
                className="button secondary"
                onClick={loadModels}
                disabled={loadingModels || !configured.length}
              >
                {loadingModels ? "Loading…" : "Load models"}
              </button>
            </div>
            <label>
              Model ID
              <input
                aria-label="Model ID"
                list="provider-models"
                required
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder="Load models or enter a model ID"
                disabled={!!active}
              />
              <datalist id="provider-models">
                {models.map((m) => (
                  <option key={m} value={m} />
                ))}
              </datalist>
            </label>
            <textarea
              aria-label="Build request"
              required
              maxLength={12000}
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              disabled={!ready || !writable || !!active || !hydrated}
              placeholder="Describe the tool you want to build…"
              rows={4}
            />
            <div className="composer-actions">
              <Link href="/settings/providers">Manage API keys</Link>
              <button
                className="button primary"
                disabled={!ready || !writable || !!active || sending || !configured.length}
              >
                {sending ? "Submitting…" : "Build application"}
                <Icon name="arrow" size={16} />
              </button>
            </div>
          </form>
          <p className="composer-caption">
            Source, recent chat, and build failures are sent to your selected provider. Do not
            include secrets or sensitive production data.
          </p>
        </section>
        <section className="preview-panel" aria-label="Application preview">
          <div className="preview-toolbar">
            <span>
              <span className="preview-dot" />
              Live preview
            </span>
            <code>/{slug}</code>
            <button
              className="icon-button"
              aria-label="Refresh preview"
              onClick={() => setRefresh((r) => r + 1)}
            >
              ↻
            </button>
          </div>
          {preview && ready ? (
            <iframe
              className="application-frame"
              title="Application preview"
              src={preview}
              sandbox="allow-scripts allow-forms allow-same-origin allow-downloads"
              referrerPolicy="no-referrer"
            />
          ) : (
            <div className="preview-empty">
              <Icon name="layers" size={40} />
              <h2>
                {state.project.status === "failed"
                  ? "Container setup needs attention."
                  : "Starting your application…"}
              </h2>
              <p>
                {state.last_error ||
                  "The worker is building a starter image, creating isolated PostgreSQL resources, and checking container health."}
              </p>
              {state.project.status === "failed" && state.project.role === "admin" && (
                <button className="button primary" onClick={retry}>
                  Retry provisioning
                </button>
              )}
            </div>
          )}
        </section>
      </div>
      <ProjectFiles slug={slug} writable={writable} />
      {state.project.role === "admin" && (
        <details className="runtime-settings">
          <summary>Sharing &amp; passwords</summary>
          <form onSubmit={saveAccess}>
            <label>
              Application access
              <select
                aria-label="Application access"
                value={mode}
                onChange={(e) => setMode(e.target.value as typeof mode)}
              >
                <option value="password">Password protected</option>
                <option value="public">Public — anyone on the internet</option>
              </select>
            </label>
            {mode === "password" && (
              <label>
                Application password
                <input
                  aria-label="New application password"
                  type="password"
                  autoComplete="new-password"
                  minLength={12}
                  maxLength={72}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={
                    state.password_set
                      ? "Leave blank to keep the existing password"
                      : "Set a password to allow visitors"
                  }
                  required={!state.password_set}
                />
              </label>
            )}
            {mode === "public" && state.project.access_mode !== "public" && (
              <label className="public-confirm">
                <input
                  type="checkbox"
                  required
                  checked={confirmPublic}
                  onChange={(e) => setConfirmPublic(e.target.checked)}
                />
                I understand this exposes the application to anyone who can reach its URL.
              </label>
            )}
            <button className="button primary" disabled={sharing}>
              {sharing ? "Saving…" : "Save access settings"}
            </button>
            <p role="status">{sharingMessage}</p>
            <p className="runtime-note">
              Employee preview stays protected. New apps deny visitors until a password is set or
              public access is explicitly enabled.
            </p>
          </form>
        </details>
      )}
      <details className="runtime-settings">
        <summary>Deployment history ({state.deployments.length})</summary>
        {state.deployments.map((d) => (
          <div className="deployment-row" key={d.id}>
            <span>
              <strong>{d.summary}</strong>
              <small>{d.created_at.slice(0, 19).replace("T", " ")} UTC</small>
            </span>
            {d.id === state.current_deployment_id ? (
              <span className="status-badge ready">Current</span>
            ) : (
              writable && (
                <button
                  className="button secondary"
                  onClick={() => rollback(d.id)}
                  disabled={!!active}
                >
                  Restore version
                </button>
              )
            )}
          </div>
        ))}
      </details>
      <p className="runtime-note">
        Local Docker development runtime. AWS hosting, employee SSO, and GitHub source
        synchronization remain unfinished. PostgreSQL data persists independently of application
        containers.
      </p>
    </main>
  );
}
