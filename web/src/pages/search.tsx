import { LibrarySongRow } from "@/components/music/library-song-row";
import { SearchSection } from "@/components/music/search-section";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation } from "@/hooks/use-api";
import type { LibrarySong, Track } from "@/lib/api";
import { api, label, requestKey, time } from "@/lib/api";
import { searchHref } from "@/lib/url-state";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { parseAsString, parseAsStringLiteral, useQueryState } from "nuqs";
import { useEffect, useRef, useState } from "react";
import { NavLink } from "react-router-dom";
type FoundAlbum = {
  id: number;
  title: string;
  artist: { name: string };
  trackCount?: number;
};
export function Search({ canDelete }: { canDelete: boolean }) {
  const [urlQuery, setUrlQuery] = useQueryState(
    "q",
    parseAsString.withDefault("").withOptions({ history: "push" }),
  );
  const [resultType, setResultType] = useQueryState(
    "type",
    parseAsStringLiteral(["Both", "Album", "Song"]).withDefault("Both"),
  );
  const showAlbums = resultType !== "Song";
  const showSongs = resultType !== "Album";
  const [query, setQuery] = useState(urlQuery);
  useEffect(() => setQuery(urlQuery), [urlQuery]);
  const searched = !!urlQuery.trim();
  const [albumRequested, setAlbumRequested] = useState<Set<number>>(new Set());
  const [requested, setRequested] = useState<Set<number>>(new Set());
  const albumKeys = useRef<Record<number, string>>({});
  const keys = useRef<Record<number, string>>({});
  const [actionError, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const { api: mutate } = useApiMutation();
  const libraryQuery = useInfiniteQuery({
    queryKey: ["api", "library-search", urlQuery],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api<LibrarySong[]>(
        "/library?q=" + encodeURIComponent(urlQuery) + "&offset=" + pageParam,
      ),
    getNextPageParam: (last, pages) =>
      last.length === 100 ? pages.length * 100 : undefined,
    enabled: searched,
  });
  const candidateQuery = useQuery({
    queryKey: ["api", "search", urlQuery],
    queryFn: () =>
      api<
        | Track[]
        | {
            tracks: Track[];
            album?: FoundAlbum;
            albums?: FoundAlbum[];
            warnings?: string[];
          }
      >("/search?q=" + encodeURIComponent(urlQuery) + "&details=1"),
    enabled: searched,
  });
  const response = candidateQuery.data;
  const found = Array.isArray(response) ? undefined : response?.album;
  const album = found?.trackCount === 1 ? undefined : found;
  const albums = Array.isArray(response)
    ? []
    : (response?.albums ?? []).filter((album) => album.trackCount !== 1);
  const warnings = Array.isArray(response) ? [] : (response?.warnings ?? []);
  const results = Array.isArray(response) ? response : (response?.tracks ?? []);
  const library = libraryQuery.data?.pages.flat() ?? [];
  const libraryError = libraryQuery.error?.message;
  const error = actionError || candidateQuery.error?.message;
  const libraryLoading =
    libraryQuery.isFetching && !libraryQuery.isFetchingNextPage;
  const importLoading = candidateQuery.isFetching;
  const moreLibrary = libraryQuery.hasNextPage;
  const loadingMore = libraryQuery.isFetchingNextPage;
  async function loadMoreLibrary() {
    await libraryQuery.fetchNextPage();
  }
  async function search() {
    setError("");
    if (urlQuery === query.trim()) {
      await Promise.all([libraryQuery.refetch(), candidateQuery.refetch()]);
    } else await setUrlQuery(query.trim());
  }
  async function request(t: Track) {
    setBusy(true);
    setError("");
    try {
      await mutate(
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
      await mutate(
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
          void search();
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
                        to={searchHref(
                          "https://www.deezer.com/album/" + found.id,
                        )}
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
