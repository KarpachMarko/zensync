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
import { FileUpload, FileUploadTrigger } from "@/components/ui/file-upload";
import { ArtworkSource } from "./artwork-source";
import { api, uploadCoverFile } from "@/lib/api";
import {
  metadataSchema,
  type MetadataEdit,
  type LyricsCandidate,
  type CoverCandidate,
} from "@/lib/metadata";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAsString, useQueryState } from "nuqs";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

export function MetadataEditorHost() {
  const [id, setID] = useQueryState("editSong", parseAsString.withDefault(""));
  return id ? (
    <MetadataEditor key={id} id={id} close={() => void setID(null)} />
  ) : null;
}
function MetadataEditor({ id, close }: { id: string; close: () => void }) {
  const [dragging, setDragging] = useState(false);
  const depth = useRef(0);
  const [dropped, setDropped] = useState<{ file: File } | null>(null);
  const result = useQuery({
    queryKey: ["api", "/metadata/songs", id],
    queryFn: ({ signal }) =>
      api<{ edit: MetadataEdit; duration: number; searchArtist?: string }>(
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
      <DialogContent className="max-h-[85vh] gap-2 overflow-y-auto rounded-md p-3 sm:max-w-xl"
        onDragEnter={(event) => { if (event.dataTransfer.types.includes("Files")) { event.preventDefault(); depth.current++; setDragging(true); } }}
        onDragOver={(event) => { if (event.dataTransfer.types.includes("Files")) { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; } }}
        onDragLeave={() => { depth.current = Math.max(0, depth.current - 1); if (!depth.current) setDragging(false); }}
        onDrop={(event) => { event.preventDefault(); depth.current = 0; setDragging(false); const files = event.dataTransfer.files; if (files.length) setDropped({ file: files[0] }); }}>
        {dragging && <div role="status" className="pointer-events-none absolute inset-0 z-50 flex items-center justify-center rounded-md border-2 border-dashed border-primary bg-background/95 p-6 text-center">Drop a JPEG or PNG to use as the cover</div>}
        <DialogTitle className="m-0 pr-8 text-base">
          Edit song metadata
        </DialogTitle>
        <DialogDescription className="m-0 text-xs">
          Save changes to file tags and rescan Navidrome. Changes are verified
          before replacing the original file.
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
            searchArtist={result.data.searchArtist ?? result.data.edit.artist}
            dropped={dropped}
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
  searchArtist,
  dropped,
  close,
}: {
  id: string;
  initial: MetadataEdit;
  duration: number;
  searchArtist: string;
  dropped: { file: File } | null;
  close: () => void;
}) {
  const client = useQueryClient();
  const [tab, setTab] = useState("details");
  const [artworkText, setArtworkText] = useState(initial.coverUrl || `${searchArtist} ${initial.album}`.trim());
  const [uploadedName, setUploadedName] = useState("");
  const [uploadError, setUploadError] = useState("");
  const [lyricsQ, setLyricsQ] = useState(`${searchArtist} ${initial.title}`.trim());
  const [coversQ, setCoversQ] = useState(`${searchArtist} ${initial.album}`.trim());
  const save = useMutation({
    mutationFn: (edit: MetadataEdit) =>
      api(`/metadata/songs/${encodeURIComponent(id)}`, "PUT", edit),
    onSuccess: async () => {
      toast.success("Tag update queued");
      await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] });
      close();
    },
  });
  const defaults: MetadataEdit = { ...initial, albumArtist: initial.albumArtist ?? "", removeCover: false, coverUploadId: "" };
  const form = useAppForm({
    defaultValues: defaults,
    validators: { onSubmit: metadataSchema },
    onSubmit: async ({ value }) => {
      if (!save.isPending) await save.mutateAsync(value).catch(() => undefined);
    },
  });
  const upload = useMutation({
    mutationFn: (file: File) => uploadCoverFile(id, file),
    onSuccess: (result, file) => {
      form.setFieldValue("coverUploadId", result.id);
      form.setFieldValue("coverUrl", "");
      form.setFieldValue("removeCover", false);
      setUploadedName(file.name); setArtworkText(""); setCoversQ(""); setUploadError("");
    },
  });
  useEffect(() => {
    if (!dropped || upload.isPending || save.isPending) return;
    setTab("artwork");
    const file = dropped.file;
    if (!["image/jpeg", "image/png"].includes(file.type) || file.size > 20 * 1024 * 1024) { setUploadError("Choose a JPEG or PNG under 20 MiB"); return; }
    setUploadError(""); upload.mutate(file);
  }, [dropped]);
  const lyrics = useQuery({
    queryKey: ["api", "/metadata/lyrics", lyricsQ],
    enabled: tab === "lyrics" && !!lyricsQ,
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
    enabled: tab === "artwork" && !!coversQ,
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
      <Tabs value={tab} onValueChange={setTab} className="gap-2">
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
                  <field.AutocompleteField label="Title" fieldName="title" />
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
            <div className="col-span-2">
              <form.AppField name="albumArtist">
                {(field) => <field.AutocompleteField label="Album artist" fieldName="albumArtist" />}
              </form.AppField>
            </div>
            <Text className="col-span-2 m-0 text-xs">
              {duration ? `Duration: ${duration}s` : "Choose existing names or enter new ones."}
            </Text>
          </form>
        </TabsContent>
        <TabsContent
          value="artwork"
          forceMount
          className="mt-0 space-y-4 data-[state=inactive]:hidden"
        >
          <ArtworkSource value={artworkText} onSubmit={(value, isURL) => {
            setArtworkText(value);
            if (isURL) {
              form.setFieldValue("coverUrl", value);
              form.setFieldValue("coverUploadId", "");
              form.setFieldValue("removeCover", false);
              setCoversQ("");
            } else setCoversQ(value);
          }} />
          <form.Subscribe selector={(state) => ({ url: state.values.coverUrl, uploadID: state.values.coverUploadId ?? "", remove: state.values.removeCover ?? false })}>
            {({ url, uploadID, remove }) => (
              <div className="flex flex-wrap items-center gap-4 py-2">
                {remove ? <Text role="status" className="m-0 text-xs">Embedded cover will be removed when you save. Shared album artwork is kept.</Text> : (
                  <div className="flex items-center gap-3">
                    <ArtworkImage key={url || uploadID || "current"} src={uploadID ? `/api/v2/metadata/songs/${encodeURIComponent(id)}/cover-upload/${uploadID}` : url || `/api/v2/metadata/songs/${encodeURIComponent(id)}/cover`} alt={uploadID ? "Uploaded cover" : url ? "Selected cover" : "Current cover"} className="size-24 shrink-0" />
                    <Text className="m-0 max-w-40 break-words text-xs">{uploadID ? uploadedName : url ? "Selected cover" : "Current cover"}</Text>
                  </div>
                )}
                <div className="flex flex-wrap items-center gap-2">
                  <FileUpload label="Upload cover" value={[]} maxFiles={1} maxSize={20 * 1024 * 1024} accept="image/jpeg,image/png" disabled={upload.isPending || save.isPending}
                    onFileReject={(_, message) => setUploadError(message)}
                    onValueChange={(files) => { if (files[0]) { setUploadError(""); upload.mutate(files[0]); } }}>
                    <FileUploadTrigger asChild><Button type="button" size="sm" variant="outline">{upload.isPending ? "Uploading…" : "Upload cover"}</Button></FileUploadTrigger>
                  </FileUpload>
                  <Button type="button" size="sm" variant="outline" disabled={upload.isPending || save.isPending} onClick={() => {
                    form.setFieldValue("removeCover", !remove);
                    form.setFieldValue("coverUrl", ""); form.setFieldValue("coverUploadId", "");
                    setArtworkText(""); setCoversQ("");
                  }}>{remove ? "Keep current cover" : "Remove cover"}</Button>
                </div>
              </div>
            )}
          </form.Subscribe>
          <Text className="m-0 text-xs">Upload a JPEG or PNG, up to 20 MiB. Artwork changes apply when you save.</Text>
          {(uploadError || upload.error) && <Text role="alert" className="m-0 text-xs">{uploadError || upload.error?.message}</Text>}
          {covers.isFetching && (
            <Text role="status" className="m-0 text-xs">
              Searching MusicBrainz and Cover Art Archive…
            </Text>
          )}
          {covers.error && (
            <Text role="alert" className="m-0 text-xs">
              {covers.error.message}
              <Button type="button" variant="outline" size="sm" onClick={() => void covers.refetch()}>Retry cover search</Button>
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
                <ArtworkImage src={cover.url} alt={`${cover.title} cover`} className="aspect-square w-full" />
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
                    form.setFieldValue("coverUploadId", "");
                    form.setFieldValue("removeCover", false);
                    setArtworkText(cover.url);
                    setCoversQ("");
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
            value={`${searchArtist} ${initial.title}`}
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
          disabled={save.isPending || upload.isPending}
        >
          {save.isPending ? "Queuing…" : "Save changes"}
        </Button>
      </div>
    </>
  );
}


function ArtworkImage({ src, alt, className }: { src: string; alt: string; className: string }) {
  const [failed, setFailed] = useState(false);
  return failed ? (
    <div role="status" className={`flex flex-col items-center justify-center rounded border border-solid border-border p-1 text-center text-xs text-muted-foreground ${className}`}>
      <span>Artwork unavailable</span>
      <Button type="button" size="sm" variant="ghost" onClick={() => setFailed(false)}>Retry image</Button>
    </div>
  ) : (
    <img src={src} alt={alt} referrerPolicy="no-referrer" onError={() => setFailed(true)} className={`rounded border border-solid border-border object-contain ${className}`} />
  );
}
