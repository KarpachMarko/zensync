import { Review } from "@/components/imports/review";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Text } from "@/components/typography";
import { api } from "@/lib/api";
import type { Migration, Source } from "@/lib/migration";
import { useQuery } from "@tanstack/react-query";

type ReviewContext = { migration: Migration; source: Source };

export function AcquisitionReviewDialog({ taskId, requestId, onClose, returnFocus, showDownloadedFiles = false }: {
  taskId?: string; requestId?: string; onClose: () => void; returnFocus?: HTMLElement | null; showDownloadedFiles?: boolean;
}) {
  const prefix = taskId ? `/tasks/${encodeURIComponent(taskId)}` : `/requests/${encodeURIComponent(requestId || "")}`;
  const context = useQuery({ queryKey: ["recording-review", prefix], queryFn: () => api<ReviewContext>(prefix + "/recording-review", "POST"),
    staleTime: Infinity, gcTime: 0, refetchOnWindowFocus: false, retry: false });
  return context.data ? <Review
    source={context.data.source} migration={context.data.migration} failurePath={prefix + "/review-audio"}
    requestReplacement returnFocus={returnFocus} showDownloadedFiles={showDownloadedFiles} onClose={onClose}
  /> : <Dialog open onOpenChange={(value) => { if (!value) onClose(); }}>
    <DialogContent className="flex flex-col gap-2 p-4 sm:max-w-lg">
      <DialogTitle className="m-0 pr-8 text-base">Review recording</DialogTitle>
      <DialogDescription className="m-0 text-xs">Loading recording options…</DialogDescription>
      {context.error && <><Text role="alert" className="m-0 text-xs">{context.error.message}</Text><Button variant="outline" onClick={() => void context.refetch()}>Try again</Button></>}
    </DialogContent>
  </Dialog>;
}
