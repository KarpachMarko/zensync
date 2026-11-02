import { useEffect, useState } from "react";
import { api } from "./api";

type Key = { id: string; created: string };
export function DeletionKeys() {
  const [keys, setKeys] = useState<Key[]>([]);
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function load() {
    setKeys(await api<Key[]>("/deletion-api-keys"));
  }
  useEffect(() => {
    load().catch((e) => setError(e.message));
  }, []);
  async function change(id?: string) {
    setBusy(true);
    setError("");
    setToken("");
    try {
      if (id) await api(`/deletion-api-keys/${id}`, "DELETE");
      else {
        const result = await api<{ token: string }>(
          "/deletion-api-keys",
          "POST",
          {},
        );
        setToken(result.token);
      }
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <h2>Deletion API keys</h2>
      <p>
        Connect an external client to request removal of songs from the shared
        library. Requests use your current deletion permission; otherwise an
        administrator must approve.
      </p>
      <p className="muted">
        These keys can submit deletion requests and read their own status.
        Approval and permission changes require signing in to ZenSync.
      </p>
      {error && <p role="alert">{error}</p>}
      {token && (
        <label>
          Copy this key now. It is shown only once.
          <input
            readOnly
            value={token}
            onFocus={(e) => e.currentTarget.select()}
            autoComplete="off"
            spellCheck={false}
          />
        </label>
      )}
      <button disabled={busy} onClick={() => void change()}>
        Create deletion API key
      </button>
      {keys.map((key) => (
        <div className="row" key={key.id}>
          <div>
            <strong>Deletion key · {key.id.slice(0, 8)}</strong>
            <small>Created {key.created}</small>
          </div>
          <button
            className="quiet"
            disabled={busy}
            onClick={() => void change(key.id)}
          >
            Revoke
          </button>
        </div>
      ))}
    </>
  );
}
