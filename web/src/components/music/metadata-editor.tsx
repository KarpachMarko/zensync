import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/typography";
import { SearchBox } from "@/components/search-box";
import { useAppForm } from "@/hooks/use-app-form";
import { api } from "@/lib/api";
import {
  metadataSchema,
  type MetadataEdit,
  type LyricsCandidate,
  type CoverCandidate,
} from "@/lib/metadata";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAsString, useQueryState } from "nuqs";
import { useState } from "react";
import { toast } from "sonner";

export function MetadataEditorHost() {
  const [id, setID] = useQueryState("editSong", parseAsString.withDefault(""));
  return id ? (
    <MetadataEditor key={id} id={id} close={() => void setID(null)} />
  ) : null;
}
function MetadataEditor({ id, close }: { id: string; close: () => void }) {
  const result = useQuery({
    queryKey: ["api", "/metadata/songs", id],
    queryFn: ({ signal }) =>
      api<{ edit: MetadataEdit; duration: number }>(
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
        if (!open) close();
      }}
    >
      <DialogContent className="max-h-[85vh] gap-2 overflow-y-auto rounded-md p-3 sm:max-w-xl">
        <DialogTitle className="m-0 pr-8 text-base">
          Edit song metadata
        </DialogTitle>
        <DialogDescription className="m-0 text-xs">
          Save changes to file tags and rescan Navidrome. Original files are
          backed up.
        </DialogDescription>
        {result.error && (
          <Text role="alert" className="my-1">
            {result.error.message}
          </Text>
        )}
        {result.isPending && <Text className="my-1">Loading…</Text>}
        {result.data && (
          <MetadataForm
            id={id}
            initial={result.data.edit}
            duration={result.data.duration}
            close={close}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
function MetadataForm({
  id,
  initial,
  duration,
  close,
}: {
  id: string;
  initial: MetadataEdit;
  duration: number;
  close: () => void;
}) {
  const client = useQueryClient();
  const [lyricsQ, setLyricsQ] = useState("");
  const [coversQ, setCoversQ] = useState("");
  const save = useMutation({
    mutationFn: (edit: MetadataEdit) =>
      api(`/metadata/songs/${encodeURIComponent(id)}`, "PUT", edit),
    onSuccess: async () => {
      toast.success("Tag update queued");
      await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] });
      close();
    },
  });
  const form = useAppForm({
    defaultValues: initial,
    validators: { onSubmit: metadataSchema },
    onSubmit: async ({ value }) => {
      if (!save.isPending) await save.mutateAsync(value).catch(() => undefined);
    },
  });
  const lyrics = useQuery({
    queryKey: ["api", "/metadata/lyrics", lyricsQ],
    enabled: !!lyricsQ,
    retry: false,
    staleTime: 300000,
    queryFn: ({ signal }) =>
      api<LyricsCandidate[]>(
        `/metadata/lyrics?q=${encodeURIComponent(lyricsQ)}`,
        "GET",
        undefined,
        undefined,
        signal,
      ),
  });
  const covers = useQuery({
    queryKey: ["api", "/metadata/covers", coversQ],
    enabled: !!coversQ,
    retry: false,
    staleTime: 300000,
    queryFn: ({ signal }) =>
      api<CoverCandidate[]>(
        `/metadata/covers?q=${encodeURIComponent(coversQ)}`,
        "GET",
        undefined,
        undefined,
        signal,
      ),
  });
  const compactLabel = "m-0 gap-1 text-xs";
  return (
    <>
      <Tabs defaultValue="details" className="gap-2">
        <TabsList className="h-8">
          <TabsTrigger value="details">Details</TabsTrigger>
          <TabsTrigger value="artwork">Artwork</TabsTrigger>
          <TabsTrigger value="lyrics">Lyrics</TabsTrigger>
        </TabsList>
        <TabsContent
          value="details"
          forceMount
          className="mt-0 data-[state=inactive]:hidden"
        >
          <form
            id="metadata-form"
            className="grid grid-cols-2 gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              void form.handleSubmit();
            }}
          >
            <div className="col-span-2">
              <form.AppField name="title">
                {(field) => (
                  <field.TextField
                    label="Title"
                    labelClassName={compactLabel}
                    className="h-8 min-h-8 py-1 text-sm"
                  />
                )}
              </form.AppField>
            </div>
            <form.AppField name="artist">
              {(field) => (
                <field.AutocompleteField label="Artist" fieldName="artist" />
              )}
            </form.AppField>
            <form.AppField name="album">
              {(field) => (
                <field.AutocompleteField label="Album" fieldName="album" />
              )}
            </form.AppField>
            <Text className="col-span-2 m-0 text-xs">
              {duration ? `Duration: ${duration}s` : "Choose existing names or enter new ones."}
            </Text>
          </form>
        </TabsContent>
        <TabsContent
          value="artwork"
          forceMount
          className="mt-0 space-y-2 data-[state=inactive]:hidden"
        >
          <form.AppField name="coverUrl">
            {(field) => (
              <field.TextField
                label="Cover URL"
                labelClassName={compactLabel}
                className="h-8 min-h-8 py-1 text-sm"
                placeholder="Select a cover below"
              />
            )}
          </form.AppField>
          <SearchBox
            compact
            label="Search covers"
            value={`${initial.artist} ${initial.album}`}
            onSearch={setCoversQ}
            disabled={covers.isFetching}
          />
          {covers.isFetching && (
            <Text role="status" className="m-0 text-xs">
              Searching MusicBrainz and Cover Art Archive…
            </Text>
          )}
          {covers.error && (
            <Text role="alert" className="m-0 text-xs">
              {covers.error.message}
            </Text>
          )}
          {covers.data?.length === 0 && (
            <Text className="m-0 text-xs">
              No covers found. Refine the artist and album.
            </Text>
          )}
          <div className="grid grid-cols-3 gap-2">
            {covers.data?.map((cover) => (
              <div key={cover.id} className="min-w-0 space-y-1">
                <img
                  src={cover.url}
                  alt={`${cover.title} cover`}
                  referrerPolicy="no-referrer"
                  className="aspect-square w-full rounded object-contain"
                />
                <p className="m-0 truncate text-xs">
                  {cover.artist} · {cover.title}
                </p>
                <a
                  className="text-xs underline"
                  href={cover.source}
                  target="_blank"
                  rel="noreferrer"
                >
                  Release details
                </a>
                <Button
                  size="sm"
                  variant="outline"
                  className="w-full"
                  onClick={() => {
                    form.setFieldValue("coverUrl", cover.url);
                    toast.info("Cover selected");
                  }}
                >
                  Use cover
                </Button>
              </div>
            ))}
          </div>
        </TabsContent>
        <TabsContent
          value="lyrics"
          forceMount
          className="mt-0 space-y-2 data-[state=inactive]:hidden"
        >
          <SearchBox
            compact
            label="Search lyrics"
            value={`${initial.artist} ${initial.title}`}
            onSearch={setLyricsQ}
            disabled={lyrics.isFetching}
          />
          {lyrics.isFetching && (
            <Text role="status" className="m-0 text-xs">
              Searching LRCLIB…
            </Text>
          )}
          {lyrics.error && (
            <Text role="alert" className="m-0 text-xs">
              {lyrics.error.message}
            </Text>
          )}
          {lyrics.data?.length === 0 && (
            <Text className="m-0 text-xs">
              No lyrics found. Try a different title or artist.
            </Text>
          )}
          <div className="max-h-40 overflow-auto">
            {lyrics.data?.map((lyric) => (
              <div key={lyric.id} className="border-b border-border py-1">
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <strong className="text-xs">
                      {lyric.artistName} — {lyric.trackName}
                    </strong>
                    <p className="m-0 text-xs text-muted-foreground">
                      {lyric.albumName} · {lyric.duration}s ·{" "}
                      {lyric.instrumental
                        ? "Instrumental"
                        : lyric.syncedLyrics
                          ? "Synced"
                          : "Plain"}
                    </p>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      form.setFieldValue("lyrics", lyric.plainLyrics || "");
                      form.setFieldValue(
                        "syncedLyrics",
                        lyric.syncedLyrics || "",
                      );
                      form.setFieldValue("lyricsSource", `LRCLIB:${lyric.id}`);
                    }}
                  >
                    Use lyrics
                  </Button>
                </div>
                <details className="text-xs">
                  <summary>Preview lyrics</summary>
                  <pre className="max-h-28 overflow-auto whitespace-pre-wrap">
                    {lyric.syncedLyrics || lyric.plainLyrics || "Instrumental"}
                  </pre>
                </details>
              </div>
            ))}
          </div>
          <form.AppField name="lyrics">
            {(field) => (
              <field.TextareaField
                label="Plain lyrics"
                labelClassName={compactLabel}
                rows={3}
                className="min-h-16 px-2 py-1 text-sm"
              />
            )}
          </form.AppField>
          <form.AppField name="syncedLyrics">
            {(field) => (
              <field.TextareaField
                label="Synced lyrics (LRC)"
                labelClassName={compactLabel}
                rows={3}
                className="min-h-16 px-2 py-1 text-sm"
              />
            )}
          </form.AppField>
        </TabsContent>
      </Tabs>
      {save.error && (
        <Text role="alert" className="m-0 text-xs">
          {save.error.message}
        </Text>
      )}
      <div className="flex justify-end gap-2 border-t border-border pt-2">
        <Button size="sm" variant="ghost" onClick={close}>
          Cancel
        </Button>
        <Button
          size="sm"
          type="submit"
          form="metadata-form"
          disabled={save.isPending}
        >
          {save.isPending ? "Queuing…" : "Save changes"}
        </Button>
      </div>
    </>
  );
}
