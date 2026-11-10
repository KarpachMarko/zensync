import { parseAsString, useQueryState } from "nuqs";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Text } from "@/components/typography";
import type { CreditIssue } from "@/lib/metadata";

export function CreditIssues({ issues }: { issues: CreditIssue[] }) {
  const [, setEditSong] = useQueryState("editSong", parseAsString);
  if (!issues.length) return null;
  return (
    <section aria-label="Combined artist credit review" className="space-y-2">
      <h2 className="mb-1 text-base font-semibold">Combined artist credits</h2>
      <Text className="my-1 text-xs">
        These names appear stored as a single artist identity and contain a known
        standalone artist. Review the original tags: collaborations can be valid,
        and legal names are not automatically aliases of stage names. No automatic
        splitting or artist merging is performed.
      </Text>
      {issues.map((issue) => (
        <Collapsible key={issue.role + issue.id + issue.name} className="rounded border border-solid border-border p-3">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">Review {issue.role} tags</Badge>
            <strong className="min-w-0 break-words text-sm">{issue.name}</strong>
          </div>
          <Text className="my-1 text-xs">Possible separate credits: {issue.parts.join(" / ")}</Text>
          <Text className="my-1 text-xs">Known standalone artists: {issue.knownArtists.join(", ")}</Text>
          <Text className="my-1 text-xs">Artist ID: {issue.id || "Not reported"} · {issue.tracks.length} affected songs</Text>
          <CollapsibleTrigger asChild>
            <Button variant="outline" size="sm">Review affected songs</Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="mt-2 space-y-2">
            {issue.tracks.map((track) => (
              <div key={track.id} className="flex flex-wrap items-center justify-between gap-2 border-t border-solid border-border pt-2">
                <div className="min-w-0 text-xs"><strong>{track.title}</strong><p className="my-1 break-words">{track.artist} · {track.album}</p></div>
                <Button size="sm" variant="outline" aria-label={`Edit metadata: ${track.title}`} onClick={() => void setEditSong(track.id)}>Edit metadata</Button>
              </div>
            ))}
          </CollapsibleContent>
        </Collapsible>
      ))}
    </section>
  );
}
