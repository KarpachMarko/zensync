import { useEffect, useState } from "react";
import { api } from "./api";
export function DownloadSettings() {
  const [limit, setLimit] = useState(3),
    [loaded, setLoaded] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  useEffect(() => {
    api<{ parallelDownloads: number }>("/settings/downloads")
      .then((s) => {
        setLimit(s.parallelDownloads);
        setLoaded(true);
      })
      .catch((e) => setError(e.message));
  }, []);
  return (
    <>
      <h2>Download concurrency</h2>
      <p>
        Choose how many music acquisitions can run at the same time across all
        users.
      </p>
      {error && <p role="alert">{error}</p>}
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
          <input
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
        <button disabled={!loaded || busy}>Save download limit</button>
      </form>
    </>
  );
}
