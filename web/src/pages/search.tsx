import { LibrarySongRow } from "@/components/music/library-song-row";
import { SearchSection } from "@/components/music/search-section";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { LibrarySong, Track } from "@/lib/api";
import { api, label, requestKey, time } from "@/lib/api";
import { useEffect, useRef, useState } from "react";
import { NavLink, useSearchParams } from "react-router-dom";
type FoundAlbum = {
  id: number;
  title: string;
  artist: { name: string };
  trackCount?: number;
};
export function Search({ canDelete }: { canDelete: boolean }) {
  const [resultType, setResultType] = useState<"Both" | "Album" | "Song">(
    "Both",
  );
  const showAlbums = resultType !== "Song";
  const showSongs = resultType !== "Album";
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
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Song, artist, album name or Deezer link"
            maxLength={500}
            required
          />
        </label>
        <Button disabled={busy}>Search</Button>
      </form>
      <div className="result-type-toggle" role="group" aria-label="Result type">
        {(["Both", "Album", "Song"] as const).map((type) => (
          <Button
            type="button"
            key={type}
            aria-pressed={resultType === type}
            onClick={() => setResultType(type)}
          >
            {type}
          </Button>
        ))}
      </div>
      {searched && (
        <>
          <div hidden={!showSongs}>
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
                <LibrarySongRow
                  key={song.id}
                  song={song}
                  canDelete={canDelete}
                />
              ))}
              {moreLibrary && (
                <Button
                  className="quiet toolbar"
                  disabled={loadingMore}
                  onClick={() => void loadMoreLibrary()}
                >
                  {loadingMore ? "Loading…" : "Load more library results"}
                </Button>
              )}
            </SearchSection>
          </div>
          <SearchSection
            title="Candidates for import"
            count={
              (showSongs ? results.length : 0) +
              (showAlbums
                ? albums.length +
                  (album && !albums.some((a) => a.id === album.id) ? 1 : 0)
                : 0)
            }
          >
            {importLoading && <p role="status">Searching Deezer…</p>}
            {showAlbums && album && (
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
                <Button
                  disabled={busy || albumRequested.has(album.id)}
                  onClick={() => void requestAlbum(album)}
                >
                  {albumRequested.has(album.id)
                    ? "Album requested"
                    : "Request album"}
                </Button>
              </section>
            )}
            {showAlbums && album && albumRequested.has(album.id) && (
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
            {showAlbums && albums.length > 0 && (
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
                      <Button
                        disabled={busy || albumRequested.has(found.id)}
                        onClick={() => void requestAlbum(found)}
                      >
                        {albumRequested.has(found.id)
                          ? "Album requested"
                          : "Request album"}
                      </Button>
                    </div>
                  </article>
                ))}
              </section>
            )}
            {showAlbums && albums.some((a) => albumRequested.has(a.id)) && (
              <p role="status">
                Album request saved.{" "}
                <NavLink to="/requests">View requests</NavLink>.
              </p>
            )}
            {showSongs &&
              results.length > 0 &&
              showAlbums &&
              albums.length > 0 && <h2>Songs</h2>}
            {searched &&
              (!showSongs || results.length === 0) &&
              (!showAlbums || (albums.length === 0 && !album)) &&
              !importLoading &&
              !error && (
                <p>
                  {resultType === "Both"
                    ? "No matches."
                    : `No ${resultType.toLowerCase()} matches in the loaded results.`}{" "}
                  Try another search or choose Both.
                </p>
              )}
            <div hidden={!showSongs}>
              {results.map((t) => (
                <article className="row" key={t.id}>
                  <div>
                    <strong>{label(t)}</strong>
                    <small>
                      {t.artist.name} / {t.album.title} / {time(t.duration)}
                    </small>
                  </div>
                  <Button
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
                  </Button>
                </article>
              ))}
            </div>
          </SearchSection>
        </>
      )}
      {showSongs && requested.size > 0 && (
        <p role="status">
          Requests saved. <NavLink to="/requests">View approval status</NavLink>{" "}
          or <NavLink to="/activity">follow acquisition progress</NavLink>.
        </p>
      )}
    </>
  );
}
