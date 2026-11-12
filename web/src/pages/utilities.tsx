import { DismissSuggestion } from "@/components/music/dismiss-suggestion";
import { LibrarySongRow } from "@/components/music/library-song-row";
import { CreditIssues } from "@/components/music/credit-issues";
import { MetadataJobs } from "@/components/music/metadata-jobs";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { api } from "@/lib/api";
import type { DuplicateGroup, DuplicateReport } from "@/lib/metadata";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAsStringLiteral, useQueryState } from "nuqs";
import { useId, useState } from "react";
import { toast } from "sonner";

export function Utilities({ admin }: { admin: boolean }) {
  const [tool, setTool] = useQueryState(
    "tool",
    parseAsStringLiteral(["duplicates"] as const),
  );
  const kinds = ["all", "song", "album", "artist", "credits"] as const;
  const [kind, setKind] = useQueryState("kind", parseAsStringLiteral(kinds).withDefault("all"));
  const filterID = useId();
  const report = useQuery({
    queryKey: ["api", "/metadata/duplicates"],
    enabled: admin && tool === "duplicates",
    queryFn: ({ signal }) =>
      api<DuplicateReport>(
        "/metadata/duplicates",
        "GET",
        undefined,
        undefined,
        signal,
      ),
    retry: false,
    staleTime: 30000,
  });
  const visibleGroups = (report.data?.groups ?? []).filter((group) => kind === "all" || group.kind === kind);
  const visibleCredits = kind === "all" || kind === "credits" ? report.data?.creditIssues ?? [] : [];
  if (!admin)
    return <Text role="alert">Utilities require an administrator.</Text>;
  return (
    <>
      <PageHeading tabIndex={-1}>Utilities</PageHeading>
      <Collapsible
        open={tool === "duplicates"}
        onOpenChange={(open) => void setTool(open ? "duplicates" : null)}
      >
        <CollapsibleTrigger asChild>
          <Button variant="outline">Review duplicates</Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="mt-3 space-y-2">
          <div className="flex items-center justify-between gap-2">
            <Text className="m-0 text-xs">
              Review candidates, delete extra songs or merge artist and album
              tags. Album candidates use album artist; featured track artists may
              differ legitimately. Compare album-artist identities before merging,
              then refresh after the Navidrome scan.
            </Text>
            <Button
              size="sm"
              variant="ghost"
              disabled={report.isFetching}
              onClick={() => void report.refetch()}
            >
              Refresh candidates
            </Button>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Label htmlFor={filterID}>Show</Label>
            <Select value={kind} onValueChange={(value) => { const next = kinds.find((item) => item === value); if (next) void setKind(next); }}>
              <SelectTrigger id={filterID} className="w-56"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All issues</SelectItem>
                <SelectItem value="song">Duplicate songs</SelectItem>
                <SelectItem value="album">Duplicate albums</SelectItem>
                <SelectItem value="artist">Duplicate artists</SelectItem>
                <SelectItem value="credits">Combined artist credits</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <MetadataJobs />
          {report.isFetching && (
            <Text role="status" className="my-1">
              Checking library…
            </Text>
          )}
          {report.error && <Text role="alert">{report.error.message}</Text>}
          {report.data && (
            <Text className="my-1 text-xs">
              {report.data.scanned} songs checked · {visibleGroups.length} of {report.data.groups.length}{" "}
              duplicate candidate groups · {visibleCredits.length} combined-credit issues.
              {report.data.limited &&
                " Partial scan: 50,000-song limit reached. Merges are disabled."}
            </Text>
          )}
          {visibleGroups.map((group) => (
            <DuplicateCandidates
              key={`${group.kind}:${group.members.map((m) => m.id).join(":")}`}
              group={group}
              limited={report.data?.limited ?? false}
            />
          ))}
          {report.data && !visibleGroups.length && kind !== "credits" && (
            <Text>No {kind === "all" ? "duplicate" : kind} candidates found.</Text>
          )}
          <CreditIssues issues={visibleCredits} limited={report.data?.limited} />
        </CollapsibleContent>
      </Collapsible>
    </>
  );
}
function DuplicateCandidates({
  group,
  limited,
}: {
  group: DuplicateGroup;
  limited: boolean;
}) {
  const client = useQueryClient();
  const [kept, setKept] = useState(group.members[0].id);
  const [source, setSource] = useState<
    DuplicateGroup["members"][number] | null
  >(null);
  const merge = useMutation({
    mutationFn: (sourceID: string) => api<{ id: string; songs: number }>("/metadata/merge", "POST", {
      kind: group.kind, source: sourceID, target: kept,
    }),
    onSuccess: (data) => {
      toast.success(`Merge queued for ${data.songs} songs`);
      setSource(null);
    },
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] });
    },
  });
  return (
    <section className="rounded border border-border px-3 py-2">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div>
          <Badge variant="secondary">{group.kind}</Badge>
          {group.kind === "album" && new Set(group.members.map((member) => member.albumArtists?.map((artist) => artist.id).filter(Boolean).sort().join("|")).filter(Boolean)).size > 1 && (
            <Badge variant="outline" className="ml-2">Album artist IDs differ · review artists first</Badge>
          )}
          <span className="ml-2 text-xs text-muted-foreground">
            {group.reason}{group.kind === "album" && " · Candidates only: compare tracks before merging; matching names do not prove identical recordings."}
          </span>
        </div>
        <DismissSuggestion rule={{ kind: group.kind, ids: group.members.map((member) => member.id) }} />

      </div>
      {group.kind === "song" ? (
        group.members.map((member) => (
          <LibrarySongRow
            key={member.id}
            song={
              member.song ?? {
                id: member.id,
                title: member.label,
                artist: "",
                album: "",
                duration: 0,
              }
            }
            canDelete
            canEdit
          />
        ))
      ) : (
        group.members.map((member) => (
              <div
                key={member.id}
                className="flex flex-wrap items-center justify-between gap-2 border-t border-border py-1.5"
              >
                <div className="min-w-0">
                  <strong className="text-sm">{member.label}</strong>
                  <span className="ml-2 text-xs text-muted-foreground">
                    {member.count ?? 0} songs · {member.id.slice(0, 8)}
                  </span>
                  {member.albumArtists?.map((artist) => (
                    <p key={artist.id + artist.name} className="my-1 text-xs text-muted-foreground">
                      Album artist: {artist.name} · ID: {artist.id || "Unknown"}
                    </p>
                  ))}
                  {member.artists?.map((artist) => (
                    <p key={artist.id + artist.name + artist.albumArtist} className="my-1 text-xs text-muted-foreground">
                      Artist: {artist.name} · ID: {artist.id || "Unknown"} · Album artist: {artist.albumArtist || artist.name}
                    </p>
                  ))}
                  {member.tracks && (
                    <details className="mt-2 text-xs text-muted-foreground">
                      <summary className="cursor-pointer">Compare tracks</summary>
                      <ul className="space-y-1 pl-4">
                        {member.tracks.map((track) => <li key={track.id}>{track.title} · {track.artist} · {track.duration}s · {track.id}</li>)}
                      </ul>
                    </details>
                  )}
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <Button size="sm" variant={member.id === kept ? "secondary" : "outline"}
                    aria-pressed={member.id === kept}
                    disabled={limited || merge.isPending}
                    onClick={() => { setKept(member.id); merge.reset(); }}>
                    {member.id === kept ? "Keeping this" : "Keep this"}
                  </Button>
                  <Button size="sm" variant="outline"
                    disabled={limited || merge.isPending || member.id === kept || merge.isSuccess}
                    onClick={() => { merge.reset(); setSource(member); }}>
                    Merge into kept {group.kind}
                  </Button>
                </div>
              </div>
            ))
      )}
      <Dialog
        open={!!source}
        onOpenChange={(open) => {
          if (!open && !merge.isPending) setSource(null);
        }}
      >
        <DialogContent className="gap-2 rounded-md p-3 sm:max-w-md">
          <DialogTitle className="m-0 pr-8 text-base">
            Merge {group.kind}
          </DialogTitle>
          <DialogDescription className="m-0 text-sm">
            Merge “{source?.label}” into “{group.members.find((member) => member.id === kept)?.label}”.
            This updates file tags and rescans Navidrome.
            Audio is verified before replacing the original file.
          </DialogDescription>
          {merge.error && (
            <Text role="alert" className="my-1 text-xs">
              {merge.error.message}
            </Text>
          )}
          <div className="flex justify-end gap-2">
            <Button
              size="sm"
              variant="ghost"
              disabled={merge.isPending}
              onClick={() => setSource(null)}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              disabled={merge.isPending}
              onClick={() => { if (source) void merge.mutateAsync(source.id).catch(() => undefined); }}
            >
              Confirm merge
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </section>
  );
}
