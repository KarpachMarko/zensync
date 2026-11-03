import { Checkbox, CheckboxField, FieldLabel } from "@/components/forms";
import { FieldGrid, Row } from "@/components/layout/content";
import { Caption, SectionHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import type { User } from "@/lib/types";
import { useQueryClient } from "@tanstack/react-query";
import React, { useState } from "react";
import { toast } from "sonner";
export function Users({
  user,
  reload,
}: {
  user: User;
  reload: () => Promise<void>;
}) {
  const { api } = useApiMutation();
  const [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false);
  const result = useApiQuery<User[]>("/users", {
    enabled: user.role === "admin",
  });
  const users = result.data ?? [];
  const loadUsers = result.reload;
  const client = useQueryClient();
  function setUsers(update: (users: User[]) => User[]) {
    client.setQueryData<User[]>(["api", "/users"], (current) =>
      update(current ?? []),
    );
  }
  async function add(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget,
      f = new FormData(form);
    setBusy(true);
    try {
      await api("/users", "POST", {
        username: f.get("username"),
        password: f.get("password"),
        direct: f.get("direct") === "on",
        directDelete: f.get("directDelete") === "on",
      });
      form.reset();
      await loadUsers();
      setMessage("");
      toast.success("User created.");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {message && <Text role="alert">{message}</Text>}
      <SectionHeading>Users & permissions</SectionHeading>
      {users.map((u) => (
        <Row className="max-[700px]:flex-wrap max-[700px]:gap-2" key={u.id}>
          <strong>
            {u.username}
            <Caption>{u.role}</Caption>
          </strong>
          <div>
            {u.role === "admin" ? (
              <span>Administrator</span>
            ) : (
              <CheckboxField className="m-0 text-[13px]">
                <Checkbox
                  checked={u.direct}
                  disabled={busy}
                  onChange={async (e) => {
                    const direct = e.target.checked;
                    setUsers((users) =>
                      users.map((current) =>
                        current.id === u.id ? { ...current, direct } : current,
                      ),
                    );
                    setBusy(true);
                    try {
                      await api("/users/" + u.id, "PUT", { direct });
                      await loadUsers();
                      setMessage("");
                      toast.success(
                        "Permissions saved for " + u.username + ".",
                        { id: "permissions-" + u.id },
                      );
                    } catch (e) {
                      setUsers((users) =>
                        users.map((current) =>
                          current.id === u.id ? u : current,
                        ),
                      );
                      setMessage((e as Error).message);
                    } finally {
                      setBusy(false);
                    }
                  }}
                />
                Allow adding music without approval
              </CheckboxField>
            )}
            {u.role !== "admin" && (
              <CheckboxField>
                <Checkbox
                  checked={u.directDelete}
                  disabled={busy}
                  onChange={async (e) => {
                    const directDelete = e.target.checked;
                    setUsers((current) =>
                      current.map((person) =>
                        person.id === u.id
                          ? { ...person, directDelete }
                          : person,
                      ),
                    );
                    setBusy(true);
                    try {
                      await api(`/users/${u.id}/permissions`, "PUT", {
                        directDelete,
                      });
                      await loadUsers();
                      if (u.id === user.id) await reload();
                      setMessage("");
                      toast.success("Deletion permission updated.", {
                        id: "permissions-" + u.id,
                      });
                    } catch (err) {
                      setUsers((current) =>
                        current.map((person) =>
                          person.id === u.id
                            ? { ...person, directDelete: !directDelete }
                            : person,
                        ),
                      );
                      setMessage((err as Error).message);
                    } finally {
                      setBusy(false);
                    }
                  }}
                />
                Allow deleting music without approval
              </CheckboxField>
            )}
          </div>
        </Row>
      ))}
      <SectionHeading>Add a user</SectionHeading>
      <form onSubmit={add}>
        <FieldGrid>
          <FieldLabel>
            Username
            <Input name="username" autoComplete="off" maxLength={80} required />
          </FieldLabel>
          <FieldLabel>
            Initial password
            <Input
              name="password"
              type="password"
              autoComplete="new-password"
              minLength={12}
              maxLength={72}
              required
            />
          </FieldLabel>
        </FieldGrid>
        <CheckboxField>
          <Checkbox name="direct" /> Allow adding music without approval
        </CheckboxField>
        <CheckboxField>
          <Checkbox name="directDelete" /> Allow deleting music without approval
        </CheckboxField>
        <Text className="text-[13px]">
          Adding and deleting have separate permissions. Other deletion requests
          require admin approval. Upgrades still require admin approval.
        </Text>
        <Button disabled={busy}>Create user</Button>
      </form>
    </>
  );
}
