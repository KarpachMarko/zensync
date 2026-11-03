import { cn } from "@/lib/utils";
import type { ComponentProps } from "react";

export function PageHeading({ className, ...props }: ComponentProps<"h1">) {
  return (
    <h1
      className={cn(
        "mt-0 mb-3 text-[32px] leading-[1.2] tracking-[-1px] focus:outline-none",
        className,
      )}
      {...props}
    />
  );
}

export function SectionHeading({ className, ...props }: ComponentProps<"h2">) {
  return <h2 className={cn("mt-[30px] text-xl", className)} {...props} />;
}

export function Subheading({ className, ...props }: ComponentProps<"h3">) {
  return <h3 className={cn("text-[17px]", className)} {...props} />;
}

export function Text({ className, role, ...props }: ComponentProps<"p">) {
  return (
    <p
      role={role}
      className={cn(
        "max-w-[75ch] text-muted-foreground",
        role === "alert" && "text-[#ffb5bb]",
        role === "status" && "text-[#a2d5bd]",
        className,
      )}
      {...props}
    />
  );
}

export function Caption({ className, ...props }: ComponentProps<"small">) {
  return (
    <small
      className={cn("block text-xs text-muted-foreground", className)}
      {...props}
    />
  );
}
