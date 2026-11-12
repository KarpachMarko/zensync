import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAsString, parseAsStringLiteral, useQueryState } from "nuqs";
import { z } from "zod";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useAppForm } from "@/hooks/use-app-form";
import { AppLink } from "@/components/navigation";
import { MetadataJobs } from "@/components/music/metadata-jobs";
import { AutoLoadMore } from "@/components/auto-load-more";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";

import type { EnrichmentLibrary, EnrichmentMode } from "@/lib/metadata";
export function MetadataEnrichment({ admin }: { admin: boolean }) {
  const client = useQueryClient();
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [view, setView] = useQueryState("view", parseAsStringLiteral(["songs", "albums"] as const).withDefault("albums"));
  const [selectedSongs, setSelectedSongs] = useState<string[]>([]);
  const [selectedAlbums, setSelectedAlbums] = useState<string[]>([]);
  const [visible, setVisible] = useState(50);
  const library = useQuery({ queryKey: ["api", "/metadata/enrichment"], enabled: admin,
    queryFn: ({ signal }) => api<EnrichmentLibrary>("/metadata/enrichment", "GET", undefined, undefined, signal), retry: false });
  const search = useAppForm({ defaultValues: { q }, validators: { onSubmit: z.object({ q: z.string().max(500) }) },
    onSubmit: async ({ value }) => { setVisible(50); await setQ(value.q); } });
  const run = useMutation({ mutationFn: (mode: EnrichmentMode) => api<{ id: string; songs: number }>("/metadata/enrichment", "POST", { mode, songIds: selectedSongs, albumIds: selectedAlbums }),
    onSuccess: (result) => { toast.success(`Metadata job queued for ${result.songs} songs`); void client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] }); },
    onError: (error) => toast.error(error.message) });
  const items = useMemo(() => {
    const songs = library.data?.songs ?? [];
    const entries = view === "songs" ? songs.map(s => ({ id: s.id, title: s.title, artist: s.artist, count: 1 })) :
      Array.from(songs.reduce((map, s) => { if (!s.albumId) return map; const existing = map.get(s.albumId); if (existing) existing.count++; else map.set(s.albumId, { id: s.albumId, title: s.album, artist: s.albumArtist || s.artist, count: 1 }); return map; }, new Map<string, { id: string; title: string; artist: string; count: number }>()).values());
    return entries.filter(item => `${item.title} ${item.artist}`.toLocaleLowerCase().includes(q.toLocaleLowerCase()));
  }, [library.data, q, view]);
  if (!admin) return <Text role="alert">Metadata utilities require an administrator.</Text>;
  const disabled = run.isPending || !library.data || library.data.limited;
  const toggle = (id: string, checked: boolean) => {
    const update = (previous: string[]) => checked ? [...previous, id] : previous.filter(value => value !== id);
    if (view === "songs") setSelectedSongs(update); else setSelectedAlbums(update);
  };
  return <>
    <AppLink to="/utilities">Utilities</AppLink>
    <PageHeading tabIndex={-1}>Covers and lyrics</PageHeading>
    <Text>Fetch covers from Deezer and lyrics from LRCLIB. Confident matches apply directly without an approval step. Existing lyrics are preserved. Ambiguous matches are skipped.</Text>
    <div className="flex flex-wrap gap-2">
      <Button disabled={disabled} onClick={() => run.mutate("missing-covers")}>Fetch all missing covers</Button>
      <Button disabled={disabled} onClick={() => run.mutate("missing-lyrics")}>Fetch all missing lyrics</Button>
    </div>
    <MetadataJobs />
    {run.data && <Text role="status">Job queued for {run.data.songs} songs. Follow progress and skipped counts in <AppLink to="/activity">Activity</AppLink>.</Text>}
    {library.error && <Text role="alert">{library.error.message}</Text>}
    {library.isLoading && <Text role="status">Loading library…</Text>}
    {library.data?.limited && <Text role="alert">Library index is incomplete. Updates are disabled to avoid a partial operation.</Text>}
    <Separator className="my-4" />
    <h2 className="text-sm font-semibold">Update selected songs or albums</h2>
    <Text>Album covers are embedded in every song in the selected album. Forced cover updates replace existing artwork. Album order updates write Deezer track and disc numbers to matched songs.</Text>
    <Tabs value={view} onValueChange={value => { if (value === "songs" || value === "albums") { setVisible(50); void setView(value); } }}>
      <TabsList><TabsTrigger value="albums">Albums</TabsTrigger><TabsTrigger value="songs">Songs</TabsTrigger></TabsList>
    </Tabs>
    <form className="my-3 flex flex-wrap items-end gap-2" onSubmit={event => { event.preventDefault(); void search.handleSubmit(); }}>
      <search.AppField name="q">{field => <field.TextField label="Filter library" />}</search.AppField>
      <Button type="submit" variant="outline">Search</Button>
    </form>
    <Text>{selectedSongs.length} songs and {selectedAlbums.length} albums selected</Text>
    <div className="mb-3 flex flex-wrap gap-2">
      <Button disabled={disabled || !selectedSongs.length && !selectedAlbums.length} onClick={() => run.mutate("force-covers")}>Replace selected covers</Button>
      <Button variant="outline" disabled={disabled || !selectedSongs.length && !selectedAlbums.length} onClick={() => run.mutate("album-order")}>Correct selected track order</Button>
      <Button variant="ghost" onClick={() => { setSelectedSongs([]); setSelectedAlbums([]); }}>Clear selection</Button>
    </div>
    <div className="space-y-2">{items.slice(0, visible).map(item => <div key={item.id} className="flex items-center gap-3 rounded-md border p-3">
      <Checkbox id={`enrich-${item.id}`} checked={(view === "songs" ? selectedSongs : selectedAlbums).includes(item.id)} onCheckedChange={checked => toggle(item.id, checked === true)} />
      <Label htmlFor={`enrich-${item.id}`} className="min-w-0 flex-1 flex-wrap">{item.title} · {item.artist}{view === "albums" ? ` · ${item.count} songs` : ""}</Label>
    </div>)}</div>
    {library.data && !items.length && <Text>No matching items.</Text>}
    <AutoLoadMore hasMore={visible < items.length} loading={false} error={false} loadMore={async () => { setVisible(count => count + 50); }} />
  </>;
}
