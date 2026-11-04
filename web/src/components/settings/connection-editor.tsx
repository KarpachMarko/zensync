import { useState } from "react";
import { FieldLabel } from "@/components/forms";
import { FieldGrid, SettingsSection } from "@/components/layout/content";
import { Caption, Subheading, Text } from "@/components/typography";
import { Input } from "@/components/ui/input";
import { SavedSecretInput } from "@/components/settings/saved-secret-input";
import { useApiMutation } from "@/hooks/use-api";
import { useAutosave } from "@/hooks/use-autosave";
import { connectionSchema, type ConnectionKind } from "@/lib/form-schemas";
import type { ConnectionDraft } from "@/lib/connections";

export function ConnectionEditor({
  kind,
  title,
  description,
  initial,
  ready = true,
  saved = false,
}: {
  kind: ConnectionKind;
  title: string;
  description: string;
  initial: ConnectionDraft;
  ready?: boolean;
  saved?: boolean;
}) {
  const [savedHere, setSavedHere] = useState(false);
  const { api } = useApiMutation({ errorToastId: "connection-" + kind });
  const editor = useAutosave({
    initialValue: initial,
    delay: 1200,
    rollbackOnError: false,
    schema: connectionSchema(kind, ready),
    save: async (draft) => {
      await api(
        "/connections",
        "PUT",
        kind === "lossless-login"
          ? { kind, email: draft.username.trim(), password: draft.password }
          : kind === "deezer"
            ? { kind, arl: draft.arl }
            : {
                kind,
                url: draft.url,
                username: draft.username,
                password: draft.password,
              },
      );
      setSavedHere(true);
    },
    normalizeSaved: (draft) => ({
      ...draft,
      username: draft.username.trim(),
      password: "",
      arl: "",
    }),
    message:
      kind === "lossless-login"
        ? "Lossless.am connected."
        : kind === "deezer"
          ? "Deezer credentials saved."
          : kind === "shared"
            ? "Navidrome server verified and saved."
            : "Personal Navidrome account linked.",
    toastId: "connection-" + kind,
  });
  function update(field: keyof ConnectionDraft, value: string) {
    editor.update({ ...editor.value, [field]: value });
  }
  return (
    <SettingsSection>
      <Subheading>{title}</Subheading>
      <Text>{description}</Text>
      <form
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void editor.form.handleSubmit();
          editor.flush();
        }}
        onBlur={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget))
            editor.flush();
        }}
      >
        {kind === "shared" && (
          <FieldLabel>
            Navidrome URL
            <Input
              type="url"
              value={editor.value.url}
              placeholder="http://navidrome:4533"
              onChange={(event) => update("url", event.target.value)}
              required
            />
          </FieldLabel>
        )}
        {kind === "deezer" ? (
          <FieldLabel>
            Deezer ARL cookie
            <SavedSecretInput
              saved={saved || savedHere}
              autoComplete="new-password"
              minLength={32}
              maxLength={4096}
              value={editor.value.arl}
              onChange={(event) => update("arl", event.target.value)}
            />
          </FieldLabel>
        ) : (
          <FieldGrid>
            <FieldLabel>
              {kind === "lossless-login"
                ? "Lossless.am email"
                : kind === "shared"
                  ? "Service username"
                  : "Personal username"}
              <Input
                autoComplete="off"
                disabled={!ready}
                value={editor.value.username}
                onChange={(event) => update("username", event.target.value)}
                required
              />
            </FieldLabel>
            <FieldLabel>
              {kind === "lossless-login"
                ? "Lossless.am password"
                : kind === "shared"
                  ? "Service password"
                  : "Personal password"}
              <SavedSecretInput
                saved={saved || savedHere}
                autoComplete="new-password"
                disabled={!ready}
                value={editor.value.password}
                onChange={(event) => update("password", event.target.value)}
                required
              />
            </FieldLabel>
          </FieldGrid>
        )}
        {editor.error && <Text role="alert">{editor.error}</Text>}
        <Caption role={editor.saving ? "status" : undefined}>
          {!ready
            ? "Connect the shared server first."
            : editor.saving
              ? "Verifying and saving…"
              : "Saves automatically when the required fields are complete. Saved credentials appear masked. Enter a replacement to change them."}
        </Caption>
      </form>
    </SettingsSection>
  );
}
