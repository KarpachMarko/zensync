import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { EllipsisIcon } from "lucide-react";
import { useRef, type Ref } from "react";

export type RowAction = { label: string; onSelect: () => void; disabled?: boolean; opensDialog?: boolean; description?: string };

export function RowActions({ title, actions, triggerRef }: { title: string; actions: RowAction[]; triggerRef?: Ref<HTMLButtonElement> }) {
  const openingDialog = useRef(false);
  return <DropdownMenu onOpenChange={(open) => { if (open) openingDialog.current = false; }}>
    <DropdownMenuTrigger asChild>
      <Button ref={triggerRef} variant="ghost" size="icon-sm" className="inline-flex shrink-0 items-center justify-center max-sm:size-11" aria-label={`Actions for ${title}`}>
        <EllipsisIcon className="size-4" aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" className="w-64" onCloseAutoFocus={(event) => { if (openingDialog.current) event.preventDefault(); }}>
      {actions.map((action) => <DropdownMenuItem key={action.label} aria-label={action.label} aria-description={action.description} disabled={action.disabled} className="min-h-8 text-xs max-sm:min-h-11" onSelect={() => { openingDialog.current = !!action.opensDialog; action.onSelect(); }}><span className="flex flex-col gap-0.5"><span>{action.label}</span>{action.description && <span className="text-[11px] leading-4 text-muted-foreground">{action.description}</span>}</span></DropdownMenuItem>)}
    </DropdownMenuContent>
  </DropdownMenu>;
}
