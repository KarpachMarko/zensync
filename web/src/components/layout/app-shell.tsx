import { StatusPill } from "@/components/status";
import type { User } from "@/lib/types";
import { cn } from "@/lib/utils";
import { useState, type ReactNode } from "react";
import { Sidebar } from "./sidebar";

export function AppShell({
  user,
  onSignOut,
  onError,
  children,
}: {
  user: User;
  onSignOut: () => void;
  onError: (error: string) => void;
  children: ReactNode;
}) {
  const [collapsed, setCollapsed] = useState(
    localStorage.getItem("sidebar") === "collapsed",
  );
  function changeCollapsed(value: boolean) {
    setCollapsed(value);
    localStorage.setItem("sidebar", value ? "collapsed" : "expanded");
  }
  return (
    <div
      className={cn(
        "grid min-h-screen grid-cols-[208px_minmax(0,1fr)] max-[700px]:grid-cols-1",
        collapsed && "min-[701px]:grid-cols-[60px_minmax(0,1fr)]",
      )}
    >
      <Sidebar
        user={user}
        collapsed={collapsed}
        onCollapsedChange={changeCollapsed}
        onSignOut={onSignOut}
        onError={onError}
      />
      <main className="w-full min-w-0 max-w-[1440px] px-11 pt-7 pb-[70px] max-[700px]:px-[18px] max-[700px]:py-6">
        <header className="mb-8 flex items-center justify-between text-[13px] text-muted-foreground max-[700px]:mb-[25px]">
          <span>Shared music workspace</span>
          <StatusPill>Music migration</StatusPill>
        </header>
        {children}
      </main>
    </div>
  );
}
