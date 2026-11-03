import { AuthLayout, Brand } from "@/components/layout/content";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useAppForm } from "@/hooks/use-app-form";
import { credentialsSchema, type CredentialsInput } from "@/lib/form-schemas";

export function Login({
  setup,
  error,
  onSubmit,
}: {
  setup: boolean;
  error: string;
  onSubmit: (credentials: CredentialsInput) => Promise<void>;
}) {
  const form = useAppForm({
    defaultValues: { username: "", password: "" },
    validators: { onSubmit: credentialsSchema(setup) },
    onSubmit: async ({ value }) =>
      onSubmit({ ...value, username: value.username.trim() }),
  });
  return (
    <AuthLayout>
      <Brand />
      <PageHeading className="mt-10">
        {setup ? "Welcome to ZenSync" : "Your music, together."}
      </PageHeading>
      <Text>
        {setup
          ? "Create the administrator account to start migrating your music."
          : "Sign in to your shared music workspace."}
      </Text>
      <form
        className="mt-[30px]"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void form.handleSubmit();
        }}
      >
        <form.AppField name="username">
          {(field) => (
            <field.TextField label="Username" autoComplete="username" />
          )}
        </form.AppField>
        <form.AppField name="password">
          {(field) => (
            <field.TextField
              label="Password"
              type="password"
              autoComplete={setup ? "new-password" : "current-password"}
            />
          )}
        </form.AppField>
        {error && <Text role="alert">{error}</Text>}
        <form.Subscribe selector={(state) => state.isSubmitting}>
          {(submitting) => (
            <Button className="w-full" disabled={submitting}>
              {setup ? "Create administrator" : "Sign in"}
            </Button>
          )}
        </form.Subscribe>
      </form>
    </AuthLayout>
  );
}
