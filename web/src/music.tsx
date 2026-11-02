import React, { useEffect, useState, useRef } from "react";
import { NavLink, useSearchParams } from "react-router-dom";
import { api, Track, LibrarySong, label, time, requestKey } from "./api";
export function Search() {
  const keys = useRef<Record<number, string>>({});
  const searchSequence = useRef(0);
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get("q") || ""),
    [results, setResults] = useState<Track[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [searched, setSearched] = useState(false),
    [requested, setRequested] = useState<Set<number>>(new Set());
  async function search(q: string) {
    if (!q.trim()) return;
    const sequence = ++searchSequence.current;
    setBusy(true);
    setError("");
    try {
      const tracks = await api<Track[]>("/search?q=" + encodeURIComponent(q));
      if (sequence === searchSequence.current) {
        setResults(tracks);
        setSearched(true);
      }
    } catch (e) {
      if (sequence === searchSequence.current) setError((e as Error).message);
    } finally {
      if (sequence === searchSequence.current) setBusy(false);
    }
  }
  useEffect(() => {
    const q = params.get("q") || "";
    setQuery(q);
    if (q) void search(q);
  }, [params.get("q")]);
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
  return (
    <>
      <h1 tabIndex={-1}>Find your next record</h1>
      <p>
        Search Deezer, or paste a song, album or share link when you know
        exactly what you want.
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
          Song, artist or Deezer song URL
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Artist, song, or https://link.deezer.com/s/…"
            maxLength={500}
            required
          />
        </label>
        <button disabled={busy}>Search</button>
      </form>
      <p className="muted">
        Album links show all tracks so you can request individual songs.
      </p>
      {error && <p role="alert">{error}</p>}
      {searched && results.length === 0 && (
        <p>
          No matches. Try a different title or paste a direct Deezer song URL.
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
            disabled={busy || requested.has(t.id)}
            onClick={() => void request(t)}
          >
            {requested.has(t.id) ? "Requested" : "Request"}
          </button>
        </article>
      ))}
      {requested.size > 0 && (
        <p role="status">
          Requests saved. <NavLink to="/requests">View approval status</NavLink>{" "}
          or <NavLink to="/activity">follow acquisition progress</NavLink>.
        </p>
      )}
    </>
  );
}
export function Library() {
  const [rows, setRows] = useState<LibrarySong[]>([]),
    [query, setQuery] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [offset, setOffset] = useState(0);
  async function load(start = 0) {
    setBusy(true);
    setError("");
    try {
      const songs = await api<LibrarySong[]>(
        "/library?q=" + encodeURIComponent(query) + "&offset=" + start,
      );
      setRows(start ? [...rows, ...songs] : songs);
      setOffset(start);
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
      <p>Available music from your shared Navidrome library.</p>
      <form
        className="searchbar"
        onSubmit={(e) => {
          e.preventDefault();
          void load();
        }}
      >
        <label className="grow">
          Search your library
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
          <span>{time(s.duration)}</span>
        </div>
      ))}
      {!busy && !error && rows.length === 0 && (
        <p>
          No music found. <NavLink to="/imports">Start a migration</NavLink>.
        </p>
      )}
      {rows.length - offset === 100 && (
        <button disabled={busy} onClick={() => void load(offset + 100)}>
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
