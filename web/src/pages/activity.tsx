import { Actions, Row } from "@/components/layout/content";
import { AcquisitionReviewButton } from "@/components/requests/acquisition-review";
import { AppLink } from "@/components/navigation";
import {
  ActiveProgress,
  StatusBadge,
  StatusSection,
  blocked,
  finished,
} from "@/components/status";
import { Caption, PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import type { TransferProgress } from "@/lib/api";
import { useState } from "react";
import { toast } from "sonner";
export function Activity({ admin }: { admin: boolean }) {
  const { api } = useApiMutation();
  const result = useApiQuery<
    {
      id: string;
      title: string;
      state: string;
      stage: string;
      canCancel?: boolean;
      canReview?: boolean;
      canDismiss?: boolean;
      transfer?: TransferProgress;
    }[]
  >("/activity", { refetchInterval: 3000 });
  const rows = result.data ?? [];
  const [actionError, setError] = useState("");
  const [pendingTask, setPendingTask] = useState<string | null>(null);
  const error = actionError || result.error?.message;
  const loading = result.isPending;
  const load = result.reload;
  async function taskAction(id: string, cancel = false, dismiss = false) {
    if (pendingTask) return;
    setPendingTask(id);
    setError("");
    try {
      await api("/tasks/" + id + (dismiss ? "/dismiss" : cancel ? "/cancel" : ""), "POST");
      await load();
      toast.success(dismiss ? "Failure dismissed. Music files are kept." : cancel ? "Migration task cancelled." : "Task queued for retry.");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setPendingTask(null);
    }
  }
  return (
    <>
      <PageHeading tabIndex={-1}>Activity</PageHeading>
      <Text>Follow downloads, library scans and migration tasks.</Text>
      {admin && (
        <AppLink to="/settings/downloads">Manage parallel downloads</AppLink>
      )}
      {error && <Text role="alert">{error}</Text>}
      {loading ? (
        <Text>Loading activity…</Text>
      ) : rows.length === 0 ? (
        <Text className="py-9 px-0">
          No execution tasks. Requests awaiting approval do not occupy a
          transfer slot.
        </Text>
      ) : (
        ["In progress", "Blocked", "Completed", "Queued"].map((title) => {
          const group = rows.filter(
            (r) =>
              title ===
              (r.state === "queued"
                ? "Queued"
                : finished(r.state)
                  ? "Completed"
                  : blocked(r.state)
                    ? "Blocked"
                    : "In progress"),
          );
          return (
            <StatusSection key={title} title={title} count={group.length}>
              {group.map((r) => (
                <div
                  className="border-b border-solid border-sidebar-border"
                  key={r.id}
                >
                  <Row>
                    <div>
                      <strong>{r.title}</strong>
                      <Caption>{r.stage}</Caption>
                    </div>
                    <Actions>
                      <StatusBadge state={r.state} />
                      {r.canReview && <AcquisitionReviewButton taskId={r.id} />}
                      {r.canDismiss && <Button variant="outline" disabled={pendingTask !== null}
                        title="Stop retrying this failed task and keep its history. Music files are kept."
                        onClick={() => void taskAction(r.id, false, true)}>Dismiss failure</Button>}
                      {(r.state === "failed" || r.state === "blocked") && (
                        <Button
                          variant="outline"
                          disabled={pendingTask !== null}
                          onClick={() => void taskAction(r.id)}
                        >
                          Retry
                        </Button>
                      )}
                      {r.canCancel && (
                        <Button variant="outline" disabled={pendingTask !== null}
                          title="Cancel this migration task. Imported songs and playlists are kept."
                          onClick={() => void taskAction(r.id, true)}>Cancel</Button>
                      )}
                    </Actions>
                  </Row>
                  <ActiveProgress
                    state={r.state}
                    title={r.title}
                    transfer={r.transfer}
                  />
                </div>
              ))}
            </StatusSection>
          );
        })
      )}
    </>
  );
}
