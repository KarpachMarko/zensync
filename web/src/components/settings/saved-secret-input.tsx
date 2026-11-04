import { useState, type ComponentProps } from "react";
import { Input } from "@/components/ui/input";

type SavedSecretInputProps = Omit<
  ComponentProps<typeof Input>,
  "type" | "value"
> & {
  value: string;
  saved: boolean;
};

// The mask is presentation only. Focus exposes the empty replacement draft;
// no saved value or mask ever enters the form or an autosave payload.
export function SavedSecretInput({
  saved,
  value,
  onFocus,
  onBlur,
  ...props
}: SavedSecretInputProps) {
  const [editing, setEditing] = useState(false);
  const masked = saved && !value && !editing;
  return (
    <Input
      {...props}
      type="password"
      value={masked ? "••••••••" : value}
      data-credential-saved={saved || undefined}
      onFocus={(event) => {
        setEditing(true);
        onFocus?.(event);
      }}
      onBlur={(event) => {
        setEditing(false);
        onBlur?.(event);
      }}
    />
  );
}
