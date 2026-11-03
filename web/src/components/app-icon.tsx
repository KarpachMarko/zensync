import { cn } from "@/lib/utils";
import type { ComponentProps } from "react";

export function AppIcon({
  className,
  ...props
}: Omit<ComponentProps<"img">, "src">) {
  return (
    <img
      src="/icon.svg"
      alt=""
      width={32}
      height={32}
      className={cn("size-8 shrink-0", className)}
      {...props}
    />
  );
}
