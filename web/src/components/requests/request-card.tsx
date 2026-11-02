import { ActiveProgress, StatusBadge } from "@/components/status";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { MusicRequest as Request, User } from "@/lib/types";
export function RequestCard({
  request: r,
  user,
  busy,
  decide,
  onApproveDeletion,
}: {
  request: Request;
  user: User;
  busy: boolean;
  decide: (
    request: Request,
    decision: string,
    reason?: string,
  ) => Promise<void>;
  onApproveDeletion: (request: Request) => void;
}) {
  return (
    <article className="request">
      <div className="row">
        <div>
          <strong>
            {r.title}
            {r.kind === "delete"
              ? " (Delete song)"
              : r.kind === "album"
                ? " (Album)"
                : ""}
          </strong>
          <small>
            {r.artist || "Artist not specified"} / {r.target}
            {r.upgrade ? " / Upgrade" : ""}
          </small>
        </div>
        <StatusBadge state={r.state || r.decision} />
      </div>
      <ActiveProgress state={r.state} title={r.title} transfer={r.transfer} />
      {r.kind === "album" && (
        <details>
          <summary>
            {r.trackCount} tracks ·{" "}
            {r.items?.filter((i) => i.state === "available").length || 0}{" "}
            available
          </summary>
          <p className="muted">
            One approval covers this saved recording list and quality
            preference.
          </p>
          {r.items?.map((i, index) => (
            <div key={index}>
              <div className="row">
                <div>
                  <strong>
                    {index + 1}. {i.title}
                  </strong>
                  <small>
                    {i.artist}
                    {i.detail ? " / " + i.detail : ""}
                  </small>
                </div>
                <StatusBadge state={i.state} />
              </div>
              <ActiveProgress
                state={i.state}
                title={i.title}
                transfer={i.transfer}
              />
            </div>
          ))}
        </details>
      )}
      {r.kind === "delete" && (
        <p className="muted">
          Removes this song for everyone in the shared library.{" "}
          {r.state && `${r.state.replaceAll("_", " ")}: ${r.detail || ""}`}
        </p>
      )}
      {r.reason && <p className="muted">{r.reason}</p>}
      {r.decision === "awaiting_approval" && (
        <div className="actions">
          {user.role === "admin" && (
            <>
              <Button
                disabled={busy}
                onClick={() => {
                  if (r.kind === "delete") onApproveDeletion(r);
                  else void decide(r, "approved");
                }}
              >
                Approve
              </Button>
              <form
                className="decline"
                onSubmit={(e) => {
                  e.preventDefault();
                  void decide(
                    r,
                    "declined",
                    String(new FormData(e.currentTarget).get("reason")),
                  );
                }}
              >
                <Input
                  name="reason"
                  aria-label={"Reason for declining " + r.title}
                  placeholder="Reason for declining"
                  maxLength={1000}
                  required
                />
                <Button className="quiet" disabled={busy}>
                  Decline
                </Button>
              </form>
            </>
          )}
          {r.owner === user.id && (
            <Button
              className="quiet"
              disabled={busy}
              onClick={() => void decide(r, "withdrawn")}
            >
              Withdraw
            </Button>
          )}
        </div>
      )}
    </article>
  );
}
