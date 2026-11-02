import { useState, type ReactNode } from "react";

export function ExpandableSection({
  title,
  count,
  defaultOpen = true,
  children,
}: {
  title: string;
  count: number;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <details
      data-slot="status-section"
      className="mt-6 border-t border-solid border-t-border"
      open={open}
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary className="py-4 px-0 cursor-pointer text-lg font-semibold focus-visible:outline-3 focus-visible:outline-solid focus-visible:outline-primary focus-visible:outline-offset-3">
        {title}
        <span className="ml-2.5 text-sm font-normal text-muted-foreground">
          {count}
        </span>
      </summary>
      {children}
    </details>
  );
}
