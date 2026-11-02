import { LibrarySongRow } from "@/components/music/library-song-row";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { LibrarySong } from "@/lib/api";
import { api } from "@/lib/api";
import { useEffect, useState } from "react";
import { NavLink, useNavigate } from "react-router-dom";
export function Library({ canDelete }: { canDelete: boolean }) {
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
      {nextOffset >= 0 && (
        <Button disabled={busy} onClick={() => void load(nextOffset)}>
          Load more
        </Button>
      )}
    </>
  );
}
