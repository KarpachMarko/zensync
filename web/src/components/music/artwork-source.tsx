import { z } from "zod";
import { useEffect } from "react";
import { useAppForm } from "@/hooks/use-app-form";
import { Button } from "@/components/ui/button";
const schema = z.object({ query: z.string().trim().max(2000).superRefine((value, context) => {
  if (/^[a-z][\w+.-]*:\/\//i.test(value)) {
    try { const url = new URL(value); if (url.protocol !== "https:" || url.username || url.password) throw new Error(); }
    catch { context.addIssue({ code: "custom", message: "Use an HTTPS image URL." }); }
  } else if (value.length > 500) context.addIssue({ code: "custom", message: "Keep the artwork search under 500 characters." });
}) });
export function ArtworkSource({ value, onSubmit }: { value: string; onSubmit: (value: string, isURL: boolean) => void }) {
  const form = useAppForm({ defaultValues: { query: value }, validators: { onChange: schema, onSubmit: schema.refine(({ query }) => query.length > 0, { path: ["query"], message: "Enter an artwork search or image URL." }) },
    onSubmit: ({ value }) => onSubmit(value.query.trim(), /^https:\/\//i.test(value.query.trim())) });
  useEffect(() => { form.setFieldValue("query", value); }, [value]);
  return <form className="flex min-w-0 items-end gap-2" noValidate onSubmit={(event) => { event.preventDefault(); event.stopPropagation(); void form.handleSubmit(); }}>
    <div className="min-w-0 flex-1"><form.AppField name="query">{(field) => <field.TextField label="Search artwork or image URL" labelClassName="m-0 gap-1 text-xs" className="h-9 min-h-9 text-sm" placeholder="Artist and album, or https://…" maxLength={2000} />}</form.AppField></div>
    <form.Subscribe selector={(state) => state.values.query}>{(query) => <Button type="submit" size="sm" className="mb-0.5 shrink-0">{/^https:\/\//i.test(query.trim()) ? "Use URL" : "Search"}</Button>}</form.Subscribe>
  </form>;
}
