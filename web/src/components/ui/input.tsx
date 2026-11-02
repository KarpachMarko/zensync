import { cn } from "cn";
import * as React from "react";

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "h-auto w-full min-w-0 max-w-full rounded-[5px] border border-solid border-[#526079] bg-background px-2.5 py-2 text-foreground [font:inherit] transition-colors file:inline-flex file:border-0 file:bg-transparent file:text-foreground placeholder:text-muted-foreground focus-visible:outline-3 focus-visible:outline-solid focus-visible:outline-primary focus-visible:outline-offset-3 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive max-[700px]:min-h-11",
        type === "file" && "max-w-[700px]",
        className,
      )}
      {...props}
    />
  );
}

export { Input };
