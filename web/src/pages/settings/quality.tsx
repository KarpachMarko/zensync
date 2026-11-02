import { Button } from "@/components/ui/button";
import { useApiMutation } from "@/hooks/use-api";
import type { User } from "@/lib/types";
import { useState } from "react";
export function Quality({
  user,
  reload,
}: {
  user: User;
  reload: () => Promise<void>;
}) {
  const { api } = useApiMutation();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  return (
    <>
      {message && <p role="status">{message}</p>}
      <h2>Your quality preference</h2>
      <p>Requests preserve a snapshot of this preference when submitted.</p>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          try {
            await api("/quality", "PUT", {
              quality: String(
                new FormData(e.currentTarget).get("quality"),
              ).split(","),
            });
            await reload();
            setMessage("Quality preference saved.");
          } catch (e) {
            setMessage((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <label>
          Allowed qualities, in order
          <select name="quality" defaultValue={user.quality.join(",")}>
            <option value="lossless,320,128">Lossless → 320 → 128</option>
            <option value="lossless">Lossless only</option>
            <option value="320,128">320 → 128</option>
            <option value="320">320 only</option>
            <option value="128">128 only</option>
          </select>
        </label>
        <Button disabled={busy}>Save preference</Button>
      </form>
      <p className="muted">
        d-fi tries only the allowed qualities in this order. Existing playable
        music is reused; replacing shared audio is not enabled yet.
      </p>
    </>
  );
}
