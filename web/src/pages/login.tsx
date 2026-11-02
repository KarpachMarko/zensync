import { FieldLabel } from "@/components/forms";
import { AuthLayout, Brand } from "@/components/layout/content";
import { PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import React from "react";
export function Login({
  setup,
  error,
  onSubmit,
}: {
  setup: boolean;
  error: string;
  onSubmit: React.FormEventHandler<HTMLFormElement>;
}) {
  return (
    <AuthLayout>
      <Brand>◉ ZenSync</Brand>
      <PageHeading className="mt-10">
        {setup ? "Welcome to ZenSync" : "Your music, together."}
      </PageHeading>
      <Text>
        {setup
          ? "Create the administrator account to start migrating your music."
          : "Sign in to your shared music workspace."}
      </Text>
      <form className="mt-[30px]" onSubmit={onSubmit}>
        <FieldLabel>
          Username
          <Input name="username" autoComplete="username" required />
        </FieldLabel>
        <FieldLabel>
          Password
          <Input
            name="password"
            type="password"
            autoComplete={setup ? "new-password" : "current-password"}
            minLength={setup ? 12 : undefined}
            maxLength={72}
            required
          />
        </FieldLabel>
        {error && <Text role="alert">{error}</Text>}
        <Button className="w-full">
          {setup ? "Create administrator" : "Sign in"}
        </Button>
      </form>
    </AuthLayout>
  );
}
