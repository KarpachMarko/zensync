import { cn } from "@/lib/utils";
import type { ComponentProps } from "react";
import { NavLink } from "react-router-dom";

export function AppLink({
  className,
  ...props
}: ComponentProps<typeof NavLink>) {
  return (
    <NavLink
      className={(state) =>
        cn(
          "text-inherit focus-visible:outline-3 focus-visible:outline-solid focus-visible:outline-primary focus-visible:outline-offset-3",
          typeof className === "function" ? className(state) : className,
        )
      }
      {...props}
    />
  );
}

export function NavigationLink({
  className,
  ...props
}: ComponentProps<typeof NavLink>) {
  return (
    <AppLink
      className={(state) =>
        cn(
          "flex min-h-9 items-center gap-2.5 rounded-[5px] px-2 py-1.5 text-sm text-muted-foreground no-underline max-[700px]:min-h-11 max-[700px]:gap-1.5 max-[700px]:p-2 max-[700px]:whitespace-nowrap",
          state.isActive && "bg-sidebar-accent text-foreground",
          typeof className === "function" ? className(state) : className,
        )
      }
      {...props}
    />
  );
}

export function TabNav({ className, ...props }: ComponentProps<"nav">) {
  return (
    <nav
      className={cn(
        "my-[25px] mx-0 flex flex-row flex-wrap gap-0.5 border-b border-solid border-border pb-2.5",
        className,
      )}
      {...props}
    />
  );
}
