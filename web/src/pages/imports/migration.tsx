import { ExpandableSection } from "@/components/expandable-section";
import { Checkbox, CheckboxField } from "@/components/forms";
import { MigrationFilters } from "@/components/imports/migration-filters";
import { MigrationHeader } from "@/components/imports/migration-header";
import { MigrationChoices } from "@/components/imports/migration-choices";
import { Review } from "@/components/imports/review";
import { Snapshot } from "@/components/imports/snapshot";
import { Actions, FieldGrid } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { StatusBadge } from "@/components/status";
import {
  Caption,
  PageHeading,
  SectionHeading,
  Text,
} from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiQuery } from "@/hooks/use-api";
import { useImportAction } from "@/hooks/use-import-action";
import { recordingAction, status, type Detail, type Source } from "@/lib/migration";
import { useState } from "react";
import { parseAsArrayOf, parseAsString, useQueryState } from "nuqs";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
export function MigrationPage() {
  const { id, sourceId, playlistId } = useParams();
  const { act: save } = useImportAction(id);
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
  async function action(path: string, body?: unknown) {
    setBusy(true);
    setError("");

    try {
      await save(path, body);
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
        await save("songs/" + sid + "/acquire");
        saved++;
      }
      setSelected(new Set());
      toast.success(`${saved} acquisition requests saved.`);
    } catch (e) {
      setError(`${saved} requests saved. ${(e as Error).message}`);
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
  return (
    <>
      <div className="flex gap-[15px] mb-[22px] text-[13px] text-muted-foreground">
        <AppLink to="/imports">Imports</AppLink>
        {playlistId && <AppLink to={"/imports/" + id}>{m.name}</AppLink>}
      </div>
      <MigrationHeader
        detail={detail}
        currentPlaylist={currentPlaylist}
        errors={[actionError, result.error?.message]}
        busy={busy}
        onAction={action}
        onDeleted={() => navigate("/imports", { replace: true })}
      />
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
          {currentPlaylist && (
            <>
              <Text>
                {currentPlaylist.entries.length} ordered entries. Repeated songs
                retain their positions. Unavailable entries are kept here until
                they can be filled.
              </Text>
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
          <MigrationFilters
            counts={statusCounts}
            hidden={hiddenStatuses}
            onStatusesChange={(values) => {
              void setHiddenStatuses(values);
              setSelected(new Set());
              setLimit(100);
            }}
            query={filter}
            onQueryChange={(value) => {
              setFilter(value);
              setLimit(100);
            }}
            shown={shown.length}
            total={shownSongs.length}
            selected={selectedVisible.length}
            busy={busy}
            onRequest={() => void acquireSelected()}
          />
          {shown.length === 0 && (
            <Text>No songs match these filters. Change the statuses or clear your search.</Text>
          )}
          <div className="@container/catalog min-w-0">
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
            <span className="hidden @min-[760px]/catalog:block">Status</span>
          </div>
          {shownEntries.slice(0, limit).map(({ song: s, position }, index) => (
            <div
              data-slot="import-song"
              className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-start gap-x-3 gap-y-2 border-b border-solid border-sidebar-border py-3 @min-[760px]/catalog:grid-cols-[auto_minmax(0,1fr)_auto_auto] @min-[760px]/catalog:items-center"
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
              <Button
                className="block min-w-0 whitespace-normal break-words text-left bg-transparent border-0 p-0 font-normal text-foreground [&_strong]:text-sm"
                aria-haspopup="dialog"
                onClick={() => void setRecordingId(s.id)}
              >
                <strong>
                  {currentPlaylist && <span data-slot="playlist-position" className="mr-2 text-xs font-normal text-muted-foreground">{position}</span>}
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
              <div className="col-start-2 @min-[760px]/catalog:col-start-auto"><StatusBadge state={status(s, scanning)} /></div>
              <div data-slot="import-song-actions" className="col-start-2 grid min-w-0 grid-flow-col auto-cols-fr gap-2 @min-[760px]/catalog:col-start-auto @min-[760px]/catalog:flex">
              <Button
                variant="outline"
                className="whitespace-normal text-xs @min-[760px]/catalog:whitespace-nowrap @min-[760px]/catalog:text-sm"
                aria-haspopup="dialog"
                onClick={() => void setRecordingId(s.id)}
              >
                {recordingAction(s)}
              </Button>
              {m.state === "open" && !s.serverId && (
                <Button
                  title="Pause new acquisition for this song. Keep its playlist entry; running downloads continue."
                  variant="outline"
                  className="whitespace-normal text-xs @min-[760px]/catalog:whitespace-nowrap @min-[760px]/catalog:text-sm"
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
            </div>
          ))}
          {shown.length > limit && (
            <Button variant="outline" onClick={() => setLimit(limit + 100)}>
              Show more ({shown.length - limit} remaining)
            </Button>
          )}
          </div>
        </>
      )}

      {currentSource && (
        <Review
          key={currentSource.id}
          source={currentSource}
          migration={m}
          onClose={() => {
            if (sourceId) navigate("/imports/" + id, { replace: true });
            else void setRecordingId(null, { history: "replace" });
          }}
        />
      )}
    </>
  );
}
