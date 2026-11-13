import { RowActions, type RowAction } from "@/components/row-actions";
import { ActiveProgress, StatusBadge } from "@/components/status";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/typography";
import type { ActivityItem } from "@/lib/types";
import { useRef, useState } from "react";
import { AcquisitionReviewDialog } from "./acquisition-review";

export function ActivityRow({ item, busy, onRetry, onDismiss, onCancel, onRestore }: {
  item: ActivityItem; busy: boolean; onRetry: () => void; onDismiss: () => void; onCancel: () => void; onRestore?: () => void;
}) {
  const [details, setDetails] = useState(false);
  const [review, setReview] = useState<"recording" | "downloaded" | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const actions: RowAction[] = [];
  if (item.canReview) {
    actions.push({ label: "Review recording", opensDialog: true, disabled: busy, onSelect: () => setReview("recording") });
    if (item.stage.includes("Downloaded duration differs")) actions.push({ label: "Review downloaded files", description: "Listen and accept duration differences", opensDialog: true, disabled: busy, onSelect: () => setReview("downloaded") });
  }
  if (["failed", "blocked"].includes(item.state)) actions.push({ label: "Retry", disabled: busy, onSelect: onRetry });
  if (item.canDismiss) actions.push({ label: "Dismiss failure", description: "Stops retries; does not accept audio", disabled: busy, onSelect: onDismiss });
  if (item.canCancel) actions.push({ label: "Cancel", disabled: busy, onSelect: onCancel });
  actions.push({ label: details ? "Hide details" : "Show details", onSelect: () => setDetails(!details) });
  return <article data-slot="activity-row" className="w-full min-w-0 border-b border-solid border-sidebar-border py-2">
    <div className="flex w-full min-w-0 items-center gap-3">
      <div className="grid min-w-0 flex-1 gap-x-4 gap-y-0.5 sm:grid-cols-[minmax(8rem,1fr)_minmax(0,2fr)] sm:items-baseline">
        <strong className="truncate text-sm" title={item.title}>{item.title}</strong>
        <span className="truncate text-xs text-muted-foreground" title={item.stage}>{item.stage}</span>
      </div>
      {item.canRestore && onRestore && <Button size="sm" variant="outline" disabled={busy} onClick={onRestore}>Restore file</Button>}
      <StatusBadge state={item.state} className="shrink-0" />
      <RowActions title={item.title} actions={actions} triggerRef={trigger} />
    </div>
    <ActiveProgress compact state={item.state} title={item.title} transfer={item.transfer} />
    {details && <div role="region" aria-label={`Details for ${item.title}`} className="pt-2"><Text className="m-0 max-w-none break-words text-xs">{item.stage}</Text></div>}
    {review && <AcquisitionReviewDialog taskId={item.id} returnFocus={trigger.current} showDownloadedFiles={review === "downloaded"} onClose={() => setReview(null)} />}
  </article>;
}
