import { ReviewDialogContent } from "@/components/music/review-dialog-content";
import { useState } from "react";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, time, label, trackTarget, type LibrarySong } from "@/lib/api";
import type { LibraryBrowse, LibraryCollection, MergeCompatibility } from "@/lib/metadata";
import { libraryCoverURL, type Recording } from "@/lib/playback";
import { useRecordingPreview } from "@/hooks/use-recording-preview";
import { PreviewPlayer } from "@/components/imports/preview-player";
import { LibraryRecordingDetails } from "./library-recording-details";
import { RecordingCover } from "./audio-player";
import { AutoLoadMore } from "@/components/auto-load-more";
import { SearchBox } from "@/components/search-box";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ProviderRecordingDetails, ProviderReplacements, ReplacementUpload, type ExternalReplacement } from "./replacement-sources";
import { Dialog, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Text } from "@/components/typography";
import { toast } from "sonner";
import { parseAsString, parseAsStringLiteral, useQueryState } from "nuqs";

type MergeCandidate = LibraryCollection & { song?: LibrarySong };
type MergeSource = { id: string; name: string; artist?: string; songCount?: number; coverArt?: string; song?: LibrarySong };

export function ManualMerge({ kind, source, onOpen }: { kind: "artist" | "album" | "song"; source: MergeSource; onOpen?: () => void }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useQueryState("mergeSearch", parseAsString.withDefault(""));
  const [target, setTarget] = useState<MergeCandidate | null>(null);
  const [external, setExternal] = useState<ExternalReplacement | null>(null);
  const [flipped, setFlipped] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [sourceTab, setSourceTab] = useQueryState("mergeSource", parseAsStringLiteral(["library", "providers", "upload"] as const).withDefault("library"));
  const preview = useRecordingPreview<Recording>();
  const from = flipped && target ? target : source;
  const to = flipped ? source : target;
  const songAction = kind === "song" ? "Merge/Replace" : `Merge ${kind}`;
  const busy = uploading;
  const client = useQueryClient();
  const candidates = useInfiniteQuery({
    queryKey: ["api", "merge-candidates", kind, query], enabled: open && !!query && (kind !== "song" || sourceTab === "library"),
    initialPageParam: 0,
    queryFn: async ({ signal, pageParam }): Promise<LibraryBrowse & { collections: MergeCandidate[] }> => {
      if (kind === "song") {
        const result = await api<{ songs: LibrarySong[]; nextOffset: number }>(`/library?details=1&q=${encodeURIComponent(query)}&offset=${pageParam}`, "GET", undefined, undefined, signal);
        return { ...result, name: "", collections: result.songs.map(song => ({ id: song.id, name: song.title, artist: song.artist, songCount: 1, song })) };
      }
      return api<LibraryBrowse>(`/library/browse?view=${kind}s&q=${encodeURIComponent(query)}&offset=${pageParam}`, "GET", undefined, undefined, signal);
    },
    getNextPageParam: last => last.nextOffset >= 0 ? last.nextOffset : undefined,
  });
  const destination = useInfiniteQuery({
    queryKey: ["api", "merge-destination", kind, target?.id], enabled: open && !!target && kind !== "song",
    initialPageParam: 0,
    queryFn: ({ signal, pageParam }) => api<LibraryBrowse>(`/library/browse?view=${kind}s&id=${encodeURIComponent(target?.id ?? "")}&contents=songs&offset=${pageParam}`, "GET", undefined, undefined, signal),
    getNextPageParam: last => last.nextOffset >= 0 ? last.nextOffset : undefined,
  });
  const compatibility = useQuery({
    queryKey: ["api", "merge-compatibility", from.id, to?.id], enabled: open && kind === "song" && !!target && !external,
    queryFn: ({ signal }) => api<MergeCompatibility>(`/metadata/merge-compatibility?source=${encodeURIComponent(from.id)}&target=${encodeURIComponent(to?.id ?? "")}`, "GET", undefined, undefined, signal),
  });
  const merge = useMutation({
    mutationFn: () => api<{ songs: number }>("/metadata/merge", "POST", { kind, source: from.id,
      ...(external?.type === "provider" ? { providerTarget: trackTarget(external.track) } : external?.type === "upload" ? { uploadId: external.id } : { target: to?.id }),
      ...(kind === "song" && (external || compatibility.data?.formatChange) ? { allowFormatChange: true } : {}),
    }),
    onSuccess: async result => {
      toast.success(`Merge queued for ${result.songs} songs`);
      preview.stop(); setOpen(false);
      await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] });
    },
  });
  function close() { if (!merge.isPending && !busy) { preview.stop(); setOpen(false); } }
  function select(item: MergeCandidate) { setTarget(item); setExternal(null); setFlipped(false); merge.reset(); }
  const items = candidates.data?.pages.flatMap(page => page.collections).filter(item => item.id !== source.id) ?? [];
  const tracks = destination.data?.pages.flatMap(page => page.songs) ?? [];
  const blocked = kind === "song" && !external && (!compatibility.data?.compatible || compatibility.isFetching || !!compatibility.error);
  const collectionDetails = (item: MergeSource & { duration?: number; year?: number }) => <div className="flex min-w-0 flex-1 items-start gap-3">
    <RecordingCover key={item.id} url={libraryCoverURL(item.coverArt)} />
    <div className="min-w-0 break-words"><strong className="text-sm">{item.name}</strong>
      <p className="m-0 mt-1 text-xs text-muted-foreground">{[item.artist, item.songCount !== undefined ? `${item.songCount} ${kind === "artist" ? "albums" : "tracks"}` : undefined, item.year, item.duration ? time(item.duration) : undefined].filter(Boolean).join(" / ")}</p>
    </div>
  </div>;
  return <>
    <Button size="sm" variant="outline" aria-label={`${songAction}: ${source.name}`} onClick={() => { onOpen?.(); preview.stop(); setTarget(null); setExternal(null); setFlipped(false); merge.reset(); void setSourceTab("library"); void setQuery(source.name); setOpen(true); }}>{songAction}</Button>
    <Dialog open={open} onOpenChange={next => { if (!next) close(); }}>
      <ReviewDialogContent>
        <header className="shrink-0 border-b border-border px-5 py-3 pr-14 max-sm:pl-3">
          <DialogTitle className="m-0 text-lg">{songAction}</DialogTitle>
          <DialogDescription className="m-0 mt-1 text-xs leading-5">{kind === "song" ? `Replace the audio of “${from.name}” with the selected recording. Original title and album tags are kept. You can listen before confirming.` : `Move songs from “${source.name}” into another ${kind}. Choose the entry to keep. This changes file tags and rescans Navidrome; audio is preserved.`}</DialogDescription>
        </header>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-2 overflow-y-auto px-5 py-3 max-sm:px-3">
          <div className="grid min-w-0 gap-2 md:grid-cols-2">
            <section aria-label="Merge source"><h2 className="mb-2 text-sm font-semibold">{kind === "song" ? "Audio to replace" : "Move from"}</h2>
              {from.song ? <LibraryRecordingDetails song={from.song} playback={preview} /> : collectionDetails(from)}
            </section>
            <section aria-label="Selected destination"><h2 className="mb-2 text-sm font-semibold">Keep {kind === "song" ? "recording" : kind}</h2>
              {external ? external.type === "provider" ? <ProviderRecordingDetails track={external.track} playback={preview} /> : <LibraryRecordingDetails song={external.song} playback={preview} /> : to ? to.song ? <LibraryRecordingDetails song={to.song} playback={preview} /> : collectionDetails(to) : <Text className="m-0 text-xs">Search and select a destination below.</Text>}
            </section>
          </div>
          {kind === "song" && target && !external && <Button variant="outline" size="sm" className="self-start" disabled={merge.isPending || busy} onClick={() => { setFlipped(value => !value); merge.reset(); }}>Flip merge direction</Button>}
          {kind === "song" && <Tabs value={sourceTab} onValueChange={value => { if (!merge.isPending && !busy && (value === "library" || value === "providers" || value === "upload")) void setSourceTab(value); }}>
            <TabsList aria-label="Recording source"><TabsTrigger value="library">Library</TabsTrigger><TabsTrigger value="providers">Providers</TabsTrigger><TabsTrigger value="upload">Upload file</TabsTrigger></TabsList>
          </Tabs>}
          {(kind !== "song" || sourceTab !== "upload") && <SearchBox className="my-0" label={`Find destination ${kind}`} value={query} disabled={merge.isPending || busy} onSearch={async value => { await setQuery(value); }} />}
          {kind === "song" && sourceTab === "providers" && <ProviderReplacements query={query} selected={external} busy={merge.isPending || busy} playback={preview} onSelect={value => { setExternal(value); setTarget(null); setFlipped(false); merge.reset(); }} />}
          {kind === "song" && sourceTab === "upload" && <ReplacementUpload source={source.id} busy={merge.isPending || busy} onBusyChange={setUploading} onSelect={value => { setExternal(value); setTarget(null); setFlipped(false); merge.reset(); }} />}
          {(kind !== "song" || sourceTab === "library") && <>

          {candidates.isFetching && !candidates.isFetchingNextPage && <Text role="status">Searching…</Text>}
          {candidates.error && <Text role="alert">{candidates.error.message}</Text>}
          <section aria-label="Merge candidates" className="space-y-2">
            {items.map(item => <article key={item.id} className={`flex min-w-0 flex-wrap items-start gap-3 rounded-md border p-3 ${target?.id === item.id ? "border-primary bg-muted/50" : "border-border"}`}>
              {item.song ? <LibraryRecordingDetails song={item.song} playback={preview} /> : collectionDetails(item)}
              <Button variant="outline" size="sm" aria-label={`Select ${kind}: ${item.name}`} aria-pressed={target?.id === item.id} disabled={merge.isPending || busy} onClick={() => select(item)}>{target?.id === item.id ? "Selected" : `Select ${kind}`}</Button>
            </article>)}
            {query && candidates.data && items.length === 0 && <Text>No other matching {kind}s found.</Text>}
            <AutoLoadMore hasMore={!!candidates.hasNextPage} loading={candidates.isFetchingNextPage} error={candidates.isFetchNextPageError} loadMore={candidates.fetchNextPage} />
          </section>
          </>}
          {target && kind !== "song" && <section aria-label="Destination recordings" className="space-y-3">
            <h2 className="text-sm font-semibold">Destination recordings</h2>
            {destination.isPending && <Text role="status">Loading recordings…</Text>}
            {destination.error && <Text role="alert">{destination.error.message}</Text>}
            {tracks.map(song => <LibraryRecordingDetails key={song.id} song={song} playback={preview} />)}
            {destination.data && !tracks.length && <Text>No recordings available.</Text>}
            <AutoLoadMore hasMore={!!destination.hasNextPage} loading={destination.isFetchingNextPage} error={destination.isFetchNextPageError} loadMore={destination.fetchNextPage} />
          </section>}
        </div>
        <footer className="shrink-0 border-t border-border px-5 py-3 max-sm:px-3">
          {target && !external && kind === "song" && compatibility.isFetching && <Text role="status">Checking recording formats…</Text>}
          {target && !external && kind === "song" && compatibility.data?.warning && <Text role="alert" className="mb-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">{compatibility.data.warning}</Text>}
          {target && !external && kind === "song" && compatibility.error && <Text role="alert">{compatibility.error.message} <Button variant="link" disabled={compatibility.isFetching} onClick={() => void compatibility.refetch()}>Retry check</Button></Text>}
          {external && <Text role="alert" className="mb-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-2 text-sm">{external.type === "upload" ? `The replacement uses ${external.song.suffix?.toUpperCase()} audio.` : "The provider's native audio format is determined after download."} If the format changes, the file extension changes too. No audio transcoding is performed. Navidrome IDs and playlist references may change after scanning.</Text>}
          {merge.error && <Text role="alert">{merge.error.message}</Text>}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Text className="m-0 text-xs">{external ? `Replace “${source.name}” using “${external.type === "provider" ? label(external.track) : external.song.title}”.` : target ? kind === "song" ? `Replace audio in “${from.name}” using “${to?.name}”.` : `Keep “${target.name}”; merge “${source.name}” into it.` : "Selecting or listening does not start a merge."}</Text>
            <div className="ml-auto flex gap-2"><Button variant="ghost" disabled={merge.isPending || busy} onClick={close}>Cancel</Button><Button disabled={(!target && !external) || merge.isPending || busy || blocked} onClick={() => merge.mutate()}>{kind === "song" ? "Confirm Merge/Replace" : "Confirm merge"}</Button></div>
          </div>
          <PreviewPlayer preview={preview} />
        </footer>
      </ReviewDialogContent>
    </Dialog>
  </>;
}
