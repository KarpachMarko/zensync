import { StatusPill } from "@/components/status";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import type { User } from "@/lib/types";
import { useState, type ReactNode } from "react";
import { WorkspaceSidebar } from "./sidebar";

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
    <SidebarProvider
      open={!collapsed}
      onOpenChange={(open) => changeCollapsed(!open)}
    >
      <WorkspaceSidebar user={user} onSignOut={onSignOut} onError={onError} />
      <SidebarInset className="block w-full min-w-0 max-w-[1440px] px-11 pt-7 pb-[70px] max-[700px]:px-[18px] max-[700px]:py-6">
        <header className="mb-8 flex items-center justify-between text-[13px] text-muted-foreground max-[700px]:mb-[25px]">
          <div className="flex items-center gap-2">
            <SidebarTrigger
              className="border-0 p-1 md:hidden"
              aria-label="Open navigation"
            />
            <span>Shared music workspace</span>
          </div>
          <StatusPill>Music migration</StatusPill>
        </header>
        {children}
      </SidebarInset>
    </SidebarProvider>
  );
}
