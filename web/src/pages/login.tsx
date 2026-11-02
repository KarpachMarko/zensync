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
    <div className="login">
      <div className="brand">◉ ZenSync</div>
      <h1>{setup ? "Welcome to ZenSync" : "Your music, together."}</h1>
      <p>
        {setup
          ? "Create the administrator account to start migrating your music."
          : "Sign in to your shared music workspace."}
      </p>
      <form onSubmit={onSubmit}>
        <label>
          Username
          <Input name="username" autoComplete="username" required />
        </label>
        <label>
          Password
          <Input
            name="password"
            type="password"
            autoComplete={setup ? "new-password" : "current-password"}
            minLength={setup ? 12 : undefined}
            maxLength={72}
            required
          />
        </label>
        {error && <p role="alert">{error}</p>}
        <Button>{setup ? "Create administrator" : "Sign in"}</Button>
      </form>
    </div>
  );
}
