import { AuthLayout, Brand } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import { useAppForm } from "@/hooks/use-app-form";
import { acceptInviteSchema } from "@/lib/form-schemas";
import { parseAsString, useQueryState } from "nuqs";
import { useState } from "react";

type InviteUser = { username: string; expires: number };
export function AcceptInvite({
  signedIn,
  onSignOut,
}: {
  signedIn: boolean;
  onSignOut: () => void;
}) {
  const [token, setToken] = useQueryState(
    "token",
    parseAsString.withDefault(""),
  );
  const preview = useApiQuery<InviteUser>(
    "/invitations/preview?token=" + encodeURIComponent(token),
    { enabled: !!token },
  );
  const { api, isPending } = useApiMutation();
  const [complete, setComplete] = useState("");
  const [error, setError] = useState("");
  const form = useAppForm({
    defaultValues: { password: "" },
    validators: { onSubmit: acceptInviteSchema },
    onSubmit: async ({ value }) => {
      setError("");
      try {
        const accepted = await api<{ username: string }>(
          "/invitations/accept",
          "POST",
          { token, password: value.password },
        );
        form.reset();
        setComplete(accepted.username);
        void setToken(null, { history: "replace" });
      } catch (cause) {
        setError(
          cause instanceof Error
            ? cause.message
            : "Unable to accept invitation",
        );
      }
    },
  });
  return (
    <AuthLayout>
      <Brand />
      <PageHeading className="mt-10">
        {complete ? "Your account is ready" : "Join ZenSync"}
      </PageHeading>
      {complete ? (
        <>
          <Text>Sign in as {complete} with the password you just chose.</Text>
          <Button asChild>
            <AppLink to="/">Sign in</AppLink>
          </Button>
        </>
      ) : signedIn ? (
        <>
          <Text>
            Sign out of your current account before accepting this invitation.
          </Text>
          <Button
            onClick={async () => {
              try {
                await api("/session", "DELETE");
                onSignOut();
              } catch {
                /* Shared mutation hook reports the error. */
              }
            }}
          >
            Sign out
          </Button>
        </>
      ) : !token || preview.error ? (
        <Text role="alert">
          This invitation is invalid, expired, or already used. Ask an
          administrator for a new link.
        </Text>
      ) : !preview.data ? (
        <Text>Checking invitation…</Text>
      ) : (
        <>
          <Text>
            Create a password for <strong>{preview.data.username}</strong>.
          </Text>
          <form
            noValidate
            onSubmit={(event) => {
              event.preventDefault();
              void form.handleSubmit();
            }}
          >
            <form.AppField name="password">
              {(field) => (
                <field.TextField
                  label="Your password"
                  type="password"
                  autoComplete="new-password"
                />
              )}
            </form.AppField>
            {error && <Text role="alert">{error}</Text>}
            <Button disabled={isPending}>Accept invitation</Button>
          </form>
        </>
      )}
    </AuthLayout>
  );
}
