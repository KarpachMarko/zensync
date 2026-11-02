import { Actions, Row } from "@/components/layout/content";
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
export function Activity({ admin }: { admin: boolean }) {
  const { api } = useApiMutation();
  const result = useApiQuery<
    {
      id: string;
      title: string;
      state: string;
      stage: string;
      transfer?: TransferProgress;
    }[]
  >("/activity", { refetchInterval: 3000 });
  const rows = result.data ?? [];
  const [actionError, setError] = useState("");
  const error = actionError || result.error?.message;
  const loading = result.isPending;
  const load = result.reload;
  async function retry(id: string) {
    try {
      await api("/tasks/" + id, "POST");
      await load();
    } catch (e) {
      setError((e as Error).message);
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
        ["In progress", "Blocked", "Completed"].map((title) => {
          const group = rows.filter(
            (r) =>
              title ===
              (finished(r.state)
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
                      {(r.state === "failed" || r.state === "blocked") && (
                        <Button
                          variant="outline"
                          onClick={() => void retry(r.id)}
                        >
                          Retry
                        </Button>
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
