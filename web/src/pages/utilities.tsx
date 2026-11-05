import { LibrarySongRow } from "@/components/music/library-song-row";
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
import { useAppForm } from "@/hooks/use-app-form";
import { api } from "@/lib/api";
import type { DuplicateGroup, DuplicateReport } from "@/lib/metadata";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAsStringLiteral, useQueryState } from "nuqs";
import { useId, useState } from "react";
import { z } from "zod";
import { toast } from "sonner";

export function Utilities({ admin }: { admin: boolean }) {
  const [tool, setTool] = useQueryState(
    "tool",
    parseAsStringLiteral(["duplicates"] as const),
  );
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
              tags.
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
          <MetadataJobs />
          {report.isFetching && (
            <Text role="status" className="my-1">
              Checking library…
            </Text>
          )}
          {report.error && <Text role="alert">{report.error.message}</Text>}
          {report.data && (
            <Text className="my-1 text-xs">
              {report.data.scanned} songs checked · {report.data.groups.length}{" "}
              candidate groups.
              {report.data.limited &&
                " Partial scan: 50,000-song limit reached. Merges are disabled."}
            </Text>
          )}
          {report.data?.groups.map((group) => (
            <DuplicateCandidates
              key={`${group.kind}:${group.members.map((m) => m.id).join(":")}`}
              group={group}
              limited={report.data.limited}
            />
          ))}
          {report.data && !report.data.groups.length && (
            <Text>No duplicate candidates found.</Text>
          )}
        </CollapsibleContent>
      </Collapsible>
    </>
  );
}
const mergeSchema = z.object({ target: z.string().min(1) });
function DuplicateCandidates({
  group,
  limited,
}: {
  group: DuplicateGroup;
  limited: boolean;
}) {
  const client = useQueryClient();
  const id = useId();
  const [source, setSource] = useState<
    DuplicateGroup["members"][number] | null
  >(null);
  const merge = useMutation({
    mutationFn: (target: string) =>
      api<{ id: string; songs: number }>("/metadata/merge", "POST", {
        kind: group.kind,
        source: source?.id,
        target,
      }),
    onSuccess: async (data) => {
      toast.success(`Merge queued for ${data.songs} songs`);
      setSource(null);
      await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] });
    },
  });
  const form = useAppForm({
    defaultValues: { target: group.members[0].id },
    validators: { onSubmit: mergeSchema },
    onSubmit: async ({ value }) => {
      if (source && source.id !== value.target)
        await merge.mutateAsync(value.target).catch(() => undefined);
    },
  });
  return (
    <section className="rounded border border-border px-3 py-2">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div>
          <Badge variant="secondary">{group.kind}</Badge>
          <span className="ml-2 text-xs text-muted-foreground">
            {group.reason}
          </span>
        </div>
        {group.kind !== "song" && (
          <form.Field name="target">
            {(field) => (
              <div className="flex items-center gap-2">
                <Label htmlFor={id} className="text-xs">
                  Keep {group.kind}
                </Label>
                <Select
                  value={field.state.value}
                  onValueChange={field.handleChange}
                >
                  <SelectTrigger id={id} className="h-8 max-w-60 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {group.members.map((member) => (
                      <SelectItem key={member.id} value={member.id}>
                        {member.label} ({member.id.slice(0, 6)})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </form.Field>
        )}
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
        <form.Subscribe selector={(state) => state.values.target}>
          {(target) =>
            group.members.map((member) => (
              <div
                key={member.id}
                className="flex items-center justify-between gap-2 border-t border-border py-1.5"
              >
                <div className="min-w-0">
                  <strong className="text-sm">{member.label}</strong>
                  <span className="ml-2 text-xs text-muted-foreground">
                    {member.count ?? 0} songs · {member.id.slice(0, 8)}
                  </span>
                </div>
                {member.id === target ? (
                  <Badge variant="outline">Keep</Badge>
                ) : (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={limited || merge.isPending}
                    onClick={() => setSource(member)}
                  >
                    Merge into selected {group.kind}
                  </Button>
                )}
              </div>
            ))
          }
        </form.Subscribe>
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
            Move {source?.count ?? "all"} songs from “{source?.label}” to the
            selected {group.kind}. This updates file tags and rescans Navidrome.
            Audio is preserved and recovery copies are kept.
          </DialogDescription>
          <form.Subscribe selector={(state) => state.values.target}>
            {(target) => (
              <Text className="my-1 text-sm">
                Destination:{" "}
                {group.members.find((member) => member.id === target)?.label}
              </Text>
            )}
          </form.Subscribe>
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
              onClick={() => void form.handleSubmit()}
            >
              Confirm merge
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </section>
  );
}
