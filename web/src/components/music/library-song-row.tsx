import { Actions, Row } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { Caption, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiMutation } from "@/hooks/use-api";
import type { LibrarySong } from "@/lib/api";
import { requestKey, time } from "@/lib/api";
import { useRef, useState } from "react";
import { DeletionDialog } from "./deletion-dialog";
export function LibrarySongRow({
  song,
  canDelete,
}: {
  song: LibrarySong;
  canDelete: boolean;
}) {
  const { api } = useApiMutation();
  const [busy, setBusy] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const key = useRef<string>("");
  async function remove() {
    setBusy(true);
    try {
      await api(
        "/deletion-requests",
        "POST",
        { songId: song.id },
        (key.current ||= requestKey()),
      );
      setSubmitted(true);
    } finally {
      setBusy(false);
    }
  }
  return (
    <article data-slot="library-song">
      <Row>
        <div>
          <strong>{song.title}</strong>
          <Caption>
            {song.artist} / {song.album} / {time(song.duration)}
          </Caption>
        </div>
        <Actions>
          <Button
            variant="outline"
            type="button"
            aria-label={
              (canDelete ? "Delete song: " : "Request deletion: ") + song.title
            }
            disabled={busy || submitted}
            onClick={() => setConfirming(true)}
          >
            {submitted
              ? "Deletion requested"
              : busy
                ? "Submitting…"
                : canDelete
                  ? "Delete song"
                  : "Request deletion"}
          </Button>
        </Actions>
      </Row>
      {submitted && (
        <Text role="status">
          {canDelete
            ? "Deletion approved and queued. Follow progress in Requests or Activity."
            : "Deletion request submitted for administrator approval."}{" "}
          <AppLink to="/requests">View requests</AppLink>
        </Text>
      )}
      <DeletionDialog
        open={confirming}
        mode={canDelete ? "delete" : "request"}
        song={song}
        onClose={() => setConfirming(false)}
        onConfirm={remove}
      />
    </article>
  );
}
