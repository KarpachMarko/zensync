import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import React, { useState } from "react";
import { NavLink } from "react-router-dom";
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
  const { api } = useApiMutation();
  const [actionError, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false);
  const result = useApiQuery<ConnectionsData>("/connections");
  const data = result.data;
  const error = actionError || result.error?.message;
  const load = result.reload;
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
                  <Input
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
                    <Input name="username" autoComplete="off" required />
                  </label>
                  <label>
                    Service password
                    <Input
                      name="password"
                      type="password"
                      autoComplete="new-password"
                      required
                    />
                  </label>
                </div>
                <Button disabled={busy}>Test and save server</Button>
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
                  <Input
                    name="username"
                    defaultValue={data.personalUsername}
                    autoComplete="off"
                    required
                  />
                </label>
                <label>
                  Personal password
                  <Input
                    name="password"
                    type="password"
                    autoComplete="new-password"
                    required
                  />
                </label>
              </div>
              <Button disabled={busy || !data.configured}>
                Test and link account
              </Button>
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
                  <Input
                    name="arl"
                    type="password"
                    autoComplete="new-password"
                    required
                    minLength={32}
                  />
                </label>
                <Button disabled={busy}>Save Deezer credentials</Button>
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
