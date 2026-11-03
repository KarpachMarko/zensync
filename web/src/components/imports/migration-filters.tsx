import { MigrationStatusFilter } from "@/components/imports/migration-status-filter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { InfoIcon } from "lucide-react";
import { useId } from "react";

export function MigrationFilters({
  counts, hidden, onStatusesChange, query, onQueryChange, shown, total,
  selected, busy, onRequest,
}: {
  counts: Map<string, number>;
  hidden: string[];
  onStatusesChange: (hidden: string[]) => void;
  query: string;
  onQueryChange: (query: string) => void;
  shown: number;
  total: number;
  selected: number;
  busy: boolean;
  onRequest: () => void;
}) {
  const inputId = useId();
  return (
    <section aria-label="Song filters" className="flex flex-col gap-3 rounded-lg border border-solid border-border p-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1.5 max-[600px]:basis-full">
          <div className="flex items-center gap-1.5">
            <Label htmlFor={inputId} className="text-sm">Filter songs</Label>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button type="button" variant="ghost" size="icon-xs" aria-label="About migration statuses" className="inline-flex items-center justify-center border-0">
                  <InfoIcon className="size-3.5" aria-hidden="true" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top" align="start" sideOffset={6} className="block w-[min(360px,calc(100vw-2rem))] max-w-none p-3 leading-5">
                <p className="m-0 mb-2">Matching scans every imported song. Available songs are already in Navidrome. For Ready to request, select songs and use Request selected.</p>
                <p className="m-0 mb-2">Review match needs you to compare recordings. For Unresolved, use Find recording or paste a Deezer song URL. Downloads require a request and any required approval.</p>
                <p className="m-0 mb-2">Publish available songs to create or update your selected Navidrome playlists.</p>
                <p className="m-0">Skip for now pauses new acquisition and keeps the playlist entry. Resume when ready. Skipping does not cancel a running download.</p>
              </TooltipContent>
            </Tooltip>
          </div>
          <Input id={inputId} value={query} onChange={(event) => onQueryChange(event.target.value)} placeholder="Title, artist or status" />
        </div>
        <Button disabled={busy || selected === 0} onClick={onRequest}>Request selected ({selected})</Button>
      </div>
      <MigrationStatusFilter counts={counts} hidden={hidden} onChange={onStatusesChange} className="border-0 p-0" />
      <p className="m-0 text-xs text-muted-foreground" role="status">
        Showing {shown} of {total} songs.
        {hidden.includes("Available") && (counts.get("Available") ?? 0) > 0 ? " Available songs are hidden." : ""}
      </p>
    </section>
  );
}
