import { PreviewPlayer } from "@/components/imports/preview-player";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Text } from "@/components/typography";
import { useApiQuery } from "@/hooks/use-api";
import { useDeezerPreview } from "@/hooks/use-deezer-preview";
import { api, time, type Track } from "@/lib/api";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

type Candidate = { quality: string; checksum: string; expected: number; measured: number };
type AudioReview = { title: string; artist: string; stage: string; canAccept: boolean; candidates: Candidate[] };

export function RetainedAudioReview({ path, onAccepted, disabled = false, onBusyChange, defaultOpen = false }: { path: string; onAccepted: () => void; disabled?: boolean; onBusyChange?: (busy: boolean) => void; defaultOpen?: boolean }) {
  const result = useApiQuery<AudioReview>(path);
  const mutation = useMutation({
    onMutate: () => onBusyChange?.(true),
    onSettled: () => onBusyChange?.(false),
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
    onAccepted();
  }
  function play(candidate: Candidate) {
    if (!review) return;
    const track: Track = { id: ["lossless", "320", "128"].indexOf(candidate.quality), title: review.title, title_version: "", link: "", artist: { name: review.artist }, album: { title: "Downloaded recording" }, duration: candidate.measured,
      preview: `/api/v2${path}/file?quality=${encodeURIComponent(candidate.quality)}&checksum=${encodeURIComponent(candidate.checksum)}` };
    preview.toggle(track);
  }
  if (result.error) return <Text role="alert" className="m-0 text-xs">{result.error.message}</Text>;
  if (!review) return <Text role="status" className="m-0 text-xs">Checking download details…</Text>;
  return <div className="space-y-2 text-xs">
    <Collapsible>
      <CollapsibleTrigger asChild><Button variant="ghost" size="sm" className="h-7 px-2 text-xs">Download failure details</Button></CollapsibleTrigger>
      <CollapsibleContent><Text className="m-0 max-h-28 overflow-y-auto break-words rounded-md bg-muted/40 px-3 py-2 text-xs">{review.stage}</Text></CollapsibleContent>
    </Collapsible>
    {defaultOpen && review.candidates.length === 0 && <Text className="m-0 text-xs">No retained file is available for duration review. Search for another recording or upload your version below.</Text>}
    {review.candidates.length > 0 && <Collapsible defaultOpen={defaultOpen}>
      <CollapsibleTrigger asChild><Button variant="outline" size="sm">Review downloaded files ({review.candidates.length})</Button></CollapsibleTrigger>
      <CollapsibleContent className="space-y-2 pt-2">
        <Text className="m-0 text-xs">Compare the catalog and downloaded lengths. Listen before accepting a shorter file; it may be another edit or incomplete.</Text>
          {review.candidates.map((candidate) => <div key={candidate.quality} className="flex flex-wrap items-center gap-3 rounded-md border p-3 text-sm">
            <span className="flex-1"><strong>{candidate.quality}</strong> · Catalog {time(candidate.expected)} → downloaded {time(candidate.measured)}</span>
            <Button variant="outline" onClick={() => play(candidate)}>Listen to downloaded file</Button>
            <Button variant="outline" disabled={disabled || !review.canAccept || mutation.isPending} onClick={() => setConfirm(candidate)}>Accept duration difference</Button>
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
      </CollapsibleContent>
    </Collapsible>}
  </div>;
}
