import { StatusBadge } from "@/components/status";
import { useApiQuery } from "@/hooks/use-api";
import { type Migration } from "@/lib/migration";
import { NavLink } from "react-router-dom";
export function Imports() {
  const result = useApiQuery<Migration[]>("/imports");
  const rows = result.data ?? [];
  const error = result.error?.message;
  const loading = result.isPending;
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
            Export your library from Apple Music as XML. ZenSync keeps every
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
            <StatusBadge state={m.state} />
          </NavLink>
        ))
      )}
    </>
  );
}
