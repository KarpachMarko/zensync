import { api, setCSRF } from "@/lib/api";
import type { User } from "@/lib/types";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type React from "react";
import { useState } from "react";
import { toast } from "sonner";
import { useApiMutation } from "./use-api";

type Session = { setup: boolean; user: User | null };
export function useSession() {
  const client = useQueryClient();
  const [error, setError] = useState("");
  const { api: mutate } = useApiMutation();
  const result = useQuery({
    queryKey: ["session"],
    queryFn: async (): Promise<Session> => {
      const first = await api<{ required: boolean }>("/setup");
      if (first.required) return { setup: true, user: null };
      try {
        const session = await api<{ user: User; csrf: string }>("/session");
        setCSRF(session.csrf);
        return { setup: false, user: session.user };
      } catch {
        return { setup: false, user: null };
      }
    },
    staleTime: Infinity,
  });
  async function load() {
    await result.refetch({ throwOnError: true });
  }
  async function login(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const credentials = Object.fromEntries(new FormData(event.currentTarget));
    setError("");
    toast.dismiss();
    try {
      if (result.data?.setup) await mutate("/setup", "POST", credentials);
      const session = await mutate<{ csrf: string }>(
        "/session",
        "POST",
        credentials,
      );
      setCSRF(session.csrf);
      await client.cancelQueries({
        predicate: (query) => query.queryKey[0] === "api",
      });
      client.removeQueries({
        predicate: (query) => query.queryKey[0] === "api",
      });
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Sign in failed");
    }
  }
  function signOut() {
    toast.dismiss();
    setCSRF("");
    void client.cancelQueries();
    client.removeQueries({ predicate: (query) => query.queryKey[0] === "api" });
    client.setQueryData<Session>(["session"], { setup: false, user: null });
  }
  return {
    setup: result.data?.setup ?? false,
    user: result.data?.user ?? null,
    loading: result.isPending,
    error: error || result.error?.message || "",
    setError,
    load,
    login,
    signOut,
  };
}
