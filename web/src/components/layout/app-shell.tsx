import { Icon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation } from "@/hooks/use-api";
import type { User } from "@/lib/types";
import { searchHref } from "@/lib/url-state";
import React, { useRef, useState } from "react";
import { NavLink, useNavigate } from "react-router-dom";
export function AppShell({
  user,
  onSignOut,
  onError,
  children,
}: {
  user: User;
  onSignOut: () => void;
  onError: (error: string) => void;
  children: React.ReactNode;
}) {
  const { api } = useApiMutation();
  const [collapsed, setCollapsed] = useState(
    localStorage.getItem("sidebar") === "collapsed",
  );
  const navigate = useNavigate();
  const [sidebarQuery, setSidebarQuery] = useState("");
  const searchInput = useRef<HTMLInputElement>(null);
  const nav = [
    ["/library/songs", "Library"],
    ["/search", "Search"],
    ["/imports", "Imports"],
    ["/requests", "Requests"],
    ["/activity", "Activity"],
    ["/settings/connections", "Settings"],
  ];
  return (
    <div className={"shell " + (collapsed ? "collapsed" : "")}>
      <aside>
        <div className="sidebar-top">
          <div className="brand">
            <span aria-hidden="true">◉</span>
            <span className="navtext">ZenSync</span>
          </div>
          <Button
            className="quiet sidebar-collapse"
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            aria-expanded={!collapsed}
            onClick={() => {
              setCollapsed(!collapsed);
              localStorage.setItem(
                "sidebar",
                collapsed ? "expanded" : "collapsed",
              );
            }}
          >
            <Icon name={collapsed ? "expand" : "collapse"} />
          </Button>
        </div>
        {collapsed ? (
          <Button
            className="sidebar-search-toggle quiet"
            aria-label="Open sidebar search"
            title="Search music"
            onClick={() => {
              setCollapsed(false);
              localStorage.setItem("sidebar", "expanded");
              requestAnimationFrame(() => searchInput.current?.focus());
            }}
          >
            <Icon name="search" />
          </Button>
        ) : (
          <form
            className="sidebar-search"
            role="search"
            onSubmit={(e) => {
              e.preventDefault();
              if (sidebarQuery.trim()) {
                navigate(searchHref(sidebarQuery.trim()));
                setSidebarQuery("");
              }
            }}
          >
            <Icon name="search" />
            <Input
              ref={searchInput}
              aria-label="Search music"
              placeholder="Search music…"
              value={sidebarQuery}
              onChange={(e) => setSidebarQuery(e.target.value)}
              maxLength={500}
            />
          </form>
        )}
        <nav aria-label="Main navigation">
          {nav.map(([to, label], i) => (
            <NavLink to={to} key={to} title={label} aria-label={label}>
              <span className="symbol">
                <Icon
                  name={
                    [
                      "library",
                      "search",
                      "imports",
                      "requests",
                      "activity",
                      "settings",
                    ][i]
                  }
                />
              </span>
              <span className="navtext">{label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="account">
          <strong>{user.username}</strong>
          <small>{user.role}</small>
          <Button
            className="quiet signout"
            aria-label="Sign out"
            title="Sign out"
            onClick={async () => {
              try {
                await api("/session", "DELETE");
                onSignOut();
              } catch (e) {
                onError((e as Error).message);
              }
            }}
          >
            <Icon name="logout" />
            <span className="navtext">Sign out</span>
          </Button>
        </div>
      </aside>
      <main>
        <header>
          <span>Shared music workspace</span>
          <span className="pill">Music migration</span>
        </header>
        {children}
      </main>
    </div>
  );
}
