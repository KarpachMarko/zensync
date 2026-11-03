import { FieldLabel, NativeSelect } from "@/components/forms";
import { SectionHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiMutation } from "@/hooks/use-api";
import type { User } from "@/lib/types";
import { useState } from "react";
import { toast } from "sonner";
export function Quality({
  user,
  reload,
}: {
  user: User;
  reload: () => Promise<void>;
}) {
  const { api } = useApiMutation();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  return (
    <>
      {message && <Text role="alert">{message}</Text>}
      <SectionHeading>Your quality preference</SectionHeading>
      <Text>
        Requests preserve a snapshot of this preference when submitted.
      </Text>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setMessage("");
          try {
            await api("/quality", "PUT", {
              quality: String(
                new FormData(e.currentTarget).get("quality"),
              ).split(","),
            });
            await reload();
            setMessage("");
            toast.success("Quality preference saved.");
          } catch (e) {
            setMessage((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <FieldLabel>
          Allowed qualities, in order
          <NativeSelect name="quality" defaultValue={user.quality.join(",")}>
            <option value="lossless,320,128">Lossless → 320 → 128</option>
            <option value="lossless">Lossless only</option>
            <option value="320,128">320 → 128</option>
            <option value="320">320 only</option>
            <option value="128">128 only</option>
          </NativeSelect>
        </FieldLabel>
        <Button disabled={busy}>Save preference</Button>
      </form>
      <Text className="text-[13px]">
        d-fi tries only the allowed qualities in this order. Existing playable
        music is reused; replacing shared audio is not enabled yet.
      </Text>
    </>
  );
}
