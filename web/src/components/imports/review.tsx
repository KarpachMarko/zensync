import { SearchField } from "@/components/forms";
import { Actions, Row, SearchForm } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { ActiveProgress, StatusBadge } from "@/components/status";
import {
  Caption,
  PageHeading,
  SectionHeading,
  Subheading,
  Text,
} from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation } from "@/hooks/use-api";
import type { LibrarySong, Track } from "@/lib/api";
import { label, time } from "@/lib/api";
import { status, type Migration, type Source } from "@/lib/migration";
import React, { useState } from "react";
import { toast } from "sonner";
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
    [busy, setBusy] = useState(false);
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
      toast.success(
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
      <AppLink to={"/imports/" + m.id}>Back to migration</AppLink>
      <PageHeading tabIndex={-1}>{s.metadata.title}</PageHeading>
      <Text>
        {s.metadata.artist} / {s.metadata.album} / {time(s.metadata.duration)}
      </Text>
      <StatusBadge state={status(s)} />
      <ActiveProgress state={s.jobState} title={s.metadata.title} />
      {error && <Text role="alert">{error}</Text>}
      {s.candidate && (
        <section className="my-6 rounded-lg border border-solid border-[#526079] p-5 [&_h2]:mt-0">
          <SectionHeading>Selected recording</SectionHeading>
          <strong>{label(s.candidate)}</strong>
          <Text>
            {s.candidate.artist.name} / {s.candidate.album.title} /{" "}
            {time(s.candidate.duration)}
          </Text>
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
        <Text>
          <AppLink to="/requests">View request and approval</AppLink> /{" "}
          <AppLink to="/activity">View acquisition progress</AppLink>
        </Text>
      )}
      <Button
        variant="outline"
        disabled={busy}
        onClick={() => void act(s.deferred ? "resume" : "defer")}
      >
        {s.deferred ? "Resume" : "Skip for now"}
      </Button>
      {s.favoriteState === "review" && m.state === "open" && (
        <section className="bg-[#29384f] border-l-3 border-solid border-l-primary py-[13px] px-[18px] my-6 mx-0 text-[14px] max-w-[850px]">
          <Text>
            The earlier favorite write has an uncertain outcome. Keep
            Navidrome’s current state, or explicitly try starring this song
            again.
          </Text>
          <Actions>
            <Button disabled={busy} onClick={() => void act("favorite-done")}>
              Keep current favorite state
            </Button>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void act("favorite-retry")}
            >
              Retry favorite import once
            </Button>
          </Actions>
        </section>
      )}
      <SectionHeading>Find the intended recording</SectionHeading>
      <Text>
        Compare album, version and duration. Resolving a match does not request
        a new download. You can also paste a Deezer song or share URL.
      </Text>
      <SearchForm onSubmit={find}>
        <SearchField>
          Search or Deezer song URL
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            maxLength={500}
            required
          />
        </SearchField>
        <Button disabled={busy}>Find candidates</Button>
      </SearchForm>
      {candidates && (
        <>
          <Subheading>Already in Navidrome</Subheading>
          {candidates.library.length === 0 && <Text>No local candidates.</Text>}
          {candidates.library.map((t) => (
            <Row key={t.id}>
              <div>
                <strong>{t.title}</strong>
                <Caption>
                  {t.artist} / {t.album} / {time(t.duration)}
                </Caption>
              </div>
              <Button
                disabled={busy}
                onClick={() => void act("resolve", { serverId: t.id })}
              >
                Use existing
              </Button>
            </Row>
          ))}
          <Subheading>Deezer recordings</Subheading>
          {candidates.deezer.length === 0 && (
            <Text>No results. Try a direct song URL.</Text>
          )}
          {candidates.deezer.map((t) => (
            <Row key={t.id}>
              <div>
                <strong>{label(t)}</strong>
                <Caption>
                  {t.artist.name} / {t.album.title} / {time(t.duration)}
                </Caption>
              </div>
              <Button
                disabled={busy}
                onClick={() =>
                  void act("resolve", { target: "deezer:track:" + t.id })
                }
              >
                Use this recording
              </Button>
            </Row>
          ))}
        </>
      )}
    </>
  );
}
