import { useAppForm } from "@/hooks/use-app-form";
import { time, type LibrarySong } from "@/lib/api";
import { libraryQuality } from "@/lib/playback";
import { mergeSongMetadataSchema, type MergeSongMetadata } from "@/lib/metadata";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/typography";

const fields = [{ name: "title", label: "Title" }, { name: "artist", label: "Artist" }, { name: "album", label: "Album" }, { name: "albumArtist", label: "Album artist" }] as const;
export function SongMergeMetadata({ original, selected, formID, busy, onConfirm }: {
  original: LibrarySong; selected: LibrarySong; formID: string; busy: boolean; onConfirm: (patch: Partial<MergeSongMetadata>) => Promise<unknown>;
}) {
  const initial = { title: selected.title, artist: selected.artist, album: selected.album, albumArtist: selected.albumArtist ?? "" };
  const form = useAppForm({ defaultValues: initial, validators: { onSubmit: mergeSongMetadataSchema },
    onSubmit: async ({ value }) => {
      const patch: Partial<MergeSongMetadata> = {};
      for (const { name } of fields) if (value[name] !== initial[name]) patch[name] = value[name];
      try { await onConfirm(patch); } catch { /* The parent mutation displays the error. */ }
    },
  });
  const differences = fields.filter(({ name }) => (original[name] ?? "") !== initial[name]);
  return <section aria-label="Merge metadata review" className="space-y-2 rounded-md border border-border p-3">
    <h2 className="m-0 text-sm font-semibold">Review metadata for the selected song</h2>
    <Text className="m-0 text-xs">{differences.length ? `${differences.length} metadata fields differ. Check the edition and credits before merging. Keep the selected values, use the original values, or edit them below.` : "The song metadata matches. You can still edit the selected song below."}</Text>
    <form id={formID} noValidate onSubmit={event => { event.preventDefault(); event.stopPropagation(); if (!busy) void form.handleSubmit(); }} className="space-y-2">
      {fields.map(({ name, label }) => {
        const previous = original[name] ?? "", kept = initial[name], different = previous !== kept;
        return <div key={name} className={`rounded-md border p-2 ${different ? "border-amber-500/40 bg-amber-500/5" : "border-border"}`}>
          <div className="flex items-center gap-2 text-xs font-medium">{label}{different && <Badge variant="outline">Different</Badge>}</div>
          <div className="mt-1 grid min-w-0 gap-2 text-xs sm:grid-cols-2">
            <div className="min-w-0 break-words"><span className="text-muted-foreground">Original: </span>{previous || "Empty"}</div>
            <div className="min-w-0 break-words"><span className="text-muted-foreground">Selected: </span>{kept || "Empty"}</div>
          </div>
          <form.AppField name={name}>{field => <field.TextField label={`After merge: ${label}`} labelClassName="my-2" disabled={busy} maxLength={name === "artist" || name === "albumArtist" ? 10500 : 500} />}</form.AppField>
          {different && <div className="flex flex-wrap gap-2"><Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => form.setFieldValue(name, previous)}>Use original {label.toLowerCase()}</Button><Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => form.setFieldValue(name, kept)}>Use selected {label.toLowerCase()}</Button></div>}
        </div>;
      })}
    </form>
    {(original.duration !== selected.duration || libraryQuality(original) !== libraryQuality(selected)) && <Text className="m-0 text-xs">Audio differs: original {time(original.duration)} / {libraryQuality(original) || "unknown format"}; selected {time(selected.duration)} / {libraryQuality(selected) || "unknown format"}. The selected audio stays unchanged.</Text>}
    <Text className="m-0 text-xs">The selected song’s file, cover and library ID stay. The original file is removed with a recovery copy. Editable playlists for connected accounts are updated; listening histories are not combined.</Text>
  </section>;
}
