import { type ReactNode } from "react";
import { ExpandableSection } from "./expandable-section";

import type { TransferProgress } from "@/lib/api";

export const finished = (state: string) =>
  ["available", "completed", "cancelled", "declined", "withdrawn"].includes(
    state,
  );
export const blocked = (state: string) =>
  ["blocked", "failed", "deferred"].includes(state);

export function StatusBadge({ state }: { state: string }) {
  return (
    <span className={"pill " + state.toLowerCase().replaceAll(" ", "_")}>
      {state.replaceAll("_", " ")}
    </span>
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
    <div className="transfer-progress">
      <div
        className={"active-progress" + (known ? "" : " indeterminate")}
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
          className="active-progress-fill"
          style={known ? { width: `${percent}%` } : undefined}
        />
      </div>
      {current && (
        <small className="transfer-stats">
          {known && `${Math.floor(percent)}% · `}
          {bytes(current.downloadedBytes)}
          {current.totalBytes > 0 && ` / ${bytes(current.totalBytes)}`}
          {current.bytesPerSecond !== undefined &&
            ` · ${bytes(current.bytesPerSecond)}/s`}
        </small>
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
        <p className="section-empty">No {title.toLowerCase()} items.</p>
      )}
    </ExpandableSection>
  );
}
