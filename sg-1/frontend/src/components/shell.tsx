import Link from "next/link";
import { Brand } from "@/components/brand";
import { Icon } from "@/components/icon";
import { WorkspaceBreadcrumb, WorkspaceNavigation } from "@/components/workspace-navigation";
import type { User } from "@/lib/types";

export function Shell({ user, children }: { user: User; children: React.ReactNode }) {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Brand />
        <div className="workspace-label">YOUR WORKSPACE</div>
        <WorkspaceNavigation />
        <div className="sidebar-bottom">
          <div className="local-indicator">
            <span />
            {process.env.SG1_ENV === "production" ? "Production workspace" : "Local development"}
          </div>
          <div className="user-row">
            <span className="avatar">{user.name.charAt(0).toUpperCase()}</span>
            <div>
              <strong>{user.name}</strong>
              <small>{user.platform_admin ? "Platform administrator" : "Team member"}</small>
            </div>
            <form action="/api/logout" method="post">
              <button className="icon-button" aria-label="Sign out" title="Sign out">
                <Icon name="logout" size={16} />
              </button>
            </form>
          </div>
        </div>
      </aside>
      <div className="main-column">
        <a className="skip-link" href="#workspace-content">
          Skip to content
        </a>
        <header className="topbar">
          <WorkspaceBreadcrumb />
          <Link
            href="/settings/providers"
            className="mobile-provider-link"
            aria-label="AI providers"
          >
            <Icon name="key" size={17} />
          </Link>
          <span className="environment-badge">
            <span />
            {process.env.SG1_ENV === "production" ? "PRODUCTION" : "LOCAL DEVELOPMENT"}
          </span>
        </header>
        <div id="workspace-content" tabIndex={-1}>
          {children}
        </div>
      </div>
    </div>
  );
}
