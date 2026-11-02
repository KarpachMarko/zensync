import { StatusBadge } from "@/components/status";
import { api } from "@/lib/api";
import { type Migration } from "@/lib/migration";
import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
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
