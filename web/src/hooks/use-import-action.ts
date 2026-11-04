import { api } from "@/lib/api";
import type { Detail, Source } from "@/lib/migration";
import { useMutation, useQueryClient } from "@tanstack/react-query";

type ActionResponse = { source?: Source; id?: string; ok?: boolean; bulk?: { created: number; existing: number; skipped: number } };

export function useImportAction(id: string | undefined) {
  const client = useQueryClient();
  const queryKey = ["api", "/imports/" + id];
  const mutation = useMutation({
    // A poll started before the save must not overwrite the newly saved row.
    onMutate: () => client.cancelQueries({ queryKey, exact: true }),
    mutationFn: ({ path, body, key }: { path: string; body?: unknown; key?: string }) =>
      api<ActionResponse>("/imports/" + id + "/" + path, "POST", body, key),
    onSuccess: async (response) => {
      // A new poll may also have started while the action was being saved.
      // Cancellation aborts that read; it does not wait for another fetch.
      await client.cancelQueries({ queryKey, exact: true });
      if (response.source) {
        const source = response.source;
        client.setQueryData<Detail>(queryKey, (detail) => detail ? {
          ...detail,
          songs: detail.songs.map((song) => song.id === source.id ? source : song),
        } : detail);
      } else {
        // Lifecycle actions need a fresh header/playlist snapshot. Do not make
        // confirmation wait for that snapshot (or report its failure as a save failure).
        void client.invalidateQueries({ queryKey, exact: true });
      }
      for (const area of ["/requests", "/activity", "/imports"]) {
        void client.invalidateQueries({ queryKey: ["api", area], exact: true });
      }
    },
  });
  return {
    ...mutation,
    act: (path: string, body?: unknown, key?: string) => mutation.mutateAsync({ path, body, key }),
  };
}
