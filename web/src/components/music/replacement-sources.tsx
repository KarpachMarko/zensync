import { useQueries, useMutation } from "@tanstack/react-query";
import { useAppForm } from "@/hooks/use-app-form";
import type { useRecordingPreview } from "@/hooks/use-recording-preview";
import { api, label, providerLabel, requestKey, time, trackTarget, uploadAudioFile, type LibrarySong, type Track } from "@/lib/api";
import { audioUploadSchema } from "@/lib/form-schemas";
import { recordingKey, type Recording } from "@/lib/playback";
import { rankSearchResults } from "@/lib/search-ranking";
import { RecordingTools } from "@/components/imports/recording-tools";
import { Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { RecordingCover } from "./audio-player";

export type ExternalReplacement = { type: "provider"; track: Track } | { type: "upload"; id: string; song: LibrarySong };
export type ReplacementPlayback = ReturnType<typeof useRecordingPreview<Recording>>;
export function ProviderRecordingDetails({ track, playback }: { track: Track; playback: ReplacementPlayback }) {
  return <div className="flex min-w-0 flex-1 items-start gap-3">
    <RecordingCover url={track.album.cover_medium} />
    <div className="min-w-0 flex-1 break-words"><strong className="text-sm">{label(track)}</strong>
      <p className="m-0 mt-1 text-xs text-muted-foreground">{track.artist.name} / {track.album.title} / {time(track.duration)} / {providerLabel(track.provider)}</p>
      <RecordingTools track={track} playing={playback.playing && !!playback.active && recordingKey(playback.active) === trackTarget(track)} onPreview={playback.toggle} />
    </div>
  </div>;
}
export function ProviderReplacements({ query, selected, busy, playback, onSelect }: {
  query: string; selected: ExternalReplacement | null; busy: boolean; playback: ReplacementPlayback; onSelect: (value: ExternalReplacement) => void;
}) {
  const providers = ["deezer", "lossless", "hitmo"] as const;
  const direct = /^(https:\/\/eu\.hitmoz\.com\/|hitmo:)/.test(query) ? "hitmo" : /^(https:\/\/lossless\.am\/|lossless:)/.test(query) ? "lossless" : /^(https?:|deezer:)/.test(query) ? "deezer" : null;
  const results = useQueries({ queries: providers.filter(provider => !direct || provider === direct).map(provider => ({
    queryKey: ["api", "replacement-search", provider, query],
    queryFn: ({ signal }: { signal: AbortSignal }) => api<Track[]>(`/search?provider=${provider}&q=${encodeURIComponent(query)}`, "GET", undefined, undefined, signal),
    enabled: !!query,
  })) });
  const tracks = rankSearchResults([...new Map(results.flatMap(result => result.data ?? []).map(track => [trackTarget(track), track])).values()], query);
  return <section aria-label="Provider recordings" className="space-y-2">
    {results.some(result => result.isFetching) && <Text role="status">Searching providers…</Text>}
    {results.map((result, index) => result.error && <Text key={index} role="alert">{result.error.message}</Text>)}
    {tracks.map(track => {
      const active = selected?.type === "provider" && trackTarget(selected.track) === trackTarget(track);
      return <article key={trackTarget(track)} className={`flex min-w-0 flex-wrap items-start gap-3 rounded-md border p-3 ${active ? "border-primary bg-muted/50" : "border-border"}`}>
        <ProviderRecordingDetails track={track} playback={playback} />
        <Button variant="outline" size="sm" aria-label={`Select provider recording: ${label(track)}`} aria-pressed={active} disabled={busy} onClick={() => onSelect({ type: "provider", track })}>{active ? "Selected" : "Select recording"}</Button>
      </article>;
    })}
    {query && results.every(result => result.isSuccess) && !tracks.length && <Text>No matching provider recordings found.</Text>}
  </section>;
}
export function ReplacementUpload({ source, busy, onBusyChange, onSelect }: { source: string; busy: boolean; onBusyChange: (busy: boolean) => void; onSelect: (value: ExternalReplacement) => void }) {
  const upload = useMutation({ mutationFn: async (file: File) => {
    const { id } = await uploadAudioFile(`/metadata/replacement-upload?source=${encodeURIComponent(source)}`, file, requestKey());
    const result = await api<{ id: string; track: Track; format: string; cover?: boolean }>(`/metadata/replacement-inputs/${encodeURIComponent(id)}`);
    return { type: "upload" as const, id, song: { id: `upload:${id}`, title: label(result.track), artist: result.track.artist.name, album: result.track.album.title, duration: result.track.duration, suffix: result.format, ...(result.cover ? { coverArt: `upload:${id}` } : {}) } };
  } });
  const form = useAppForm({ defaultValues: { file: null as File | null }, validators: { onSubmit: audioUploadSchema },
    onSubmit: async ({ value }) => {
      if (!value.file || busy) return;
      onBusyChange(true);
      try { onSelect(await upload.mutateAsync(value.file)); form.reset(); } catch { /* Mutation error is rendered below. */ } finally { onBusyChange(false); }
    },
  });
  return <section aria-label="Upload replacement" className="space-y-2">
    <Text className="my-1 text-xs">Choose one complete audio file, up to 1 GiB. Uploading validates and stages it; Confirm Merge/Replace starts the replacement.</Text>
    <form noValidate onSubmit={event => { event.preventDefault(); event.stopPropagation(); void form.handleSubmit(); }}>
      <form.AppField name="file">{field => <field.FileField label="Replacement audio file" description="FLAC, MP3, M4A, OGG, OPUS, WAV or AIFF" accept=".flac,.mp3,.m4a,.ogg,.opus,.wav,.aif,.aiff" disabled={busy} onFileValidate={file => { const result = audioUploadSchema.shape.file.safeParse(file); return result.success ? null : result.error.issues[0]?.message; }} />}</form.AppField>
      {upload.error && <Text role="alert">{upload.error.message}</Text>}
      <Button disabled={busy}>{upload.isPending ? "Uploading and validating…" : "Upload and select"}</Button>
    </form>
  </section>;
}
