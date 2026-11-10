import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";

export function UpdateNotice() {
  const current = document.querySelector<HTMLScriptElement>('script[type="module"][src]')?.src;
  const result = useQuery({
    queryKey: ["web", "deployment", current],
    enabled: !!current && new URL(current).pathname.startsWith("/assets/"),
    queryFn: async ({ signal }) => {
      const response = await fetch("/", { signal, cache: "no-store", headers: { Accept: "text/html" } });
      if (!response.ok || !response.headers.get("Content-Type")?.includes("text/html")) return false;
      const html = new DOMParser().parseFromString(await response.text(), "text/html");
      const next = html.querySelector<HTMLScriptElement>('script[type="module"][src]')?.getAttribute("src");
      return !!next && !!current && new URL(next, window.location.origin).pathname !== new URL(current).pathname;
    },
    staleTime: 60000,
    refetchInterval: 60000,
    refetchOnWindowFocus: true,
    retry: false,
  });
  return result.data ? (
    <div role="status" className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded border border-solid border-primary/40 bg-muted p-3 text-sm">
      <span>An application update is available. Finish your edits, then reload to use the new features.</span>
      <Button size="sm" onClick={() => window.location.reload()}>Reload application</Button>
    </div>
  ) : null;
}
