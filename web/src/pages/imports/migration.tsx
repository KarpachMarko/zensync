import { Review } from "@/components/imports/review";
import { Snapshot } from "@/components/imports/snapshot";
import { StatusBadge } from "@/components/status";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import { status, type Detail, type Source } from "@/lib/migration";
import { useState } from "react";
import { NavLink, useNavigate, useParams } from "react-router-dom";
export function MigrationPage() {
  const { api } = useApiMutation();
  const { id, sourceId, playlistId } = useParams();
  const [actionError, setError] = useState(""),
    [notice, setNotice] = useState(""),
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
    setNotice("");
    try {
      await api("/imports/" + id + "/" + path, "POST", body);
      await load();
      if (path === "match" || path === "publish")
        setNotice(
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
      for (const sid of selected) {
        await api("/imports/" + id + "/songs/" + sid + "/acquire", "POST");
        saved++;
      }
      setSelected(new Set());
      await load();
      setNotice(`${saved} acquisition requests saved.`);
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
        <h1 tabIndex={-1}>Migration</h1>
        <p>{error || "Loading your migration…"}</p>
      </>
    );
  const { migration: m, songs, playlists } = detail;
  const currentSource = songs.find((s) => s.id === sourceId),
    currentPlaylist = playlists.find((p) => p.id === playlistId);
  const shownSongs = currentPlaylist
    ? currentPlaylist.entries
        .map((sid) => songs.find((s) => s.id === sid))
        .filter((s): s is Source => !!s)
    : songs;
  const shown = shownSongs.filter((s) =>
    (s.metadata.title + " " + s.metadata.artist + " " + status(s))
      .toLowerCase()
      .includes(filter.toLowerCase()),
  );
  const available = songs.filter((s) => s.serverId).length;
  if (currentSource)
    return (
      <Review
        key={currentSource.id}
        source={currentSource}
        migration={m}
        reload={load}
      />
    );
  return (
    <>
      <div className="breadcrumbs">
        <NavLink to="/imports">Imports</NavLink>
        {playlistId && <NavLink to={"/imports/" + id}>{m.name}</NavLink>}
      </div>
      <div className="titlebar">
        <h1 tabIndex={-1}>{currentPlaylist?.name || m.name}</h1>
        <StatusBadge state={m.state} />
      </div>
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {m.error && (
        <div className="notice">
          {m.error} <NavLink to="/activity">Open Activity</NavLink>
        </div>
      )}
      {m.state === "staged" ? (
        <>
          <p>
            Your snapshot is saved. Select the playlists you want to migrate;
            all {m.report.songs} catalog entries will be retained.
          </p>
          <div className="summaryline">
            <span>{m.report.unplaylisted} unplaylisted songs</span>
            <span>{m.report.missing} missing references</span>
            <span>{m.report.excluded} non-music records excluded</span>
          </div>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              void action("commit", {
                playlists: f.getAll("playlist"),
                favorites: f.get("favorites") === "on",
              });
            }}
          >
            {playlists.map((p) => (
              <label className="check" key={p.id}>
                <input
                  type="checkbox"
                  name="playlist"
                  value={p.id}
                  defaultChecked={p.selected}
                  disabled={p.metadata.system || p.metadata.folder}
                />
                <span>
                  {p.name}{" "}
                  <small>
                    {p.entries.length} entries
                    {p.metadata.folder
                      ? " / Folder"
                      : p.metadata.system
                        ? " / System playlist"
                        : p.metadata.smart
                          ? " / Smart playlist snapshot"
                          : ""}
                  </small>
                </span>
              </label>
            ))}
            <label className="check">
              <input name="favorites" type="checkbox" defaultChecked /> Import
              positive favorite/loved flags when songs become available
            </label>
            <Button disabled={busy}>Commit migration</Button>
          </form>
          {m.report.warnings.length > 0 && (
            <details>
              <summary>Import warnings ({m.report.warnings.length})</summary>
              {m.report.warnings.slice(0, 100).map((w, i) => (
                <p key={i}>{w}</p>
              ))}
            </details>
          )}
        </>
      ) : (
        <>
          <div className="coverage">
            <strong>
              {available} <span>of {songs.length} songs available</span>
            </strong>
            <progress max={Math.max(1, songs.length)} value={available} />
          </div>
          {m.state === "open" ? (
            <div className="actions toolbar">
              <Button disabled={busy} onClick={() => void action("match")}>
                Find matches
              </Button>
              <Button disabled={busy} onClick={() => void action("publish")}>
                {m.targetUser
                  ? "Publish available songs"
                  : "Publish to Navidrome"}
              </Button>
              <NavLink to="/settings/connections">Connections</NavLink>
              <Button
                className="quiet"
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
            </div>
          ) : (
            <div className="notice">
              Navidrome now owns these collections. Pending acquisitions
              continue, but will not change your playlists or favorites.
            </div>
          )}
          {!currentPlaylist && (
            <section className="playlist-list">
              <h2>Migration playlists</h2>
              {playlists
                .filter((p) => p.selected)
                .map((p) => (
                  <NavLink
                    className="row migration-link"
                    to={"/imports/" + id + "/playlists/" + p.id}
                    key={p.id}
                  >
                    <div>
                      <strong>{p.name}</strong>
                      <small>
                        {p.entries.length} ordered entries
                        {p.remoteId ? " / Published" : " / Not published yet"}
                      </small>
                    </div>
                    <StatusBadge state={p.state} />
                  </NavLink>
                ))}
            </section>
          )}
          {currentPlaylist && (
            <>
              <p>
                {currentPlaylist.entries.length} ordered entries. Repeated songs
                retain their positions. Unavailable entries are kept here until
                they can be filled.
              </p>
              {currentPlaylist.error && (
                <div className="notice">{currentPlaylist.error}</div>
              )}
              {currentPlaylist.state === "paused" && (
                <section className="review-diff">
                  <h2>Review external changes</h2>
                  <p>
                    Resuming keeps the remote playlist’s current name and
                    settings, then permits replacing its song order with the
                    available migration entries listed below.
                  </p>
                  <div className="fields">
                    <Snapshot
                      title="Last verified playlist"
                      value={currentPlaylist.baseline}
                    />
                    <Snapshot
                      title="Current external playlist"
                      value={currentPlaylist.desired}
                    />
                  </div>
                  <div className="actions">
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
                      className="quiet"
                      disabled={busy}
                      onClick={() =>
                        void action(
                          "playlists/" + currentPlaylist.id + "/handover",
                        )
                      }
                    >
                      Hand over this playlist
                    </Button>
                  </div>
                </section>
              )}
            </>
          )}
          <h2>{currentPlaylist ? "Ordered songs" : "Complete catalog"}</h2>
          <div className="searchbar">
            <label className="grow">
              Filter songs
              <Input
                value={filter}
                onChange={(e) => {
                  setFilter(e.target.value);
                  setLimit(100);
                }}
                placeholder="Title, artist or status"
              />
            </label>
            <Button
              disabled={busy || selected.size === 0}
              onClick={() => void acquireSelected()}
            >
              Request selected ({selected.size})
            </Button>
          </div>
          <p className="muted">
            Find matches identifies exact album/title/artist/duration matches.
            Review other recordings before requesting. Missing audio downloads
            only after an explicit request and any required approval.
          </p>
          <div className="tablehead">
            <label className="check">
              <input
                type="checkbox"
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
                onChange={(e) =>
                  setSelected(
                    e.target.checked
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
            </label>
            <span>Status</span>
          </div>
          {shown.slice(0, limit).map((s, index) => (
            <div className="songrow" key={s.id + "-" + index}>
              <input
                type="checkbox"
                aria-label={"Select " + s.metadata.title}
                disabled={
                  !s.candidate || !!s.serverId || s.deferred || !!s.requestId
                }
                checked={selected.has(s.id)}
                onChange={(e) => {
                  const next = new Set(selected);
                  e.target.checked ? next.add(s.id) : next.delete(s.id);
                  setSelected(next);
                }}
              />
              {currentPlaylist && <span className="position">{index + 1}</span>}
              <Button
                className="songlink"
                onClick={() => navigate("/imports/" + id + "/songs/" + s.id)}
              >
                <strong>
                  {s.metadata.title}
                  {s.metadata.favorite ? " ♥" : ""}
                </strong>
                <small>
                  {s.metadata.artist || "Unknown artist"} /{" "}
                  {s.metadata.album || "Unknown album"}
                  {s.favoriteState === "review"
                    ? " / Favorite outcome needs review"
                    : ""}
                </small>
              </Button>
              <StatusBadge state={status(s)} />
              <Button
                className="quiet"
                disabled={busy}
                onClick={() =>
                  void action(
                    "songs/" + s.id + "/" + (s.deferred ? "resume" : "defer"),
                  )
                }
              >
                {s.deferred ? "Resume" : "Skip for now"}
              </Button>
            </div>
          ))}
          {shown.length > limit && (
            <Button className="quiet" onClick={() => setLimit(limit + 100)}>
              Show more ({shown.length - limit} remaining)
            </Button>
          )}
        </>
      )}
    </>
  );
}
