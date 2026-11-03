import { CheckboxField } from "@/components/forms";
import { Caption } from "@/components/typography";
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
      <form.Field name="playlists">
        {(field) => (
          <>
            {playlists.map((playlist) => (
              <CheckboxField key={playlist.id}>
                <Checkbox
                  checked={field.state.value.includes(playlist.id)}
                  disabled={
                    playlist.metadata.system || playlist.metadata.folder
                  }
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
                    {playlist.metadata.folder
                      ? " / Folder"
                      : playlist.metadata.system
                        ? " / System playlist"
                        : playlist.metadata.smart
                          ? " / Smart playlist snapshot"
                          : ""}
                  </Caption>
                </span>
              </CheckboxField>
            ))}
          </>
        )}
      </form.Field>
      <form.AppField name="favorites">
        {(field) => (
          <field.CheckboxField label="Import positive favorite/loved flags when songs become available" />
        )}
      </form.AppField>
      <Button disabled={busy}>Commit migration</Button>
    </form>
  );
}
