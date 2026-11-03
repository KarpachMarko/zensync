import { Text } from "@/components/typography";
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
        className="block w-[min(480px,calc(100vw_-_32px))] max-w-none data-[size=default]:max-w-none data-[size=default]:sm:max-w-none max-h-[calc(100dvh_-_48px)] overflow-y-auto rounded-xl border border-solid border-border bg-card p-7 text-foreground shadow-[0_24px_80px_#0008] [&_h2]:mt-0 [&_h2]:mb-5 [&_h2]:text-2xl [&_p]:leading-[1.6]"
        aria-describedby={`${identity} ${description}`}
        aria-busy={pending}
        onEscapeKeyDown={(event) => {
          if (pending) event.preventDefault();
        }}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <AlertDialogTitle>{title}</AlertDialogTitle>
        <Text
          id={identity}
          className="rounded-md bg-background px-4 py-3.5 [&_strong]:block [&_strong]:wrap-anywhere [&_span]:block [&_span]:wrap-anywhere [&_span]:mt-[3px] [&_span]:text-sm [&_span]:text-muted-foreground"
        >
          <strong>{song.title}</strong>
          <span>
            {song.artist}
            {song.album ? ` / ${song.album}` : ""}
          </span>
        </Text>
        <AlertDialogDescription id={description}>
          {request
            ? "An administrator must approve this request before the song is removed from the shared library for everyone."
            : "This removes the song from the shared library for everyone. A recovery copy will be retained."}
        </AlertDialogDescription>
        {error && (
          <Text role="alert" className="text-[#ffc3bd]">
            {error}
          </Text>
        )}
        {pending && (
          <Text role="status">
            {request ? "Submitting deletion request…" : "Submitting deletion…"}
          </Text>
        )}
        <div className="mt-6 flex flex-wrap justify-end gap-2.5">
          <AlertDialogCancel variant="outline" type="button" disabled={pending}>
            Cancel
          </AlertDialogCancel>
          <Button
            type="button"
            variant={request ? "default" : "destructive"}
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
