import {
  ActiveProgress,
  StatusBadge,
  StatusSection,
  blocked,
  finished,
} from "@/components/status";
import { Button } from "@/components/ui/button";
import type { TransferProgress } from "@/lib/api";
import { api } from "@/lib/api";
import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
export function Activity({ admin }: { admin: boolean }) {
  const [rows, setRows] = useState<
      {
        id: string;
        title: string;
        state: string;
        stage: string;
        transfer?: TransferProgress;
      }[]
    >([]),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  async function load() {
    try {
      setRows(await api<typeof rows>("/activity"));
      setLoading(false);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 3000);
    return () => clearInterval(timer);
  }, []);
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
      <h1 tabIndex={-1}>Activity</h1>
      <p>Follow downloads, library scans and migration tasks.</p>
      {admin && (
        <NavLink to="/settings/downloads">Manage parallel downloads</NavLink>
      )}
      {error && <p role="alert">{error}</p>}
      {loading ? (
        <p>Loading activity…</p>
      ) : rows.length === 0 ? (
        <p className="emptytext">
          No execution tasks. Requests awaiting approval do not occupy a
          transfer slot.
        </p>
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
                <div className="activity-item" key={r.id}>
                  <div className="row">
                    <div>
                      <strong>{r.title}</strong>
                      <small>{r.stage}</small>
                    </div>
                    <div className="actions">
                      <StatusBadge state={r.state} />
                      {(r.state === "failed" || r.state === "blocked") && (
                        <Button
                          className="quiet"
                          onClick={() => void retry(r.id)}
                        >
                          Retry
                        </Button>
                      )}
                    </div>
                  </div>
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
