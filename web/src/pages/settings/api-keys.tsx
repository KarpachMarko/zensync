import { FieldLabel } from "@/components/forms";
import { Row } from "@/components/layout/content";
import { Caption, SectionHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import { useState } from "react";

type Key = { id: string; created: string };
export function DeletionKeys() {
  const { api } = useApiMutation();
  const result = useApiQuery<Key[]>("/deletion-api-keys");
  const keys = result.data ?? [];
  const [token, setToken] = useState("");
  const [actionError, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const load = result.reload;
  const error = actionError || result.error?.message;
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
      <SectionHeading>Deletion API keys</SectionHeading>
      <Text>
        Connect an external client to request removal of songs from the shared
        library. Requests use your current deletion permission; otherwise an
        administrator must approve.
      </Text>
      <Text className="text-[13px]">
        These keys can submit deletion requests and read their own status.
        Approval and permission changes require signing in to ZenSync.
      </Text>
      {error && <Text role="alert">{error}</Text>}
      {token && (
        <FieldLabel>
          Copy this key now. It is shown only once.
          <Input
            readOnly
            value={token}
            onFocus={(e) => e.currentTarget.select()}
            autoComplete="off"
            spellCheck={false}
          />
        </FieldLabel>
      )}
      <Button disabled={busy} onClick={() => void change()}>
        Create deletion API key
      </Button>
      {keys.map((key) => (
        <Row key={key.id}>
          <div>
            <strong>Deletion key · {key.id.slice(0, 8)}</strong>
            <Caption>Created {key.created}</Caption>
          </div>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => void change(key.id)}
          >
            Revoke
          </Button>
        </Row>
      ))}
    </>
  );
}
