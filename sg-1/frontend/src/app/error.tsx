"use client";

export default function ErrorPage({ reset }: { reset: () => void }) {
  return (
    <main className="standalone-state">
      <span className="tiny-label">CONNECTION INTERRUPTED</span>
      <h1>The workspace is unavailable.</h1>
      <p>
        Check that PostgreSQL and the Go API are running, migrations are applied, and the
        development user has been seeded.
      </p>
      <button className="button primary" onClick={reset}>
        Try again
      </button>
    </main>
  );
}
