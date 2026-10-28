"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Icon } from "@/components/icon";

export function WorkspaceNavigation() {
  const settings = usePathname().startsWith("/settings");
  return (
    <nav aria-label="Main navigation">
      <Link
        className={`nav-link ${!settings ? "active" : ""}`}
        href="/"
        aria-current={!settings ? "page" : undefined}
      >
        <Icon name="grid" />
        Projects{!settings && <span className="nav-dot" />}
      </Link>
      <Link
        className={`nav-link ${settings ? "active" : ""}`}
        href="/settings/providers"
        aria-current={settings ? "page" : undefined}
      >
        <Icon name="key" />
        AI providers{settings && <span className="nav-dot" />}
      </Link>
    </nav>
  );
}

export function WorkspaceBreadcrumb() {
  const settings = usePathname().startsWith("/settings");
  return (
    <span className="breadcrumb">
      Workspace <span>/</span> <strong>{settings ? "AI providers" : "Projects"}</strong>
    </span>
  );
}
