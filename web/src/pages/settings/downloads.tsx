import { FieldLabel } from "@/components/forms";
import { Caption, SectionHeading, Text } from "@/components/typography";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import { useAutosave } from "@/hooks/use-autosave";
import { downloadLimitSchema } from "@/lib/form-schemas";

export function DownloadSettings() {
  const result = useApiQuery<{ parallelDownloads: number }>(
    "/settings/downloads",
  );
  return (
    <>
      <SectionHeading>Parallel downloads</SectionHeading>
      <Text>
        Choose how many downloads can run at the same time across the workspace.
      </Text>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {result.data ? (
        <DownloadEditor initialValue={result.data.parallelDownloads} />
      ) : (
        <Text>Loading download settings…</Text>
      )}
    </>
  );
}

function DownloadEditor({ initialValue }: { initialValue: number }) {
  const { api } = useApiMutation({ errorToastId: "download-settings" });
  const editor = useAutosave({
    initialValue: String(initialValue),
    delay: 600,
    schema: downloadLimitSchema,
    save: async (value) => {
      await api("/settings/downloads", "PUT", {
        parallelDownloads: Number(value),
      });
    },
    message: "Download limit saved.",
    toastId: "download-settings",
  });
  return (
    <div className="max-w-[700px]">
      <FieldLabel>
        Parallel downloads
        <Input
          type="number"
          min={1}
          step={1}
          className="max-w-36"
          value={editor.value}
          onChange={(event) => editor.update(event.target.value)}
          onBlur={editor.flush}
          aria-invalid={!!editor.validationError}
        />
      </FieldLabel>
      {editor.validationError && (
        <Text role="alert">{editor.validationError}</Text>
      )}
      {editor.error && <Text role="alert">{editor.error}</Text>}
      {editor.saving && <Caption role="status">Saving…</Caption>}
      <Text className="text-[13px]">
        Changes save automatically. There is no fixed upper limit. Lowering the
        limit lets active transfers finish; waiting-for-scan jobs do not hold a
        download slot.
      </Text>
    </div>
  );
}
