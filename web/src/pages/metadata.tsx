import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { SearchBox } from "@/components/search-box";
import { PageHeading, SectionHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { useAppForm } from "@/hooks/use-app-form";
import { api, type LibrarySong } from "@/lib/api";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { parseAsString, parseAsStringLiteral, useQueryState } from "nuqs";
import { useState } from "react";
import { toast } from "sonner";
import { z } from "zod";

const editSchema = z.object({
  title: z.string().trim().min(1).max(500),
  artist: z.string().trim().min(1).max(500),
  album: z.string().max(500),
  coverUrl: z.union([z.literal(""), z.url().startsWith("https://").max(2000)]),
  lyrics: z.string().max(200000),
  syncedLyrics: z.string().max(200000),
  lyricsSource: z.string().max(500),
});
type Edit = z.infer<typeof editSchema>;
type Duplicates = {
  scanned: number;
  limited: boolean;
  groups: {
    kind: string;
    reason: string;
    members: { id: string; label: string }[];
  }[];
};
type Lyrics = {
  id: number;
  trackName: string;
  artistName: string;
  albumName: string;
  duration: number;
  instrumental: boolean;
  plainLyrics: string;
  syncedLyrics: string;
};
type Cover = {
  id: string;
  title: string;
  artist: string;
  url: string;
  source: string;
};

export function Metadata({ admin }: { admin: boolean }) {
  const [tab, setTab] = useQueryState(
    "tab",
    parseAsStringLiteral(["songs", "duplicates"] as const).withDefault("songs"),
  );
  const [query, setQuery] = useQueryState("q", parseAsString.withDefault(""));
  const [song, setSong] = useQueryState("song", parseAsString.withDefault(""));
  const library = useInfiniteQuery({
    queryKey: ["metadata-library", query],
    enabled: admin && tab === "songs",
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      api<{ songs: LibrarySong[]; nextOffset: number }>(
        `/library?details=1&q=${encodeURIComponent(query)}&offset=${pageParam}`,
        "GET",
        undefined,
        undefined,
        signal,
      ),
    getNextPageParam: (last) =>
      last.nextOffset >= 0 ? last.nextOffset : undefined,
  });
  const duplicates = useQuery({
    queryKey: ["metadata-duplicates"],
    enabled: admin && tab === "duplicates",
    staleTime: 30000,
    queryFn: ({ signal }) =>
      api<Duplicates>(
        "/metadata/duplicates",
        "GET",
        undefined,
        undefined,
        signal,
      ),
    retry: false,
  });
  if (!admin)
    return (
      <Text role="alert">Metadata management requires an administrator.</Text>
    );
  return (
    <>
      <PageHeading tabIndex={-1}>Metadata</PageHeading>
      <Text>
        Review library metadata, artwork and lyrics. Saved edits stay in
        ZenSync; audio tags and Navidrome are not updated in this initial
        version.
      </Text>
      <Tabs
        value={tab}
        onValueChange={(value) =>
          void setTab(value === "duplicates" ? "duplicates" : "songs")
        }
      >
        <TabsList>
          <TabsTrigger value="songs">Songs</TabsTrigger>
          <TabsTrigger value="duplicates">Possible duplicates</TabsTrigger>
        </TabsList>
        <TabsContent value="songs">
          <SearchBox
            label="Find library songs"
            value={query}
            onSearch={async (q) => {
              await setQuery(q);
            }}
          />
          {library.isPending && <Text role="status">Loading songs…</Text>}
          {library.error && <Text role="alert">{library.error.message}</Text>}
          {library.data?.pages
            .flatMap((p) => p.songs)
            .map((s) => (
              <div
                key={s.id}
                className="flex items-center justify-between gap-3 border-b border-border py-3"
              >
                <div>
                  <strong>{s.title}</strong>
                  <Text className="m-0 text-sm">
                    {s.artist} · {s.album}
                  </Text>
                </div>
                <Button variant="outline" onClick={() => void setSong(s.id)}>
                  Edit <span className="sr-only">{s.title}</span>
                </Button>
              </div>
            ))}
          {library.data?.pages.every((p) => !p.songs.length) && (
            <Text>No library songs found.</Text>
          )}
          {library.hasNextPage && (
            <Button
              disabled={library.isFetching}
              onClick={() => void library.fetchNextPage()}
            >
              Load more
            </Button>
          )}
        </TabsContent>
        <TabsContent value="duplicates">
          <Text>
            Candidates use normalized names and distinct library IDs. Songs also
            need durations within two seconds. Different editions and artists
            with the same name can be legitimate. Nothing is merged or deleted
            automatically.
          </Text>
          <Button
            variant="outline"
            disabled={duplicates.isFetching}
            onClick={() => void duplicates.refetch()}
          >
            Refresh candidates
          </Button>
          {duplicates.isFetching && (
            <Text role="status">Checking library…</Text>
          )}
          {duplicates.error && (
            <Text role="alert">{duplicates.error.message}</Text>
          )}
          {duplicates.data && (
            <Text>
              {duplicates.data.scanned} songs checked.{" "}
              {duplicates.data.groups.length} candidate groups.
              {duplicates.data.limited &&
                " Partial scan: the 50,000-song limit was reached."}
            </Text>
          )}
          {duplicates.data?.groups.map((g, i) => (
            <section
              key={i}
              className="my-4 rounded-md border border-border p-4"
            >
              <Badge variant="secondary">{g.kind}</Badge>
              <Text>{g.reason}</Text>
              <ul className="space-y-2 pl-5">
                {g.members.map((m) => (
                  <li key={m.id}>
                    {m.label}{" "}
                    <span className="text-xs text-muted-foreground">
                      ({m.id})
                    </span>
                    {g.kind === "song" && (
                      <Button
                        variant="ghost"
                        onClick={() => void setSong(m.id)}
                      >
                        Review
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </TabsContent>
      </Tabs>
      {song && (
        <SongEditor key={song} id={song} onClose={() => void setSong("")} />
      )}
    </>
  );
}
function SongEditor({ id, onClose }: { id: string; onClose: () => void }) {
  const result = useQuery({
    queryKey: ["metadata-edit", id],
    queryFn: ({ signal }) =>
      api<{ edit: Edit; saved: boolean; duration?: number }>(
        `/metadata/songs/${encodeURIComponent(id)}`,
        "GET",
        undefined,
        undefined,
        signal,
      ),
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogTitle>Review song metadata</DialogTitle>
        <DialogDescription>
          Save reviewed metadata in ZenSync. Audio tags and Navidrome stay
          unchanged.
        </DialogDescription>
        {result.isPending && <Text>Loading metadata…</Text>}
        {result.error && <Text role="alert">{result.error.message}</Text>}
        {result.data?.duration !== undefined && (
          <Text>
            Library duration: {result.data.duration} seconds. Match this against
            lyrics candidates.
          </Text>
        )}
        {result.data && (
          <EditForm
            key={JSON.stringify(result.data)}
            id={id}
            initial={result.data.edit}
            saved={result.data.saved}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
function EditForm({
  id,
  initial,
  saved,
}: {
  id: string;
  initial: Edit;
  saved: boolean;
}) {
  const client = useQueryClient();
  const [lyricsQ, setLyricsQ] = useState("");
  const [coversQ, setCoversQ] = useState("");
  const save = useMutation({
    mutationFn: (edit: Edit) =>
      api(`/metadata/songs/${encodeURIComponent(id)}`, "PUT", edit),
    onSuccess: async () => {
      toast.success("Metadata saved in ZenSync");
      await client.invalidateQueries({ queryKey: ["metadata-edit", id] });
    },
  });
  const reset = useMutation({
    mutationFn: () =>
      api(`/metadata/songs/${encodeURIComponent(id)}`, "DELETE"),
    onSuccess: async () => {
      toast.success("Source metadata restored");
      await client.invalidateQueries({ queryKey: ["metadata-edit", id] });
    },
  });
  const form = useAppForm({
    defaultValues: initial,
    validators: { onSubmit: editSchema },
    onSubmit: async ({ value }) => {
      await save.mutateAsync(value).catch(() => undefined);
    },
  });
  const lyrics = useQuery({
    queryKey: ["metadata-lyrics", lyricsQ],
    enabled: !!lyricsQ,
    retry: false,
    staleTime: 300000,
    queryFn: ({ signal }) =>
      api<Lyrics[]>(
        `/metadata/lyrics?q=${encodeURIComponent(lyricsQ)}`,
        "GET",
        undefined,
        undefined,
        signal,
      ),
  });
  const covers = useQuery({
    queryKey: ["metadata-covers", coversQ],
    enabled: !!coversQ,
    retry: false,
    staleTime: 300000,
    queryFn: ({ signal }) =>
      api<Cover[]>(
        `/metadata/covers?q=${encodeURIComponent(coversQ)}`,
        "GET",
        undefined,
        undefined,
        signal,
      ),
  });
  return (
    <>
      <form
        className="grid gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          void form.handleSubmit();
        }}
      >
        {(["title", "artist", "album", "coverUrl"] as const).map((name) => (
          <form.AppField key={name} name={name}>
            {(field) => (
              <field.TextField
                label={
                  {
                    title: "Title",
                    artist: "Artist",
                    album: "Album",
                    coverUrl: "Cover URL",
                  }[name]
                }
              />
            )}
          </form.AppField>
        ))}
        <form.AppField name="lyrics">
          {(field) => <field.TextareaField label="Plain lyrics" rows={6} />}
        </form.AppField>
        <form.AppField name="syncedLyrics">
          {(field) => (
            <field.TextareaField label="Synced lyrics (LRC)" rows={6} />
          )}
        </form.AppField>
        {initial.lyricsSource && (
          <Text className="text-xs">Lyrics source: {initial.lyricsSource}</Text>
        )}
        <div className="flex flex-wrap gap-2">
          <Button disabled={save.isPending || reset.isPending}>
            Save metadata
          </Button>
          {saved && (
            <Button
              type="button"
              variant="outline"
              disabled={save.isPending || reset.isPending}
              onClick={() => reset.mutate()}
            >
              Restore source metadata
            </Button>
          )}
        </div>
        {(save.error || reset.error) && (
          <Text role="alert">
            {save.error?.message || reset.error?.message}
          </Text>
        )}
      </form>
      <SectionHeading>Find lyrics · LRCLIB</SectionHeading>
      <Text>
        Review the artist, album, duration and text before selecting a result.
        Selection fills the editor; save to keep it.
      </Text>
      <SearchBox
        label="Search lyrics"
        value={`${initial.artist} ${initial.title}`}
        onSearch={setLyricsQ}
        disabled={lyrics.isFetching}
      />
      {lyrics.isFetching && <Text role="status">Searching lyrics…</Text>}
      {lyrics.error && <Text role="alert">{lyrics.error.message}</Text>}
      {lyrics.data?.length === 0 && (
        <Text>No lyrics found. Try a different search.</Text>
      )}
      {lyrics.data?.map((l) => (
        <div key={l.id} className="my-3 rounded-md border border-border p-3">
          <strong>
            {l.artistName} — {l.trackName}
          </strong>
          <Text>
            {l.albumName} · {l.duration}s{" "}
            {l.instrumental
              ? "· Instrumental"
              : l.syncedLyrics
                ? "· Synced lyrics"
                : "· Plain lyrics"}
          </Text>
          <details>
            <summary>Preview lyrics</summary>
            <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-sm">
              {l.syncedLyrics || l.plainLyrics || "Instrumental"}
            </pre>
          </details>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              form.setFieldValue("lyrics", l.plainLyrics || "");
              form.setFieldValue("syncedLyrics", l.syncedLyrics || "");
              form.setFieldValue(
                "lyricsSource",
                `LRCLIB:${l.id}${l.instrumental ? ":instrumental" : ""}`,
              );
              toast.info("Lyrics selected. Save metadata to keep them.");
            }}
          >
            Use lyrics
          </Button>
        </div>
      ))}
      <SectionHeading>
        Find covers · MusicBrainz / Cover Art Archive
      </SectionHeading>
      <SearchBox
        label="Search covers"
        value={`${initial.artist} ${initial.album}`}
        onSearch={setCoversQ}
        disabled={covers.isFetching}
      />
      {covers.isFetching && (
        <Text role="status">Searching releases and artwork…</Text>
      )}
      {covers.error && <Text role="alert">{covers.error.message}</Text>}
      {covers.data?.length === 0 && (
        <Text>
          No front covers in the first five releases. Refine the artist and
          album search.
        </Text>
      )}
      <div className="grid grid-cols-[repeat(auto-fill,minmax(160px,1fr))] gap-4">
        {covers.data?.map((c) => (
          <div key={c.id} className="rounded-md border border-border p-3">
            <img
              src={c.url}
              alt={`${c.title} cover`}
              loading="lazy"
              referrerPolicy="no-referrer"
              className="aspect-square w-full object-contain"
            />
            <Text>
              {c.artist} — {c.title}
            </Text>
            <a
              href={c.source}
              target="_blank"
              rel="noreferrer"
              className="text-sm underline"
            >
              Release details
            </a>
            <Button
              type="button"
              variant="outline"
              className="mt-2"
              onClick={() => {
                form.setFieldValue("coverUrl", c.url);
                toast.info("Cover selected. Save metadata to keep it.");
              }}
            >
              Use cover
            </Button>
          </div>
        ))}
      </div>
    </>
  );
}
