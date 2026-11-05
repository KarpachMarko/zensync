import { Icon } from "@/components/icons";
import { AppLink } from "@/components/navigation";
import { SearchBox } from "@/components/search-box";
import { Caption } from "@/components/typography";
import { Button } from "@/components/ui/button";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import { useApiMutation } from "@/hooks/use-api";
import type { User } from "@/lib/types";
import { searchHref } from "@/lib/url-state";
import { cn } from "@/lib/utils";
import { useRef } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Brand } from "./content";

const navigation = [
  { to: "/library/songs", label: "Library", icon: "library" },
  { to: "/metadata", label: "Metadata", icon: "settings" },
  { to: "/search", label: "Search", icon: "search" },
  { to: "/imports", label: "Imports", icon: "imports" },
  { to: "/requests", label: "Requests", icon: "requests" },
  { to: "/activity", label: "Activity", icon: "activity" },
  { to: "/settings/connections", label: "Settings", icon: "settings" },
];

export function WorkspaceSidebar({
  user,
  onSignOut,
  onError,
}: {
  user: User;
  onSignOut: () => void;
  onError: (error: string) => void;
}) {
  const { api } = useApiMutation();
  const navigate = useNavigate();
  const location = useLocation();
  const { state, isMobile, setOpen, setOpenMobile } = useSidebar();
  const collapsed = state === "collapsed" && !isMobile;
  const searchInput = useRef<HTMLInputElement>(null);
  return (
    <Sidebar
      collapsible="icon"
      className="border-sidebar-border"
      role="complementary"
    >
      <SidebarHeader className="gap-3 px-2.5 pt-3 pb-0">
        <div
          className={cn(
            "flex min-h-8 items-center justify-between",
            collapsed && "justify-center",
          )}
        >
          {!collapsed && <Brand />}
          <SidebarTrigger
            className="flex size-8 min-h-8 shrink-0 items-center justify-center border-0 p-0"
            title={collapsed ? "Expand sidebar" : undefined}
            aria-label={
              isMobile
                ? "Close navigation"
                : collapsed
                  ? "Expand sidebar"
                  : "Collapse sidebar"
            }
            aria-expanded={!collapsed}
          />
        </div>
        {collapsed ? (
          <Button
            variant="ghost"
            className="flex h-[34px] w-[38px] items-center justify-center border-0 p-0"
            aria-label="Open sidebar search"
            title="Search music"
            onClick={() => {
              setOpen(true);
              requestAnimationFrame(() => searchInput.current?.focus());
            }}
          >
            <Icon name="search" />
          </Button>
        ) : (
          <SearchBox
            compact
            clearOnSubmit
            inputRef={searchInput}
            label="Search music"
            placeholder="Search music…"
            onSearch={(query) => {
              navigate(searchHref(query));
              setOpenMobile(false);
            }}
          />
        )}
      </SidebarHeader>
      <SidebarContent className="px-2.5 pt-3">
        <SidebarGroup className="p-0">
          <SidebarGroupContent>
            <nav aria-label="Main navigation">
              <SidebarMenu>
                {navigation
                  .filter(
                    (item) => item.to !== "/metadata" || user.role === "admin",
                  )
                  .map(({ to, label, icon }) => (
                    <SidebarMenuItem key={to}>
                      <SidebarMenuButton
                        asChild
                        tooltip={label}
                        isActive={location.pathname.startsWith(
                          "/" + to.split("/")[1],
                        )}
                        className="h-auto min-h-9 gap-2.5 px-2 py-1.5 font-normal text-muted-foreground no-underline max-[700px]:min-h-11 data-active:text-foreground group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:p-0"
                      >
                        <AppLink
                          to={to}
                          aria-label={label}
                          onClick={() => setOpenMobile(false)}
                        >
                          <span
                            data-slot="nav-icon"
                            className="flex size-5 shrink-0 items-center justify-center"
                          >
                            <Icon name={icon} />
                          </span>
                          <span className={cn(collapsed && "hidden")}>
                            {label}
                          </span>
                        </AppLink>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ))}
              </SidebarMenu>
            </nav>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarSeparator />
      <SidebarFooter className="gap-0 px-2.5 py-3">
        {!collapsed && (
          <>
            <strong className="px-2">{user.username}</strong>
            <Caption className="mb-2 px-2">{user.role}</Caption>
          </>
        )}
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              tooltip="Sign out"
              className="h-auto min-h-9 gap-2.5 border-0 bg-transparent px-2 py-1.5 text-[13px] text-muted-foreground cursor-pointer"
              aria-label="Sign out"
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
              <span className="flex size-5 shrink-0 items-center justify-center">
                <Icon name="logout" />
              </span>
              {!collapsed && <span>Sign out</span>}
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}
