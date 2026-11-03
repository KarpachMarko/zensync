import { Badge } from "@/components/ui/badge";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import { ChevronRightIcon } from "lucide-react";
import { useState, type ReactNode } from "react";

export function ExpandableSection({
  title,
  count,
  defaultOpen = true,
  compact = false,
  children,
}: {
  title: ReactNode;
  count?: number;
  defaultOpen?: boolean;
  compact?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <Collapsible
      data-slot="status-section"
      className={cn(!compact && "mt-6")}
      open={open}
      onOpenChange={setOpen}
    >
      {!compact && <Separator />}
      <CollapsibleTrigger
        className={cn(
          "flex w-full items-center gap-1.5 border-0 [font-family:inherit] bg-transparent px-0 text-left text-foreground cursor-pointer focus-visible:outline-3 focus-visible:outline-solid focus-visible:outline-primary focus-visible:outline-offset-3",
          compact ? "py-1.5 text-base" : "py-4 text-lg font-semibold",
        )}
      >
        <ChevronRightIcon
          aria-hidden="true"
          className={cn(
            "size-4 shrink-0 transition-transform motion-reduce:transition-none",
            open && "rotate-90",
          )}
        />
        {title}
        {count !== undefined && (
          <Badge
            variant="ghost"
            className="ml-1 h-auto border-0 p-0 text-sm font-normal text-muted-foreground"
          >
            {count}
          </Badge>
        )}
      </CollapsibleTrigger>
      <CollapsibleContent>{children}</CollapsibleContent>
    </Collapsible>
  );
}
