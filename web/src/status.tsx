import { useState, type ReactNode } from "react";

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
}: {
  state: string;
  title: string;
}) {
  if (
    !state ||
    finished(state) ||
    blocked(state) ||
    ["queued", "awaiting_approval", "approved"].includes(state)
  )
    return null;
  return (
    <progress
      className="active-progress"
      aria-label={title + ": " + state.replaceAll("_", " ")}
    />
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
  const [open, setOpen] = useState(title !== "Completed");
  return (
    <details
      className="status-section"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>
        {title}
        <span className="count">{count}</span>
      </summary>
      {count ? (
        children
      ) : (
        <p className="section-empty">No {title.toLowerCase()} items.</p>
      )}
    </details>
  );
}
