import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { toast } from "sonner";
import { useAppForm } from "@/hooks/use-app-form";
import { api } from "@/lib/api";
import type { CreditIssue, DuplicateReport, MetadataJob } from "@/lib/metadata";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Text } from "@/components/typography";

const schema = z.object({
  artists: z.array(z.string().trim().min(1).max(500)).min(2).max(20)
    .refine((names) => new Set(names.map((name) => name.trim().toLocaleLowerCase())).size === names.length, "List each artist once"),
});
const nameKey = (name: string) => name.toLocaleLowerCase().replace(/[^\p{L}\p{N}\p{M}]/gu, "");

export function CreditRepairDialog({ issue, disabled }: { issue: CreditIssue; disabled: boolean }) {
  const [open, setOpen] = useState(false);
  const [jobID, setJobID] = useState("");
  const jobs = useQuery({
    queryKey: ["api", "/metadata/jobs"], enabled: !!jobID,
    queryFn: ({ signal }) => api<MetadataJob[]>("/metadata/jobs", "GET", undefined, undefined, signal),
    refetchInterval: (query) => jobID && !["completed", "failed"].includes(query.state.data?.find((job) => job.id === jobID)?.state ?? "") ? 2000 : false,
  });
  const failed = jobs.data?.find((job) => job.id === jobID)?.state === "failed";
  useQuery({
    queryKey: ["api", "/metadata/duplicates"], enabled: !!jobID && !failed,
    queryFn: ({ signal }) => api<DuplicateReport>("/metadata/duplicates?refresh=true", "GET", undefined, undefined, signal),
    refetchInterval: jobID && !failed ? 5000 : false,
  });
  const client = useQueryClient();
  const proposed = issue.parts.map((part) => issue.knownArtists.find((name) => nameKey(name) === nameKey(part)) ?? part);
  const save = useMutation({
    mutationFn: (artists: string[]) => api<{ id: string; songs: number }>("/metadata/credits/separate", "POST", {
      source: issue.id, name: issue.name, role: issue.role, artists: artists.map((name) => name.trim()),
    }),
    onSuccess: async (result) => {
      setJobID(result.id);
      setOpen(false);
      toast.info(`Separating credits queued for ${result.songs} ${result.songs === 1 ? "song" : "songs"}`);
      await client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] });
      await client.invalidateQueries({ queryKey: ["api", "/metadata/duplicates"] });
    },
  });
  const form = useAppForm({
    defaultValues: { artists: proposed },
    validators: { onChange: schema, onSubmit: schema },
    onSubmit: async ({ value }) => {
      if (!save.isPending) await save.mutateAsync(value.artists).catch(() => undefined);
    },
  });
  const changeOpen = (next: boolean) => {
    if (save.isPending) return;
    if (next) { form.reset({ artists: proposed }); save.reset(); }
    setOpen(next);
  };
  return (
    <>
      <Button size="sm" disabled={disabled || !!jobID} onClick={() => changeOpen(true)}>{jobID ? failed ? "Separation failed — retry above" : "Separation queued" : "Separate artist credits"}</Button>
      <Dialog open={open} onOpenChange={changeOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
          <DialogTitle>Separate artist credits</DialogTitle>
          <DialogDescription>
            Replace “{issue.name}” with the artists below on {issue.tracks.length} {issue.tracks.length === 1 ? "song" : "songs"}.
            Each row is one artist. Other credits and audio are preserved.
          </DialogDescription>
          <form noValidate className="space-y-3" onSubmit={(event) => { event.preventDefault(); void form.handleSubmit(); }}>
            <form.Field name="artists" mode="array">
              {(array) => (
                <div className="space-y-2">
                  {array.state.value.map((_, index) => (
                    <div key={index} className="flex min-w-0 items-end gap-1">
                      <div className="min-w-0 flex-1">
                        <form.AppField name={`artists[${index}]`}>
                          {(field) => <field.TextField disabled={save.isPending} label={`Artist ${index + 1}`} labelClassName="m-0 text-xs" className="h-9 min-h-9 text-sm" />}
                        </form.AppField>
                      </div>
                      <Button type="button" size="sm" variant="outline" aria-label={`Move artist ${index + 1} up`} disabled={index === 0 || save.isPending}
                        onClick={() => { const names = [...array.state.value]; [names[index - 1], names[index]] = [names[index], names[index - 1]]; array.handleChange(names); }}>↑</Button>
                      <Button type="button" size="sm" variant="outline" aria-label={`Remove artist ${index + 1}`} disabled={array.state.value.length <= 2 || save.isPending}
                        onClick={() => array.handleChange(array.state.value.filter((_, position) => position !== index))}>×</Button>
                    </div>
                  ))}
                  <Button type="button" variant="outline" size="sm" disabled={save.isPending || array.state.value.length >= 20}
                    onClick={() => array.handleChange([...array.state.value, ""])}>Add artist</Button>
                </div>
              )}
            </form.Field>
            <form.Subscribe selector={(state) => state.errors}>
              {(errors) => errors.length ? <Text role="alert" className="text-xs">Enter at least two different artist names.</Text> : null}
            </form.Subscribe>
            {save.error && <Text role="alert" className="text-xs">{save.error.message}</Text>}
            <div className="flex justify-end gap-2 border-t border-solid border-border pt-3">
              <Button type="button" variant="ghost" size="sm" disabled={save.isPending} onClick={() => changeOpen(false)}>Cancel</Button>
              <Button type="submit" size="sm" disabled={save.isPending}>{save.isPending ? "Queuing…" : "Confirm separate artists"}</Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
