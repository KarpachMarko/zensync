import { ConnectionEditor } from "@/components/settings/connection-editor";
import { StatusPill } from "@/components/status";
import { SectionHeading, Text } from "@/components/typography";
import { useApiQuery } from "@/hooks/use-api";
import type { ConnectionsData } from "@/lib/connections";

export function Connections({ admin }: { admin: boolean }) {
  const result = useApiQuery<ConnectionsData>("/connections");
  const data = result.data;
  return (
    <>
      <SectionHeading>Navidrome connections</SectionHeading>
      <Text>
        Connect your music library and choose the account that receives your
        playlists and favorites.
      </Text>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {data && (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-3">
            <StatusPill>
              Server: {data.configured ? "connected" : "not configured"}
            </StatusPill>
            <StatusPill>
              Music mount: {data.musicMounted ? "present" : "missing"}
            </StatusPill>
          </div>
          {admin && (
            <ConnectionEditor
              kind="shared"
              title="Shared Navidrome server"
              saved={data.servicePasswordConfigured ?? data.configured}
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
            saved={data.personalPasswordConfigured ?? !!data.personalUsername}
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
        </>
      )}
    </>
  );
}
