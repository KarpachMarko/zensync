import React, { useEffect, useState } from "react";
import { NavLink, useNavigate, useParams } from "react-router-dom";
import { api, upload, Track, LibrarySong, label, time } from "./api";
type Migration = {
  id: string;
  name: string;
  state: string;
  favorites: boolean;
  targetUser: string;
  error: string;
  report: {
    songs: number;
    playlists: number;
    unplaylisted: number;
    missing: number;
    excluded: number;
    warnings: string[];
  };
};
type Source = {
  id: string;
  metadata: {
    title: string;
    artist: string;
    album: string;
    duration: number;
    favorite: boolean;
    missing: boolean;
  };
  serverId: string;
  candidate: Track | null;
  matchState: string;
  deferred: boolean;
  favoriteState: string;
  requestId: string;
  decision: string;
  jobState: string;
};
type Playlist = {
  id: string;
  name: string;
  selected: boolean;
  state: string;
  remoteId: string;
  baseline: string;
  desired: string;
  error: string;
  entries: string[];
  metadata: {
    folder: boolean;
    system: boolean;
    smart: boolean;
    parentId: string;
  };
};
type Detail = { migration: Migration; songs: Source[]; playlists: Playlist[] };
function status(s: Source) {
  return s.serverId
    ? "Available"
    : s.deferred
      ? "Skipped for now"
      : s.jobState
        ? s.jobState.replaceAll("_", " ")
        : s.decision
          ? s.decision.replaceAll("_", " ")
          : s.candidate
            ? "Ready to request"
            : s.matchState === "ambiguous"
              ? "Review match"
              : "Unresolved";
}
export function Imports() {
  const [rows, setRows] = useState<Migration[]>([]),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  useEffect(() => {
    api<Migration[]>("/imports")
      .then(setRows)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);
  return (
    <>
      <div className="titlebar">
        <h1 tabIndex={-1}>Imports</h1>
        <NavLink className="button" to="/imports/new">
          New migration
        </NavLink>
      </div>
      <p>
        Bring your Apple Music catalog, playlists and favorites into Navidrome.
      </p>
      {error && <p role="alert">{error}</p>}
      {loading ? (
        <p>Loading migrations…</p>
      ) : rows.length === 0 ? (
        <section className="empty">
          <div className="record" aria-hidden="true">
            ◉
          </div>
          <h2>Your collection has a new home.</h2>
          <p>
            Export your library as XML or a single playlist as XML or text. ZenSync keeps every
            song and playlist position while you resolve and acquire missing
            music.
          </p>
          <NavLink to="/settings/connections">
            Configure your connections
          </NavLink>
        </section>
      ) : (
        rows.map((m) => (
          <NavLink
            className="row migration-link"
            key={m.id}
            to={"/imports/" + m.id}
          >
            <div>
              <strong>{m.name}</strong>
              <small>
                {m.report.songs} songs / {m.report.playlists} playlists
              </small>
            </div>
            <span className="pill">{m.state.replaceAll("_", " ")}</span>
          </NavLink>
        ))
      )}
    </>
  );
}
export function NewImport() {
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const file = new FormData(e.currentTarget).get("xml") as File;
    if (file.size > 32 * 1024 * 1024) {
      setError("Choose an export smaller than 32 MiB.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const m = await upload(file);
      navigate("/imports/" + m.id);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <NavLink to="/imports">Imports</NavLink>
      <h1 tabIndex={-1}>Start a migration</h1>
      <p>
        In Apple Music, choose File → Library → Export Library. For one
        playlist, select it, choose File → Library → Export Playlist and select XML or Text. Text exports use the filename as the playlist name.
      </p>
      <form onSubmit={submit}>
        <label>
          Apple Music or iTunes export (XML or TXT)
          <input
            name="xml"
            type="file"
            accept=".xml,.txt,application/xml,text/xml,text/plain"
            required
          />
        </label>
        <p>
          Uploading saves a private snapshot. It does not download music or
          change Navidrome. You’ll choose playlists and favorites in the
          preview.
        </p>
        {error && <p role="alert">{error}</p>}
        <button disabled={busy}>
          {busy ? "Reading your export…" : "Upload and preview"}
        </button>
      </form>
    </>
  );
}
export function MigrationPage() {
  const { id, sourceId, playlistId } = useParams();
  const [detail, setDetail] = useState<Detail>(),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false),
    [selected, setSelected] = useState<Set<string>>(new Set()),
    [filter, setFilter] = useState(""),
    [limit, setLimit] = useState(100);
  const navigate = useNavigate();
  async function load() {
    const d = await api<Detail>("/imports/" + id);
    setDetail(d);
  }
  useEffect(() => {
    setDetail(undefined);
    load().catch((e) => setError(e.message));
    const timer = setInterval(() => load().catch(() => {}), 4000);
    return () => clearInterval(timer);
  }, [id]);
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
        <span className="pill">{m.state.replaceAll("_", " ")}</span>
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
            <button disabled={busy}>Commit migration</button>
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
              <button disabled={busy} onClick={() => void action("match")}>
                Find matches
              </button>
              <button disabled={busy} onClick={() => void action("publish")}>
                {m.targetUser
                  ? "Publish available songs"
                  : "Publish to Navidrome"}
              </button>
              <NavLink to="/settings/connections">Connections</NavLink>
              <button
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
              </button>
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
                    <span className="pill">{p.state.replaceAll("_", " ")}</span>
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
                    <button
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
                    </button>
                    <button
                      className="quiet"
                      disabled={busy}
                      onClick={() =>
                        void action(
                          "playlists/" + currentPlaylist.id + "/handover",
                        )
                      }
                    >
                      Hand over this playlist
                    </button>
                  </div>
                </section>
              )}
            </>
          )}
          <h2>{currentPlaylist ? "Ordered songs" : "Complete catalog"}</h2>
          <div className="searchbar">
            <label className="grow">
              Filter songs
              <input
                value={filter}
                onChange={(e) => {
                  setFilter(e.target.value);
                  setLimit(100);
                }}
                placeholder="Title, artist or status"
              />
            </label>
            <button
              disabled={busy || selected.size === 0}
              onClick={() => void acquireSelected()}
            >
              Request selected ({selected.size})
            </button>
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
              <button
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
              </button>
              <span className="pill">{status(s)}</span>
              <button
                className="quiet"
                disabled={busy}
                onClick={() =>
                  void action(
                    "songs/" + s.id + "/" + (s.deferred ? "resume" : "defer"),
                  )
                }
              >
                {s.deferred ? "Resume" : "Skip for now"}
              </button>
            </div>
          ))}
          {shown.length > limit && (
            <button className="quiet" onClick={() => setLimit(limit + 100)}>
              Show more ({shown.length - limit} remaining)
            </button>
          )}
        </>
      )}
    </>
  );
}
function Snapshot({ title, value }: { title: string; value: string }) {
  let data: { name: string; ids: string[] } | null = null;
  try {
    data = JSON.parse(value);
  } catch {}
  return (
    <div>
      <h3>{title}</h3>
      {data ? (
        <>
          <p>{data.name}</p>
          <ol className="snapshot">
            {data.ids.map((id, i) => (
              <li key={i}>{id}</li>
            ))}
          </ol>
        </>
      ) : (
        <p>No verified snapshot.</p>
      )}
    </div>
  );
}
function Review({
  source: s,
  migration: m,
  reload,
}: {
  source: Source;
  migration: Migration;
  reload: () => Promise<void>;
}) {
  const [query, setQuery] = useState(
      s.metadata.artist + " " + s.metadata.title,
    ),
    [candidates, setCandidates] = useState<{
      deezer: Track[];
      library: LibrarySong[];
    }>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState("");
  const prefix = "/imports/" + m.id + "/songs/" + s.id;
  async function find(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      setCandidates(await api(prefix + "/candidates", "POST", { query }));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function act(action: string, body?: unknown) {
    setBusy(true);
    setError("");
    try {
      await api(prefix + "/" + action, "POST", body);
      await reload();
      setNotice(
        action === "acquire" ? "Acquisition request saved." : "Song updated.",
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <NavLink to={"/imports/" + m.id}>Back to migration</NavLink>
      <h1 tabIndex={-1}>{s.metadata.title}</h1>
      <p>
        {s.metadata.artist} / {s.metadata.album} / {time(s.metadata.duration)}
      </p>
      <span className="pill">{status(s)}</span>
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {s.candidate && (
        <section className="chosen">
          <h2>Selected recording</h2>
          <strong>{label(s.candidate)}</strong>
          <p>
            {s.candidate.artist.name} / {s.candidate.album.title} /{" "}
            {time(s.candidate.duration)}
          </p>
          <button
            disabled={
              busy ||
              s.deferred ||
              !!s.serverId ||
              s.decision === "approved" ||
              s.decision === "awaiting_approval"
            }
            onClick={() => void act("acquire")}
          >
            Request missing audio
          </button>
        </section>
      )}
      {s.requestId && (
        <p>
          <NavLink to="/requests">View request and approval</NavLink> /{" "}
          <NavLink to="/activity">View acquisition progress</NavLink>
        </p>
      )}
      <button
        className="quiet"
        disabled={busy}
        onClick={() => void act(s.deferred ? "resume" : "defer")}
      >
        {s.deferred ? "Resume" : "Skip for now"}
      </button>
      {s.favoriteState === "review" && m.state === "open" && (
        <section className="notice">
          <p>
            The earlier favorite write has an uncertain outcome. Keep
            Navidrome’s current state, or explicitly try starring this song
            again.
          </p>
          <div className="actions">
            <button disabled={busy} onClick={() => void act("favorite-done")}>
              Keep current favorite state
            </button>
            <button
              className="quiet"
              disabled={busy}
              onClick={() => void act("favorite-retry")}
            >
              Retry favorite import once
            </button>
          </div>
        </section>
      )}
      <h2>Find the intended recording</h2>
      <p>
        Compare album, version and duration. Resolving a match does not request
        a new download. You can also paste a Deezer song or share URL.
      </p>
      <form className="searchbar" onSubmit={find}>
        <label className="grow">
          Search or Deezer song URL
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            maxLength={500}
            required
          />
        </label>
        <button disabled={busy}>Find candidates</button>
      </form>
      {candidates && (
        <>
          <h3>Already in Navidrome</h3>
          {candidates.library.length === 0 && <p>No local candidates.</p>}
          {candidates.library.map((t) => (
            <div className="row" key={t.id}>
              <div>
                <strong>{t.title}</strong>
                <small>
                  {t.artist} / {t.album} / {time(t.duration)}
                </small>
              </div>
              <button
                disabled={busy}
                onClick={() => void act("resolve", { serverId: t.id })}
              >
                Use existing
              </button>
            </div>
          ))}
          <h3>Deezer recordings</h3>
          {candidates.deezer.length === 0 && (
            <p>No results. Try a direct song URL.</p>
          )}
          {candidates.deezer.map((t) => (
            <div className="row" key={t.id}>
              <div>
                <strong>{label(t)}</strong>
                <small>
                  {t.artist.name} / {t.album.title} / {time(t.duration)}
                </small>
              </div>
              <button
                disabled={busy}
                onClick={() =>
                  void act("resolve", { target: "deezer:track:" + t.id })
                }
              >
                Use this recording
              </button>
            </div>
          ))}
        </>
      )}
    </>
  );
}
