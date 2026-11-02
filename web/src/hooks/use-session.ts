import { api, setCSRF } from "@/lib/api";
import type { User } from "@/lib/types";
import type React from "react";
import { useEffect, useState } from "react";

export function useSession() {
  const [setup, setSetup] = useState(false);
  const [user, setUser] = useState<User | null>(null),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  async function load() {
    try {
      const first = await api<{ required: boolean }>("/setup");
      setSetup(first.required);
      if (first.required) {
        setUser(null);
        return;
      }
      const s = await api<{ user: User; csrf: string }>("/session");
      setCSRF(s.csrf);
      setUser(s.user);
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    void load();
  }, []);
  async function login(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    try {
      if (setup) await api("/setup", "POST", Object.fromEntries(f));
      const s = await api<{ csrf: string }>(
        "/session",
        "POST",
        Object.fromEntries(f),
      );
      setCSRF(s.csrf);
      await load();
    } catch (e) {
      setError(String((e as Error).message));
    }
  }
  return {
    setup,
    user,
    loading,
    error,
    setError,
    load,
    login,
    signOut: () => setUser(null),
  };
}
