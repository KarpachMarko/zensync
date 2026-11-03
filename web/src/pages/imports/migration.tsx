import { DeleteMigration } from "@/components/imports/delete-migration";
import { ExpandableSection } from "@/components/expandable-section";
import { Checkbox, CheckboxField, SearchField } from "@/components/forms";
import { MigrationStatusFilter } from "@/components/imports/migration-status-filter";
import { MigrationChoices } from "@/components/imports/migration-choices";
import { Review } from "@/components/imports/review";
import { Snapshot } from "@/components/imports/snapshot";
import {
  Actions,
  FieldGrid,
  Notice,
  PageTitle,
  Row,
  SearchBar,
} from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { StatusBadge } from "@/components/status";
import {
  Caption,
  PageHeading,
  SectionHeading,
  Text,
} from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import { recordingAction, status, type Detail, type Source } from "@/lib/migration";
import { useState } from "react";
import { parseAsArrayOf, parseAsString, useQueryState } from "nuqs";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
export function MigrationPage() {
  const { api } = useApiMutation();
  const { id, sourceId, playlistId } = useParams();
  const [recordingId, setRecordingId] = useQueryState(
    "recording",
    parseAsString.withOptions({ history: "push" }),
  );
  const [hiddenStatuses, setHiddenStatuses] = useQueryState(
    "hideStatus",
    parseAsArrayOf(parseAsString).withDefault(["Available"]),
  );
  const [actionError, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [selected, setSelected] = useState<Set<string>>(new Set()),
    [filter, setFilter] = useState(""),
    [limit, setLimit] = useState(100);
  const navigate = useNavigate();
  const result = useApiQuery<Detail>("/imports/" + id, {
    enabled: !!id,
    refetchInterval: 4000,
  });
  const detail = result.data;
  const error = actionError || result.error?.message;
  const load = result.reload;
  async function action(path: string, body?: unknown) {
    setBusy(true);
    setError("");

    try {
      await api("/imports/" + id + "/" + path, "POST", body);
      await load();
      if (path === "match" || path === "publish")
        toast.success(
          "Task queued. Progress is shown in Activity and updates here.",
        );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function acquireSelected() {
    setBusy(true);
    setError("");
    let saved = 0;
    try {
      for (const song of selectedVisible) {
        const sid = song.id;
        await api("/imports/" + id + "/songs/" + sid + "/acquire", "POST");
        saved++;
      }
      setSelected(new Set());
      await load();
      toast.success(`${saved} acquisition requests saved.`);
    } catch (e) {
      setError(`${saved} requests saved. ${(e as Error).message}`);
      await load();
    } finally {
      setBusy(false);
    }
  }
  if (!detail)
    return (
      <>
        <PageHeading tabIndex={-1}>Migration</PageHeading>
        <Text>{error || "Loading your migration…"}</Text>
      </>
    );
  const { migration: m, songs, playlists, matchTask } = detail;
  const scanning = ["queued", "running"].includes(matchTask?.state ?? "");
  const currentSource = songs.find((s) => s.id === (recordingId ?? sourceId)),
    currentPlaylist = playlists.find((p) => p.id === playlistId);
  const songsById = new Map<string, Source>(songs.map((song) => [song.id, song]));
  const shownSongs = currentPlaylist
    ? currentPlaylist.entries
        .map((sid) => songsById.get(sid))
        .filter((s): s is Source => !!s)
    : songs;
  const statusCounts = new Map<string, number>();
  for (const song of shownSongs) {
    const value = status(song, scanning);
    statusCounts.set(value, (statusCounts.get(value) ?? 0) + 1);
  }
  const shownEntries = shownSongs
    .map((song, index) => ({ song, position: index + 1 }))
    .filter(({ song }) =>
      !hiddenStatuses.includes(status(song, scanning)) &&
      (song.metadata.title + " " + song.metadata.artist + " " + status(song, scanning))
        .toLowerCase().includes(filter.toLowerCase()),
    );
  const shown = shownEntries.map(({ song }) => song);
  const selectedVisible = [...new Map<string, Source>(shown
    .filter((song) => selected.has(song.id) && song.candidate && !song.serverId && !song.deferred && !song.requestId)
    .map((song) => [song.id, song]),
  ).values()];
  const available = songs.filter((s) => s.serverId).length;
  return (
    <>
      <div className="flex gap-[15px] mb-[22px] text-[13px] text-muted-foreground">
        <AppLink to="/imports">Imports</AppLink>
        {playlistId && <AppLink to={"/imports/" + id}>{m.name}</AppLink>}
      </div>
      <PageTitle>
        <PageHeading tabIndex={-1}>
          {currentPlaylist?.name || m.name}
        </PageHeading>
        <Actions>
          <StatusBadge state={m.state} />
          <DeleteMigration
            migration={m}
            onDeleted={() => navigate("/imports", { replace: true })}
          />
        </Actions>
      </PageTitle>
      {error && <Text role="alert">{error}</Text>}
      {m.error && (
        <Notice>
          {m.error} <AppLink to="/activity">Open Activity</AppLink>
        </Notice>
      )}
      {m.state === "staged" ? (
        <>
          <Text>
            <strong className="text-foreground">All {m.report.songs} songs are included</strong>,
            including {m.report.unplaylisted} songs outside playlists. You do not
            need to select Library or Music to include them. Committing
            automatically starts a scan of every song in Navidrome and Deezer.
            Request missing audio after matching; downloads require a separate request.
          </Text>
          <div className="my-6 flex flex-wrap items-center gap-4 text-muted-foreground max-[700px]:gap-2">
            <span>{m.report.unplaylisted} unplaylisted songs</span>
            <span>{m.report.missing} missing references</span>
            <span>{m.report.excluded} non-music records excluded</span>
          </div>
          <MigrationChoices
            playlists={playlists}
            busy={busy}
            onCommit={(values) => action("commit", values)}
          />

          {m.report.warnings.length > 0 && (
            <ExpandableSection
              title={`Import warnings (${m.report.warnings.length})`}
              compact
              defaultOpen={false}
            >
              {m.report.warnings.slice(0, 100).map((w, i) => (
                <Text key={i}>{w}</Text>
              ))}
            </ExpandableSection>
          )}
        </>
      ) : (
        <>
          <div className="my-7 flex items-center gap-5 max-[700px]:flex-wrap [&_strong]:text-[25px] [&_span]:text-sm [&_span]:font-normal [&_span]:text-muted-foreground">
            <strong>
              {available} <span>of {songs.length} songs available</span>
            </strong>
            <Progress
              aria-label="Migration availability"
              max={Math.max(1, songs.length)}
              value={available}
              className="h-[7px] w-[200px]"
              indicatorClassName="bg-[#a2d5bd]"
            />
          </div>
          {m.state === "open" && matchTask?.state === "failed" && (
            <Notice role="alert">
              Match scan failed. Check your connections, then use Scan again.
            </Notice>
          )}
          {m.state === "open" && scanning && (
            <Notice role="status">
              {matchTask?.state === "queued"
                ? "Match scan queued. It will start automatically."
                : matchTask?.message || "Scanning Navidrome and Deezer…"}
            </Notice>
          )}
          {m.state === "open" ? (
            <Actions className="my-[22px] mx-0">
              <Button
                disabled={busy || scanning}
                onClick={() => void action("match")}
              >
                {scanning ? "Scanning…" : "Scan again"}
              </Button>
              <Button disabled={busy} onClick={() => void action("publish")}>
                {m.targetUser
                  ? "Publish available songs"
                  : "Publish to Navidrome"}
              </Button>
              <AppLink to="/settings/connections">Connections</AppLink>
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => {
                  if (
                    window.confirm(
                      "Finish this migration? ZenSync will stop all playlist and favorite updates. Pending acquisitions will continue and unresolved entries will remain in the report. This cannot be undone.",
                    )
                  )
                    void action("handover");
                }}
              >
                Finish migration
              </Button>
            </Actions>
          ) : (
            <Notice>
              Navidrome now owns these collections. Pending acquisitions
              continue, but will not change your playlists or favorites.
            </Notice>
          )}
          {!currentPlaylist && (
            <section className="mb-8">
              <SectionHeading>Migration playlists</SectionHeading>
              {playlists
                .filter((p) => p.selected)
                .map((p) => (
                  <Row asChild key={p.id}>
                    <AppLink
                      className="max-[700px]:items-start no-underline"
                      to={"/imports/" + id + "/playlists/" + p.id}
                    >
                      <div>
                        <strong>{p.name}</strong>
                        <Caption>
                          {p.entries.length} ordered entries
                          {p.remoteId ? " / Published" : " / Not published yet"}
                        </Caption>
                      </div>
                      <StatusBadge state={p.state} />
                    </AppLink>
                  </Row>
                ))}
            </section>
          )}
          {currentPlaylist && (
            <>
              <Text>
                {currentPlaylist.entries.length} ordered entries. Repeated songs
                retain their positions. Unavailable entries are kept here until
                they can be filled.
              </Text>
              {currentPlaylist.error && (
                <Notice>{currentPlaylist.error}</Notice>
              )}
              {currentPlaylist.state === "paused" && (
                <section className="my-6 rounded-lg border border-solid border-[#526079] p-5">
                  <SectionHeading>Review external changes</SectionHeading>
                  <Text>
                    Resuming keeps the remote playlist’s current name and
                    settings, then permits replacing its song order with the
                    available migration entries listed below.
                  </Text>
                  <FieldGrid>
                    <Snapshot
                      title="Last verified playlist"
                      value={currentPlaylist.baseline}
                    />
                    <Snapshot
                      title="Current external playlist"
                      value={currentPlaylist.desired}
                    />
                  </FieldGrid>
                  <Actions>
                    <Button
                      disabled={
                        busy ||
                        !currentPlaylist.remoteId ||
                        !currentPlaylist.desired
                      }
                      onClick={() => {
                        if (
                          window.confirm(
                            "Allow ZenSync to replace the external song order with this migration’s available entries?",
                          )
                        )
                          void action(
                            "playlists/" + currentPlaylist.id + "/resume",
                            { baseline: currentPlaylist.desired },
                          );
                      }}
                    >
                      Resume after review
                    </Button>
                    <Button
                      variant="outline"
                      disabled={busy}
                      onClick={() =>
                        void action(
                          "playlists/" + currentPlaylist.id + "/handover",
                        )
                      }
                    >
                      Hand over this playlist
                    </Button>
                  </Actions>
                </section>
              )}
            </>
          )}
          <SectionHeading>
            {currentPlaylist ? "Ordered songs" : "Complete catalog"}
          </SectionHeading>
          <MigrationStatusFilter
            counts={statusCounts}
            hidden={hiddenStatuses}
            onChange={(values) => {
              void setHiddenStatuses(values);
              setSelected(new Set());
              setLimit(100);
            }}
          />
          <Text className="my-2 text-xs" role="status">
            Showing {shown.length} of {shownSongs.length} songs.
            {hiddenStatuses.includes("Available") && (statusCounts.get("Available") ?? 0) > 0
              ? " Available songs are hidden."
              : ""}
          </Text>
          {shown.length === 0 && (
            <Text>No songs match these filters. Change the statuses or clear your search.</Text>
          )}
          <SearchBar>
            <SearchField>
              Filter songs
              <Input
                value={filter}
                onChange={(e) => {
                  setFilter(e.target.value);
                  setLimit(100);
                }}
                placeholder="Title, artist or status"
              />
            </SearchField>
            <Button
              disabled={busy || selectedVisible.length === 0}
              onClick={() => void acquireSelected()}
            >
              Request selected ({selectedVisible.length})
            </Button>
          </SearchBar>
          <Text className="text-[13px]">
            Matching runs automatically after commit. Available songs already
            exist in Navidrome. Ready to request songs have a matched recording;
            select them and use Request selected. For Review match, compare
            recordings before choosing one. For Unresolved, use Find recording
            to search or paste a Deezer song URL. Publish available songs to
            create or update your selected playlists. Downloads require an
            explicit request and any required approval.
          </Text>
          {m.state === "open" && (
            <Text className="text-[13px]">
              Skip for now pauses new acquisition for a song and keeps its
              playlist entry. Resume it when you are ready. Skipping does not
              cancel a running download.
            </Text>
          )}
          <div className="flex min-h-[52px] items-center justify-between gap-5 border-b border-solid border-sidebar-border py-2.5 text-xs text-muted-foreground [&_label]:m-0">
            <CheckboxField>
              <Checkbox
                checked={
                  shown.some(
                    (s) =>
                      s.candidate && !s.serverId && !s.deferred && !s.requestId,
                  ) &&
                  shown
                    .filter(
                      (s) =>
                        s.candidate &&
                        !s.serverId &&
                        !s.deferred &&
                        !s.requestId,
                    )
                    .every((s) => selected.has(s.id))
                }
                onCheckedChange={(checked) =>
                  setSelected(
                    checked === true
                      ? new Set(
                          shown
                            .filter(
                              (s) =>
                                s.candidate &&
                                !s.serverId &&
                                !s.deferred &&
                                !s.requestId,
                            )
                            .map((s) => s.id),
                        )
                      : new Set(),
                  )
                }
              />{" "}
              Select ready songs
            </CheckboxField>
            <span>Status</span>
          </div>
          {shownEntries.slice(0, limit).map(({ song: s, position }, index) => (
            <div
              data-slot="import-song"
              className="flex min-h-[52px] items-center gap-3 border-b border-solid border-sidebar-border py-1.5 [&>input]:min-h-0 max-[700px]:flex-wrap"
              key={s.id + "-" + index}
            >
              <Checkbox
                aria-label={"Select " + s.metadata.title}
                disabled={
                  !s.candidate || !!s.serverId || s.deferred || !!s.requestId
                }
                checked={selected.has(s.id)}
                onCheckedChange={(checked) => {
                  const next = new Set(selected);
                  checked === true ? next.add(s.id) : next.delete(s.id);
                  setSelected(next);
                }}
              />
              {currentPlaylist && (
                <span data-slot="playlist-position" className="text-[12px] text-muted-foreground w-[22px]">
                  {position}
                </span>
              )}
              <Button
                className="block flex-1 min-w-0 text-left bg-transparent border-0 p-0 font-normal text-foreground [&_strong]:text-sm max-[700px]:basis-[65%]"
                aria-haspopup="dialog"
                onClick={() => void setRecordingId(s.id)}
              >
                <strong>
                  {s.metadata.title}
                  {s.metadata.favorite ? " ♥" : ""}
                </strong>
                <Caption>
                  {s.metadata.artist || "Unknown artist"} /{" "}
                  {s.metadata.album || "Unknown album"}
                  {s.favoriteState === "review"
                    ? " / Favorite outcome needs review"
                    : ""}
                </Caption>
              </Button>
              <StatusBadge state={status(s, scanning)} />
              <Button
                variant="outline"
                aria-haspopup="dialog"
                onClick={() => void setRecordingId(s.id)}
              >
                {recordingAction(s)}
              </Button>
              {m.state === "open" && !s.serverId && (
                <Button
                  title="Pause new acquisition for this song. Keep its playlist entry; running downloads continue."
                  variant="outline"
                  disabled={busy}
                  onClick={() =>
                    void action(
                      "songs/" + s.id + "/" + (s.deferred ? "resume" : "defer"),
                    )
                  }
                >
                  {s.deferred ? "Resume" : "Skip for now"}
                </Button>
              )}
            </div>
          ))}
          {shown.length > limit && (
            <Button variant="outline" onClick={() => setLimit(limit + 100)}>
              Show more ({shown.length - limit} remaining)
            </Button>
          )}
        </>
      )}

      {currentSource && (
        <Review
          key={currentSource.id}
          source={currentSource}
          migration={m}
          reload={load}
          onClose={() => {
            if (sourceId) navigate("/imports/" + id, { replace: true });
            else void setRecordingId(null, { history: "replace" });
          }}
        />
      )}
    </>
  );
}
