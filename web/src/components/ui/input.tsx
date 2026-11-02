// Adapted from shadcn/ui (MIT), using the existing ZenSync form styles.
import { cn } from "@/lib/utils";
import * as React from "react";
export const Input = React.forwardRef<
  HTMLInputElement,
  React.ComponentProps<"input">
>(({ className, type, ...props }, ref) => (
  <input type={type} className={cn("input", className)} ref={ref} {...props} />
));
Input.displayName = "Input";
