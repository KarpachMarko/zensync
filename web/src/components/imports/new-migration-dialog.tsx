import { Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useAppForm } from "@/hooks/use-app-form";
import { upload } from "@/lib/api";
import { uploadSchema } from "@/lib/form-schemas";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export function NewMigrationDialog({ onClose }: { onClose: () => void }) {
  const client = useQueryClient();
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
        await client.invalidateQueries({ queryKey: ["api", "/imports"] });
        navigate("/imports/" + migration.id);
        toast.success(
          "Export uploaded. Review the migration preview before committing.",
        );
      } catch (cause) {
        setError(
          cause instanceof Error ? cause.message : "Unable to upload export",
        );
      } finally {
        setBusy(false);
      }
    },
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent className="sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle>New migration</DialogTitle>
          <DialogDescription>
            In Apple Music, choose File → Library → Export Library. For one
            playlist, select it, choose File → Library → Export Playlist and
            select XML or Text. Text exports use the filename as the playlist
            name.
          </DialogDescription>
        </DialogHeader>
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
                label="Apple Music or iTunes export (XML or TXT)"
                disabled={busy}
                description="One XML or TXT file, up to 32 MiB"
                onFileValidate={(file) => {
                  const result = uploadSchema.shape.file.safeParse(file);
                  return result.success ? null : result.error.issues[0]?.message;
                }}
                accept=".xml,.txt,application/xml,text/xml,text/plain"
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
            {busy ? "Reading your export…" : "Upload and preview"}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
