import { LibrarySongRow } from "@/components/music/library-song-row";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { LibrarySong } from "@/lib/api";
import { api } from "@/lib/api";
import { searchHref } from "@/lib/url-state";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { NavLink, useNavigate } from "react-router-dom";
export function Library({ canDelete }: { canDelete: boolean }) {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const result = useInfiniteQuery({
    queryKey: ["api", "library"],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api<LibrarySong[] | { songs: LibrarySong[]; nextOffset: number }>(
        "/library?q=&offset=" + pageParam + "&details=1",
      ),
    getNextPageParam: (last, pages) => {
      const next = Array.isArray(last)
        ? last.length === 100
          ? pages.length * 100
          : -1
        : last.nextOffset;
      return next >= 0 ? next : undefined;
    },
  });
  const rows =
    result.data?.pages.flatMap((page) =>
      Array.isArray(page) ? page : page.songs,
    ) ?? [];
  const busy = result.isFetching;
  const error = result.error?.message;
  return (
    <>
      <h1 tabIndex={-1}>Library</h1>
      <p>
        Available music from your shared Navidrome library. Deleting a song
        removes it for everyone.
      </p>
      <form
        className="searchbar"
        onSubmit={(e) => {
          e.preventDefault();
          if (query.trim()) navigate(searchHref(query.trim()));
        }}
      >
        <label className="grow">
          Search music
          <Input value={query} onChange={(e) => setQuery(e.target.value)} />
        </label>
        <Button disabled={busy}>Search</Button>
      </form>
      {error && (
        <p role="alert">
          {error}{" "}
          <NavLink to="/settings/connections">Connection settings</NavLink>
        </p>
      )}
      {rows.map((song) => (
        <LibrarySongRow key={song.id} song={song} canDelete={canDelete} />
      ))}
      {!busy && !error && rows.length === 0 && (
        <p>
          No music found.{" "}
          <NavLink to="/search">Search for music to import</NavLink>.
        </p>
      )}
      {result.hasNextPage && (
        <Button disabled={busy} onClick={() => void result.fetchNextPage()}>
          Load more
        </Button>
      )}
    </>
  );
}
