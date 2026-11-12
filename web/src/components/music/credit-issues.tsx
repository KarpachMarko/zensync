import { DismissSuggestion } from "./dismiss-suggestion";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { toast } from "sonner";
import { CreditRepairDialog } from "./credit-repair-dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Text } from "@/components/typography";
import type { CreditIssue } from "@/lib/metadata";

export function CreditIssues({ issues, limited = false }: { issues: CreditIssue[]; limited?: boolean }) {
  const client = useQueryClient();
  const eligible = issues.filter((issue) => issue.parts.length >= 2 && issue.parts.every((part) => issue.knownArtists.some((name) => name.toLowerCase() === part.trim().toLowerCase())));
  const bulk = useMutation({ mutationFn: (includeUnknown: boolean) => api<{ songs: number; issues: number }>("/metadata/credits/separate-known", "POST", { includeUnknown }),
    onSuccess: async (result) => { toast.info(`Queued ${result.issues} credit fixes across ${result.songs} songs`); await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] }); } });
  if (!issues.length) return null;
  return (
    <section aria-label="Combined artist credit review" className="space-y-2">
      <h2 className="mb-1 text-base font-semibold">Combined artist credits</h2>
      <Text className="my-1 text-xs">
        Confirm the proposed artists to separate a combined credit into individual
        artist entries. You can adjust names and order before applying. Legal names
        are not automatically mapped to stage names.
      </Text>
      <Button size="sm" disabled={limited || !eligible.length || bulk.isPending || bulk.isSuccess} onClick={() => bulk.mutate(false)}>{bulk.isSuccess && !bulk.variables ? "Known artist fixes queued" : `Queue all known artist fixes (${eligible.length})`}</Button>
      <Button size="sm" variant="outline" disabled={limited || bulk.isPending || bulk.isSuccess} onClick={() => bulk.mutate(true)}>{bulk.isSuccess && bulk.variables ? "All artist fixes queued" : `Queue all artist fixes (${issues.length})`}</Button>
      <Text className="my-1 text-xs">Including unknown artists uses the proposed names shown below as new artist credits. Review those names first if needed; legal names are kept as written.</Text>
      {bulk.error && <Text role="alert">{bulk.error.message}</Text>}
      <Text className="my-1 text-xs">The known-only option queues exact matches and leaves unmatched names for review. Follow progress in Activity.</Text>
      {issues.map((issue) => (
        <Collapsible key={issue.role + issue.id + issue.name} className="rounded border border-solid border-border p-3">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">Review {issue.role} tags</Badge>
            <strong className="min-w-0 break-words text-sm">{issue.name}</strong>
          </div>
          <div className="my-2 flex flex-wrap items-center gap-1.5">
            {issue.parts.map((part, index) => <Badge key={index} variant="secondary" className="max-w-full whitespace-normal break-words">{part}</Badge>)}
          </div>
          <Text className="my-1 text-xs">{issue.knownArtists.length} names match existing artist entries</Text>
          <Text className="my-1 text-xs">{issue.tracks.length} affected {issue.tracks.length === 1 ? "song" : "songs"}</Text>
          <div className="my-2 flex flex-wrap items-start gap-2"><CreditRepairDialog issue={issue} disabled={limited} /><DismissSuggestion rule={{ kind: "credit", name: issue.name }} /></div>
          {limited && <Text className="my-1 text-xs">Changes require a complete library scan.</Text>}
          <CollapsibleTrigger asChild>
            <Button variant="outline" size="sm">Review affected songs</Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="mt-2 space-y-2">
            {issue.tracks.map((track) => (
              <div key={track.id} className="flex flex-wrap items-center justify-between gap-2 border-t border-solid border-border pt-2">
                <div className="min-w-0 text-xs"><strong>{track.title}</strong><p className="my-1 break-words">{track.artist} · {track.album}</p></div>

              </div>
            ))}
          </CollapsibleContent>
        </Collapsible>
      ))}
    </section>
  );
}
