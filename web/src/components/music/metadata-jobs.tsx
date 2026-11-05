import { api } from "@/lib/api";
import type { MetadataJob } from "@/lib/metadata";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/typography";
export function MetadataJobs() {
  const client = useQueryClient();
  const result = useQuery({
    queryKey: ["api", "/metadata/jobs"],
    queryFn: ({ signal }) =>
      api<MetadataJob[]>("/metadata/jobs", "GET", undefined, undefined, signal),
    refetchInterval: (query) =>
      query.state.data?.some(
        (j) => j.state === "queued" || j.state === "running",
      )
        ? 2000
        : false,
  });
  const retry = useMutation({
    mutationFn: (id: string) => api("/metadata/jobs", "POST", { id }),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["api", "/metadata/jobs"] }),
  });
  return (
    <>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {retry.error && <Text role="alert">{retry.error.message}</Text>}
      {result.data?.slice(0, 3).map((job) => (
        <div
          key={job.id}
          className="my-2 flex items-start justify-between gap-2 rounded border border-border px-2 py-1.5 text-xs"
          role={job.state === "failed" ? "alert" : "status"}
        >
          <span>
            <strong>
              {job.state} · {job.done}/{job.total}
            </strong>{" "}
            {job.message}
          </span>
          {job.state === "failed" && (
            <Button
              size="sm"
              variant="outline"
              disabled={retry.isPending}
              onClick={() => retry.mutate(job.id)}
            >
              Retry update
            </Button>
          )}
        </div>
      ))}
    </>
  );
}
