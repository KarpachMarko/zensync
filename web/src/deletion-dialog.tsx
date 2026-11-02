import { useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";

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
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const heading = useId();
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
    const previousOverflow = document.body.style.overflow;
    const modal = dialog.current;
    setError("");
    setPending(false);
    modal?.showModal();
    cancel.current?.focus();
    document.body.style.overflow = "hidden";
    return () => {
      modal?.close();
      document.body.style.overflow = previousOverflow;
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

  if (!open) return null;
  return createPortal(
    <dialog
      ref={dialog}
      className="deletion-dialog"
      role="alertdialog"
      aria-labelledby={heading}
      aria-describedby={`${identity} ${description}`}
      aria-modal="true"
      aria-busy={pending}
      onCancel={(event) => {
        event.preventDefault();
        if (!pending) onClose();
      }}
      onKeyDown={(event) => {
        if (event.key !== "Tab") return;
        const buttons = event.currentTarget.querySelectorAll<HTMLButtonElement>(
          "button:not(:disabled)",
        );
        const first = buttons[0],
          last = buttons[buttons.length - 1];
        if (!first) {
          event.preventDefault();
          return;
        }
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }}
    >
      <h2 id={heading}>{title}</h2>
      <p id={identity} className="deletion-song">
        <strong>{song.title}</strong>
        <span>
          {song.artist}
          {song.album ? ` / ${song.album}` : ""}
        </span>
      </p>
      <p id={description}>
        {request
          ? "An administrator must approve this request before the song is removed from the shared library for everyone."
          : "This removes the song from the shared library for everyone. A recovery copy will be retained."}
      </p>
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
        <button
          ref={cancel}
          type="button"
          className="quiet"
          disabled={pending}
          onClick={onClose}
        >
          Cancel
        </button>
        <button
          type="button"
          className={request ? "" : "danger"}
          disabled={pending}
          onClick={() => void confirm()}
        >
          {pending ? "Submitting…" : action}
        </button>
      </div>
    </dialog>,
    document.body,
  );
}
