"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";
import { Icon } from "@/components/icon";
import { suggestSlug } from "@/lib/slug";
import type { Department, Project } from "@/lib/types";

export function ProjectDashboard({
  projects,
  departments,
}: {
  projects: Project[];
  departments: Department[];
}) {
  const [query, setQuery] = useState("");
  const [department, setDepartment] = useState("");
  const [creating, setCreating] = useState(false);
  const allowed = departments.filter((d) => d.role === "admin");
  const visible = projects.filter(
    (p) =>
      (!department || p.department_id === department) &&
      `${p.name} ${p.description} ${p.department_name}`.toLowerCase().includes(query.toLowerCase()),
  );
  const ready = projects.filter((p) => p.status === "ready").length;

  return (
    <main className="dashboard">
      <div className="page-heading">
        <div>
          <div className="eyebrow">
            <span className="small-line" />
            YOUR IDEAS, MADE USEFUL
          </div>
          <h1>
            Tools built around
            <br />
            <span>the way you work.</span>
          </h1>
          <p>Create a home for your next idea. Build it together, one conversation at a time.</p>
        </div>
        <button
          className="button primary"
          onClick={() => setCreating(true)}
          disabled={!allowed.length}
        >
          <Icon name="plus" />
          New project
        </button>
      </div>
      <div className="stats-row">
        <div>
          <span className="stat-icon">
            <Icon name="layers" />
          </span>
          <span>
            <strong>{projects.length}</strong>
            <small>Total projects</small>
          </span>
        </div>
        <div>
          <span className="stat-icon green">
            <Icon name="grid" />
          </span>
          <span>
            <strong>{ready}</strong>
            <small>Ready to use</small>
          </span>
        </div>
        <div>
          <span className="stat-icon amber">
            <Icon name="clock" />
          </span>
          <span>
            <strong>{projects.filter((p) => p.status === "provisioning").length}</strong>
            <small>Awaiting provisioning</small>
          </span>
        </div>
        <div className="stats-caption">
          <Icon name="shield" size={16} />
          <span>
            New projects are
            <br />
            <strong>protected by default.</strong>
          </span>
        </div>
      </div>
      <div className="section-heading">
        <div>
          <h2>
            Your projects <span>{projects.length.toString().padStart(2, "0")}</span>
          </h2>
          <p>A little less friction for every team.</p>
        </div>
        <div className="filters">
          <label className="search-field">
            <Icon name="search" size={16} />
            <input
              aria-label="Search projects"
              placeholder="Search projects…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <select
            aria-label="Filter department"
            value={department}
            onChange={(e) => setDepartment(e.target.value)}
          >
            <option value="">All departments</option>
            {departments.map((d) => (
              <option key={d.id} value={d.id}>
                {d.name}
              </option>
            ))}
          </select>
        </div>
      </div>
      {visible.length ? (
        <div className="project-grid">
          {visible.map((project, index) => (
            <Link key={project.id} href={`/build/${project.slug}`} className="project-card">
              <div className="card-top">
                <span className={`project-icon color-${index % 3}`}>
                  <Icon name="code" size={22} />
                </span>
                <span
                  className={`status-badge ${project.status === "ready" ? "ready" : project.status === "failed" ? "failed" : "pending"}`}
                >
                  <span />
                  {statusLabel(project.status)}
                </span>
              </div>
              <span className="department-label">{project.department_name}</span>
              <h3>{project.name}</h3>
              <p>{project.description || "Your next useful tool starts here."}</p>
              <div className="card-bottom">
                <span>
                  <Icon name="lock" size={13} />
                  {project.access_mode === "password" ? "Password protected" : "Public"}
                </span>
                <span className="card-arrow">
                  <Icon name="arrow" size={17} />
                </span>
              </div>
            </Link>
          ))}
        </div>
      ) : (
        <div className="empty-state">
          <div className="empty-illustration">
            <span />
            <span />
            <Icon name="spark" size={30} />
          </div>
          <span className="tiny-label">A CLEAN SLATE. A LOT OF POTENTIAL.</span>
          <h3>{projects.length ? "No projects match your search." : "What could work better?"}</h3>
          <p>
            {projects.length
              ? "Try another name or department."
              : "The spreadsheet that takes too long. The process with too many steps. Give your next tool a place to start."}
          </p>
          {!projects.length && (
            <button
              className="button primary"
              onClick={() => setCreating(true)}
              disabled={!allowed.length}
            >
              Create your first project
              <Icon name="arrow" size={16} />
            </button>
          )}
        </div>
      )}
      <div className="foundation-note">
        <Icon name="code" size={18} />
        <p>
          <strong>The foundation is taking shape.</strong> Project records and permissions are
          persistent. Container provisioning, SSO, and AI builds are not connected yet.
        </p>
      </div>
      <footer className="page-footer">
        <span>BUILT FOR YOUR PEOPLE. BY YOUR PEOPLE.</span>
        <span>
          SG-1 <span className="footer-star">✳</span> Internal studio
        </span>
      </footer>
      {creating && <CreateProject departments={allowed} close={() => setCreating(false)} />}
    </main>
  );
}

function statusLabel(status: Project["status"]): string {
  return {
    provisioning: "Awaiting provisioner",
    ready: "Ready",
    failed: "Needs attention",
    archiving: "Archiving",
    archived: "Archived",
  }[status];
}

function CreateProject({ departments, close }: { departments: Department[]; close: () => void }) {
  const router = useRouter();
  const dialog = useRef<HTMLDivElement>(null);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const request = useRef<{ payload: string; key: string } | null>(null);

  async function submit(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const payload = JSON.stringify({
      department_id: data.get("department"),
      name: name.trim(),
      slug,
      description: data.get("description"),
    });
    if (request.current?.payload !== payload)
      request.current = { payload, key: crypto.randomUUID() };
    setBusy(true);
    setError("");
    try {
      const response = await fetch("/api/projects", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": request.current.key },
        body: payload,
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error?.message || "Could not create the project");
      router.push(`/build/${result.project.slug}`);
      router.refresh();
    } catch (error) {
      setError(
        error instanceof Error ? error.message : "Could not reach SG-1. You can safely retry.",
      );
      setBusy(false);
    }
  }

  return (
    <div
      className="modal-backdrop"
      onClick={(e) => {
        if (e.target === e.currentTarget && !busy) close();
      }}
    >
      <div
        ref={dialog}
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-title"
        onKeyDown={(e) => {
          if (e.key === "Escape" && !busy) close();
          if (e.key === "Tab") {
            const elements = dialog.current?.querySelectorAll<HTMLElement>(
              "button:not(:disabled), input, select, textarea",
            );
            if (!elements?.length) return;
            const first = elements[0],
              last = elements[elements.length - 1];
            if (e.shiftKey && document.activeElement === first) {
              e.preventDefault();
              last.focus();
            } else if (!e.shiftKey && document.activeElement === last) {
              e.preventDefault();
              first.focus();
            }
          }
        }}
      >
        <div className="modal-heading">
          <span className="project-icon color-0">
            <Icon name="spark" size={22} />
          </span>
          <button className="icon-button" onClick={close} disabled={busy} aria-label="Close dialog">
            <Icon name="close" />
          </button>
        </div>
        <h2 id="create-title">A new idea starts here.</h2>
        <p>Give your tool a name and a team to call home.</p>
        <form onSubmit={submit}>
          <label>
            Project name
            <input
              autoFocus
              required
              minLength={2}
              maxLength={100}
              placeholder="Maintenance tracker"
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                if (!slugEdited) setSlug(suggestSlug(e.target.value));
              }}
            />
          </label>
          <label htmlFor="project-slug">Project URL</label>
          <input
            className="standalone-input"
            id="project-slug"
            aria-describedby="slug-help"
            required
            minLength={2}
            maxLength={48}
            pattern="[a-z][a-z0-9]*(-[a-z0-9]+)*"
            value={slug}
            onChange={(e) => {
              setSlugEdited(true);
              setSlug(e.target.value);
            }}
            placeholder="maintenance-tracker"
          />
          <small className="input-help" id="slug-help">
            /build/{slug || "your-project"} · names are unique across all departments
          </small>
          <label>
            Department
            <select name="department" required>
              {departments.map((d) => (
                <option value={d.id} key={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Description <span className="optional">optional</span>
            <textarea
              name="description"
              maxLength={1000}
              rows={2}
              placeholder="What would this help your team do?"
            />
          </label>
          <div className="info-box">
            <Icon name="lock" size={16} />
            <span>
              Protected by default. This currently reserves the workspace and queues provisioning;
              it does not start a container yet.
            </span>
          </div>
          {error && (
            <p role="alert" className="form-error">
              {error}
            </p>
          )}
          <button className="button primary full-width" disabled={busy}>
            {busy ? "Creating workspace…" : "Create workspace"}
            <Icon name="arrow" size={16} />
          </button>
        </form>
      </div>
    </div>
  );
}
