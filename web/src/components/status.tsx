import { Caption, Text } from "@/components/typography";
import { cn } from "@/lib/utils";
import type { ComponentProps } from "react";
import { type CSSProperties, type ReactNode } from "react";
import { ExpandableSection } from "./expandable-section";

import type { TransferProgress } from "@/lib/api";

export const finished = (state: string) =>
  ["available", "completed", "cancelled", "declined", "withdrawn"].includes(
    state,
  );
export const blocked = (state: string) =>
  ["blocked", "failed", "deferred"].includes(state);

const stateColors: Record<string, string> = {
  awaiting_approval: "bg-[#423c30] text-[#e9cb86]",
  approved: "bg-[#233f3d] text-[#a2d5bd]",
  ...Object.fromEntries(
    ["completed", "available", "connected", "published"].map((state) => [
      state,
      "bg-[#293f3b] text-[#b3d3c4]",
    ]),
  ),
  ...Object.fromEntries(
    ["downloading", "running", "importing", "planned"].map((state) => [
      state,
      "bg-[#2d3e57] text-[#bbcde8]",
    ]),
  ),
  ...Object.fromEntries(
    ["checking", "validating", "awaiting_scan", "scanning"].map((state) => [
      state,
      "bg-[#3b3650] text-[#d2c4e5]",
    ]),
  ),
  ...Object.fromEntries(
    ["queued", "deferred", "staged"].map((state) => [
      state,
      "bg-[#3d3b32] text-[#d6cda9]",
    ]),
  ),
  ...Object.fromEntries(
    ["failed", "blocked", "declined"].map((state) => [
      state,
      "bg-[#49353d] text-[#e1b7c0]",
    ]),
  ),
  ...Object.fromEntries(
    ["cancelled", "withdrawn"].map((state) => [
      state,
      "bg-[#343c49] text-[#b9c0cd]",
    ]),
  ),
};

export function StatusPill({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "rounded-[20px] bg-[#29384f] px-2.5 py-1 text-xs whitespace-nowrap text-[#c5cde0] max-[700px]:whitespace-normal",
        className,
      )}
      {...props}
    />
  );
}

export function StatusBadge({
  state,
  className,
}: {
  state: string;
  className?: string;
}) {
  return (
    <StatusPill
      className={cn(
        stateColors[state.toLowerCase().replaceAll(" ", "_")],
        className,
      )}
    >
      {state.replaceAll("_", " ")}
    </StatusPill>
  );
}

export function ActiveProgress({
  state,
  title,
  transfer,
}: {
  state: string;
  title: string;
  transfer?: TransferProgress;
}) {
  if (
    !state ||
    finished(state) ||
    blocked(state) ||
    ["queued", "awaiting_approval", "approved"].includes(state)
  )
    return null;
  const current = state === "downloading" ? transfer : undefined;
  const percent = current?.percent;
  const known =
    percent !== undefined &&
    Number.isFinite(percent) &&
    percent >= 0 &&
    percent <= 100;
  function bytes(value: number) {
    if (value >= 1024 * 1024)
      return `${(value / (1024 * 1024)).toFixed(1)} MiB`;
    if (value >= 1024) return `${(value / 1024).toFixed(1)} KiB`;
    return `${Math.round(value)} B`;
  }
  return (
    <div className="mt-1.5 mb-3.5">
      <div
        className="relative block h-1.5 w-full overflow-hidden rounded-[3px] bg-[#29384f]"
        role="progressbar"
        aria-label={title + ": " + state.replaceAll("_", " ")}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={known ? percent : undefined}
        aria-valuetext={
          known
            ? `${percent.toFixed(1)}% downloaded`
            : "Running; progress unknown"
        }
      >
        <span
          data-slot="transfer-fill"
          className={cn(
            "block h-full rounded-[inherit] bg-[#b5c9e3] transition-[width] duration-250 ease-[ease]",
            known
              ? "w-[var(--transfer-percent)]"
              : "w-[38%] animate-transfer motion-reduce:animate-none motion-reduce:translate-x-[80%]",
          )}
          style={
            known
              ? ({ "--transfer-percent": `${percent}%` } as CSSProperties)
              : undefined
          }
        />
      </div>
      {current && (
        <Caption className="mt-[5px] tabular-nums">
          {known && `${Math.floor(percent)}% · `}
          {bytes(current.downloadedBytes)}
          {current.totalBytes > 0 && ` / ${bytes(current.totalBytes)}`}
          {current.bytesPerSecond !== undefined &&
            ` · ${bytes(current.bytesPerSecond)}/s`}
        </Caption>
      )}
    </div>
  );
}

export function StatusSection({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: ReactNode;
}) {
  return (
    <ExpandableSection
      title={title}
      count={count}
      defaultOpen={title !== "Completed"}
    >
      {count ? (
        children
      ) : (
        <Text className="mt-0 mb-[18px] text-sm">
          No {title.toLowerCase()} items.
        </Text>
      )}
    </ExpandableSection>
  );
}
