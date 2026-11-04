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
  kind: "shared" | "personal" | "deezer";
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
        kind === "deezer"
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
      kind === "deezer"
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
            <Input
              type="password"
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
              {kind === "shared" ? "Service username" : "Personal username"}
              <Input
                autoComplete="off"
                disabled={!ready}
                value={editor.value.username}
                onChange={(event) => update("username", event.target.value)}
                required
              />
            </FieldLabel>
            <FieldLabel>
              {kind === "shared" ? "Service password" : "Personal password"}
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
              ? "Verifying and saving…"
              : "Saves automatically when the required fields are complete. Credentials are cleared after saving."}
        </Caption>
      </form>
    </SettingsSection>
  );
}
