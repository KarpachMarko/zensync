import {
  executionGroup,
  executionSections,
  sortExecutionRows,
} from "@/lib/execution-state";
import { ActivityRow } from "@/components/requests/activity-row";
import { AppLink } from "@/components/navigation";
import { StatusSection } from "@/components/status";
import { PageHeading, Text } from "@/components/typography";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import type { ActivityItem } from "@/lib/types";
import { useState } from "react";
import { toast } from "sonner";
export function Activity({ admin }: { admin: boolean }) {
  const { api } = useApiMutation();
  const result = useApiQuery<ActivityItem[]>("/activity", {
    refetchInterval: 3000,
  });
  const rows = sortExecutionRows(result.data ?? [], (row) => row.state);
  const [actionError, setError] = useState("");
  const [pendingTask, setPendingTask] = useState<string | null>(null);
  const error = actionError || result.error?.message;
  const loading = result.isPending;
  const load = result.reload;
  async function taskAction(item: ActivityItem, cancel = false, dismiss = false, restore = false) {
    const id = item.id;
    if (pendingTask) return;
    setPendingTask(id);
    setError("");
    try {
      if (restore) {
        await api("/tasks/"+id+"/restore","POST");
      } else if (item.kind === "metadata") {
        await api("/metadata/jobs", "POST", { id });
      } else {
        await api("/tasks/" + id + (dismiss ? "/dismiss" : cancel ? "/cancel" : ""), "POST");
      }
      await load();
      toast.success(
        restore ? "File restored. Playlist replacements are unchanged." : dismiss
          ? "Failure dismissed. Music files are kept."
          : cancel
            ? "Migration task cancelled."
            : item.kind === "metadata" ? "Metadata update queued for retry." : "Task queued for retry.",
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setPendingTask(null);
    }
  }
  return (
    <>
      <PageHeading tabIndex={-1}>Activity</PageHeading>
      <Text>Follow downloads, library scans, metadata updates and migration tasks.</Text>
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
        executionSections.map((title) => {
          const group = rows.filter((r) => title === executionGroup(r.state));
          return (
            <StatusSection key={title} title={title} count={group.length}>
              {group.map((r) => (
                <ActivityRow
                  key={`${r.kind ?? "task"}:${r.id}`}
                  item={r}
                  busy={pendingTask !== null}
                  onRestore={admin ? () => void taskAction(r, false, false, true) : undefined}
                  onRetry={() => void taskAction(r)}
                  onDismiss={() => void taskAction(r, false, true)}
                  onCancel={() => void taskAction(r, true)}
                />
              ))}
            </StatusSection>
          );
        })
      )}
    </>
  );
}
