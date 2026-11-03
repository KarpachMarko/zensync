import { AppLink } from "@/components/navigation";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useAppForm } from "@/hooks/use-app-form";
import { upload } from "@/lib/api";
import { uploadSchema } from "@/lib/form-schemas";
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
export function NewImport() {
  const { mutateAsync: uploadFile } = useMutation({ mutationFn: upload });
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  const form = useAppForm({
    defaultValues: { file: null as File | null },
    validators: { onSubmit: uploadSchema },
    onSubmit: async ({ value }) => {
      if (!value.file) return;
      setBusy(true);
      setError("");
      try {
        const migration = await uploadFile(value.file);
        navigate("/imports/" + migration.id);
        toast.success(
          "XML uploaded. Review the migration preview before committing.",
        );
      } catch (cause) {
        setError(
          cause instanceof Error ? cause.message : "Unable to upload XML",
        );
      } finally {
        setBusy(false);
      }
    },
  });
  return (
    <>
      <AppLink to="/imports">Imports</AppLink>
      <PageHeading tabIndex={-1}>Start a migration</PageHeading>
      <Text>
        In Apple Music, choose File → Library → Export Library. For one
        playlist, choose File → Library → Export Playlist and select XML.
      </Text>
      <form
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void form.handleSubmit();
        }}
      >
        <form.AppField name="file">
          {(field) => (
            <field.FileField
              label="Apple Music or iTunes XML export"
              accept=".xml,application/xml,text/xml"
            />
          )}
        </form.AppField>
        <Text>
          Uploading saves a private snapshot. It does not download music or
          change Navidrome. You’ll choose playlists and favorites in the
          preview.
        </Text>
        {error && <Text role="alert">{error}</Text>}
        <Button disabled={busy}>
          {busy ? "Reading your library…" : "Upload and preview"}
        </Button>
      </form>
    </>
  );
}
