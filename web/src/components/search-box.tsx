import { useAppForm } from "@/hooks/use-app-form";
import { searchSchema } from "@/lib/form-schemas";
import { useEffect, type Ref } from "react";
import { Icon } from "./icons";
import { SearchForm } from "./layout/content";
import { Button } from "./ui/button";

type SearchBoxProps = {
  value?: string;
  onSearch: (query: string) => void | Promise<void>;
  label: string;
  placeholder?: string;
  buttonLabel?: string;
  disabled?: boolean;
  compact?: boolean;
  clearOnSubmit?: boolean;
  inputRef?: Ref<HTMLInputElement>;
};
export function SearchBox({
  value = "",
  onSearch,
  label,
  placeholder,
  buttonLabel = "Search",
  disabled,
  compact = false,
  clearOnSubmit = false,
  inputRef,
}: SearchBoxProps) {
  const form = useAppForm({
    defaultValues: { query: value },
    validators: { onSubmit: searchSchema },
    onSubmit: async ({ value }) => {
      await onSearch(value.query.trim());
      if (clearOnSubmit) form.reset({ query: "" });
    },
  });
  useEffect(() => {
    form.setFieldValue("query", value);
  }, [value]);
  const fields = (
    <form.AppField name="query">
      {(field) => (
        <field.TextField
          label={label}
          hideLabel={compact}
          ref={inputRef}
          labelClassName={
            compact
              ? "m-0 min-w-0 flex-1"
              : "m-0 min-w-0 flex-1 max-[700px]:basis-full"
          }
          className={
            compact
              ? "h-7 min-h-0 max-[700px]:min-h-0 border-0 bg-transparent p-0 text-[13px] focus-visible:outline-none"
              : undefined
          }
          placeholder={placeholder}
          maxLength={500}
        />
      )}
    </form.AppField>
  );
  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    event.stopPropagation();
    void form.handleSubmit();
  }
  return compact ? (
    <form
      role="search"
      className="flex h-[34px] min-w-0 items-center gap-2 rounded-[5px] border border-solid border-border px-2 text-muted-foreground focus-within:outline-2 focus-within:outline-solid focus-within:outline-primary"
      noValidate
      onSubmit={submit}
    >
      <Icon name="search" />
      {fields}
    </form>
  ) : (
    <SearchForm noValidate onSubmit={submit}>
      {fields}
      <Button disabled={disabled}>{buttonLabel}</Button>
    </SearchForm>
  );
}
