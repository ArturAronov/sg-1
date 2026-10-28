import Link from "next/link";
import { notFound } from "next/navigation";
import { Icon } from "@/components/icon";
import { APIError, apiGet } from "@/lib/api";
import type { Project } from "@/lib/types";

export default async function BuilderPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  let project: Project;
  try {
    ({ project } = await apiGet<{ project: Project }>(`/v1/projects/${encodeURIComponent(slug)}`));
  } catch (error) {
    if (error instanceof APIError && error.status === 404) notFound();
    throw error;
  }

  return (
    <main className="builder">
      <header className="builder-heading">
        <div>
          <Link href="/" className="back-link">
            ← All projects
          </Link>
          <h1>{project.name}</h1>
          <span className="department-label">{project.department_name}</span>
        </div>
        <span className="status-badge pending">
          <span />
          Awaiting provisioner
        </span>
      </header>
      <div className="builder-panels">
        <section className="chat-panel" aria-label="Build conversation">
          <div className="panel-title">
            <Icon name="message" size={16} />
            <strong>Build together</strong>
            <span>CHAT</span>
          </div>
          <div className="chat-messages">
            <span className="assistant-avatar">
              <Icon name="spark" size={19} />
            </span>
            <div>
              <strong>Your workspace is reserved.</strong>
              <p>{project.description || "A new home for your team's next useful tool."}</p>
              <p>
                Project details, access settings, and the provisioning request are saved in
                PostgreSQL. Container provisioning and the AI provider still need to be connected
                before building can begin.
              </p>
              <div className="saved-details">
                <span>
                  <Icon name="lock" size={14} />
                  Password protection enabled
                </span>
                <span>
                  <Icon name="clock" size={14} />
                  Provisioning request queued
                </span>
              </div>
            </div>
          </div>
          <div className="chat-composer">
            <textarea
              aria-label="Build request"
              disabled
              placeholder="AI chat is not connected yet."
              rows={3}
            />
            <div>
              <span>Your idea starts with a conversation.</span>
              <span className="disabled-send">
                <Icon name="arrow" size={16} />
              </span>
            </div>
          </div>
          <p className="composer-caption">No messages are sent to an AI provider in this build.</p>
        </section>
        <section className="preview-panel" aria-label="Application preview">
          <div className="preview-toolbar">
            <span>
              <span className="preview-dot" />
              Preview
            </span>
            <code>/{project.slug}</code>
            <span className="preview-locked">
              <Icon name="lock" size={14} />
            </span>
          </div>
          <div className="preview-empty">
            <div className="preview-art">
              <div className="mock-window">
                <span />
                <span />
                <span />
                <div />
                <div />
                <div />
              </div>
              <span className="art-spark">✳</span>
            </div>
            <span className="tiny-label">A PLACE FOR WHAT COMES NEXT</span>
            <h2>Your application will live here.</h2>
            <p>
              No container is running for this project yet. Once provisioning is connected, a live
              preview will replace this empty state.
            </p>
            <span className="protected-note">
              <Icon name="shield" size={15} />
              Preview access stays employee-only.
            </span>
          </div>
        </section>
      </div>
    </main>
  );
}
