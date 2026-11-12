import { useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/typography";

export function AutoLoadMore({ hasMore, loading, error, loadMore }: {
  hasMore: boolean; loading: boolean; error: boolean; loadMore: () => Promise<unknown>;
}) {
  const sentinel = useRef<HTMLDivElement>(null);
  const pending = useRef(false);
  useEffect(() => {
    const element = sentinel.current;
    if (!element || !hasMore || loading || error) return;
    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting) || pending.current) return;
      pending.current = true;
      void loadMore().catch(() => undefined).finally(() => { pending.current = false; });
    }, { rootMargin: "200px" });
    observer.observe(element);
    return () => observer.disconnect();
  }, [hasMore, loading, error, loadMore]);
  if (!hasMore) return null;
  return <div ref={sentinel} className="min-h-1 py-2">
    {loading && <Text role="status">Loading more…</Text>}
    {error && <Button variant="outline" disabled={loading} onClick={() => void loadMore()}>Retry loading</Button>}
  </div>;
}
