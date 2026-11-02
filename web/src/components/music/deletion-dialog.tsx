import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { useEffect, useId, useState } from "react";

type Mode = "delete" | "request" | "approve";
export function DeletionDialog({
  open,
  mode,
  song,
  onClose,
  onConfirm,
}: {
  open: boolean;
  mode: Mode;
  song: { title: string; artist: string; album?: string };
  onClose: () => void;
  onConfirm: () => Promise<void>;
}) {
  const description = useId();
  const identity = useId();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const request = mode === "request";
  const title = request
    ? "Request deletion?"
    : mode === "approve"
      ? "Approve song deletion?"
      : "Delete this song?";
  const action = request
    ? "Request deletion"
    : mode === "approve"
      ? "Approve deletion"
      : "Delete song";

  useEffect(() => {
    if (!open) return;
    const trigger =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    setError("");
    setPending(false);
    return () => {
      if (trigger?.isConnected && !trigger.matches(":disabled"))
        trigger.focus();
      else document.querySelector<HTMLElement>("main h1")?.focus();
    };
  }, [open]);

  async function confirm() {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      await onConfirm();
      onClose();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Unable to submit deletion. Try again.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !pending) onClose();
      }}
    >
      <AlertDialogContent
        className="deletion-dialog"
        aria-describedby={`${identity} ${description}`}
        aria-busy={pending}
        onEscapeKeyDown={(event) => {
          if (pending) event.preventDefault();
        }}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <AlertDialogTitle>{title}</AlertDialogTitle>
        <p id={identity} className="deletion-song">
          <strong>{song.title}</strong>
          <span>
            {song.artist}
            {song.album ? ` / ${song.album}` : ""}
          </span>
        </p>
        <AlertDialogDescription id={description}>
          {request
            ? "An administrator must approve this request before the song is removed from the shared library for everyone."
            : "This removes the song from the shared library for everyone. A recovery copy will be retained."}
        </AlertDialogDescription>
        {error && (
          <p role="alert" className="deletion-error">
            {error}
          </p>
        )}
        {pending && (
          <p role="status">
            {request ? "Submitting deletion request…" : "Submitting deletion…"}
          </p>
        )}
        <div className="deletion-dialog-actions">
          <AlertDialogCancel type="button" className="quiet" disabled={pending}>
            Cancel
          </AlertDialogCancel>
          <Button
            type="button"
            className={request ? "" : "danger"}
            disabled={pending}
            onClick={() => void confirm()}
          >
            {pending ? "Submitting…" : action}
          </Button>
        </div>
      </AlertDialogContent>
    </AlertDialog>
  );
}
