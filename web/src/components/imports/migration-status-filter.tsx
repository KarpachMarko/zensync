import { CheckboxField } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";

export function MigrationStatusFilter({
  counts,
  hidden,
  onChange,
}: {
  counts: Map<string, number>;
  hidden: string[];
  onChange: (hidden: string[]) => void;
}) {
  const statuses = [...new Set([
    "Ready to request", "Review match", "Unresolved", "Waiting for scan",
    "Skipped for now", ...counts.keys(), "Available",
  ])].filter((value) => counts.has(value) || value === "Available" || hidden.includes(value));
  return (
    <fieldset className="m-0 min-w-0 rounded-md border border-solid border-border px-3 py-2">
      <legend className="px-1 text-xs text-muted-foreground">Show statuses</legend>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-3">
        {statuses.map((value) => (
          <CheckboxField key={value} className="m-0 gap-2 text-xs">
            <Checkbox
              checked={!hidden.includes(value)}
              onCheckedChange={(checked) => onChange(
                checked === true ? hidden.filter((status) => status !== value) : [...hidden, value],
              )}
            />
            {value} ({counts.get(value) ?? 0})
          </CheckboxField>
        ))}
      </div>
      <div className="mt-2 flex flex-wrap gap-2">
        <Button size="xs" variant="outline" onClick={() => onChange([])}>Show all</Button>
        <Button size="xs" variant="outline" onClick={() => onChange(["Available"])}>Hide available</Button>
      </div>
    </fieldset>
  );
}
