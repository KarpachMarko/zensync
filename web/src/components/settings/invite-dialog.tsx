import { FieldLabel } from "@/components/forms";
import { Caption, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useApiMutation } from "@/hooks/use-api";
import { useAppForm } from "@/hooks/use-app-form";
import { inviteSchema } from "@/lib/form-schemas";
import { CopyIcon } from "lucide-react";
import { createSerializer, parseAsString } from "nuqs";
import { useState } from "react";
import { toast } from "sonner";

type CreatedInvite = {
  id: string;
  token: string;
  username: string;
  expires: number;
};
const inviteHref = createSerializer({ token: parseAsString });
export function InviteDialog({ onClose }: { onClose: () => void }) {
  const { api, isPending } = useApiMutation();
  const [created, setCreated] = useState<CreatedInvite>();
  const [error, setError] = useState("");
  const form = useAppForm({
    defaultValues: { username: "" },
    validators: { onSubmit: inviteSchema },
    onSubmit: async ({ value }) => {
      setError("");
      try {
        setCreated(
          await api<CreatedInvite>("/invitations", "POST", {
            username: value.username.trim(),
          }),
        );
        toast.success("Invitation created.");
      } catch (cause) {
        setError(
          cause instanceof Error
            ? cause.message
            : "Unable to create invitation",
        );
      }
    },
  });
  const link = created
    ? location.origin + inviteHref("/invite", { token: created.token })
    : "";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>
            {created ? "Invitation ready" : "Invite user"}
          </DialogTitle>
          <DialogDescription>
            {created
              ? "Share this single-use link with the person you want to invite. They choose their own password."
              : "Create a single-use invite link. New users require approval for acquisitions and deletions until you change their permissions."}
          </DialogDescription>
        </DialogHeader>
        {created ? (
          <>
            <FieldLabel>
              Invitation link
              <Input
                readOnly
                value={link}
                onFocus={(event) => event.currentTarget.select()}
                autoComplete="off"
              />
            </FieldLabel>
            <Caption>
              For {created.username}. Expires{" "}
              {new Date(created.expires * 1000).toLocaleDateString()}.
            </Caption>
            <Button
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(link);
                  toast.success("Invitation link copied.");
                } catch {
                  toast.error(
                    "Select the invitation link and copy it manually.",
                  );
                }
              }}
            >
              <CopyIcon className="mr-2 inline size-4" />
              Copy invitation link
            </Button>
          </>
        ) : (
          <form
            noValidate
            onSubmit={(event) => {
              event.preventDefault();
              void form.handleSubmit();
            }}
          >
            <form.AppField name="username">
              {(field) => (
                <field.TextField label="Username" autoComplete="off" />
              )}
            </form.AppField>
            {error && <Text role="alert">{error}</Text>}
            <Button disabled={isPending}>
              {isPending ? "Creating invitation…" : "Create invitation"}
            </Button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
