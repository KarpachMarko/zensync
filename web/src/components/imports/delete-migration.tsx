import { Text } from "@/components/typography";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { useApiMutation } from "@/hooks/use-api";
import type { Migration } from "@/lib/migration";
import { useState } from "react";
import { toast } from "sonner";

export function DeleteMigration({
  migration,
  onDeleted,
}: {
  migration: Migration;
  onDeleted?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const { api, isPending } = useApiMutation();
  if (!["staged", "open"].includes(migration.state)) return null;

  async function remove() {
    if (isPending) return;
    setError("");
    try {
      await api("/imports/" + migration.id, "DELETE");
      setOpen(false);
      onDeleted?.();
      toast.success("Migration deleted.");
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to delete migration.",
      );
    }
  }

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!isPending) {
          setOpen(next);
          setError("");
        }
      }}
    >
      <AlertDialogTrigger asChild>
        <Button
          variant="outline"
          aria-label={"Delete migration: " + migration.name}
        >
          Delete migration
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent
        aria-busy={isPending}
        onEscapeKeyDown={(event) => {
          if (isPending) event.preventDefault();
        }}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>Delete this migration?</AlertDialogTitle>
          <AlertDialogDescription>
            Delete “{migration.name}” and its import progress permanently.
            Matching and playlist updates will stop. Songs, favorites and
            playlists already in Navidrome will remain. Existing download
            requests will remain in Requests and continue independently.
            You can upload the export again to start over.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {error && <Text role="alert">{error}</Text>}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isPending}>Cancel</AlertDialogCancel>
          <Button
            variant="destructive"
            disabled={isPending}
            onClick={() => void remove()}
          >
            {isPending ? "Deleting…" : "Delete migration"}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
