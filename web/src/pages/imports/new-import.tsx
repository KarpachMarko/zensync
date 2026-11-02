import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { upload } from "@/lib/api";
import React, { useState } from "react";
import { NavLink, useNavigate } from "react-router-dom";
export function NewImport() {
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const file = new FormData(e.currentTarget).get("xml") as File;
    if (file.size > 32 * 1024 * 1024) {
      setError("Choose an export smaller than 32 MiB.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const m = await upload(file);
      navigate("/imports/" + m.id);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <NavLink to="/imports">Imports</NavLink>
      <h1 tabIndex={-1}>Start a migration</h1>
      <p>
        In Apple Music, choose File → Library → Export Library. For one
        playlist, choose File → Library → Export Playlist and select XML.
      </p>
      <form onSubmit={submit}>
        <label>
          Apple Music or iTunes XML export
          <Input
            name="xml"
            type="file"
            accept=".xml,application/xml,text/xml"
            required
          />
        </label>
        <p>
          Uploading saves a private snapshot. It does not download music or
          change Navidrome. You’ll choose playlists and favorites in the
          preview.
        </p>
        {error && <p role="alert">{error}</p>}
        <Button disabled={busy}>
          {busy ? "Reading your library…" : "Upload and preview"}
        </Button>
      </form>
    </>
  );
}
