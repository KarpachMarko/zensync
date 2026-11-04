import { FieldLabel } from "@/components/forms";
import { FieldGrid, SettingsSection } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { StatusPill } from "@/components/status";
import {
  Caption,
  SectionHeading,
  Subheading,
  Text,
} from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import { useAutosave } from "@/hooks/use-autosave";
import { connectionSchema } from "@/lib/form-schemas";

type ConnectionsData = {
  url: string;
  serviceUsername?: string;
  configured: boolean;
  personalUsername: string;
  deezerConfigured: boolean;
  losslessConfigured?: boolean;
  downloaderReady: boolean;
  musicMounted: boolean;
  downloaderVersion: string;
};
type ConnectionDraft = {
  url: string;
  username: string;
  password: string;
  arl: string;
};

export function Connections({ admin }: { admin: boolean }) {
  const result = useApiQuery<ConnectionsData>("/connections"),
    data = result.data;
  return (
    <>
      <SectionHeading>Connections</SectionHeading>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {data && (
        <>
          <div className="flex flex-wrap items-center gap-4">
            <StatusPill>
              Navidrome: {data.configured ? "connected" : "not configured"}
            </StatusPill>
            <StatusPill>
              d-fi {data.downloaderVersion}:{" "}
              {data.downloaderReady ? "installed" : "not found"}
            </StatusPill>
            <StatusPill>
              Music mount: {data.musicMounted ? "present" : "missing"}
            </StatusPill>
          </div>
          {admin && (
            <ConnectionEditor
              kind="shared"
              title="Shared Navidrome server"
              initial={{
                url: data.url,
                username: data.serviceUsername ?? "",
                password: "",
                arl: "",
              }}
              description="Use a service account with library access and scan permission. Enter its password to verify and save changes automatically."
            />
          )}
          <ConnectionEditor
            kind="personal"
            title="Your personal Navidrome account"
            ready={data.configured}
            initial={{
              url: data.url,
              username: data.personalUsername,
              password: "",
              arl: "",
            }}
            description={
              data.personalUsername
                ? `Playlists and favorites are published to ${data.personalUsername}. Enter your password to change this link.`
                : "Link the account that should receive your playlists and favorites. This is separate from ZenSync sign-in."
            }
          />
          {admin && (
            <ConnectionEditor
              kind="deezer"
              title="Deezer acquisition"
              initial={{ url: "", username: "", password: "", arl: "" }}
              description={
                data.deezerConfigured
                  ? "An ARL cookie is saved. Enter a new one to replace it automatically."
                  : "Enter your Deezer ARL cookie to enable missing-song acquisition."
              }
            />
          )}
          {admin && (
            <ConnectionEditor
              kind="lossless-login"
              title="Lossless.am sign-in"
              initial={{ url: "", username: "", password: "", arl: "" }}
              description={
                data.losslessConfigured
                  ? "A session is saved. Sign in to replace it. ZenSync renews access automatically until the refresh session expires or is revoked. Your password is not stored."
                  : "Enter your lossless.am email and password to connect. ZenSync saves an encrypted session and renews access automatically. Your password is not stored."
              }
            />
          )}
          {admin && (
            <ConnectionEditor
              kind="lossless"
              title="Lossless.am cookie connection (alternative)"
              initial={{ url: "", username: "", password: "", arl: "" }}
              description={
                data.losslessConfigured
                  ? "Enter a Cookie header to replace the saved connection instead of using email/password."
                  : "Search and FLAC downloads use lossless.am. If the service requires login, copy the Cookie header from an authenticated request in your browser. The saved session is encrypted and can renew access when a refresh cookie is included; it is not verified when saved."
              }
            />
          )}
          <Text>
            <Button asChild>
              <AppLink to="/imports/new">Import Apple Music export</AppLink>
            </Button>
          </Text>
        </>
      )}
    </>
  );
}

function ConnectionEditor({
  kind,
  title,
  description,
  initial,
  ready = true,
}: {
  kind: "shared" | "personal" | "deezer" | "lossless" | "lossless-login";
  title: string;
  description: string;
  initial: ConnectionDraft;
  ready?: boolean;
}) {
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
          : kind === "deezer" || kind === "lossless"
            ? { kind, arl: draft.arl }
            : {
                kind,
                url: draft.url,
                username: draft.username,
                password: draft.password,
              },
      );
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
        : kind === "deezer" || kind === "lossless"
          ? kind === "lossless"
            ? "Lossless.am session saved."
            : "Deezer credentials saved."
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
        {kind === "deezer" || kind === "lossless" ? (
          <FieldLabel>
            {kind === "lossless"
              ? "Lossless.am session Cookie header"
              : "Deezer ARL cookie"}
            <Input
              type="password"
              autoComplete="new-password"
              minLength={kind === "lossless" ? 3 : 32}
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
              <Input
                type="password"
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
              ? kind === "lossless"
                ? "Saving session…"
                : "Verifying and saving…"
              : "Saves automatically when the required fields are complete. Credentials are cleared after saving."}
        </Caption>
      </form>
    </SettingsSection>
  );
}
