import React, { useEffect, useState, useRef } from "react";
import { NavLink, useSearchParams, useNavigate } from "react-router-dom";
import { api, Track, LibrarySong, label, time, requestKey } from "./api";
import { StatusBadge } from "./status";
type FoundAlbum = {
  id: number;
  title: string;
  artist: { name: string };
  trackCount?: number;
};
export function Search() {
  const [album, setAlbum] = useState<FoundAlbum>();
  const [albums, setAlbums] = useState<FoundAlbum[]>([]);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [albumRequested, setAlbumRequested] = useState<Set<number>>(new Set());
  const albumKeys = useRef<Record<number, string>>({});
  const keys = useRef<Record<number, string>>({});
  const searchSequence = useRef(0);
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get("q") || ""),
    [results, setResults] = useState<Track[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [searched, setSearched] = useState(false),
    [requested, setRequested] = useState<Set<number>>(new Set());
  const [library, setLibrary] = useState<LibrarySong[]>([]);
  const [libraryError, setLibraryError] = useState("");
  const [libraryLoading, setLibraryLoading] = useState(false);
  const [importLoading, setImportLoading] = useState(false);
  const [libraryOffset, setLibraryOffset] = useState(0);
  const [moreLibrary, setMoreLibrary] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  async function search(q: string) {
    if (!q.trim()) return;
    const sequence = ++searchSequence.current;
    setAlbum(undefined);
    setAlbums([]);
    setWarnings([]);
    setResults([]);
    setLibrary([]);
    setLibraryOffset(0);
    setMoreLibrary(false);
    setLoadingMore(false);
    setLibraryError("");
    setLibraryLoading(true);
    setImportLoading(true);
    setSearched(true);
    setError("");
    await Promise.allSettled([
      api<LibrarySong[]>("/library?q=" + encodeURIComponent(q) + "&offset=0")
        .then((songs) => {
          if (sequence !== searchSequence.current) return;
          setLibrary(songs);
          setMoreLibrary(songs.length === 100);
        })
        .catch((e) => {
          if (sequence === searchSequence.current) setLibraryError(e.message);
        })
        .finally(() => {
          if (sequence === searchSequence.current) setLibraryLoading(false);
        }),
      api<
        | Track[]
        | {
            tracks: Track[];
            album?: FoundAlbum;
            albums?: FoundAlbum[];
            warnings?: string[];
          }
      >("/search?q=" + encodeURIComponent(q) + "&details=1")
        .then((response) => {
          if (sequence !== searchSequence.current) return;
          const found = Array.isArray(response) ? undefined : response.album;
          setAlbum(found?.trackCount === 1 ? undefined : found);
          setResults(Array.isArray(response) ? response : response.tracks);
          setAlbums(
            Array.isArray(response)
              ? []
              : (response.albums || []).filter((a) => a.trackCount !== 1),
          );
          setWarnings(Array.isArray(response) ? [] : response.warnings || []);
        })
        .catch((e) => {
          if (sequence === searchSequence.current) setError(e.message);
        })
        .finally(() => {
          if (sequence === searchSequence.current) setImportLoading(false);
        }),
    ]);
  }
  useEffect(() => {
    const q = params.get("q") || "";
    setQuery(q);
    if (q.trim()) void search(q.trim());
    else {
      setSearched(false);
      setLibraryLoading(false);
      setImportLoading(false);
    }
    return () => {
      searchSequence.current++;
    };
  }, [params.get("q")]);
  async function loadMoreLibrary() {
    const sequence = searchSequence.current;
    const offset = libraryOffset + 100;
    setLoadingMore(true);
    setLibraryError("");
    try {
      const songs = await api<LibrarySong[]>(
        "/library?q=" +
          encodeURIComponent(params.get("q") || "") +
          "&offset=" +
          offset,
      );
      if (sequence !== searchSequence.current) return;
      setLibrary((current) => [...current, ...songs]);
      setLibraryOffset(offset);
      setMoreLibrary(songs.length === 100);
    } catch (e) {
      if (sequence === searchSequence.current)
        setLibraryError((e as Error).message);
    } finally {
      if (sequence === searchSequence.current) setLoadingMore(false);
    }
  }
  async function request(t: Track) {
    setBusy(true);
    setError("");
    try {
      await api(
        "/requests",
        "POST",
        { target: `deezer:track:${t.id}` },
        (keys.current[t.id] ||= requestKey()),
      );
      setRequested(new Set([...requested, t.id]));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function requestAlbum(chosen: FoundAlbum) {
    setBusy(true);
    setError("");
    try {
      await api(
        "/requests",
        "POST",
        { target: "deezer:album:" + chosen.id },
        (albumKeys.current[chosen.id] ||= requestKey()),
      );
      setAlbumRequested(new Set([...albumRequested, chosen.id]));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <h1 tabIndex={-1}>Search</h1>
      <p>
        Find music in your library and on Deezer, or paste a song or album link.
      </p>
      <form
        className="searchbar"
        onSubmit={(e) => {
          e.preventDefault();
          if (params.get("q") === query) void search(query);
          else setParams({ q: query });
        }}
      >
        <label className="grow">
          Song, artist, album or Deezer URL
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Song, artist, album name or Deezer link"
            maxLength={500}
            required
          />
        </label>
        <button disabled={busy}>Search</button>
      </form>
      {searched && (
        <>
          <SearchSection title="Existing library" count={library.length}>
            {libraryLoading && <p role="status">Searching your library…</p>}
            {libraryError && (
              <p role="alert">
                {libraryError}{" "}
                <NavLink to="/settings/connections">
                  Connection settings
                </NavLink>
              </p>
            )}
            {!libraryLoading && !libraryError && library.length === 0 && (
              <p>
                No matching music in your library. See candidates for import
                below.
              </p>
            )}
            {library.map((song) => (
              <article className="row" key={song.id}>
                <div>
                  <strong>{song.title}</strong>
                  <small>
                    {song.artist} / {song.album} / {time(song.duration)}
                  </small>
                </div>
                <StatusBadge state="available" />
              </article>
            ))}
            {moreLibrary && (
              <button
                className="quiet toolbar"
                disabled={loadingMore}
                onClick={() => void loadMoreLibrary()}
              >
                {loadingMore ? "Loading…" : "Load more library results"}
              </button>
            )}
          </SearchSection>
          <SearchSection
            title="Candidates for import"
            count={results.length + albums.length}
          >
            {importLoading && <p role="status">Searching Deezer…</p>}
            {album && (
              <section className="album-result">
                <div>
                  <h2>{album.title}</h2>
                  <p>
                    {album.artist.name} / {album.trackCount}{" "}
                    {album.trackCount === 1 ? "track" : "tracks"}
                  </p>
                  <p className="muted">
                    Request the whole album with one approval, or choose
                    individual songs below.
                  </p>
                </div>
                <button
                  disabled={busy || albumRequested.has(album.id)}
                  onClick={() => void requestAlbum(album)}
                >
                  {albumRequested.has(album.id)
                    ? "Album requested"
                    : "Request album"}
                </button>
              </section>
            )}
            {album && albumRequested.has(album.id) && (
              <p role="status">
                Album request saved.{" "}
                <NavLink to="/requests">View the single album request</NavLink>.
              </p>
            )}
            {error && <p role="alert">{error}</p>}
            {warnings.map((w) => (
              <p className="notice" role="status" key={w}>
                {w}
              </p>
            ))}
            {albums.length > 0 && (
              <section aria-label="Album results">
                <h2>Albums</h2>
                {albums.map((found) => (
                  <article className="row" key={found.id}>
                    <div>
                      <strong>{found.title}</strong>
                      <small>
                        {found.artist.name}
                        {found.trackCount
                          ? " / " +
                            found.trackCount +
                            (found.trackCount === 1 ? " track" : " tracks")
                          : ""}
                      </small>
                    </div>
                    <div className="actions">
                      <NavLink
                        to={
                          "/search?q=" +
                          encodeURIComponent(
                            "https://www.deezer.com/album/" + found.id,
                          )
                        }
                      >
                        View tracks
                      </NavLink>
                      <button
                        disabled={busy || albumRequested.has(found.id)}
                        onClick={() => void requestAlbum(found)}
                      >
                        {albumRequested.has(found.id)
                          ? "Album requested"
                          : "Request album"}
                      </button>
                    </div>
                  </article>
                ))}
              </section>
            )}
            {albums.some((a) => albumRequested.has(a.id)) && (
              <p role="status">
                Album request saved.{" "}
                <NavLink to="/requests">View requests</NavLink>.
              </p>
            )}
            {results.length > 0 && albums.length > 0 && <h2>Songs</h2>}
            {searched &&
              results.length === 0 &&
              albums.length === 0 &&
              !album &&
              !importLoading &&
              !error && (
                <p>
                  No matches. Try another song, artist or album name, or paste a
                  Deezer link.
                </p>
              )}
            {results.map((t) => (
              <article className="row" key={t.id}>
                <div>
                  <strong>{label(t)}</strong>
                  <small>
                    {t.artist.name} / {t.album.title} / {time(t.duration)}
                  </small>
                </div>
                <button
                  disabled={
                    busy ||
                    requested.has(t.id) ||
                    !!(album && albumRequested.has(album.id))
                  }
                  onClick={() => void request(t)}
                >
                  {album && albumRequested.has(album.id)
                    ? "Included in album"
                    : requested.has(t.id)
                      ? "Requested"
                      : "Request"}
                </button>
              </article>
            ))}
          </SearchSection>
        </>
      )}
      {requested.size > 0 && (
        <p role="status">
          Requests saved. <NavLink to="/requests">View approval status</NavLink>{" "}
          or <NavLink to="/activity">follow acquisition progress</NavLink>.
        </p>
      )}
    </>
  );
}
function SearchSection({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(true);
  return (
    <section aria-label={title}>
      <details
        className="status-section"
        open={open}
        onToggle={(e) => setOpen(e.currentTarget.open)}
      >
        <summary>
          {title}
          <span className="count">{count}</span>
        </summary>
        {children}
      </details>
    </section>
  );
}

export function Library({ canDelete }: { canDelete: boolean }) {
  const [notice, setNotice] = useState("");
  const [deletions, setDeletions] = useState<Set<string>>(new Set());
  const deletionKeys = useRef<Record<string, string>>({});
  async function remove(song: LibrarySong) {
    if (
      !window.confirm(
        canDelete
          ? `Remove “${song.title}” from the shared library for everyone? A recovery copy will be retained.`
          : `Request removal of “${song.title}” from the shared library for everyone? An administrator must approve.`,
      )
    )
      return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api(
        "/deletion-requests",
        "POST",
        { songId: song.id },
        (deletionKeys.current[song.id] ||= requestKey()),
      );
      setDeletions((previous) => new Set([...previous, song.id]));
      setNotice(
        canDelete
          ? "Deletion approved and queued. Follow progress in Requests or Activity."
          : "Deletion request submitted for administrator approval.",
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const navigate = useNavigate();
  const [rows, setRows] = useState<LibrarySong[]>([]),
    [query, setQuery] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [nextOffset, setNextOffset] = useState(-1);
  async function load(start = 0) {
    setBusy(true);
    setError("");
    try {
      const response = await api<
        LibrarySong[] | { songs: LibrarySong[]; nextOffset: number }
      >("/library?q=&offset=" + start + "&details=1");
      const songs = Array.isArray(response) ? response : response.songs;
      setRows(start ? [...rows, ...songs] : songs);
      setNextOffset(
        Array.isArray(response)
          ? songs.length === 100
            ? start + 100
            : -1
          : response.nextOffset,
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void load();
  }, []);
  return (
    <>
      <h1 tabIndex={-1}>Library</h1>
      <p>
        Available music from your shared Navidrome library. Deleting a song
        removes it for everyone.
      </p>
      {notice && (
        <p role="status">
          {notice} <NavLink to="/requests">View requests</NavLink>
        </p>
      )}
      <form
        className="searchbar"
        onSubmit={(e) => {
          e.preventDefault();
          if (query.trim())
            navigate("/search?q=" + encodeURIComponent(query.trim()));
        }}
      >
        <label className="grow">
          Search music
          <input value={query} onChange={(e) => setQuery(e.target.value)} />
        </label>
        <button disabled={busy}>Search</button>
      </form>
      {error && (
        <p role="alert">
          {error}{" "}
          <NavLink to="/settings/connections">Connection settings</NavLink>
        </p>
      )}
      {rows.map((s) => (
        <div className="row" key={s.id}>
          <div>
            <strong>{s.title}</strong>
            <small>
              {s.artist} / {s.album}
            </small>
          </div>
          <div className="actions">
            <span>{time(s.duration)}</span>
            <button
              className="quiet"
              aria-label={
                (canDelete ? "Delete song: " : "Request deletion: ") + s.title
              }
              disabled={busy || deletions.has(s.id)}
              onClick={() => void remove(s)}
            >
              {deletions.has(s.id)
                ? "Deletion requested"
                : canDelete
                  ? "Delete song"
                  : "Request deletion"}
            </button>
          </div>
        </div>
      ))}
      {!busy && !error && rows.length === 0 && (
        <p>
          No music found.{" "}
          <NavLink to="/search">Search for music to import</NavLink>.
        </p>
      )}
      {nextOffset >= 0 && (
        <button disabled={busy} onClick={() => void load(nextOffset)}>
          Load more
        </button>
      )}
    </>
  );
}
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
  const [data, setData] = useState<ConnectionsData>(),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false);
  async function load() {
    setData(await api<ConnectionsData>("/connections"));
  }
  useEffect(() => {
    load().catch((e) => setError(e.message));
  }, []);
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
      <h2>Connections</h2>
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {data && (
        <>
          <div className="connection-status">
            <span className="pill">
              Navidrome: {data.configured ? "connected" : "not configured"}
            </span>
            <span className="pill">
              d-fi {data.downloaderVersion}:{" "}
              {data.downloaderReady ? "installed" : "not found"}
            </span>
            <span className="pill">
              Music mount: {data.musicMounted ? "present" : "missing"}
            </span>
          </div>
          {admin && (
            <section className="connection">
              <h3>Shared Navidrome server</h3>
              <p>
                Use a service account with library access and scan permission.
                In Docker, localhost is this container; use the server’s
                reachable hostname.
              </p>
              <form onSubmit={(e) => void save(e, "shared")}>
                <label>
                  Navidrome URL
                  <input
                    type="url"
                    name="url"
                    defaultValue={data.url}
                    placeholder="http://navidrome:4533"
                    required
                  />
                </label>
                <div className="fields">
                  <label>
                    Service username
                    <input name="username" autoComplete="off" required />
                  </label>
                  <label>
                    Service password
                    <input
                      name="password"
                      type="password"
                      autoComplete="new-password"
                      required
                    />
                  </label>
                </div>
                <button disabled={busy}>Test and save server</button>
              </form>
            </section>
          )}
          <section className="connection">
            <h3>Your personal Navidrome account</h3>
            <p>
              Playlists and favorites will be published to{" "}
              {data.personalUsername ? (
                <strong>{data.personalUsername}</strong>
              ) : (
                "this account"
              )}
              . This link is separate from ZenSync sign-in.
            </p>
            <form onSubmit={(e) => void save(e, "personal")}>
              <div className="fields">
                <label>
                  Personal username
                  <input
                    name="username"
                    defaultValue={data.personalUsername}
                    autoComplete="off"
                    required
                  />
                </label>
                <label>
                  Personal password
                  <input
                    name="password"
                    type="password"
                    autoComplete="new-password"
                    required
                  />
                </label>
              </div>
              <button disabled={busy || !data.configured}>
                Test and link account
              </button>
            </form>
          </section>
          {admin && (
            <section className="connection">
              <h3>Deezer acquisition</h3>
              <p>
                {data.deezerConfigured
                  ? "An ARL cookie is saved. Enter a new one to replace it."
                  : "Enter your Deezer ARL cookie to enable missing-song acquisition."}{" "}
                Credentials are encrypted on disk and are never returned to the
                browser.
              </p>
              <form onSubmit={(e) => void save(e, "deezer")}>
                <label>
                  Deezer ARL cookie
                  <input
                    name="arl"
                    type="password"
                    autoComplete="new-password"
                    required
                    minLength={32}
                  />
                </label>
                <button disabled={busy}>Save Deezer credentials</button>
              </form>
            </section>
          )}
          <p>
            <NavLink to="/imports/new" className="button">
              Import Apple Music XML
            </NavLink>
          </p>
        </>
      )}
    </>
  );
}
