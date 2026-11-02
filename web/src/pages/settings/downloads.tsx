import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import { useState } from "react";
export function DownloadSettings() {
  const { api } = useApiMutation();
  const [draftLimit, setLimit] = useState<number>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const result = useApiQuery<{ parallelDownloads: number }>(
    "/settings/downloads",
  );
  const loaded = result.isSuccess;
  const limit = draftLimit ?? result.data?.parallelDownloads ?? 3;
  return (
    <>
      <h2>Parallel downloads</h2>
      <p>
        Choose how many downloads can run at the same time across all users.
      </p>
      {(error || result.error) && (
        <p role="alert">{error || result.error?.message}</p>
      )}
      {message && <p role="status">{message}</p>}
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          setMessage("");
          try {
            await api("/settings/downloads", "PUT", {
              parallelDownloads: limit,
            });
            setMessage("Download limit saved.");
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <label>
          Parallel downloads
          <Input
            type="number"
            min={1}
            max={8}
            step={1}
            required
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value))}
          />
        </label>
        <p className="muted">
          1–8 downloads; default 3. Changes apply immediately. Lowering the
          limit lets active transfers finish before starting more. Queued and
          waiting-for-scan jobs do not hold idle download slots.
        </p>
        <Button disabled={!loaded || busy}>Save download limit</Button>
      </form>
    </>
  );
}
