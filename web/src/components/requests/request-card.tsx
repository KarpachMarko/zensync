import { RowActions, type RowAction } from "@/components/row-actions";
import { ActiveProgress, StatusBadge } from "@/components/status";
import { Text } from "@/components/typography";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import type { MusicRequest as Request, User } from "@/lib/types";
import { DeclineForm } from "./decline-form";
import { useRef, useState } from "react";
import { AcquisitionReviewDialog } from "./acquisition-review";

export function RequestCard({ request: r, user, busy, decide, onApproveDeletion }: {
  request: Request; user: User; busy: boolean;
  decide: (request: Request, decision: string, reason?: string) => Promise<void>;
  onApproveDeletion: (request: Request) => void;
}) {
  const [coverFailed, setCoverFailed] = useState(false);
  const [details, setDetails] = useState(false);
  const [tracks, setTracks] = useState(false);
  const [declining, setDeclining] = useState(false);
  const [review, setReview] = useState<"recording" | "downloaded" | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const actions: RowAction[] = [];
  if ((r.state === "failed" || r.state === "blocked") && r.kind === "track") {
    actions.push({ label: "Review recording", disabled: busy, opensDialog: true, onSelect: () => setReview("recording") });
    if (r.detail?.includes("Downloaded duration differs")) actions.push({ label: "Review downloaded files", description: "Listen and accept duration differences", disabled: busy, opensDialog: true, onSelect: () => setReview("downloaded") });
  }
  if (r.decision === "awaiting_approval") {
    if (user.role === "admin") {
      actions.push({ label: "Approve", disabled: busy, opensDialog: r.kind === "delete", onSelect: () => { if (r.kind === "delete") onApproveDeletion(r); else void decide(r, "approved"); } });
      actions.push({ label: "Decline", disabled: busy, opensDialog: true, onSelect: () => setDeclining(true) });
    }
    if (r.owner === user.id) actions.push({ label: "Withdraw", disabled: busy, onSelect: () => void decide(r, "withdrawn") });
  }
  if (r.kind === "album") actions.push({ label: tracks ? "Hide tracks" : `Show tracks (${r.trackCount ?? 0})`, onSelect: () => setTracks(!tracks) });
  actions.push({ label: details ? "Hide details" : "Show details", onSelect: () => setDetails(!details) });
  const suffix = r.kind === "delete" ? "Delete song" : r.kind === "album" ? `${r.trackCount ?? 0} tracks · ${r.items?.filter((item) => item.state === "available").length ?? 0} available` : r.kind === "upload" ? "Uploaded audio" : "";
  return <article data-slot="request" className="w-full min-w-0 border-b border-solid border-sidebar-border py-2">
    <div className="flex w-full min-w-0 items-center gap-3">
      {r.kind === "upload" && !coverFailed && <img src={`/api/v2/requests/${encodeURIComponent(r.id)}/cover`} alt="Uploaded album cover" loading="lazy" className="size-8 shrink-0 rounded object-cover" onError={() => setCoverFailed(true)} />}
      <div className="grid min-w-0 flex-1 gap-x-4 gap-y-0.5 sm:grid-cols-[minmax(8rem,1fr)_minmax(0,1fr)] sm:items-baseline">
        <strong className="truncate text-sm" title={r.title}>{r.title}{r.kind === "delete" ? " (Delete song)" : ""}</strong>
        <span className="truncate text-xs text-muted-foreground" title={[r.artist, suffix].filter(Boolean).join(" · ")}>{r.artist || "Artist not specified"}{suffix && ` · ${suffix}`}</span>
      </div>
      <StatusBadge state={r.state || r.decision} className="shrink-0" />
      <RowActions title={r.title} actions={actions} triggerRef={trigger} />
    </div>
    <ActiveProgress compact state={r.state} title={r.title} transfer={r.transfer} />
    {details && <div role="region" aria-label={`Details for ${r.title}`} className="space-y-1 pt-2 text-xs">
      <Text className="m-0 max-w-none break-words text-xs">{r.target}{r.upgrade ? " · Upgrade" : ""}</Text>
      {r.detail && <Text className="m-0 max-w-none break-words text-xs">{r.detail}</Text>}
      {r.reason && <Text className="m-0 max-w-none break-words text-xs">{r.reason}</Text>}
      {r.kind === "delete" && <Text className="m-0 text-xs">Removes this song for everyone in the shared library.</Text>}
    </div>}
    {tracks && <div role="region" aria-label={`Tracks in ${r.title}`} className="mt-2 border-t border-border pt-2">
      <Text className="m-0 mb-1 text-xs">One approval covers this saved recording list and quality preference.</Text>
      {r.items?.map((item, index) => <div key={index} className="py-1">
        <div className="flex min-w-0 items-center justify-between gap-3 text-xs"><span className="min-w-0 truncate" title={item.detail}>{index + 1}. {item.title} · {item.artist}</span><StatusBadge state={item.state} /></div>
        <ActiveProgress compact state={item.state} title={item.title} transfer={item.transfer} />
      </div>)}
    </div>}
    {review && <AcquisitionReviewDialog taskId={r.jobId} requestId={r.id} returnFocus={trigger.current} showDownloadedFiles={review === "downloaded"} onClose={() => setReview(null)} />}
    {declining && r.decision === "awaiting_approval" && <Dialog open onOpenChange={(open) => { if (!open && !busy) setDeclining(false); }}>
      <DialogContent className="flex flex-col gap-3 p-4 sm:max-w-lg" onCloseAutoFocus={(event) => { event.preventDefault(); trigger.current?.focus(); }}>
        <DialogTitle className="m-0 pr-8 text-base">Decline request</DialogTitle>
        <DialogDescription className="m-0 text-xs">{r.title}</DialogDescription>
        <DeclineForm title={r.title} busy={busy} onDecline={(reason) => decide(r, "declined", reason)} />
      </DialogContent>
    </Dialog>}
  </article>;
}
