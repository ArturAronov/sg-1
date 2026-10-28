"use client";
import { useEffect, useState } from "react";

type FileRecord = { id: string; name: string; size: number };
export function ProjectFiles({ slug, writable }: { slug: string; writable: boolean }) {
  const [files, setFiles] = useState<FileRecord[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const base = `/api/projects/${slug}/files`;
  useEffect(() => {
    let stopped = false;
    fetch(base)
      .then(async (r) => {
        if (!r.ok) throw new Error("Private file storage is unavailable");
        return r.json();
      })
      .then((r) => {
        if (!stopped) setFiles(r.files);
      })
      .catch((e) => {
        if (!stopped) setError(e.message);
      });
    return () => {
      stopped = true;
    };
  }, [base]);
  async function upload(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    setBusy(true);
    setError("");
    try {
      const response = await fetch(base, { method: "POST", body: data });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error?.message || "Upload failed");
      setFiles((old) => [result.file, ...old]);
      form.reset();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Upload failed");
    } finally {
      setBusy(false);
    }
  }
  return (
    <details className="runtime-settings">
      <summary>Private project files ({files.length})</summary>
      <p className="runtime-note">
        Stored in private S3-compatible object storage. Files are available only to authorized
        builder users and are not automatically sent to the AI or exposed by public applications.
        Production uploads require malware scanning before use.
      </p>
      {writable && (
        <form onSubmit={upload}>
          <label>
            Upload a file (maximum 10 MiB)
            <input aria-label="Project file" name="file" type="file" required disabled={busy} />
          </label>
          <button className="button primary" disabled={busy}>
            {busy ? "Uploading…" : "Upload file"}
          </button>
        </form>
      )}
      {error && (
        <p role="alert" className="form-error">
          {error}
        </p>
      )}
      {files.map((file) => (
        <div className="deployment-row" key={file.id}>
          <span>
            <strong>{file.name}</strong>
            <small>{file.size.toLocaleString()} bytes</small>
          </span>
          <a className="button secondary" href={`${base}/${file.id}`} download>
            Download
          </a>
        </div>
      ))}
    </details>
  );
}
