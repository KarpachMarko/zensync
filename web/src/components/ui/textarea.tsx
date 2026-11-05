import * as React from "react";
import { cn } from "cn";

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "min-h-24 w-full min-w-0 max-w-full rounded-[5px] border border-solid border-[#526079] bg-background px-2.5 py-2 text-foreground [font:inherit] transition-colors placeholder:text-muted-foreground focus-visible:outline-3 focus-visible:outline-solid focus-visible:outline-primary focus-visible:outline-offset-3 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive",
        className,
      )}
      {...props}
    />
  );
}

export { Textarea };
