import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import type { User } from "@/lib/types";
import { useState, type ReactNode } from "react";
import { WorkspaceSidebar } from "./sidebar";
import { Brand } from "./content";

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
      <SidebarInset className="block w-full min-w-0 max-w-[1440px] px-11 pt-7 pb-[70px] max-lg:px-4 max-lg:pt-4 max-lg:pb-8">
        <div className="mb-4 flex items-center gap-3 lg:hidden">
          <SidebarTrigger
            className="size-11 border-0 p-1"
            aria-label="Open navigation"
          />
          <Brand />
        </div>
        {children}
      </SidebarInset>
    </SidebarProvider>
  );
}
