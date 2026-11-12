import { LibrarySongRow } from "@/components/music/library-song-row";
import { MetadataJobs } from "@/components/music/metadata-jobs";
import { AppLink } from "@/components/navigation";
import { SearchBox } from "@/components/search-box";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api, type LibrarySong } from "@/lib/api";
import type { LibraryBrowse } from "@/lib/metadata";
import { useInfiniteQuery } from "@tanstack/react-query";
import { createSerializer, parseAsString, useQueryStates } from "nuqs";
import { useNavigate, useParams } from "react-router-dom";
const parsers = {
  q: parseAsString.withDefault(""),
  collection: parseAsString.withDefault(""),
  artist: parseAsString.withDefault(""),
  contents: parseAsString.withDefault(""),
};
const libraryHref = createSerializer(parsers);
const views = ["songs", "playlists", "artists", "albums"] as const;
type View = (typeof views)[number];
export function Library({
  canDelete,
  canEdit = false,
}: {
  canDelete: boolean;
  canEdit?: boolean;
}) {
  const navigate = useNavigate();
  const params = useParams();
  const view: View = views.find((v) => v === params.view) ?? "songs";
  const [state, setState] = useQueryStates(parsers, { history: "push" });
  const result = useInfiniteQuery({
    queryKey: [
      "api",
      "library",
      view,
      state.collection,
      state.q,
      state.contents,
    ],
    initialPageParam: 0,
    queryFn: async ({ pageParam, signal }): Promise<LibraryBrowse> => {
      if (view === "songs") {
        const page = await api<
          LibrarySong[] | { songs: LibrarySong[]; nextOffset: number }
        >(
          `/library?details=1&q=${encodeURIComponent(state.q)}&contents=${state.contents}&offset=${pageParam}`,
          "GET",
          undefined,
          undefined,
          signal,
        );
        return {
          collections: [],
          songs: Array.isArray(page) ? page : page.songs,
          name: "",
          nextOffset: Array.isArray(page)
            ? page.length === 100
              ? pageParam + 100
              : -1
            : page.nextOffset,
        };
      }
      return api<LibraryBrowse>(
        `/library/browse?view=${view}&id=${encodeURIComponent(state.collection)}&q=${encodeURIComponent(state.q)}&contents=${state.contents}&offset=${pageParam}`,
        "GET",
        undefined,
        undefined,
        signal,
      );
    },
    getNextPageParam: (last) =>
      last.nextOffset >= 0 ? last.nextOffset : undefined,
  });
  const rows = result.data?.pages.flatMap((p) => p.songs) ?? [];
  const collections = result.data?.pages.flatMap((p) => p.collections) ?? [];
  const name = result.data?.pages[0]?.name;
  return (
    <>
      <PageHeading tabIndex={-1}>Library</PageHeading>
      <Tabs
        value={view}
        onValueChange={(value) => navigate(`/library/${value}`)}
      >
        <TabsList>
          <TabsTrigger value="songs">Songs</TabsTrigger>
          <TabsTrigger value="playlists">Playlists</TabsTrigger>
          <TabsTrigger value="artists">Artists</TabsTrigger>
          <TabsTrigger value="albums">Albums</TabsTrigger>
        </TabsList>
      </Tabs>
      {state.collection && (
        <div className="my-2 flex flex-wrap items-center gap-2 text-sm">
          <AppLink to={`/library/${view}`}>
            {view.charAt(0).toUpperCase() + view.slice(1)}
          </AppLink>
          {state.artist && (
            <>
              <span>/</span>
              <AppLink
                to={libraryHref("/library/artists", {
                  collection: state.artist,
                })}
              >
                Artist albums
              </AppLink>
            </>
          )}
          <span>/</span>
          <strong>{name || "Loading…"}</strong>
        </div>
      )}
      {view === "artists" && state.collection && (
        <Button
          size="sm"
          variant="outline"
          className="my-2"
          onClick={() =>
            void setState({
              contents: state.contents === "songs" ? null : "songs",
              q: null,
            })
          }
        >
          {state.contents === "songs" ? "Show albums" : "Show all songs"}
        </Button>
      )}
      <div className="my-3">
        <SearchBox
          compact
          value={state.q}
          label="Filter library"
          placeholder={`Filter ${state.collection && view !== "artists" ? "songs" : view}…`}
          onSearch={async (q) => {
            await setState({ q });
          }}
        />
      </div>
      {canEdit && <MetadataJobs />}
      {result.error && (
        <Text role="alert">
          {result.error.message}{" "}
          <AppLink to="/settings/connections">Connection settings</AppLink>
        </Text>
      )}
      {result.isPending && <Text role="status">Loading library…</Text>}
      {collections.map((item) => (
        <div
          key={item.id}
          className="flex items-center justify-between gap-2 border-b border-border py-2"
        >
          <AppLink
            className="min-w-0 flex-1 no-underline"
            aria-label={
              item.artist ? `${item.name} — ${item.artist}` : item.name
            }
            to={libraryHref(
              `/library/${view === "artists" && state.collection ? "albums" : view}`,
              {
                collection: item.id,
                artist:
                  view === "artists" && state.collection
                    ? state.collection
                    : null,
              },
            )}
          >
            <strong className="text-sm">{item.name}</strong>
            {item.artist && (
              <span className="ml-2 text-xs text-muted-foreground">
                {item.artist}
              </span>
            )}
          </AppLink>
          <span className="text-xs text-muted-foreground">
            {item.songCount}{" "}
            {view === "artists" && !state.collection ? "albums" : "songs"}
          </span>
        </div>
      ))}
      {rows.map((song, index) => (
        <LibrarySongRow
          key={`${song.id}-${index}`}
          song={song}
          canDelete={canDelete}
          canEdit={canEdit}
        />
      ))}
      {!result.isPending &&
        !result.error &&
        !rows.length &&
        !collections.length && (
          <Text>
            No{" "}
            {view === "songs" || (state.collection && view !== "artists")
              ? "songs"
              : view}{" "}
            found.
          </Text>
        )}
      {result.hasNextPage && (
        <Button
          variant="outline"
          disabled={result.isFetching}
          onClick={() => void result.fetchNextPage()}
        >
          Load more
        </Button>
      )}
    </>
  );
}
