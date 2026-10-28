"use client";

import { useState } from "react";
import { Icon } from "@/components/icon";
import { useHydrated } from "@/lib/hydration";
import type { LLMCredentialStatus, LLMProvider, LLMSettings } from "@/lib/types";

export function ProviderSettings({ settings }: { settings: LLMSettings }) {
  return (
    <main className="dashboard provider-settings">
      <div className="page-heading">
        <div>
          <div className="eyebrow">
            <span className="small-line" />
            YOUR ACCOUNT / AI PROVIDERS
          </div>
          <h1>
            Your keys.
            <br />
            <span>Your choice of AI.</span>
          </h1>
          <p>
            Add your own provider API keys. These credentials belong to your account, not your
            department or project.
          </p>
        </div>
        <span className="settings-shield">
          <Icon name="shield" size={32} />
        </span>
      </div>
      <div className="credential-notice">
        <Icon name="lock" size={20} />
        <div>
          <strong>Private to your account. Encrypted at rest.</strong>
          <p>
            Saved keys cannot be viewed again. You can replace or remove them at any time. Keys are
            never added to project code, chat, or browser storage.
          </p>
        </div>
      </div>
      <div className="provider-grid">
        {settings.providers.map((provider) => (
          <ProviderCard
            key={provider.id}
            provider={provider}
            initial={
              settings.credentials.find((c) => c.provider === provider.id) || {
                provider: provider.id,
                configured: false,
                updated_at: null,
              }
            }
          />
        ))}
      </div>
      <div className="foundation-note">
        <Icon name="code" size={18} />
        <p>
          <strong>Your account pays for model usage.</strong> Saving a key makes no provider call.
          Load models and submit builds from a project to use it. Local sign-ins share one
          development account; use restricted test keys until employee SSO is connected.
        </p>
      </div>
    </main>
  );
}

function ProviderCard({
  provider,
  initial,
}: {
  provider: LLMProvider;
  initial: LLMCredentialStatus;
}) {
  const hydrated = useHydrated();
  const [status, setStatus] = useState(initial);
  const [key, setKey] = useState("");
  const [editing, setEditing] = useState(!initial.configured);
  const [busy, setBusy] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  async function save(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch(`/api/settings/llm-providers/${provider.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ api_key: key.trim() }),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error?.message || "Could not save the key");
      setKey("");
      setStatus(result.credential);
      setEditing(false);
      setConfirmRemove(false);
      setMessage("Key saved securely. Not verified with the provider.");
    } catch (error) {
      setError(error instanceof Error ? error.message : "Could not save the key");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch(`/api/settings/llm-providers/${provider.id}`, {
        method: "DELETE",
      });
      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error?.message || "Could not remove the key");
      }
      setKey("");
      setStatus({ provider: provider.id, configured: false, updated_at: null });
      setEditing(true);
      setConfirmRemove(false);
      setMessage("Key removed from your account.");
    } catch (error) {
      setError(error instanceof Error ? error.message : "Could not remove the key");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="provider-card" aria-labelledby={`provider-${provider.id}`}>
      <div className="provider-card-heading">
        <span className={`provider-mark provider-${provider.id}`}>
          {provider.name === "OpenAI" ? "O" : provider.name.charAt(0)}
        </span>
        <span className={`status-badge ${status.configured ? "ready" : "pending"}`}>
          <span />
          {status.configured ? "Key stored" : "Not configured"}
        </span>
      </div>
      <h2 id={`provider-${provider.id}`}>{provider.name}</h2>
      <p className="provider-description">{provider.description}</p>
      {editing ? (
        <form onSubmit={save}>
          <label htmlFor={`key-${provider.id}`}>{provider.name} API key</label>
          <input
            className="standalone-input"
            id={`key-${provider.id}`}
            type="password"
            autoComplete="new-password"
            spellCheck={false}
            autoCapitalize="none"
            required
            minLength={16}
            maxLength={4096}
            value={key}
            disabled={busy || !hydrated}
            onChange={(e) => {
              setKey(e.target.value);
              setMessage("");
            }}
            placeholder={status.configured ? "Enter a replacement key" : "Paste your API key"}
            aria-describedby={`key-help-${provider.id}`}
          />
          <small className="input-help" id={`key-help-${provider.id}`}>
            Use an API key, not your account password or chat subscription.
          </small>
          <div className="provider-actions">
            <button className="button primary" disabled={busy || key.trim().length < 16}>
              {busy ? "Saving…" : `Save ${provider.name} key`}
            </button>
            {status.configured && (
              <button
                className="button secondary"
                type="button"
                disabled={busy}
                onClick={() => {
                  setEditing(false);
                  setKey("");
                  setError("");
                }}
              >
                Cancel
              </button>
            )}
          </div>
        </form>
      ) : (
        <div className="stored-key">
          <span>
            <Icon name="lock" size={16} />
            Stored securely · cannot be displayed
          </span>
          {status.updated_at && <small>Last updated {status.updated_at.slice(0, 10)}</small>}
          <div className="provider-actions">
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => {
                setEditing(true);
                setConfirmRemove(false);
                setMessage("");
              }}
            >
              Replace {provider.name} key
            </button>
            <button
              className="button danger-text"
              disabled={busy}
              onClick={() => {
                setConfirmRemove(true);
                setMessage("");
              }}
            >
              Remove {provider.name} key
            </button>
          </div>
        </div>
      )}
      {confirmRemove && (
        <div className="remove-confirmation">
          <p>Remove your {provider.name} key? Future builds will not be able to use it.</p>
          <div className="provider-actions">
            <button className="button danger" disabled={busy} onClick={remove}>
              Confirm removal
            </button>
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => setConfirmRemove(false)}
            >
              Keep key
            </button>
          </div>
        </div>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <p className="credential-message" role="status" aria-live="polite">
        {message}
      </p>
    </section>
  );
}
