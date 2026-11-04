import { DeleteMigration } from "@/components/imports/delete-migration";
import { Actions } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { StatusBadge } from "@/components/status";
import { Caption, PageHeading, Text } from "@/components/typography";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import type { Detail, Playlist } from "@/lib/migration";
import { AlertCircleIcon } from "lucide-react";

export function MigrationHeader({
  detail, currentPlaylist, errors, busy, onAction, onDeleted,
}: {
  detail: Detail;
  currentPlaylist?: Playlist;
  errors: (string | undefined)[];
  busy: boolean;
  onAction: (action: string) => Promise<void>;
  onDeleted: () => void;
}) {
  const { migration, songs, playlists, matchTask } = detail;
  const scanning = ["queued", "running"].includes(matchTask?.state ?? "");
  const scanFailed = migration.state === "open" && matchTask?.state === "failed";
  const messages = [...new Set([
    ...errors, migration.error, currentPlaylist?.error,
    scanFailed ? matchTask?.message : undefined,
  ].map((message) => message?.trim()).filter((message): message is string => !!message))];
  const available = songs.filter((song) => song.serverId).length;
  const selectedPlaylists = playlists.filter((playlist) => playlist.selected);

  return (
    <header data-slot="migration-header" className="mb-6 flex flex-col items-stretch gap-5 lg:flex-row lg:items-start">
      <div className="min-w-0 flex-1 space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <PageHeading className="m-0 break-words" tabIndex={-1}>{currentPlaylist?.name || migration.name}</PageHeading>
          <Actions>
            <StatusBadge state={migration.state} />
            <DeleteMigration migration={migration} onDeleted={onDeleted} />
          </Actions>
        </div>
        {migration.state !== "staged" && (
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
            <span className="text-sm text-muted-foreground"><strong className="text-xl text-foreground">{available}</strong> of {songs.length} songs available</span>
            <Progress aria-label="Migration availability" max={Math.max(1, songs.length)} value={available} className="h-1.5 w-[180px]" indicatorClassName="bg-[#a2d5bd]" />
          </div>
        )}
        {(messages.length > 0 || scanFailed) && (
          <Alert className="box-border border-solid border-destructive/40 bg-destructive/5">
            <AlertCircleIcon aria-hidden="true" />
            <AlertTitle>{scanFailed ? "Match scan failed" : "Migration needs attention"}</AlertTitle>
            <AlertDescription className="space-y-2 text-sm">
              {messages.map((message) => <div key={message}>{message}</div>)}
              {scanFailed && <div>Check your connections, then use Scan again.</div>}
              <Actions className="gap-3 text-xs">
                <AppLink to="/settings/connections">Check connections</AppLink>
                <AppLink to="/activity">Open Activity</AppLink>
              </Actions>
            </AlertDescription>
          </Alert>
        )}
        {migration.state === "open" && (
          <>
            {scanning && (
              <Text role="status" className="m-0 text-xs">
                {matchTask?.state === "queued" ? "Match scan queued. It will start automatically." : matchTask?.message || "Scanning Navidrome and Deezer…"}
              </Text>
            )}
            <Actions>
              <Button disabled={busy || scanning} onClick={() => void onAction("match")}>{scanning ? "Scanning…" : "Scan again"}</Button>
              <Button disabled={busy} onClick={() => void onAction("publish")}>{migration.targetUser ? "Publish available songs" : "Publish to Navidrome"}</Button>
              <Button variant="outline" disabled={busy} onClick={() => {
                if (window.confirm("Finish this migration? ZenSync will stop all playlist and favorite updates. Pending acquisitions will continue and unresolved entries will remain in the report. This cannot be undone.")) void onAction("handover");
              }}>Finish migration</Button>
            </Actions>
          </>
        )}
        {!["staged", "open"].includes(migration.state) && (
          <Text className="m-0 text-sm">Navidrome now owns these collections. Pending acquisitions continue, but will not change your playlists or favorites.</Text>
        )}
      </div>
      {migration.state !== "staged" && (
        <Card size="sm" className="min-w-0 shrink-0 gap-2 lg:w-[300px]" role="region" aria-labelledby="migration-playlists-title">
          <CardHeader><CardTitle id="migration-playlists-title">Playlists ({selectedPlaylists.length})</CardTitle></CardHeader>
          <CardContent className="max-h-[260px] overflow-y-auto">
            {selectedPlaylists.length === 0 ? <Text className="m-0 text-xs">No playlists selected. All songs remain in your catalog.</Text> : (
              <nav aria-label="Migration playlists">
                {selectedPlaylists.map((playlist) => (
                  <AppLink
                    key={playlist.id}
                    to={"/imports/" + migration.id + "/playlists/" + playlist.id}
                    aria-current={playlist.id === currentPlaylist?.id ? "page" : undefined}
                    className="flex items-start justify-between gap-3 rounded-md px-2 py-2 text-foreground no-underline hover:bg-accent aria-[current=page]:bg-accent"
                  >
                    <div className="min-w-0 break-words">
                      <strong className="text-sm">{playlist.name}</strong>
                      <Caption>{playlist.entries.length} ordered entries / {playlist.remoteId ? "Published" : "Not published yet"}</Caption>
                    </div>
                    <StatusBadge state={playlist.state} />
                  </AppLink>
                ))}
              </nav>
            )}
          </CardContent>
        </Card>
      )}
    </header>
  );
}
