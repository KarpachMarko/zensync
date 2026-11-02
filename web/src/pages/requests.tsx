import { DeletionDialog } from "@/components/music/deletion-dialog";
import { RequestCard } from "@/components/requests/request-card";
import { StatusSection, finished } from "@/components/status";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import type { MusicRequest as Request, User } from "@/lib/types";
import { useState } from "react";
import { NavLink } from "react-router-dom";
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
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <h1 tabIndex={-1}>
        Requests <span className="count">{rows.length}</span>
      </h1>
      <p>
        {user.role === "admin"
          ? "Review acquisition and deletion requests across the workspace."
          : "Your music requests and approval decisions."}
      </p>
      <div className="actions toolbar">
        <NavLink className="button" to="/search">
          Search or paste a Deezer link
        </NavLink>
        <NavLink to="/imports">Request from a migration</NavLink>
      </div>
      {error && <p role="alert">{error}</p>}
      {!loaded ? (
        <p>Loading requests…</p>
      ) : rows.length === 0 ? (
        <p className="emptytext">
          No requests yet. Find a song in Search or select missing songs from a
          migration.
        </p>
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
