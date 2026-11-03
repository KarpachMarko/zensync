import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import type { ComponentProps } from "react";

export function FieldLabel({ className, ...props }: ComponentProps<"label">) {
  return (
    <Label
      className={cn(
        "mt-0 mb-[18px] flex flex-col items-stretch gap-[7px] text-sm leading-normal font-normal",
        className,
      )}
      {...props}
    />
  );
}

export { Checkbox } from "@/components/ui/checkbox";

export function CheckboxField({
  className,
  ...props
}: ComponentProps<typeof FieldLabel>) {
  return (
    <FieldLabel
      className={cn("mt-[18px] flex-row items-center", className)}
      {...props}
    />
  );
}

export function SearchField({
  className,
  ...props
}: ComponentProps<typeof FieldLabel>) {
  return (
    <FieldLabel
      className={cn("m-0 min-w-0 flex-1 max-[700px]:basis-full", className)}
      {...props}
    />
  );
}
