import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/typography";

type DismissalRule = { kind: "credit"; name: string } | { kind: "song" | "album" | "artist"; ids: string[] };

export function DismissSuggestion({ rule }: { rule: DismissalRule }) {
  const client = useQueryClient();
  const refresh = () => client.invalidateQueries({ queryKey: ["api", "/metadata/duplicates"] });
  const dismiss = useMutation({
    mutationFn: (restore: boolean) => api("/metadata/dismissals", "POST", restore ? { ...rule, restore: true } : rule),
    onSuccess: async (_data, restore) => {
      if (!restore) toast.info(rule.kind === "credit" ? "Marked as a single artist" : "Duplicate suggestion dismissed", {
        action: { label: "Undo", onClick: () => dismiss.mutate(true) },
      });
      await refresh();
    },
    onError: (error, restore) => { if (restore) toast.error(error.message); },
  });
  return <div className="inline-flex flex-col items-start gap-1">
    <Button type="button" variant="outline" size="sm" disabled={dismiss.isPending} onClick={() => dismiss.mutate(false)}>
      {rule.kind === "credit" ? "Keep as one artist" : "Not duplicates"}
    </Button>
    {dismiss.error && <Text role="alert" className="m-0 text-xs">{dismiss.error.message}</Text>}
  </div>;
}
