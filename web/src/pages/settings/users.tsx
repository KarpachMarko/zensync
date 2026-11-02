import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApiMutation, useApiQuery } from "@/hooks/use-api";
import type { User } from "@/lib/types";
import { useQueryClient } from "@tanstack/react-query";
import React, { useState } from "react";
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
      setMessage("User created.");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      {result.error && <p role="alert">{result.error.message}</p>}
      {message && <p role="status">{message}</p>}
      <h2>Users & permissions</h2>
      {users.map((u) => (
        <div className="row" key={u.id}>
          <strong>
            {u.username}
            <small>{u.role}</small>
          </strong>
          <div>
            {u.role === "admin" ? (
              <span>Administrator</span>
            ) : (
              <label className="check permission-toggle">
                <input
                  type="checkbox"
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
                      setMessage("Permissions saved for " + u.username + ".");
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
              </label>
            )}
            {u.role !== "admin" && (
              <label className="check">
                <input
                  type="checkbox"
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
                      setMessage("Deletion permission updated.");
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
              </label>
            )}
          </div>
        </div>
      ))}
      <h2>Add a user</h2>
      <form onSubmit={add}>
        <div className="fields">
          <label>
            Username
            <Input name="username" autoComplete="off" maxLength={80} required />
          </label>
          <label>
            Initial password
            <Input
              name="password"
              type="password"
              autoComplete="new-password"
              minLength={12}
              maxLength={72}
              required
            />
          </label>
        </div>
        <label className="check">
          <input name="direct" type="checkbox" /> Allow adding music without
          approval
        </label>
        <label className="check">
          <input name="directDelete" type="checkbox" /> Allow deleting music
          without approval
        </label>
        <p className="muted">
          Adding and deleting have separate permissions. Other deletion requests
          require admin approval. Upgrades still require admin approval.
        </p>
        <Button disabled={busy}>Create user</Button>
      </form>
    </>
  );
}
