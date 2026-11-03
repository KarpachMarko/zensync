import { useFieldContext } from "@/hooks/form-context";
import { cn } from "@/lib/utils";
import { useId, type ComponentProps, type ReactNode } from "react";
import { FieldLabel } from "./forms";
import { Text } from "./typography";
import { Checkbox } from "./ui/checkbox";
import { Input } from "./ui/input";

export function ValidationErrors({
  errors,
  id,
}: {
  errors: readonly unknown[];
  id?: string;
}) {
  const messages = errors.flatMap((error) =>
    typeof error === "string"
      ? [error]
      : typeof error === "object" &&
          error !== null &&
          "message" in error &&
          typeof error.message === "string"
        ? [error.message]
        : [],
  );
  return messages.length ? (
    <Text id={id} role="alert" className="m-0 text-xs">
      {[...new Set(messages)].join(" ")}
    </Text>
  ) : null;
}

type TextFieldProps = Omit<
  ComponentProps<typeof Input>,
  "value" | "defaultValue" | "onChange" | "onBlur" | "name"
> & { label: ReactNode; labelClassName?: string; hideLabel?: boolean };
export function TextField({
  label,
  labelClassName,
  hideLabel,
  ...props
}: TextFieldProps) {
  const field = useFieldContext<string>(),
    id = useId();
  const invalid = field.state.meta.isTouched && !field.state.meta.isValid;
  return (
    <FieldLabel htmlFor={id} className={labelClassName}>
      <span id={id + "-label"} className={cn(hideLabel && "sr-only")}>
        {label}
      </span>
      <Input
        {...props}
        id={id}
        name={field.name}
        aria-labelledby={id + "-label"}
        value={field.state.value}
        onChange={(event) => field.handleChange(event.target.value)}
        onBlur={field.handleBlur}
        aria-invalid={invalid}
        aria-describedby={invalid ? id + "-error" : undefined}
      />
      {invalid && (
        <ValidationErrors id={id + "-error"} errors={field.state.meta.errors} />
      )}
    </FieldLabel>
  );
}

export function FileField({
  label,
  ...props
}: Omit<
  ComponentProps<typeof Input>,
  "type" | "value" | "onChange" | "onBlur"
> & { label: ReactNode }) {
  const field = useFieldContext<File | null>(),
    id = useId();
  const invalid = field.state.meta.isTouched && !field.state.meta.isValid;
  return (
    <FieldLabel htmlFor={id}>
      <span id={id + "-label"}>{label}</span>
      <Input
        {...props}
        id={id}
        type="file"
        aria-labelledby={id + "-label"}
        name={field.name}
        onChange={(event) =>
          field.handleChange(event.target.files?.[0] ?? null)
        }
        onBlur={field.handleBlur}
        aria-invalid={invalid}
        aria-describedby={invalid ? id + "-error" : undefined}
      />
      {invalid && (
        <ValidationErrors id={id + "-error"} errors={field.state.meta.errors} />
      )}
    </FieldLabel>
  );
}

export function CheckboxField({
  label,
  disabled,
}: {
  label: ReactNode;
  disabled?: boolean;
}) {
  const field = useFieldContext<boolean>(),
    id = useId();
  return (
    <FieldLabel htmlFor={id} className="mt-[18px] flex-row items-center">
      <Checkbox
        id={id}
        name={field.name}
        disabled={disabled}
        checked={field.state.value}
        onCheckedChange={(value) => field.handleChange(value === true)}
        onBlur={field.handleBlur}
      />
      {label}
    </FieldLabel>
  );
}
