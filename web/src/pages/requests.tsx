import { Actions } from "@/components/layout/content";
import { DeletionDialog } from "@/components/music/deletion-dialog";
import { AppLink } from "@/components/navigation";
import { RequestCard } from "@/components/requests/request-card";
import { StatusSection, finished } from "@/components/status";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import type { MusicRequest as Request, User } from "@/lib/types";
import { useState } from "react";
import { toast } from "sonner";
export function Requests({ user }: { user: User }) {
  const { api } = useApiMutation();
  const [deletionToApprove, setDeletionToApprove] = useState<Request | null>(
    null,
  );
  const result = useApiQuery<Request[]>("/requests", { refetchInterval: 4000 });
  const rows = result.data ?? [];
  const [actionError, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const error = actionError || result.error?.message;
  const loaded = !result.isPending;
  const load = result.reload;
  async function decide(r: Request, decision: string, reason = "") {
    setBusy(true);
    setError("");
    try {
      await api("/requests/" + r.id, "POST", { decision, reason });
      await load();
      toast.success(
        decision === "approved"
          ? "Request approved."
          : decision === "declined"
            ? "Request declined."
            : "Request withdrawn.",
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading tabIndex={-1}>
        Requests{" "}
        <span className="text-[18px] text-muted-foreground ml-2.5">
          {rows.length}
        </span>
      </PageHeading>
      <Text>
        {user.role === "admin"
          ? "Review acquisition and deletion requests across the workspace."
          : "Your music requests and approval decisions."}
      </Text>
      <Actions className="my-[22px] mx-0">
        <Button asChild>
          <AppLink to="/search">Search or paste a Deezer link</AppLink>
        </Button>
        <AppLink to="/imports">Request from a migration</AppLink>
      </Actions>
      {error && <Text role="alert">{error}</Text>}
      {!loaded ? (
        <Text>Loading requests…</Text>
      ) : rows.length === 0 ? (
        <Text className="py-9 px-0">
          No requests yet. Find a song in Search or select missing songs from a
          migration.
        </Text>
      ) : (
        ["In progress", "Requested", "Completed"].map((title) => {
          const group = rows.filter(
            (r) =>
              title ===
              (r.decision === "awaiting_approval"
                ? "Requested"
                : finished(r.state || r.decision)
                  ? "Completed"
                  : "In progress"),
          );
          return (
            <StatusSection key={title} title={title} count={group.length}>
              {group.map((r) => (
                <RequestCard
                  key={r.id}
                  request={r}
                  user={user}
                  busy={busy}
                  decide={decide}
                  onApproveDeletion={setDeletionToApprove}
                />
              ))}
            </StatusSection>
          );
        })
      )}
      <DeletionDialog
        open={deletionToApprove !== null}
        mode="approve"
        song={deletionToApprove || { title: "", artist: "" }}
        onClose={() => setDeletionToApprove(null)}
        onConfirm={async () => {
          if (!deletionToApprove) return;
          await api("/requests/" + deletionToApprove.id, "POST", {
            decision: "approved",
            reason: "",
          });
          await load();
        }}
      />
    </>
  );
}
