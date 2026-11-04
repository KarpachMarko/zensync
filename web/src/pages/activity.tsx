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
  async function taskAction(id: string, cancel = false, dismiss = false) {
    if (pendingTask) return;
    setPendingTask(id);
    setError("");
    try {
      await api(
        "/tasks/" + id + (dismiss ? "/dismiss" : cancel ? "/cancel" : ""),
        "POST",
      );
      await load();
      toast.success(
        dismiss
          ? "Failure dismissed. Music files are kept."
          : cancel
            ? "Migration task cancelled."
            : "Task queued for retry.",
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
        executionSections.map((title) => {
          const group = rows.filter((r) => title === executionGroup(r.state));
          return (
            <StatusSection key={title} title={title} count={group.length}>
              {group.map((r) => (
                <ActivityRow
                  key={r.id}
                  item={r}
                  busy={pendingTask !== null}
                  onRetry={() => void taskAction(r.id)}
                  onDismiss={() => void taskAction(r.id, false, true)}
                  onCancel={() => void taskAction(r.id, true)}
                />
              ))}
            </StatusSection>
          );
        })
      )}
    </>
  );
}
