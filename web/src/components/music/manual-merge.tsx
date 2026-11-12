import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type LibrarySong } from "@/lib/api";
import type { LibraryBrowse, LibraryCollection } from "@/lib/metadata";
import { SearchBox } from "@/components/search-box";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Text } from "@/components/typography";
import { toast } from "sonner";
import { parseAsString, useQueryState } from "nuqs";

export function ManualMerge({ kind, source }: { kind: "artist" | "album" | "song"; source: { id: string; name: string } }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useQueryState("mergeSearch", parseAsString.withDefault(""));
  const [target, setTarget] = useState<LibraryCollection | null>(null);
  const client = useQueryClient();
  const candidates = useQuery({
    queryKey: ["api", "merge-candidates", kind, query], enabled: open && !!query,
    queryFn: async ({ signal }): Promise<LibraryBrowse> => {
      if (kind === "song") {
        const result = await api<{ songs: LibrarySong[]; nextOffset: number }>(`/library?details=1&q=${encodeURIComponent(query)}&offset=0`, "GET", undefined, undefined, signal);
        return { ...result, name: "", collections: result.songs.map((song) => ({ id: song.id, name: song.title, artist: `${song.artist} — ${song.album}`, songCount: 1 })) };
      }
      return api<LibraryBrowse>(`/library/browse?view=${kind}s&q=${encodeURIComponent(query)}&offset=0`, "GET", undefined, undefined, signal);
    },
  });
  const merge = useMutation({
    mutationFn: () => api<{ songs: number }>("/metadata/merge", "POST", { kind, source: source.id, target: target?.id }),
    onSuccess: async (result) => {
      toast.success(`Merge queued for ${result.songs} songs`);
      setOpen(false);
      await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] });
    },
  });
  return <>
    <Button size="sm" variant="outline" aria-label={`Merge ${kind}: ${source.name}`} onClick={() => { setTarget(null); merge.reset(); void setQuery(""); setOpen(true); }}>Merge {kind}</Button>
    <Dialog open={open} onOpenChange={(next) => { if (!merge.isPending) setOpen(next); }}>
      <DialogContent className="max-h-[85vh] overflow-y-auto">
        <DialogTitle>Merge {kind}</DialogTitle>
        <DialogDescription>{kind === "song" ? `Replace the audio of “${source.name}” with the kept recording. Its file location, album tags and playlist references are preserved. Matching file formats are required. Existing play counts stay with their entries.` : `Move songs from “${source.name}” into another ${kind}. Choose the entry to keep. This changes file tags and rescans Navidrome; audio is preserved.`}</DialogDescription>
        <SearchBox label={`Find destination ${kind}`} onSearch={async (value) => { setTarget(null); await setQuery(value); }} />
        {candidates.isFetching && <Text role="status">Searching…</Text>}
        {candidates.error && <Text role="alert">{candidates.error.message}</Text>}
        <div className="space-y-2">
          {candidates.data?.collections.filter((item) => item.id !== source.id).map((item) => <Button key={item.id} variant={target?.id === item.id ? "secondary" : "outline"} className="h-auto w-full whitespace-normal text-left" aria-pressed={target?.id === item.id} disabled={merge.isPending} onClick={() => setTarget(item)}>{item.name}{item.artist ? ` — ${item.artist}` : ""} · {item.id}</Button>)}
          {query && candidates.data && !candidates.data.collections.some((item) => item.id !== source.id) && <Text>No other matching {kind}s found.</Text>}
        </div>
        {target && <Text>Keep “{target.name}”; merge “{source.name}” into it.</Text>}
        {merge.error && <Text role="alert">{merge.error.message}</Text>}
        <div className="flex justify-end gap-2"><Button variant="ghost" disabled={merge.isPending} onClick={() => setOpen(false)}>Cancel</Button><Button disabled={!target || merge.isPending} onClick={() => merge.mutate()}>Confirm merge</Button></div>
      </DialogContent>
    </Dialog>
  </>;
}
