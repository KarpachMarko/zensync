import { ConnectionEditor } from "@/components/settings/connection-editor";
import { SectionHeading, Text } from "@/components/typography";
import { useApiQuery } from "@/hooks/use-api";
import { emptyConnection, type ConnectionsData } from "@/lib/connections";

export function MusicProviders() {
  const result = useApiQuery<ConnectionsData>("/connections");
  const data = result.data;
  return (
    <>
      <SectionHeading>Music providers</SectionHeading>
      <Text>
        Connect the services ZenSync uses to find and download missing music.
        Quality preferences apply to new requests.
      </Text>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {data && (
        <>
          <ConnectionEditor
            kind="deezer"
            title="Deezer"
            saved={data.deezerConfigured}
            initial={emptyConnection}
            description={`${data.deezerConfigured ? "An ARL cookie is saved. Enter a new one to replace it." : "Enter your Deezer ARL cookie to enable downloads."} Downloader ${data.downloaderVersion}: ${data.downloaderReady ? "ready" : "not installed"}.`}
          />
          <ConnectionEditor
            kind="lossless-login"
            title="Lossless.am"
            saved={!!data.losslessConfigured}
            initial={{ ...emptyConnection, username: data.losslessEmail ?? "" }}
            description={
              data.losslessConfigured
                ? "Connected. Access renews automatically until the session expires or is revoked. Enter your password to sign in again; it is not stored."
                : "Enter your lossless.am email and password. ZenSync saves an encrypted session and renews access automatically. Your password is not stored."
            }
          />
        </>
      )}
    </>
  );
}
