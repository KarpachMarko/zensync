import { ReviewDialogContent } from "@/components/music/review-dialog-content";
import { useState } from "react";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, time, type LibrarySong } from "@/lib/api";
import type { LibraryBrowse, LibraryCollection, MergeCompatibility } from "@/lib/metadata";
import { libraryCoverURL } from "@/lib/playback";
import { useRecordingPreview } from "@/hooks/use-recording-preview";
import { PreviewPlayer } from "@/components/imports/preview-player";
import { LibraryRecordingDetails } from "./library-recording-details";
import { RecordingCover } from "./audio-player";
import { AutoLoadMore } from "@/components/auto-load-more";
import { SearchBox } from "@/components/search-box";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Text } from "@/components/typography";
import { toast } from "sonner";
import { parseAsString, useQueryState } from "nuqs";

type MergeCandidate = LibraryCollection & { song?: LibrarySong };
type MergeSource = { id: string; name: string; artist?: string; songCount?: number; coverArt?: string; song?: LibrarySong };

export function ManualMerge({ kind, source, onOpen }: { kind: "artist" | "album" | "song"; source: MergeSource; onOpen?: () => void }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useQueryState("mergeSearch", parseAsString.withDefault(""));
  const [target, setTarget] = useState<MergeCandidate | null>(null);
  const preview = useRecordingPreview<LibrarySong>();
  const client = useQueryClient();
  const candidates = useInfiniteQuery({
    queryKey: ["api", "merge-candidates", kind, query], enabled: open && !!query,
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
    queryKey: ["api", "merge-compatibility", source.id, target?.id], enabled: open && kind === "song" && !!target,
    queryFn: ({ signal }) => api<MergeCompatibility>(`/metadata/merge-compatibility?source=${encodeURIComponent(source.id)}&target=${encodeURIComponent(target?.id ?? "")}`, "GET", undefined, undefined, signal),
  });
  const merge = useMutation({
    mutationFn: () => api<{ songs: number }>("/metadata/merge", "POST", { kind, source: source.id, target: target?.id }),
    onSuccess: async result => {
      toast.success(`Merge queued for ${result.songs} songs`);
      preview.stop(); setOpen(false);
      await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] });
    },
  });
  function close() { if (!merge.isPending) { preview.stop(); setOpen(false); } }
  function select(item: MergeCandidate) { setTarget(item); merge.reset(); }
  const items = candidates.data?.pages.flatMap(page => page.collections).filter(item => item.id !== source.id) ?? [];
  const tracks = destination.data?.pages.flatMap(page => page.songs) ?? [];
  const blocked = kind === "song" && (!compatibility.data?.compatible || compatibility.isFetching || !!compatibility.error);
  const collectionDetails = (item: MergeSource & { duration?: number; year?: number }) => <div className="flex min-w-0 flex-1 items-start gap-3">
    <RecordingCover key={item.id} url={libraryCoverURL(item.coverArt)} />
    <div className="min-w-0 break-words"><strong className="text-sm">{item.name}</strong>
      <p className="m-0 mt-1 text-xs text-muted-foreground">{[item.artist, item.songCount !== undefined ? `${item.songCount} ${kind === "artist" ? "albums" : "tracks"}` : undefined, item.year, item.duration ? time(item.duration) : undefined].filter(Boolean).join(" / ")}</p>
    </div>
  </div>;
  return <>
    <Button size="sm" variant="outline" aria-label={`Merge ${kind}: ${source.name}`} onClick={() => { onOpen?.(); preview.stop(); setTarget(null); merge.reset(); void setQuery(""); setOpen(true); }}>Merge {kind}</Button>
    <Dialog open={open} onOpenChange={next => { if (!next) close(); }}>
      <ReviewDialogContent>
        <header className="shrink-0 border-b border-border px-5 py-3 pr-14 max-sm:pl-3">
          <DialogTitle className="m-0 text-lg">Merge {kind}</DialogTitle>
          <DialogDescription className="m-0 mt-1 text-xs leading-5">{kind === "song" ? `Replace the audio of “${source.name}” with the kept recording. Its file location, album tags and playlist references are preserved. Existing play counts stay with their entries.` : `Move songs from “${source.name}” into another ${kind}. Choose the entry to keep. This changes file tags and rescans Navidrome; audio is preserved.`}</DialogDescription>
        </header>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-3 max-sm:px-3">
          <div className="grid min-w-0 gap-4 md:grid-cols-2">
            <section aria-label="Merge source"><h2 className="mb-2 text-sm font-semibold">{kind === "song" ? "Audio to replace" : "Move from"}</h2>
              {source.song ? <LibraryRecordingDetails song={source.song} playback={preview} /> : collectionDetails(source)}
            </section>
            <section aria-label="Selected destination"><h2 className="mb-2 text-sm font-semibold">Keep {kind === "song" ? "recording" : kind}</h2>
              {target ? target.song ? <LibraryRecordingDetails song={target.song} playback={preview} /> : collectionDetails(target) : <Text className="m-0 text-xs">Search and select a destination below.</Text>}
            </section>
          </div>
          <SearchBox label={`Find destination ${kind}`} value={query} disabled={merge.isPending} onSearch={async value => { setTarget(null); await setQuery(value); }} />
          {candidates.isFetching && !candidates.isFetchingNextPage && <Text role="status">Searching…</Text>}
          {candidates.error && <Text role="alert">{candidates.error.message}</Text>}
          <section aria-label="Merge candidates" className="space-y-2">
            {items.map(item => <article key={item.id} className={`flex min-w-0 flex-wrap items-start gap-3 rounded-md border p-3 ${target?.id === item.id ? "border-primary bg-muted/50" : "border-border"}`}>
              {item.song ? <LibraryRecordingDetails song={item.song} playback={preview} /> : collectionDetails(item)}
              <Button variant="outline" size="sm" aria-label={`Select ${kind}: ${item.name}`} aria-pressed={target?.id === item.id} disabled={merge.isPending} onClick={() => select(item)}>{target?.id === item.id ? "Selected" : `Select ${kind}`}</Button>
              {target?.id === item.id && <Badge variant="outline">Selected destination</Badge>}
            </article>)}
            {query && candidates.data && items.length === 0 && <Text>No other matching {kind}s found.</Text>}
            <AutoLoadMore hasMore={!!candidates.hasNextPage} loading={candidates.isFetchingNextPage} error={candidates.isFetchNextPageError} loadMore={candidates.fetchNextPage} />
          </section>
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
          {target && kind === "song" && compatibility.isFetching && <Text role="status">Checking recording formats…</Text>}
          {target && kind === "song" && compatibility.data?.warning && <Text role="alert" className="mb-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">{compatibility.data.warning}</Text>}
          {target && kind === "song" && compatibility.error && <Text role="alert">{compatibility.error.message} <Button variant="link" disabled={compatibility.isFetching} onClick={() => void compatibility.refetch()}>Retry check</Button></Text>}
          {merge.error && <Text role="alert">{merge.error.message}</Text>}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Text className="m-0 text-xs">{target ? `Keep “${target.name}”; merge “${source.name}” into it.` : "Selecting or listening does not start a merge."}</Text>
            <div className="ml-auto flex gap-2"><Button variant="ghost" disabled={merge.isPending} onClick={close}>Cancel</Button><Button disabled={!target || merge.isPending || blocked} onClick={() => merge.mutate()}>Confirm merge</Button></div>
          </div>
          <PreviewPlayer preview={preview} />
        </footer>
      </ReviewDialogContent>
    </Dialog>
  </>;
}
