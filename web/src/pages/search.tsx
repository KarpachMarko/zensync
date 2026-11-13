import { AutoLoadMore } from "@/components/auto-load-more";
import { MetadataJobs } from "@/components/music/metadata-jobs";
import { RecordingTools } from "@/components/imports/recording-tools";
import { PreviewPlayer } from "@/components/imports/preview-player";
import { useRecordingPreview } from "@/hooks/use-recording-preview";
import { Actions, Row } from "@/components/layout/content";
import { LibrarySongRow } from "@/components/music/library-song-row";
import { SearchSection } from "@/components/music/search-section";
import { AppLink } from "@/components/navigation";
import { SearchBox } from "@/components/search-box";
import {
  Caption,
  PageHeading,
  SectionHeading,
  Text,
} from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiMutation } from "@/hooks/use-api";
import type { LibrarySong, Track } from "@/lib/api";
import { api, label, providerLabel, requestKey, time, trackTarget } from "@/lib/api";
import { searchHref } from "@/lib/url-state";
import { useInfiniteQuery, useQueries } from "@tanstack/react-query";
import { parseAsString, parseAsStringLiteral, useQueryState } from "nuqs";
import { useRef, useState } from "react";
import { toast } from "sonner";
type FoundAlbum = {
  id: number;
  title: string;
  artist: { name: string };
  trackCount?: number;
};
export function Search({
  canDelete,
  canEdit = false,
}: {
  canDelete: boolean;
  canEdit?: boolean;
}) {
  const preview = useRecordingPreview();
  const [urlQuery, setUrlQuery] = useQueryState(
    "q",
    parseAsString.withDefault("").withOptions({ history: "push" }),
  );
  const [resultType, setResultType] = useQueryState(
    "type",
    parseAsStringLiteral(["Both", "Album", "Song"]).withDefault("Both"),
  );
  const [provider, setProvider] = useQueryState(
    "provider",
    parseAsStringLiteral(["all", "deezer", "lossless", "hitmo"]).withDefault("all"),
  );
  const showAlbums = resultType !== "Song";
  const showSongs = resultType !== "Album";
  const searched = !!urlQuery.trim();
  const [albumRequested, setAlbumRequested] = useState<Set<number>>(new Set());
  const [requested, setRequested] = useState<Set<string>>(new Set());
  const albumKeys = useRef<Record<number, string>>({});
  const keys = useRef<Record<string, string>>({});
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
  type SearchResponse = Track[] | { tracks: Track[]; album?: FoundAlbum; albums?: FoundAlbum[]; warnings?: string[] };
  const providers = ["deezer", "lossless", "hitmo"] as const;
  const directProvider = /^(https:\/\/eu\.hitmoz\.com\/|hitmo:)/.test(urlQuery) ? "hitmo"
    : /^(https:\/\/lossless\.am\/|lossless:)/.test(urlQuery) ? "lossless"
    : /^(https?:|deezer:)/.test(urlQuery) ? "deezer" : null;
  const selectedProviders = provider === "all" ? providers.filter(name => !directProvider || directProvider === name) : [provider];
  const candidateQueries = useQueries({ queries: selectedProviders.map(name => ({
    queryKey: ["api", "search", urlQuery, name],
    queryFn: ({ signal }) => api<SearchResponse>("/search?q=" + encodeURIComponent(urlQuery) + "&details=1&provider=" + name, "GET", undefined, undefined, signal),
    enabled: searched,
  })) });
  const responses = candidateQueries.map(query => query.data);
  const found = responses.flatMap(response => !response || Array.isArray(response) || !response.album ? [] : [response.album])[0];
  const album = found?.trackCount === 1 ? undefined : found;
  const albums = [...new Map(responses.flatMap(response => !response || Array.isArray(response) ? [] : response.albums ?? []).filter(album => album.trackCount !== 1).map(album => [album.id, album])).values()];
  const warnings = [...new Set(responses.flatMap(response => !response || Array.isArray(response) ? [] : response.warnings ?? []))];
  candidateQueries.forEach((query, index) => {
    if (query.error) warnings.push(providerLabel(selectedProviders[index] as Track["provider"]) + ": " + query.error.message);
  });
  const results = [...new Map(responses.flatMap(response => Array.isArray(response) ? response : response?.tracks ?? []).map(track => [trackTarget(track), track])).values()];
  const library = libraryQuery.data?.pages.flat() ?? [];
  const libraryError = libraryQuery.error?.message;
  const error = actionError;
  const libraryLoading =
    libraryQuery.isFetching && !libraryQuery.isFetchingNextPage;
  const importLoading = candidateQueries.some(query => query.isFetching);
  const moreLibrary = libraryQuery.hasNextPage;
  async function search(query: string) {
    setError("");
    if (urlQuery === query.trim()) {
      await Promise.all([libraryQuery.refetch(), ...candidateQueries.map(query => query.refetch())]);
    } else await setUrlQuery(query.trim());
  }
  async function request(t: Track) {
    setBusy(true);
    setError("");
    try {
      await mutate(
        "/requests",
        "POST",
        { target: trackTarget(t) },
        (keys.current[trackTarget(t)] ||= requestKey()),
      );
      setRequested(new Set([...requested, trackTarget(t)]));
      toast.success("Song request saved.");
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
      toast.success("Album request saved.");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading tabIndex={-1}>Search</PageHeading>
      {canEdit && <MetadataJobs />}
      <Text>
        Find music in your library, on Deezer, lossless.am or Hitmo. Deezer also
        supports song and album links.
      </Text>
      <div className="mb-4 flex flex-wrap gap-2" role="group" aria-label="Media provider">
        {(["all", "deezer", "lossless", "hitmo"] as const).map((name) => (
          <Button
            key={name}
            variant="outline"
            aria-pressed={provider === name}
            onClick={() => void setProvider(name)}
          >
            {name === "all" ? "All providers" : name === "lossless" ? "lossless.am" : providerLabel(name)}
          </Button>
        ))}
      </div>
      <SearchBox
        label={
          (provider === "deezer" || provider === "all")
            ? "Song, artist, album or Deezer URL"
            : provider === "hitmo" ? "Song, artist or Hitmo URL" : "Song or artist on lossless.am"
        }
        placeholder="Song, artist, album name or Deezer link"
        value={urlQuery}
        disabled={busy}
        onSearch={search}
      />

      <div
        className="mb-5 flex w-fit rounded-md border border-solid border-border"
        role="group"
        aria-label="Result type"
      >
        {(["Both", "Album", "Song"] as const).map((type) => (
          <Button
            type="button"
            key={type}
            className="rounded-none border-border bg-transparent text-muted-foreground first:rounded-l-[5px] last:rounded-r-[5px] aria-pressed:bg-primary aria-pressed:text-background"
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
              {libraryLoading && (
                <Text role="status">Searching your library…</Text>
              )}
              {libraryError && (
                <Text role="alert">
                  {libraryError}{" "}
                  <AppLink to="/settings/connections">
                    Connection settings
                  </AppLink>
                </Text>
              )}
              {!libraryLoading && !libraryError && library.length === 0 && (
                <Text>
                  No matching music in your library. See candidates for import
                  below.
                </Text>
              )}
              {library.map((song) => (
                <LibrarySongRow
                  key={song.id}
                  song={song}
                  canDelete={canDelete}
                  canEdit={canEdit}
                />
              ))}
              <AutoLoadMore key={urlQuery} hasMore={!!moreLibrary} loading={libraryQuery.isFetching}
                error={libraryQuery.isFetchNextPageError} loadMore={libraryQuery.fetchNextPage} />
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
            {importLoading && (
              <Text role="status">
                Searching {provider === "all" ? "all providers" : providerLabel(provider)}…
              </Text>
            )}
            {showAlbums && album && (
              <section className="my-5 flex items-center justify-between gap-5 border-y border-solid border-border py-[18px] [&_h2]:m-0 [&_p]:my-1.5 [&_button]:shrink-0 max-[700px]:items-start max-[700px]:flex-col">
                <div>
                  <SectionHeading>{album.title}</SectionHeading>
                  <Text>
                    {album.artist.name} / {album.trackCount}{" "}
                    {album.trackCount === 1 ? "track" : "tracks"}
                  </Text>
                  <Text className="text-[13px]">
                    Request the whole album with one approval, or choose
                    individual songs below.
                  </Text>
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
              <Text role="status">
                Album request saved.{" "}
                <AppLink to="/requests">View the single album request</AppLink>.
              </Text>
            )}
            {error && <Text role="alert">{error}</Text>}
            {warnings.map((w) => (
              <Text
                className="bg-[#29384f] border-l-3 border-solid border-l-primary py-[13px] px-[18px] my-6 mx-0 text-[14px] max-w-[850px]"
                role="status"
                key={w}
              >
                {w}
              </Text>
            ))}
            {showAlbums && albums.length > 0 && (
              <section aria-label="Album results">
                <SectionHeading>Albums</SectionHeading>
                {albums.map((found) => (
                  <Row asChild key={found.id}>
                    <article className="max-[700px]:items-start">
                      <div>
                        <strong>{found.title}</strong>
                        <Caption>
                          {found.artist.name}
                          {found.trackCount
                            ? " / " +
                              found.trackCount +
                              (found.trackCount === 1 ? " track" : " tracks")
                            : ""}
                        </Caption>
                      </div>
                      <Actions>
                        <AppLink
                          to={searchHref(
                            "https://www.deezer.com/album/" + found.id,
                          )}
                        >
                          View tracks
                        </AppLink>
                        <Button
                          disabled={busy || albumRequested.has(found.id)}
                          onClick={() => void requestAlbum(found)}
                        >
                          {albumRequested.has(found.id)
                            ? "Album requested"
                            : "Request album"}
                        </Button>
                      </Actions>
                    </article>
                  </Row>
                ))}
              </section>
            )}
            {showAlbums && albums.some((a) => albumRequested.has(a.id)) && (
              <Text role="status">
                Album request saved.{" "}
                <AppLink to="/requests">View requests</AppLink>.
              </Text>
            )}
            {showSongs &&
              results.length > 0 &&
              showAlbums &&
              albums.length > 0 && <SectionHeading>Songs</SectionHeading>}
            {searched &&
              (!showSongs || results.length === 0) &&
              (!showAlbums || (albums.length === 0 && !album)) &&
              !importLoading &&
              !error && (
                <Text>
                  {resultType === "Both"
                    ? "No matches."
                    : `No ${resultType.toLowerCase()} matches in the loaded results.`}{" "}
                  Try another search or choose Both.
                </Text>
              )}
            <div hidden={!showSongs}>
              {results.map((t) => (
                <Row asChild key={trackTarget(t)}>
                  <article className="max-[700px]:items-start">
                    <div>
                      <strong>{label(t)}</strong>
                      <Caption>
                        {t.artist.name} / {t.album.title} / {time(t.duration)} /{" "}
                        {providerLabel(t.provider)}
                      </Caption>
                      <RecordingTools
                        track={t}
                        playing={
                          preview.playing &&
                          !!preview.active &&
                          trackTarget(preview.active) === trackTarget(t)
                        }
                        onPreview={preview.toggle}
                      />
                    </div>
                    <Button
                      disabled={
                        busy ||
                        requested.has(trackTarget(t)) ||
                        !!(album && albumRequested.has(album.id))
                      }
                      onClick={() => void request(t)}
                    >
                      {album && albumRequested.has(album.id)
                        ? "Included in album"
                        : requested.has(trackTarget(t))
                          ? "Requested"
                          : "Request"}
                    </Button>
                  </article>
                </Row>
              ))}
            </div>
          </SearchSection>
        </>
      )}
      <PreviewPlayer preview={preview} />
      {showSongs && requested.size > 0 && (
        <Text role="status">
          Requests saved. <AppLink to="/requests">View approval status</AppLink>{" "}
          or <AppLink to="/activity">follow acquisition progress</AppLink>.
        </Text>
      )}
    </>
  );
}
