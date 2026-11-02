import { Icon } from "@/components/icons";
import { NavigationLink } from "@/components/navigation";
import { Caption } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation } from "@/hooks/use-api";
import type { User } from "@/lib/types";
import { searchHref } from "@/lib/url-state";
import { cn } from "@/lib/utils";
import { useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Brand } from "./content";

const navigation = [
  { to: "/library/songs", label: "Library", icon: "library" },
  { to: "/search", label: "Search", icon: "search" },
  { to: "/imports", label: "Imports", icon: "imports" },
  { to: "/requests", label: "Requests", icon: "requests" },
  { to: "/activity", label: "Activity", icon: "activity" },
  { to: "/settings/connections", label: "Settings", icon: "settings" },
];

export function Sidebar({
  user,
  collapsed,
  onCollapsedChange,
  onSignOut,
  onError,
}: {
  user: User;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  onSignOut: () => void;
  onError: (error: string) => void;
}) {
  const { api } = useApiMutation();
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const searchInput = useRef<HTMLInputElement>(null);
  const labelClassName = collapsed ? "hidden max-[700px]:inline" : undefined;
  return (
    <aside className="sticky top-0 flex h-screen min-w-0 flex-col gap-3 border-r border-solid border-sidebar-border bg-card px-2.5 py-3 max-[700px]:static max-[700px]:h-auto max-[700px]:gap-2 max-[700px]:px-3.5 max-[700px]:py-2.5">
      <div
        className={cn(
          "flex min-h-8 items-center justify-between",
          collapsed && "justify-center max-[700px]:justify-between",
        )}
      >
        <Brand className={cn(collapsed && "hidden max-[700px]:flex")}>
          <span aria-hidden="true">◉</span>
          <span>ZenSync</span>
        </Brand>
        <Button
          variant="ghost"
          className="flex size-8 shrink-0 items-center justify-center border-0 p-0 max-[700px]:hidden"
          title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          aria-expanded={!collapsed}
          onClick={() => onCollapsedChange(!collapsed)}
        >
          <Icon name={collapsed ? "expand" : "collapse"} />
        </Button>
      </div>
      {collapsed ? (
        <Button
          variant="ghost"
          className="flex h-[34px] w-[38px] items-center justify-center border-0 p-0 max-[700px]:hidden"
          aria-label="Open sidebar search"
          title="Search music"
          onClick={() => {
            onCollapsedChange(false);
            requestAnimationFrame(() => searchInput.current?.focus());
          }}
        >
          <Icon name="search" />
        </Button>
      ) : (
        <form
          className="flex h-[34px] min-w-0 items-center gap-2 rounded-[5px] border border-solid border-border px-2 py-0 text-muted-foreground focus-within:outline-2 focus-within:outline-solid focus-within:outline-primary"
          role="search"
          onSubmit={(event) => {
            event.preventDefault();
            if (query.trim()) {
              navigate(searchHref(query.trim()));
              setQuery("");
            }
          }}
        >
          <Icon name="search" />
          <Input
            className="h-7 min-h-0 max-[700px]:min-h-0 w-full min-w-0 border-0 bg-transparent p-0 text-[13px] focus-visible:outline-none"
            ref={searchInput}
            aria-label="Search music"
            placeholder="Search music…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            maxLength={500}
          />
        </form>
      )}
      <nav
        className="flex flex-col gap-0.5 max-[700px]:flex-row max-[700px]:gap-1 max-[700px]:overflow-auto"
        aria-label="Main navigation"
      >
        {navigation.map(({ to, label, icon }) => (
          <NavigationLink
            key={to}
            to={to}
            title={label}
            aria-label={label}
            className={cn(
              collapsed &&
                "min-[701px]:justify-center min-[701px]:px-0 min-[701px]:py-2",
            )}
          >
            <span
              data-slot="nav-icon"
              className="flex size-5 shrink-0 items-center justify-center"
            >
              <Icon name={icon} />
            </span>
            <span className={labelClassName}>{label}</span>
          </NavigationLink>
        ))}
      </nav>
      <div className="mt-auto border-t border-solid border-sidebar-border pt-3 max-[700px]:flex max-[700px]:items-center max-[700px]:pt-1.5">
        <strong
          className={cn("px-2", collapsed && "hidden max-[700px]:inline")}
        >
          {user.username}
        </strong>
        <Caption
          className={cn("mb-2 px-2 max-[700px]:hidden", collapsed && "hidden")}
        >
          {user.role}
        </Caption>
        <Button
          variant="ghost"
          className={cn(
            "flex min-h-9 w-full items-center gap-2.5 border-0 px-2 py-1.5 text-[13px] max-[700px]:ml-auto max-[700px]:w-auto",
            collapsed && "justify-center px-0",
          )}
          aria-label="Sign out"
          title="Sign out"
          onClick={async () => {
            try {
              await api("/session", "DELETE");
              onSignOut();
            } catch (error) {
              onError(
                error instanceof Error ? error.message : "Sign out failed",
              );
            }
          }}
        >
          <Icon name="logout" />
          <span className={labelClassName}>Sign out</span>
        </Button>
      </div>
    </aside>
  );
}
