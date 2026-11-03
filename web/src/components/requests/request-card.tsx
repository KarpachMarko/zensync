import { ExpandableSection } from "@/components/expandable-section";
import { Actions, Row } from "@/components/layout/content";
import { ActiveProgress, StatusBadge } from "@/components/status";
import { Caption, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import type { MusicRequest as Request, User } from "@/lib/types";
import { DeclineForm } from "./decline-form";
import { useState } from "react";
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
  const [coverFailed, setCoverFailed] = useState(false);
  return (
    <article
      data-slot="request"
      className="border-b border-solid border-sidebar-border pb-2.5"
    >
      <Row>
        <div className="flex min-w-0 items-center gap-3">
          {r.kind === "upload" && !coverFailed && <img src={`/api/v2/requests/${encodeURIComponent(r.id)}/cover`} alt="Uploaded album cover" loading="lazy" className="size-12 shrink-0 rounded-md object-cover" onError={() => setCoverFailed(true)} />}
          <div>
          <strong>
            {r.title}
            {r.kind === "delete"
              ? " (Delete song)"
              : r.kind === "album"
                ? " (Album)"
                : r.kind === "upload"
                  ? " (Uploaded audio)"
                  : ""}
          </strong>
          <Caption>
            {r.artist || "Artist not specified"} / {r.target}
            {r.upgrade ? " / Upgrade" : ""}
          </Caption>
          </div>
        </div>
        <StatusBadge state={r.state || r.decision} />
      </Row>
      <ActiveProgress state={r.state} title={r.title} transfer={r.transfer} />
      {r.kind === "album" && (
        <ExpandableSection
          compact
          defaultOpen={false}
          title={
            <>
              {r.trackCount} tracks ·{" "}
              {r.items?.filter((item) => item.state === "available").length ??
                0}{" "}
              available
            </>
          }
        >
          <Text className="text-[13px]">
            One approval covers this saved recording list and quality
            preference.
          </Text>
          {r.items?.map((i, index) => (
            <div key={index}>
              <Row>
                <div>
                  <strong>
                    {index + 1}. {i.title}
                  </strong>
                  <Caption>
                    {i.artist}
                    {i.detail ? " / " + i.detail : ""}
                  </Caption>
                </div>
                <StatusBadge state={i.state} />
              </Row>
              <ActiveProgress
                state={i.state}
                title={i.title}
                transfer={i.transfer}
              />
            </div>
          ))}
        </ExpandableSection>
      )}
      {r.kind === "delete" && (
        <Text className="text-[13px]">
          Removes this song for everyone in the shared library.{" "}
          {r.state && `${r.state.replaceAll("_", " ")}: ${r.detail || ""}`}
        </Text>
      )}
      {r.reason && <Text className="text-[13px]">{r.reason}</Text>}
      {r.decision === "awaiting_approval" && (
        <Actions>
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
              <DeclineForm
                title={r.title}
                busy={busy}
                onDecline={(reason) => decide(r, "declined", reason)}
              />
            </>
          )}
          {r.owner === user.id && (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void decide(r, "withdrawn")}
            >
              Withdraw
            </Button>
          )}
        </Actions>
      )}
    </article>
  );
}
