import { Undo2Icon, CopyIcon, ImageIcon, ChevronRightIcon } from "lucide-react";
import { AppLink } from "@/components/navigation";
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
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import type { DuplicateGroup, DuplicateReport, MetadataJob } from "@/lib/metadata";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAsStringLiteral, useQueryState } from "nuqs";
import { useEffect, useId, useRef, useState } from "react";
import { toast } from "sonner";

export function Utilities({ admin }: { admin: boolean }) {
  const [tool] = useQueryState(
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
      {tool !== "duplicates" ? <>
        <PageHeading tabIndex={-1}>Utilities</PageHeading>
        <section aria-label="Organize your library" className="mx-auto mt-6 max-w-3xl rounded-xl border border-solid border-border p-4 sm:p-6">
          <h2 className="m-0 mb-3 text-sm font-medium text-muted-foreground">Organize your library</h2>
          {[{to:"/utilities?tool=duplicates",label:"Review duplicates",icon:CopyIcon},{to:"/utilities/metadata",label:"Covers and lyrics",icon:ImageIcon}].map(item => <AppLink key={item.to} to={item.to} className="flex items-center gap-4 rounded-md px-2 py-4 hover:bg-accent focus-visible:bg-accent">
            <item.icon aria-hidden="true" className="size-5 text-primary" /><span className="flex-1 font-medium">{item.label}</span><ChevronRightIcon aria-hidden="true" className="size-4 text-muted-foreground" />
          </AppLink>)}
        </section>
      </> : <>
        <AppLink to="/utilities" className="text-sm text-muted-foreground">← Utilities</AppLink>
        <PageHeading tabIndex={-1}>Review duplicates</PageHeading>
        <div className="space-y-3">
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
        </div>
      </>}
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
  const [scheduled, setScheduled] = useState<string | null>(null);
  const pending = useRef<{ timer: ReturnType<typeof setTimeout>; toast: string | number } | null>(null);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => {
    if (pending.current) {
      clearTimeout(pending.current.timer);
      toast.dismiss(pending.current.toast);
    }
    controller.current?.abort();
  }, []);
  const merge = useMutation({
    mutationFn: async (target: string) => {
      const abort = new AbortController();
      controller.current = abort;
      const sources = group.members.filter((member) => member.id !== target);
      let songs = 0;
      for (const [index, source] of sources.entries()) {
        abort.signal.throwIfAborted();
        const result = await api<{ id: string; songs: number }>("/metadata/merge", "POST", {
          kind: group.kind, source: source.id, target,
        }, undefined, abort.signal);
        songs += result.songs;
        // Larger groups can touch the same files. Finish each source before
        // preparing the next merge snapshot.
        if (index < sources.length - 1) {
          while (true) {
            abort.signal.throwIfAborted();
            const jobs = await client.fetchQuery({
              queryKey: ["api", "/metadata/jobs"], staleTime: 0,
              queryFn: () => api<MetadataJob[]>("/metadata/jobs", "GET", undefined, undefined, abort.signal),
            });
            const job = jobs.find((item) => item.id === result.id);
            if (!job || job.state === "failed") throw new Error(job?.message ?? "Merge job disappeared; refresh Activity before retrying");
            if (job.state === "completed") break;
            await new Promise<void>((resolve) => setTimeout(resolve, 2000));
          }
        }
      }
      return { songs };
    },
    onSuccess: (data) => { if (!controller.current?.signal.aborted) toast.success(`Merge queued for ${data.songs} songs`); },
    onError: (error) => { if (!controller.current?.signal.aborted) toast.error(error.message); },
    onSettled: async () => { await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] }); },
  });
  function scheduleMerge(member: DuplicateGroup["members"][number]) {
    if (pending.current || merge.isPending || merge.isSuccess) return;
    merge.reset();
    setScheduled(member.id);
    const notification = toast("Merge starts in 5 seconds", {
      description: `Keep “${member.label}” and merge the other ${group.kind} entries.`,
      duration: 5000,
      action: { label: <><Undo2Icon className="size-3.5 shrink-0" aria-hidden="true" />Undo</>, onClick: () => {
        if (!pending.current) return;
        clearTimeout(pending.current.timer);
        toast.dismiss(pending.current.toast);
        pending.current = null;
        setScheduled(null);
      } },
    });
    const timer = setTimeout(() => {
      pending.current = null;
      setScheduled(null);
      toast.dismiss(notification);
      merge.mutate(member.id);
    }, 5000);
    pending.current = { timer, toast: notification };
  }
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
                <Button size="sm" variant="outline"
                  disabled={limited || scheduled !== null || merge.isPending || merge.isSuccess}
                  onClick={() => scheduleMerge(member)}>
                  Merge into this
                </Button>
              </div>
            ))
      )}
      {scheduled && <Text role="status" className="my-1 text-xs">Merge scheduled. Use Undo in the notification to cancel.</Text>}
      {merge.error && !controller.current?.signal.aborted && <Text role="alert">{merge.error.message}</Text>}

    </section>
  );
}
