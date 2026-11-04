import { ExpandableSection } from "@/components/expandable-section";
import { CheckboxField } from "@/components/forms";
import { Caption, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { useAppForm } from "@/hooks/use-app-form";
import { migrationChoicesSchema } from "@/lib/form-schemas";
import type { Playlist } from "@/lib/migration";

export function MigrationChoices({
  playlists,
  busy,
  onCommit,
}: {
  playlists: Playlist[];
  busy: boolean;
  onCommit: (values: {
    playlists: string[];
    favorites: boolean;
  }) => Promise<void>;
}) {
  const selectable = playlists.filter((playlist) => !playlist.metadata.system && !playlist.metadata.folder);
  const views = playlists.filter((playlist) => playlist.metadata.system || playlist.metadata.folder);
  const form = useAppForm({
    defaultValues: {
      playlists: playlists
        .filter(
          (playlist) =>
            playlist.selected &&
            !playlist.metadata.system &&
            !playlist.metadata.folder,
        )
        .map((playlist) => playlist.id),
      favorites: true,
    },
    validators: { onSubmit: migrationChoicesSchema },
    onSubmit: async ({ value }) => onCommit(value),
  });
  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        void form.handleSubmit();
      }}
    >
      <h2 className="m-0 mb-2 text-base">Playlists to create in Navidrome</h2>
      <Text className="mt-0 text-sm">
        Optional: choose playlists to recreate. These choices do not limit
        which songs are imported. You can leave them all unchecked to migrate
        only your songs and favorites.
      </Text>
      {selectable.length === 0 && <Text className="text-sm">No user playlists in this export. All songs are still included.</Text>}
      <form.Field name="playlists">
        {(field) => (
          <>
            {selectable.map((playlist) => (
              <CheckboxField key={playlist.id}>
                <Checkbox
                  checked={field.state.value.includes(playlist.id)}
                  disabled={busy}
                  onCheckedChange={(checked) =>
                    field.handleChange(
                      checked === true
                        ? [...field.state.value, playlist.id]
                        : field.state.value.filter((id) => id !== playlist.id),
                    )
                  }
                  onBlur={field.handleBlur}
                />
                <span>
                  {playlist.name}
                  <Caption>
                    {playlist.entries.length} entries
                    {playlist.metadata.smart ? " / Smart playlist snapshot" : ""}
                  </Caption>
                </span>
              </CheckboxField>
            ))}
          </>
        )}
      </form.Field>
      {views.length > 0 && (
        <ExpandableSection title="Apple Music views and folders" count={views.length} compact defaultOpen={false}>
          <Text className="mt-0 text-sm">
            Songs in Library, Music, Downloaded and other system views are
            already included in All songs. System views are not recreated as
            Navidrome playlists. Folders organise playlists; select the individual
            playlists above.
          </Text>
          {views.map((playlist) => (
            <div key={playlist.id} className="my-2 text-sm">
              {playlist.name}
              <Caption>{playlist.entries.length} entries / {playlist.metadata.folder ? "Folder" : "System view — songs already included"}</Caption>
            </div>
          ))}
        </ExpandableSection>
      )}
      <form.AppField name="favorites">
        {(field) => (
          <field.CheckboxField label="Import positive favorite/loved flags when songs become available" />
        )}
      </form.AppField>
      <Button disabled={busy}>Commit migration</Button>
    </form>
  );
}
