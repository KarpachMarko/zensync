import { AudioUpload } from "@/components/imports/audio-upload";
import { RetainedAudioReview } from "@/components/requests/retained-audio-review";
import { MatchDetails } from "@/components/imports/match-details";
import { PreviewPlayer } from "@/components/imports/preview-player";
import { DeezerRecordingTools } from "@/components/imports/deezer-recording-tools";
import { useDeezerPreview } from "@/hooks/use-deezer-preview";
import { Actions, Row } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { SearchBox } from "@/components/search-box";
import { ActiveProgress, StatusBadge } from "@/components/status";
import { Caption, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { useImportAction } from "@/hooks/use-import-action";
import type { LibrarySong, Track } from "@/lib/api";
import { api as readAPI, label, time, trackTarget } from "@/lib/api";
import { songGuidance, status, type Migration, type Source, type RecordingMatch } from "@/lib/migration";
import { useQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { toast } from "sonner";

type ProviderCandidates = {
 id: string; name: string; tracks: Track[]; matches?: Record<string, RecordingMatch>; error?: string; ambiguous?: boolean;
};
type Candidates = {
 providers?: ProviderCandidates[];
  deezer: Track[];
  library: LibrarySong[];
  libraryError?: string;
  deezerError?: string;
  deezerMatches?: Record<string, RecordingMatch>;
  libraryMatches?: Record<string, RecordingMatch>;
  deezerAmbiguous?: boolean;
};

export function Review({
  source,
  migration: m,
  onClose,
  failurePath,
  requestReplacement = false,
  returnFocus,
  showDownloadedFiles = false,
}: {
  source: Source;
  migration: Migration;
  onClose: () => void;
  failurePath?: string;
  requestReplacement?: boolean;
  returnFocus?: HTMLElement | null;
  showDownloadedFiles?: boolean;
}) {
  const [updatedSource, setUpdatedSource] = useState<Source | null>(null);
  const s = updatedSource ?? source;
  const { act: save } = useImportAction(m.id);
  const preview = useDeezerPreview();
  const [query, setQuery] = useState(
    (s.metadata.artist + " " + s.metadata.title).trim().slice(0, 500),
  );
  const [searchRequested, setSearchRequested] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const trigger = useRef(returnFocus ?? document.activeElement);
  const prefix = "/imports/" + m.id + "/songs/" + s.id;
  const candidates = useQuery({
    queryKey: ["migration-candidates", m.id, s.id, query],
    queryFn: ({ signal }) => readAPI<Candidates>(prefix + "/candidates", "POST", { query }, undefined, signal),
    enabled: !s.serverId || searchRequested,
    staleTime: 60_000,
  });
  async function find(nextQuery: string) {
    setSearchRequested(true);
    if (nextQuery === query) await candidates.refetch();
    else setQuery(nextQuery);
  }
  async function act(action: string, body?: unknown) {
    setBusy(true);
    setError("");
    try {
      const response = await save("songs/" + s.id + "/" + action, body);
      if (response.source) setUpdatedSource(response.source);
      const replacing = requestReplacement && action === "resolve" && typeof body === "object" && body !== null && "target" in body;
      if (replacing) await save("songs/" + s.id + "/acquire");
      toast.success(
        action === "acquire" || replacing ? "Acquisition request saved." : "Song updated.",
      );
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to update song.");
    } finally {
      setBusy(false);
    }
  }
  const editable = m.state === "open";
  // Older saved candidates may predate preview metadata. Use the current
  // search result for their media URL when it is available.
  const providerResults: ProviderCandidates[] = candidates.data?.providers ?? (candidates.data ? [{id:"deezer",name:"Deezer",tracks:candidates.data.deezer,matches:candidates.data.deezerMatches,error:candidates.data.deezerError,ambiguous:candidates.data.deezerAmbiguous}] : []);
  const freshCandidate = providerResults.flatMap((provider) => provider.tracks).find((track) => s.candidate && trackTarget(track) === trackTarget(s.candidate));
  const selectedRecording = s.candidate ? {
    ...s.candidate,
    preview: freshCandidate?.preview || s.candidate.preview,
    album: { ...s.candidate.album, cover_medium: freshCandidate?.album.cover_medium || s.candidate.album.cover_medium },
  } : null;
  return (
    <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose(); }}>
      <DialogContent
        className="flex h-[min(840px,90dvh)] w-[calc(100vw-2rem)] max-w-[1100px] flex-col gap-0 overflow-hidden p-0 sm:max-w-[1100px] max-sm:h-dvh max-sm:w-screen max-sm:max-w-none max-sm:rounded-none"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (trigger.current instanceof HTMLElement && trigger.current.isConnected)
            trigger.current.focus();
          else document.querySelector<HTMLElement>("main h1")?.focus();
        }}
      >
        <header className="shrink-0 border-0 border-b border-solid border-border px-5 py-3 pr-14 max-sm:pl-3">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <DialogTitle className="m-0 text-lg leading-tight break-words">
              {s.metadata.title}
            </DialogTitle>
            <StatusBadge state={status(s)} />
          </div>
          <DialogDescription className="m-0 mt-1 text-xs leading-5 break-words">
            {s.metadata.artist || "Unknown artist"} / {s.metadata.album || "Unknown album"} / {time(s.metadata.duration)}
          </DialogDescription>
        </header>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-y-auto px-5 py-3 max-sm:px-3">
          {s.requestId && (s.jobState === "failed" || s.jobState === "blocked") && <RetainedAudioReview path={failurePath ?? `/requests/${encodeURIComponent(s.requestId)}/review-audio`} onAccepted={onClose} disabled={busy} onBusyChange={setBusy} defaultOpen={showDownloadedFiles} />}
          <SearchBox
            className="my-0 gap-2"
            label="Search or provider song URL"
            value={query}
            disabled={busy || candidates.isFetching}
            buttonLabel={candidates.isFetching ? "Searching…" : "Search"}
            onSearch={find}
          />
          {editable && (
            <AudioUpload
              source={s}
              prefix={prefix}
              busy={busy}
              onBusyChange={setBusy}
              onUploaded={async () => { onClose(); }}
            />
          )}
          {!requestReplacement && <Text className="m-0 max-w-none text-xs leading-5">
            {songGuidance(s)}
          </Text>}
          <ActiveProgress state={s.jobState} title={s.metadata.title} />
          {error && <Text role="alert" className="m-0 text-xs">{error}</Text>}
          {s.candidate && (
            <section className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-solid border-border px-3 py-2">
              <div className="min-w-0">
                <h2 className="m-0 mb-1 text-xs font-semibold text-muted-foreground">{requestReplacement && !updatedSource ? "Failed recording" : "Selected recording"}</h2>
                <strong className="text-sm">{label(s.candidate)}</strong>
                <Caption>
                  {s.candidate.artist.name} / {s.candidate.album.title} / {time(s.candidate.duration)}
                </Caption>
                {selectedRecording && (
                  <DeezerRecordingTools
                    track={selectedRecording}
                    playing={preview.playing && !!preview.active && trackTarget(preview.active) === trackTarget(selectedRecording)}
                    onPreview={preview.toggle}
                  />
                )}
              </div>
              {(!requestReplacement || updatedSource) && <Button
                disabled={busy || !editable || s.deferred || !!s.serverId || s.decision === "approved" || s.decision === "awaiting_approval"}
                onClick={() => void act("acquire")}
              >
                Request missing audio
              </Button>}
            </section>
          )}
          {s.requestId && !requestReplacement && (
            <Actions className="text-xs">
              <AppLink to="/requests">View request and approval</AppLink>
              <AppLink to="/activity">View acquisition progress</AppLink>
            </Actions>
          )}
          {s.favoriteState === "review" && editable && (
            <section className="rounded-md bg-accent px-3 py-2">
              <Text className="m-0 mb-2 text-xs">The earlier favorite write has an uncertain outcome. Keep the current state or retry.</Text>
              <Actions>
                <Button disabled={busy} onClick={() => void act("favorite-done")}>Keep current favorite state</Button>
                <Button variant="outline" disabled={busy} onClick={() => void act("favorite-retry")}>Retry favorite import once</Button>
              </Actions>
            </section>
          )}
          {candidates.isFetching && <Text role="status" className="m-0 text-xs">Searching Navidrome and music providers…</Text>}
          {candidates.error && (
            <div role="alert" className="flex flex-wrap items-center gap-3 text-xs text-destructive">
              <span>{candidates.error.message} Try another query or retry the search.</span>
              <Button variant="outline" disabled={candidates.isFetching} onClick={() => void candidates.refetch()}>Retry search</Button>
            </div>
          )}
          {candidates.data && (
            <div className="grid min-w-0 grid-cols-1 items-start gap-5 md:grid-cols-2" aria-busy={candidates.isFetching}>
              <section aria-labelledby="local-recordings-heading">
                <h2 id="local-recordings-heading" className="m-0 border-0 border-b border-solid border-border pb-2 text-sm font-semibold">
                  Already in Navidrome <span className="text-xs font-normal text-muted-foreground">({candidates.data.library.length})</span>
                </h2>
                {candidates.data.libraryError ? (
                  <Text role="alert" className="my-2 text-xs">
                    Library search failed: {candidates.data.libraryError} Search again to retry.
                  </Text>
                ) : candidates.data.library.length === 0 && (
                  <Text className="my-2 text-xs">No local candidates.</Text>
                )}
                {candidates.data.library.map((t) => (
                  <Row key={t.id} className="gap-3 py-2">
                    <div className="min-w-0 break-words">
                      <strong>{t.title}</strong>
                      <Caption>{t.artist} / {t.album} / {time(t.duration)}</Caption>
                      <MatchDetails match={candidates.data.libraryMatches?.[t.id]} />
                    </div>
                    <Button variant="outline" disabled={busy || !editable || candidates.isFetching} onClick={() => void act("resolve", { serverId: t.id })}>Use existing</Button>
                  </Row>
                ))}
              </section>
              <div className="min-w-0 space-y-5">{providerResults.map((provider) => <section key={provider.id} aria-labelledby={provider.id + "-recordings-heading"}>
                <h2 id={provider.id + "-recordings-heading"} className="m-0 border-0 border-b border-solid border-border pb-2 text-sm font-semibold">
                  {provider.name} recordings <span className="text-xs font-normal text-muted-foreground">({provider.tracks.length})</span>
                </h2>
                {provider.ambiguous && (
                  <Text className="my-2 text-xs">Multiple recordings match the metadata. Compare their versions before choosing.</Text>
                )}
                {provider.error ? (
                  <Text role="alert" className="my-2 text-xs">
                    {provider.name} search failed: {provider.error} Search again to retry.
                  </Text>
                ) : provider.tracks.length === 0 && (
                  <Text className="my-2 text-xs">No results. Try a shorter title, another spelling, or a direct provider song URL.</Text>
                )}
                {provider.tracks.map((t) => (
                  <Row key={trackTarget(t)} className="gap-3 py-2">
                    <div className="min-w-0 break-words">
                      <strong>{label(t)}</strong>
                      <Caption>{t.artist.name} / {t.album.title} / {time(t.duration)}</Caption>
                      <MatchDetails match={provider.matches?.[trackTarget(t)] ?? provider.matches?.[t.id]} />
                      <DeezerRecordingTools
                        track={t}
                        playing={preview.playing && !!preview.active && trackTarget(preview.active) === trackTarget(t)}
                        onPreview={preview.toggle}
                      />
                    </div>
                    <Button variant="outline" disabled={busy || !editable || candidates.isFetching} onClick={() => void act("resolve", { target: trackTarget(t) })}>{requestReplacement ? "Use and request" : "Use this recording"}</Button>
                  </Row>
                ))}
              </section>)}</div>
            </div>
          )}
        </div>
        <footer className="flex min-w-0 shrink-0 flex-col gap-2 border-0 border-t border-solid border-border px-5 py-3 max-sm:px-3">
          <PreviewPlayer preview={preview} />
          {preview.error && <Text role="alert" className="m-0 text-xs">{preview.error}</Text>}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Text className="m-0 text-xs">{requestReplacement ? "Choosing a provider recording creates a replacement request; approval may be required." : "Selecting a recording does not start a download."}</Text>
            {editable && !s.serverId && (
              <Button
                title="Pause new acquisition for this song. Keep its playlist entry; running downloads continue."
                variant="outline"
                className="ml-auto shrink-0"
                disabled={busy}
                onClick={() => void act(s.deferred ? "resume" : "defer")}
              >
                {s.deferred ? "Resume" : "Skip for now"}
              </Button>
            )}
          </div>
        </footer>
      </DialogContent>
    </Dialog>
  );
}
