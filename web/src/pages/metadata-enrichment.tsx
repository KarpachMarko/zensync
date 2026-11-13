import { UtilitiesBreadcrumbs } from "@/components/utilities-breadcrumbs";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAsString, parseAsStringLiteral, useQueryState } from "nuqs";
import { useStore } from "@tanstack/react-form";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
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
  const [, setEditSong] = useQueryState("editSong", parseAsString);
  const [coverFilter, setCoverFilter] = useQueryState("covers", parseAsStringLiteral(["all", "missing"] as const).withDefault("all"));
  const [view, setView] = useQueryState("view", parseAsStringLiteral(["songs", "albums"] as const).withDefault("albums"));
  const [selectedSongs, setSelectedSongs] = useState<string[]>([]);
  const [selectedAlbums, setSelectedAlbums] = useState<string[]>([]);
  const [visible, setVisible] = useState(50);
  const library = useQuery({ queryKey: ["api", "/metadata/enrichment"], enabled: admin,
    queryFn: ({ signal }) => api<EnrichmentLibrary>("/metadata/enrichment", "GET", undefined, undefined, signal), retry: false });
  const search = useAppForm({ defaultValues: { q }, validators: { onSubmit: z.object({ q: z.string().max(500) }) },
    onSubmit: async ({ value }) => { setVisible(50); await setQ(value.q); } });
  const queryText = useStore(search.store, state => state.values.q);
  const debouncedQuery = useDebouncedValue(queryText, 300);
  useEffect(() => { if (debouncedQuery !== q) { setVisible(50); void setQ(debouncedQuery); } }, [debouncedQuery]);
  useEffect(() => { if (q !== search.getFieldValue("q")) search.setFieldValue("q", q); }, [q]);
  const run = useMutation({ mutationFn: (mode: EnrichmentMode) => api<{ id: string; songs: number }>("/metadata/enrichment", "POST", { mode, songIds: mode === "album-order" ? [] : selectedSongs, albumIds: selectedAlbums }),
    onSuccess: (result) => { toast.success(`Metadata job queued for ${result.songs} songs`); void client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] }); },
    onError: (error) => toast.error(error.message) });
  const items = useMemo(() => {
    const songs = library.data?.songs ?? [];
    const entries = view === "songs" ? songs.map(s => ({ id:s.id, title:s.title, artist:s.artist, album:s.album, count:1, missing:!s.coverArt, editID:s.id })) :
      Array.from(songs.reduce((map,s) => {
        if (!s.albumId) return map;
        const existing=map.get(s.albumId);
        if (existing) { existing.count++; if (!s.coverArt) {existing.missing=true;existing.editID=s.id;} }
        else map.set(s.albumId,{id:s.albumId,title:s.album,artist:s.albumArtist || s.artist,album:s.album,count:1,missing:!s.coverArt,editID:s.id});
        return map;
      },new Map<string,{id:string;title:string;artist:string;album:string;count:number;missing:boolean;editID:string}>()).values());
    return entries.filter(item => (coverFilter!=="missing" || item.missing) && `${item.title} ${item.artist} ${item.album}`.toLocaleLowerCase().includes(q.toLocaleLowerCase()));
  }, [library.data, q, view, coverFilter]);
  if (!admin) return <Text role="alert">Metadata utilities require an administrator.</Text>;
  const disabled = run.isPending || !library.data || library.data.limited;
  const toggle = (id: string, checked: boolean) => {
    const update = (previous: string[]) => checked ? [...previous, id] : previous.filter(value => value !== id);
    if (view === "songs") setSelectedSongs(update); else setSelectedAlbums(update);
  };
  return <>
    <UtilitiesBreadcrumbs current="Library metadata" />
    <PageHeading tabIndex={-1}>Library metadata</PageHeading>
    <Text>Update artwork, lyrics and album track order. Choose songs or albums for artwork, and albums for track ordering. Existing lyrics are kept; uncertain matches are skipped.</Text>
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
    <Text>Album selection updates every song in that album. Open an album’s songs to edit individual covers manually. Track order uses the current album name and artist to find a reference tracklist, including Deluxe editions. Matched songs use positions from that tracklist; unmatched songs are skipped and duplicates can share a number.</Text>
    <Tabs value={view} onValueChange={value => { if (value === "songs" || value === "albums") { setVisible(50); void setView(value); } }}>
      <TabsList><TabsTrigger value="albums">Albums</TabsTrigger><TabsTrigger value="songs">Songs</TabsTrigger></TabsList>
    </Tabs>
    <form className="my-3 max-w-xl" onSubmit={event => { event.preventDefault(); void search.handleSubmit(); }}>
      <search.AppField name="q">{field => <field.TextField label="Filter library" labelClassName="m-0" placeholder="Search by title or artist…" maxLength={500} />}</search.AppField>

    </form>
    <div className="my-3 flex items-center gap-2"><Switch id="missing-covers" checked={coverFilter==="missing"} onCheckedChange={checked => {setVisible(50);void setCoverFilter(checked ? "missing" : "all");}} /><Label htmlFor="missing-covers">Missing covers only</Label></div>
    <Text>{selectedSongs.length} songs and {selectedAlbums.length} albums selected</Text>
    <div className="mb-3 flex flex-wrap gap-2">
      <Button disabled={disabled || !selectedSongs.length && !selectedAlbums.length} onClick={() => run.mutate("force-covers")}>Replace selected covers</Button>
      {view === "albums" && <Button variant="outline" disabled={disabled || !selectedAlbums.length} onClick={() => run.mutate("album-order")}>Correct selected album order</Button>}
      <Button variant="ghost" onClick={() => { setSelectedSongs([]); setSelectedAlbums([]); }}>Clear selection</Button>
    </div>
    <div className="space-y-2">{items.slice(0, visible).map(item => <div key={item.id} className="flex flex-wrap items-center gap-3 border-b border-solid border-border py-3">
      <Checkbox id={`enrich-${item.id}`} checked={(view === "songs" ? selectedSongs : selectedAlbums).includes(item.id)} onCheckedChange={checked => toggle(item.id, checked === true)} />
      <Label htmlFor={`enrich-${item.id}`} className="min-w-0 flex-1 flex-col items-start gap-1">
        <span className="break-words font-medium">{item.title}</span>
        <span className="text-xs font-normal text-muted-foreground">{item.artist}{view === "albums" ? ` · ${item.count} ${item.count === 1 ? "song" : "songs"}` : ""}</span>
      </Label>
      {item.missing && <Badge variant="outline">Missing cover</Badge>}
      {view === "songs" ? <Button variant="ghost" size="sm" aria-label={`Edit cover: ${item.title}`} onClick={() => void setEditSong(item.editID)}>Edit cover</Button> : <Button variant="ghost" size="sm" aria-label={`View songs: ${item.title}`} onClick={() => {setVisible(50);void setView("songs");void setQ(item.title);}}>View songs</Button>}
    </div>)}</div>
    {library.data && !items.length && <Text>No matching items.</Text>}
    <AutoLoadMore hasMore={visible < items.length} loading={false} error={false} loadMore={async () => { setVisible(count => count + 50); }} />
  </>;
}
