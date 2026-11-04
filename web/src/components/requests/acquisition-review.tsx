import { Review } from "@/components/imports/review";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Text } from "@/components/typography";
import { api } from "@/lib/api";
import type { Migration, Source } from "@/lib/migration";
import { useMutation } from "@tanstack/react-query";
import { useRef, useState } from "react";

type ReviewContext = { migration: Migration; source: Source };

export function AcquisitionReviewButton({ taskId, requestId, label = "Review recording", onAccepted }: { taskId?: string; requestId?: string; label?: string; onAccepted?: () => void }) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const prefix = taskId ? `/tasks/${encodeURIComponent(taskId)}` : `/requests/${encodeURIComponent(requestId || "")}`;
  const context = useMutation({ mutationFn: () => api<ReviewContext>(prefix + "/recording-review", "POST") });
  if (!taskId && !requestId) return null;
  function close() { setOpen(false); }
  return <>
    <Button ref={trigger} variant="outline" disabled={context.isPending} onClick={() => { setOpen(true); context.mutate(); }}>{label}</Button>
    {open && (context.data && context.isSuccess ? <Review
      source={context.data.source} migration={context.data.migration} failurePath={prefix + "/review-audio"}
      requestReplacement returnFocus={trigger.current} onClose={() => { close(); onAccepted?.(); }}
    /> : <Dialog open onOpenChange={(value) => { if (!value) close(); }}>
      <DialogContent className="flex flex-col gap-2 p-4 sm:max-w-lg">
        <DialogTitle className="m-0 pr-8 text-base">Review recording</DialogTitle>
        <DialogDescription className="m-0 text-xs">Loading recording options…</DialogDescription>
        {context.error && <><Text role="alert" className="m-0 text-xs">{context.error.message}</Text><Button variant="outline" onClick={() => context.mutate()}>Try again</Button></>}
      </DialogContent>
    </Dialog>)}
  </>;
}
