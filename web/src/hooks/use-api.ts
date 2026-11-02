import { api } from "@/lib/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

type MutationInput = {
  path: string;
  method: string;
  body?: unknown;
  key?: string;
};

export function useApiQuery<T>(
  path: string,
  options: { enabled?: boolean; refetchInterval?: number } = {},
) {
  const query = useQuery({
    queryKey: ["api", path],
    queryFn: () => api<T>(path),
    ...options,
  });
  return {
    ...query,
    reload: async () => {
      await query.refetch({ throwOnError: true });
    },
  };
}

// One mutation observer per owning feature. Response types follow the API contract,
// just as in the transport helper, while request bodies remain unknown at this boundary.
export function useApiMutation() {
  const client = useQueryClient();
  const mutation = useMutation({
    onSuccess: async (_data, input) => {
      const area = input.path.split("/")[1];
      if (["session", "setup"].includes(area)) return;
      await client.invalidateQueries({
        predicate: (query) =>
          query.queryKey[0] === "api" &&
          typeof query.queryKey[1] === "string" &&
          query.queryKey[1].startsWith("/" + area),
      });
    },
    mutationFn: ({ path, method, body, key }: MutationInput) =>
      api<unknown>(path, method, body, key),
  });
  async function mutate<T = unknown>(
    path: string,
    method: string,
    body?: unknown,
    key?: string,
  ): Promise<T> {
    return (await mutation.mutateAsync({ path, method, body, key })) as T;
  }
  return { ...mutation, api: mutate };
}
