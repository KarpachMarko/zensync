import type { RecordingMatch } from "@/lib/migration";

export function MatchDetails({ match }: { match?: RecordingMatch }) {
  if (!match) return null;
  return <>
    <p className="m-0 mt-1 text-xs leading-5 text-muted-foreground">
      {match.confident ? "Title, artist credits, album and duration match." : match.reasons.join(" · ")}
    </p>
    {match.warnings?.map((warning) => (
      <p key={warning} className="m-0 mt-1 text-xs leading-5 text-muted-foreground">{warning}</p>
    ))}
  </>;
}
