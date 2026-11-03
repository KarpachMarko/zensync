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

export function Checkbox({
  className,
  ...props
}: Omit<ComponentProps<"input">, "type">) {
  return (
    <input
      type="checkbox"
      className={cn(
        "min-w-0 max-w-full rounded-[5px] border border-solid border-[#526079] bg-background px-2.5 py-2 text-foreground accent-primary [font:inherit] focus-visible:outline-3 focus-visible:outline-solid focus-visible:outline-primary focus-visible:outline-offset-3 max-[700px]:min-h-11",
        className,
      )}
      {...props}
    />
  );
}

export function NativeSelect({
  className,
  ...props
}: ComponentProps<"select">) {
  return (
    <select
      className={cn(
        "w-80 min-w-0 max-w-full rounded-[5px] border border-solid border-[#526079] bg-background px-2.5 py-2 text-foreground [font:inherit] focus-visible:outline-3 focus-visible:outline-solid focus-visible:outline-primary focus-visible:outline-offset-3 max-[700px]:min-h-11",
        className,
      )}
      {...props}
    />
  );
}

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
