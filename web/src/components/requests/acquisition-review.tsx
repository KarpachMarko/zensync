import { AppLink } from "@/components/navigation";
import { PreviewPlayer } from "@/components/imports/preview-player";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Text } from "@/components/typography";
import { useApiQuery } from "@/hooks/use-api";
import { useDeezerPreview } from "@/hooks/use-deezer-preview";
import { api, time, type Track } from "@/lib/api";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

type Candidate = { quality: string; checksum: string; expected: number; measured: number };
type AudioReview = { title: string; artist: string; stage: string; reviewURL: string; canAccept: boolean; candidates: Candidate[] };

export function AcquisitionReviewButton({ taskId, requestId, label = "Review recording", onAccepted }: { taskId?: string; requestId?: string; label?: string; onAccepted?: () => void }) {
  const [open, setOpen] = useState(false);
  if (!taskId && !requestId) return null;
  const path = taskId ? `/tasks/${encodeURIComponent(taskId)}/review-audio` : `/requests/${encodeURIComponent(requestId || "")}/review-audio`;
  return <>
    <Button variant="outline" onClick={() => setOpen(true)}>{label}</Button>
    {open && <AcquisitionReview path={path} onClose={() => setOpen(false)} onAccepted={onAccepted} />}
  </>;
}

function AcquisitionReview({ path, onClose, onAccepted }: { path: string; onClose: () => void; onAccepted?: () => void }) {
  const result = useApiQuery<AudioReview>(path);
  const mutation = useMutation({
    mutationFn: (candidate: Candidate) => api(path, "POST", { quality: candidate.quality, checksum: candidate.checksum }),
    onError: (error) => toast.error(error.message),
  });
  const client = useQueryClient();
  const [confirm, setConfirm] = useState<Candidate | null>(null);
  const preview = useDeezerPreview({ maxDuration: Infinity, errorMessage: "Retained audio could not be played. Close and reopen review to refresh it.", resolveURL: (track) => {
    if (!track.preview) return null;
    const url = new URL(track.preview, window.location.origin);
    return url.origin === window.location.origin && url.pathname === "/api/v2" + path + "/file" ? url.href : null;
  } });
  const review = result.data;
  async function accept(candidate: Candidate) {
    await mutation.mutateAsync(candidate);
    for (const area of ["/activity", "/requests", "/imports"]) {
      void client.invalidateQueries({ predicate: (query) => query.queryKey[0] === "api" && typeof query.queryKey[1] === "string" && query.queryKey[1].startsWith(area) });
    }
    toast.success("Duration difference accepted for this file. Task queued to reuse the retained audio.");
    onClose();
    onAccepted?.();
  }
  function play(candidate: Candidate) {
    if (!review) return;
    const track: Track = { id: ["lossless", "320", "128"].indexOf(candidate.quality), title: review.title, title_version: "", link: "", artist: { name: review.artist }, album: { title: "Downloaded recording" }, duration: candidate.measured,
      preview: `/api/v2${path}/file?quality=${encodeURIComponent(candidate.quality)}&checksum=${encodeURIComponent(candidate.checksum)}` };
    preview.toggle(track);
  }
  return <Dialog open onOpenChange={(open) => { if (!open && !mutation.isPending) onClose(); }}>
    <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-3xl">
      <DialogTitle>Review failed recording</DialogTitle>
      <DialogDescription>{review ? `${review.artist} — ${review.title}` : "Checking retained audio…"}</DialogDescription>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {review && <>
        <Text className="m-0 break-words text-xs">{review.stage}</Text>
        <Button asChild variant="outline"><AppLink to={review.reviewURL} onClick={onClose}>Review match / choose another recording</AppLink></Button>
        {review.candidates.length === 0 ? <Text className="m-0 text-sm">No validated file with an approvable duration difference is retained. Choose another recording or upload your version from import review. A country restriction requires another accessible recording or correcting the configured Deezer account; it cannot be overridden here.</Text> : <>
          <Text className="m-0 text-sm">Listen before accepting. A shorter file may be a different edit or incomplete. Acceptance applies only to this exact file and recording; identity, quality and decoding checks still apply.</Text>
          {review.candidates.map((candidate) => <div key={candidate.quality} className="flex flex-wrap items-center gap-3 rounded-md border p-3 text-sm">
            <span className="flex-1"><strong>{candidate.quality}</strong> · Catalog {time(candidate.expected)} → downloaded {time(candidate.measured)}</span>
            <Button variant="outline" onClick={() => play(candidate)}>Listen to downloaded file</Button>
            <Button variant="outline" disabled={!review.canAccept || mutation.isPending} onClick={() => setConfirm(candidate)}>Accept duration difference</Button>
          </div>)}
          <PreviewPlayer preview={preview} caption="Downloaded file" />
          {preview.error && <Text role="alert">{preview.error}</Text>}
          {confirm && <div className="rounded-md border border-primary p-3 text-sm">
            <p className="m-0 mb-3">Accept the {time(confirm.measured)} {confirm.quality} file instead of the catalog's {time(confirm.expected)} recording?</p>
            <div className="flex flex-wrap gap-2">
              <Button disabled={mutation.isPending} onClick={() => { void accept(confirm).catch(() => undefined); }}>{mutation.isPending ? "Accepting…" : "Accept this file and retry"}</Button>
              <Button variant="outline" disabled={mutation.isPending} onClick={() => setConfirm(null)}>Keep reviewing</Button>
            </div>
          </div>}
        </>}
      </>}
    </DialogContent>
  </Dialog>;
}
