import { FieldLabel } from "@/components/forms";
import { FieldGrid, SettingsSection } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { StatusPill } from "@/components/status";
import { SectionHeading, Subheading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import React, { useState } from "react";
type ConnectionsData = {
  url: string;
  configured: boolean;
  personalUsername: string;
  deezerConfigured: boolean;
  downloaderReady: boolean;
  musicMounted: boolean;
  downloaderVersion: string;
};
export function Connections({ admin }: { admin: boolean }) {
  const { api } = useApiMutation();
  const [actionError, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false);
  const result = useApiQuery<ConnectionsData>("/connections");
  const data = result.data;
  const error = actionError || result.error?.message;
  const load = result.reload;
  async function save(e: React.FormEvent<HTMLFormElement>, kind: string) {
    e.preventDefault();
    const form = e.currentTarget;
    const input = Object.fromEntries(new FormData(form));
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api("/connections", "PUT", { kind, ...input });
      form.reset();
      await load();
      setNotice(
        kind === "deezer"
          ? "Deezer credentials saved. They will be verified during the first acquisition."
          : "Navidrome account verified and saved.",
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <SectionHeading>Connections</SectionHeading>
      {error && <Text role="alert">{error}</Text>}
      {notice && <Text role="status">{notice}</Text>}
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
            <SettingsSection>
              <Subheading>Shared Navidrome server</Subheading>
              <Text>
                Use a service account with library access and scan permission.
                In Docker, localhost is this container; use the server’s
                reachable hostname.
              </Text>
              <form onSubmit={(e) => void save(e, "shared")}>
                <FieldLabel>
                  Navidrome URL
                  <Input
                    type="url"
                    name="url"
                    defaultValue={data.url}
                    placeholder="http://navidrome:4533"
                    required
                  />
                </FieldLabel>
                <FieldGrid>
                  <FieldLabel>
                    Service username
                    <Input name="username" autoComplete="off" required />
                  </FieldLabel>
                  <FieldLabel>
                    Service password
                    <Input
                      name="password"
                      type="password"
                      autoComplete="new-password"
                      required
                    />
                  </FieldLabel>
                </FieldGrid>
                <Button disabled={busy}>Test and save server</Button>
              </form>
            </SettingsSection>
          )}
          <SettingsSection>
            <Subheading>Your personal Navidrome account</Subheading>
            <Text>
              Playlists and favorites will be published to{" "}
              {data.personalUsername ? (
                <strong>{data.personalUsername}</strong>
              ) : (
                "this account"
              )}
              . This link is separate from ZenSync sign-in.
            </Text>
            <form onSubmit={(e) => void save(e, "personal")}>
              <FieldGrid>
                <FieldLabel>
                  Personal username
                  <Input
                    name="username"
                    defaultValue={data.personalUsername}
                    autoComplete="off"
                    required
                  />
                </FieldLabel>
                <FieldLabel>
                  Personal password
                  <Input
                    name="password"
                    type="password"
                    autoComplete="new-password"
                    required
                  />
                </FieldLabel>
              </FieldGrid>
              <Button disabled={busy || !data.configured}>
                Test and link account
              </Button>
            </form>
          </SettingsSection>
          {admin && (
            <SettingsSection>
              <Subheading>Deezer acquisition</Subheading>
              <Text>
                {data.deezerConfigured
                  ? "An ARL cookie is saved. Enter a new one to replace it."
                  : "Enter your Deezer ARL cookie to enable missing-song acquisition."}{" "}
                Credentials are encrypted on disk and are never returned to the
                browser.
              </Text>
              <form onSubmit={(e) => void save(e, "deezer")}>
                <FieldLabel>
                  Deezer ARL cookie
                  <Input
                    name="arl"
                    type="password"
                    autoComplete="new-password"
                    required
                    minLength={32}
                  />
                </FieldLabel>
                <Button disabled={busy}>Save Deezer credentials</Button>
              </form>
            </SettingsSection>
          )}
          <Text>
            <Button asChild>
              <AppLink to="/imports/new">Import Apple Music XML</AppLink>
            </Button>
          </Text>
        </>
      )}
    </>
  );
}
