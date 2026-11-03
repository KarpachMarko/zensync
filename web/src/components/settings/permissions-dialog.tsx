import { Caption, Text } from "@/components/typography";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useApiMutation } from "@/hooks/use-api";
import { useAutosave } from "@/hooks/use-autosave";
import { permissionsSchema } from "@/lib/form-schemas";
import type { User } from "@/lib/types";

export function PermissionsDialog({
  user,
  onClose,
}: {
  user: User;
  onClose: () => void;
}) {
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[calc(100dvh-32px)] overflow-auto sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Permissions for {user.username}</DialogTitle>
          <DialogDescription>
            Choose which shared-library actions can proceed without
            administrator approval. Changes save automatically.
          </DialogDescription>
        </DialogHeader>
        <PermissionEditor user={user} />
      </DialogContent>
    </Dialog>
  );
}
function PermissionEditor({ user }: { user: User }) {
  const { api } = useApiMutation({ errorToastId: "permissions-" + user.id });
  const editor = useAutosave({
    initialValue: { direct: user.direct, directDelete: user.directDelete },
    schema: permissionsSchema,
    save: async (permissions) => {
      await api(`/users/${user.id}/permissions`, "PUT", permissions);
    },
    message: `Permissions saved for ${user.username}.`,
    toastId: "permissions-" + user.id,
  });
  return (
    <div className="flex flex-col gap-5 py-2">
      <div className="flex items-start justify-between gap-4">
        <div>
          <Label
            htmlFor="permission-acquire"
            className="leading-normal font-semibold"
          >
            Add music without approval
          </Label>
          <Caption className="mt-1">
            Otherwise, an administrator reviews each acquisition.
          </Caption>
        </div>
        <Switch
          id="permission-acquire"
          checked={editor.value.direct}
          onCheckedChange={(direct) =>
            editor.update({ ...editor.value, direct })
          }
        />
      </div>
      <div className="flex items-start justify-between gap-4">
        <div>
          <Label
            htmlFor="permission-delete"
            className="leading-normal font-semibold"
          >
            Delete music without approval
          </Label>
          <Caption className="mt-1">
            Deletion removes shared music for everyone.
          </Caption>
        </div>
        <Switch
          id="permission-delete"
          checked={editor.value.directDelete}
          onCheckedChange={(directDelete) =>
            editor.update({ ...editor.value, directDelete })
          }
        />
      </div>
      {editor.error && <Text role="alert">{editor.error}</Text>}
      {editor.saving && <Caption role="status">Saving…</Caption>}
    </div>
  );
}
