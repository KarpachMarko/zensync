import { Row } from "@/components/layout/content";
import { InviteDialog } from "@/components/settings/invite-dialog";
import { PermissionsDialog } from "@/components/settings/permissions-dialog";
import { Caption, SectionHeading, Text } from "@/components/typography";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import type { User } from "@/lib/types";
import { Settings2Icon, UserPlusIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

type Invitation = {
  id: string;
  username: string;
  expires: number;
  created: string;
};
export function Users() {
  const result = useApiQuery<User[]>("/users", { refetchInterval: 10000 });
  const invitations = useApiQuery<Invitation[]>("/invitations", {
    refetchInterval: 10000,
  });
  const { api, isPending } = useApiMutation();
  const [selected, setSelected] = useState<User>();
  const [inviting, setInviting] = useState(false);
  return (
    <div className="max-w-[850px]">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <SectionHeading>Users & permissions</SectionHeading>
        <Button
          className="flex items-center gap-2"
          onClick={() => setInviting(true)}
        >
          <UserPlusIcon className="size-4" />
          Invite user
        </Button>
      </div>
      <Text>
        Manage access to your shared music workspace. Members join through a
        single-use invitation.
      </Text>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {result.data?.map((user) => (
        <Row key={user.id} className="max-[700px]:flex-wrap">
          <div>
            <strong>{user.username}</strong>
            <Caption>
              {user.role === "admin"
                ? "Full workspace access"
                : user.direct || user.directDelete
                  ? "Custom approval permissions"
                  : "Administrator approval required"}
            </Caption>
          </div>
          {user.role === "admin" ? (
            <Badge variant="secondary">Administrator</Badge>
          ) : (
            <Button
              variant="outline"
              className="flex items-center gap-2"
              aria-label={`Manage permissions for ${user.username}`}
              onClick={() => setSelected(user)}
            >
              <Settings2Icon className="size-4" />
              Permissions
            </Button>
          )}
        </Row>
      ))}
      {!!invitations.data?.length && (
        <SectionHeading>Pending invitations</SectionHeading>
      )}
      {invitations.error && (
        <Text role="alert">{invitations.error.message}</Text>
      )}
      {invitations.data?.map((invitation) => (
        <Row key={invitation.id}>
          <div>
            <strong>{invitation.username}</strong>
            <Caption>
              Expires {new Date(invitation.expires * 1000).toLocaleDateString()}
            </Caption>
          </div>
          <Button
            variant="outline"
            disabled={isPending}
            aria-label={`Revoke invitation for ${invitation.username}`}
            onClick={async () => {
              try {
                await api(`/invitations/${invitation.id}`, "DELETE");
                toast.success("Invitation revoked.");
              } catch {
                /* Mutation errors are announced by the shared hook. */
              }
            }}
          >
            Revoke invitation
          </Button>
        </Row>
      ))}
      {selected && (
        <PermissionsDialog
          user={selected}
          onClose={() => setSelected(undefined)}
        />
      )}
      {inviting && <InviteDialog onClose={() => setInviting(false)} />}
    </div>
  );
}
