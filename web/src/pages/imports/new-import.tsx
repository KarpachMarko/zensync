import { FieldLabel } from "@/components/forms";
import { AppLink } from "@/components/navigation";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { upload } from "@/lib/api";
import { useMutation } from "@tanstack/react-query";
import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
export function NewImport() {
  const { mutateAsync: uploadFile } = useMutation({ mutationFn: upload });
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
      const m = await uploadFile(file);
      navigate("/imports/" + m.id);
      toast.success(
        "XML uploaded. Review the migration preview before committing.",
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <AppLink to="/imports">Imports</AppLink>
      <PageHeading tabIndex={-1}>Start a migration</PageHeading>
      <Text>
        In Apple Music, choose File → Library → Export Library. For one
        playlist, choose File → Library → Export Playlist and select XML.
      </Text>
      <form onSubmit={submit}>
        <FieldLabel>
          Apple Music or iTunes XML export
          <Input
            name="xml"
            type="file"
            accept=".xml,application/xml,text/xml"
            required
          />
        </FieldLabel>
        <Text>
          Uploading saves a private snapshot. It does not download music or
          change Navidrome. You’ll choose playlists and favorites in the
          preview.
        </Text>
        {error && <Text role="alert">{error}</Text>}
        <Button disabled={busy}>
          {busy ? "Reading your library…" : "Upload and preview"}
        </Button>
      </form>
    </>
  );
}
