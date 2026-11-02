import { ActiveProgress, StatusBadge } from "@/components/status";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation } from "@/hooks/use-api";
import type { LibrarySong, Track } from "@/lib/api";
import { label, time } from "@/lib/api";
import { status, type Migration, type Source } from "@/lib/migration";
import React, { useState } from "react";
import { NavLink } from "react-router-dom";
export function Review({
  source: s,
  migration: m,
  reload,
}: {
  source: Source;
  migration: Migration;
  reload: () => Promise<void>;
}) {
  const { api } = useApiMutation();
  const [query, setQuery] = useState(
      s.metadata.artist + " " + s.metadata.title,
    ),
    [candidates, setCandidates] = useState<{
      deezer: Track[];
      library: LibrarySong[];
    }>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState("");
  const prefix = "/imports/" + m.id + "/songs/" + s.id;
  async function find(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      setCandidates(await api(prefix + "/candidates", "POST", { query }));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function act(action: string, body?: unknown) {
    setBusy(true);
    setError("");
    try {
      await api(prefix + "/" + action, "POST", body);
      await reload();
      setNotice(
        action === "acquire" ? "Acquisition request saved." : "Song updated.",
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <NavLink to={"/imports/" + m.id}>Back to migration</NavLink>
      <h1 tabIndex={-1}>{s.metadata.title}</h1>
      <p>
        {s.metadata.artist} / {s.metadata.album} / {time(s.metadata.duration)}
      </p>
      <StatusBadge state={status(s)} />
      <ActiveProgress state={s.jobState} title={s.metadata.title} />
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {s.candidate && (
        <section className="chosen">
          <h2>Selected recording</h2>
          <strong>{label(s.candidate)}</strong>
          <p>
            {s.candidate.artist.name} / {s.candidate.album.title} /{" "}
            {time(s.candidate.duration)}
          </p>
          <Button
            disabled={
              busy ||
              s.deferred ||
              !!s.serverId ||
              s.decision === "approved" ||
              s.decision === "awaiting_approval"
            }
            onClick={() => void act("acquire")}
          >
            Request missing audio
          </Button>
        </section>
      )}
      {s.requestId && (
        <p>
          <NavLink to="/requests">View request and approval</NavLink> /{" "}
          <NavLink to="/activity">View acquisition progress</NavLink>
        </p>
      )}
      <Button
        className="quiet"
        disabled={busy}
        onClick={() => void act(s.deferred ? "resume" : "defer")}
      >
        {s.deferred ? "Resume" : "Skip for now"}
      </Button>
      {s.favoriteState === "review" && m.state === "open" && (
        <section className="notice">
          <p>
            The earlier favorite write has an uncertain outcome. Keep
            Navidrome’s current state, or explicitly try starring this song
            again.
          </p>
          <div className="actions">
            <Button disabled={busy} onClick={() => void act("favorite-done")}>
              Keep current favorite state
            </Button>
            <Button
              className="quiet"
              disabled={busy}
              onClick={() => void act("favorite-retry")}
            >
              Retry favorite import once
            </Button>
          </div>
        </section>
      )}
      <h2>Find the intended recording</h2>
      <p>
        Compare album, version and duration. Resolving a match does not request
        a new download. You can also paste a Deezer song or share URL.
      </p>
      <form className="searchbar" onSubmit={find}>
        <label className="grow">
          Search or Deezer song URL
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            maxLength={500}
            required
          />
        </label>
        <Button disabled={busy}>Find candidates</Button>
      </form>
      {candidates && (
        <>
          <h3>Already in Navidrome</h3>
          {candidates.library.length === 0 && <p>No local candidates.</p>}
          {candidates.library.map((t) => (
            <div className="row" key={t.id}>
              <div>
                <strong>{t.title}</strong>
                <small>
                  {t.artist} / {t.album} / {time(t.duration)}
                </small>
              </div>
              <Button
                disabled={busy}
                onClick={() => void act("resolve", { serverId: t.id })}
              >
                Use existing
              </Button>
            </div>
          ))}
          <h3>Deezer recordings</h3>
          {candidates.deezer.length === 0 && (
            <p>No results. Try a direct song URL.</p>
          )}
          {candidates.deezer.map((t) => (
            <div className="row" key={t.id}>
              <div>
                <strong>{label(t)}</strong>
                <small>
                  {t.artist.name} / {t.album.title} / {time(t.duration)}
                </small>
              </div>
              <Button
                disabled={busy}
                onClick={() =>
                  void act("resolve", { target: "deezer:track:" + t.id })
                }
              >
                Use this recording
              </Button>
            </div>
          ))}
        </>
      )}
    </>
  );
}
