import { cn } from "cn";
import { Progress as ProgressPrimitive } from "radix-ui";
import * as React from "react";

function Progress({
  className,
  value = null,
  max = 100,
  indicatorClassName,
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root> & {
  indicatorClassName?: string;
}) {
  const maximum = Number.isFinite(max) && max > 0 ? max : 100;
  const known =
    value !== null && Number.isFinite(value) && value >= 0 && value <= maximum;
  const percent = known ? (value / maximum) * 100 : null;
  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      value={known ? value : null}
      max={maximum}
      className={cn(
        "relative block h-1.5 w-full overflow-hidden rounded-[3px] bg-[#29384f]",
        className,
      )}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot="progress-indicator"
        className={cn(
          "block h-full rounded-[inherit] bg-[#b5c9e3] transition-transform duration-250 ease-[ease]",
          percent !== null
            ? "w-full -translate-x-[var(--progress-offset)]"
            : "w-[38%] animate-transfer motion-reduce:animate-none motion-reduce:translate-x-[80%]",
          indicatorClassName,
        )}
        style={
          percent !== null
            ? ({
                "--progress-offset": `${100 - percent}%`,
              } as React.CSSProperties)
            : undefined
        }
      />
    </ProgressPrimitive.Root>
  );
}

export { Progress };
