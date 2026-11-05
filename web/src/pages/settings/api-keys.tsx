import { ValidationErrors } from "@/components/form-fields";
import { FieldLabel } from "@/components/forms";
import { Row } from "@/components/layout/content";
import { Caption, SectionHeading, Text } from "@/components/typography";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import { useAppForm } from "@/hooks/use-app-form";
import { apiKeySchema, apiKeyScopes, type APIKey } from "@/lib/api-keys";
import { useState } from "react";
import { toast } from "sonner";

export function ApiKeys() {
  const { api, isPending } = useApiMutation();
  const result = useApiQuery<APIKey[]>("/api-keys");
  const [token, setToken] = useState("");
  const [actionError, setError] = useState("");
  const form = useAppForm({
    defaultValues: { label: "", expires: "", scopes: ["search:read", "library:read", "requests:read", "requests:create"] },
    validators: { onSubmit: apiKeySchema },
    onSubmit: async ({ value }) => {
      setError("");
      setToken("");
      try {
        const created = await api<APIKey & { token: string }>("/api-keys", "POST", {
          label: value.label.trim(),
          expires: value.expires ? Math.floor(new Date(value.expires).getTime() / 1000) : null,
          scopes: value.scopes,
        });
        setToken(created.token);
        form.reset();
        toast.success("API key created. Copy it now.");
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : "Unable to create API key");
      }
    },
  });
  return (
    <div className="max-w-[780px]">
      <SectionHeading>API keys</SectionHeading>
      <Text>Give each app one key with the scopes it needs. Download and deletion requests follow your current account permissions and approval rules.</Text>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {token && (
        <FieldLabel>
          Copy this key now. It is shown only once.
          <Input readOnly value={token} onFocus={(event) => event.currentTarget.select()} autoComplete="off" spellCheck={false} />
        </FieldLabel>
      )}
      <form noValidate className="my-6" onSubmit={(event) => { event.preventDefault(); void form.handleSubmit(); }}>
        <form.AppField name="label">
          {(field) => <field.TextField label="Label" placeholder="e.g. My music app" autoComplete="off" disabled={isPending} />}
        </form.AppField>
        <form.AppField name="expires">
          {(field) => <field.TextField label="Expiration date (optional)" type="datetime-local" disabled={isPending} />}
        </form.AppField>
        <Caption>Leave empty for a key that does not expire. Dates use your local time.</Caption>
        <form.Field name="scopes">
          {(field) => (
            <fieldset className="my-5 space-y-3 border-0 p-0" disabled={isPending}>
              <legend className="mb-3 text-sm font-semibold">Scopes</legend>
              {apiKeyScopes.map((scope) => (
                <FieldLabel key={scope.id} className="m-0 flex-row items-center gap-3">
                  <Checkbox checked={field.state.value.includes(scope.id)} onBlur={field.handleBlur} onCheckedChange={(checked) => field.handleChange(checked === true ? [...field.state.value, scope.id] : field.state.value.filter((value) => value !== scope.id))} />
                  {scope.label}
                </FieldLabel>
              ))}
              <ValidationErrors errors={field.state.meta.errors} />
            </fieldset>
          )}
        </form.Field>
        {actionError && <Text role="alert">{actionError}</Text>}
        <Button disabled={isPending}>{isPending ? "Creating API key…" : "Create API key"}</Button>
      </form>
      {result.data?.length === 0 && <Text>No API keys yet.</Text>}
      {result.data?.map((key) => (
        <Row key={key.id} className="flex-wrap gap-4">
          <div className="min-w-0 flex-1">
            <strong className="break-words">{key.label}</strong>
            <Caption>Created {key.created} · {key.expires ? `Expires ${new Date(key.expires * 1000).toLocaleString()}` : "No expiration"}</Caption>
            <div className="mt-2 flex flex-wrap gap-2">
              {key.scopes.map((scope) => <Badge key={scope} variant="secondary">{scope}</Badge>)}
              {key.expires !== null && key.expires * 1000 <= Date.now() && <Badge variant="outline">Expired</Badge>}
            </div>
          </div>
          <Button variant="outline" disabled={isPending} aria-label={`Revoke ${key.label}`} onClick={async () => {
            try {
              await api(`/api-keys/${key.id}`, "DELETE");
              setToken("");
              toast.success("API key revoked.");
            } catch { /* Shared mutation hook reports failures. */ }
          }}>Revoke</Button>
        </Row>
      ))}
    </div>
  );
}
