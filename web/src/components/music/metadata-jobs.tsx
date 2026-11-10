import { api } from "@/lib/api";
import type { MetadataJob } from "@/lib/metadata";
import { sortExecutionRows } from "@/lib/execution-state";
import { useQuery } from "@tanstack/react-query";
import { ActivityRow } from "@/components/requests/activity-row";
import { AppLink } from "@/components/navigation";
import { Text } from "@/components/typography";

const noAction = () => undefined;

export function MetadataJobs() {
  const result = useQuery({
    queryKey: ["api", "/metadata/jobs"],
    queryFn: ({ signal }) => api<MetadataJob[]>("/metadata/jobs", "GET", undefined, undefined, signal),
    refetchInterval: (query) => query.state.data?.some((job) => job.state === "queued" || job.state === "running") ? 2000 : false,
  });
  const active = sortExecutionRows(
    (result.data ?? []).filter((job) => job.state === "queued" || job.state === "running"),
    (job) => job.state,
  ).slice(0, 3);
  return (
    <>
      {result.error && <Text role="alert">{result.error.message}</Text>}
      {active.length > 0 && (
        <section aria-label="Active library updates" className="my-2">
          <div className="flex items-center justify-between gap-2 text-xs">
            <strong>Library updates</strong>
            <AppLink to="/activity" className="underline">View all activity</AppLink>
          </div>
          {active.map((job) => {
            const progress = `${job.done}/${job.total}`;
            return <ActivityRow key={job.id} busy={false} item={{
              id: job.id, kind: "metadata", title: job.title ?? "Metadata update",
              state: job.state,
              stage: job.message.startsWith(progress) ? job.message : `${progress} · ${job.message}`,
            }} onRetry={noAction} onDismiss={noAction} onCancel={noAction} />;
          })}
        </section>
      )}
    </>
  );
}
