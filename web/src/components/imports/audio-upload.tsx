import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Text } from "@/components/typography";
import { useAppForm } from "@/hooks/use-app-form";
import { requestKey, uploadAudioFile } from "@/lib/api";
import { audioUploadSchema } from "@/lib/form-schemas";
import type { Source } from "@/lib/migration";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UploadIcon } from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

export function AudioUpload({ source, prefix, busy, onBusyChange, onUploaded }: {
  source: Source;
  prefix: string;
  busy: boolean;
  onBusyChange: (busy: boolean) => void;
  onUploaded: () => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const attempt = useRef<{ file: File; key: string } | null>(null);
  const client = useQueryClient();
  const upload = useMutation({
    mutationFn: ({ file, key }: { file: File; key: string }) => uploadAudioFile(prefix + "/upload", file, key),
  });
  const pendingRequest = !!source.requestId && ["approved", "awaiting_approval"].includes(source.decision) && !["failed", "cancelled", "available"].includes(source.jobState);
  const form = useAppForm({
    defaultValues: { file: null as File | null },
    validators: { onSubmit: audioUploadSchema },
    onSubmit: async ({ value }) => {
      if (!value.file || busy || pendingRequest) return;
      if (attempt.current?.file !== value.file) attempt.current = { file: value.file, key: requestKey() };
      onBusyChange(true);
      setError("");
      try {
        await upload.mutateAsync(attempt.current);
        for (const area of ["/imports", "/requests", "/activity"]) {
          void client.invalidateQueries({
            predicate: (query) => query.queryKey[0] === "api" && typeof query.queryKey[1] === "string" && query.queryKey[1].startsWith(area),
          });
        }
        await onUploaded();
        form.reset();
        attempt.current = null;
        setOpen(false);
        toast.success("Audio uploaded. Follow approval and library indexing in Requests and Activity.");
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : "Audio upload failed. Try again.");
      } finally {
        onBusyChange(false);
      }
    },
  });
  return (
    <Collapsible open={open} onOpenChange={(next) => { if (!busy) setOpen(next); }} className="rounded-md border border-solid border-border px-3 py-2">
      <div className="flex flex-wrap items-center gap-2">
        <CollapsibleTrigger asChild>
          <Button variant="outline" disabled={busy || pendingRequest} className="inline-flex items-center gap-1.5 text-xs">
            <UploadIcon className="size-3.5" aria-hidden="true" /> Upload your version
          </Button>
        </CollapsibleTrigger>
        <span className="text-xs text-muted-foreground">{pendingRequest ? "Finish or withdraw the current request before uploading another recording." : "Use a local audio file when the recording is missing or you prefer your own version."}</span>
      </div>
      <CollapsibleContent className="pt-3">
        <Text className="m-0 mb-3 text-xs">
          Upload one audio file, up to 1 GiB. It will be validated and linked to this imported song after approval and Navidrome indexing. Audio is not re-encoded; existing library files are preserved.
        </Text>
        <form noValidate onSubmit={(event) => { event.preventDefault(); void form.handleSubmit(); }}>
          <form.AppField name="file">
            {(field) => <field.FileField
              label="Your audio file"
              description="FLAC, MP3, M4A, OGG, OPUS, WAV or AIFF · up to 1 GiB"
              accept=".flac,.mp3,.m4a,.ogg,.opus,.wav,.aif,.aiff"
              disabled={busy}
              onFileValidate={(file) => {
                const result = audioUploadSchema.shape.file.safeParse(file);
                return result.success ? null : result.error.issues[0]?.message;
              }}
            />}
          </form.AppField>
          {error && <Text role="alert" className="my-2 text-xs">{error}</Text>}
          <Button disabled={busy || pendingRequest}>{upload.isPending ? "Uploading and validating…" : "Upload and import"}</Button>
        </form>
      </CollapsibleContent>
    </Collapsible>
  );
}
