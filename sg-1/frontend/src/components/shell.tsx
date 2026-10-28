import Link from "next/link";
import { Brand } from "@/components/brand";
import { Icon } from "@/components/icon";
import type { User } from "@/lib/types";

export function Shell({ user, children }: { user: User; children: React.ReactNode }) {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Brand />
        <div className="workspace-label">YOUR WORKSPACE</div>
        <nav aria-label="Main navigation">
          <Link className="nav-link active" href="/">
            <Icon name="grid" />
            Projects
            <span className="nav-dot" />
          </Link>
        </nav>
        <div className="sidebar-note">
          <span className="tiny-label">FROM IDEA TO INTERNAL TOOL</span>
          <p>
            Less busywork.
            <br />
            More possibility.
          </p>
          <span className="orbit">✳</span>
        </div>
        <div className="sidebar-bottom">
          <div className="local-indicator">
            <span />
            Local development
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
        <header className="topbar">
          <span className="breadcrumb">
            Workspace <span>/</span> <strong>Projects</strong>
          </span>
          <span className="environment-badge">
            <span />
            FOUNDATION BUILD
          </span>
        </header>
        {children}
      </div>
    </div>
  );
}
