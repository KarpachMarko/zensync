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
