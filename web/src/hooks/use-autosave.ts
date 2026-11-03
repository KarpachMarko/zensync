import { useForm, useStore, type DeepValue } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import type { z } from "zod";

type AutosaveOptions<T> = {
  initialValue: T;
  save: (value: T) => Promise<void>;
  schema: z.ZodType<T>;
  normalizeSaved?: (value: T) => T;
  rollbackOnError?: boolean;
  delay?: number;
  message: string;
  toastId: string;
};

// Debounce text edits and serialize writes. Newer edits never get overwritten by
// an older response, and a pending valid edit is flushed when its editor unmounts.
export function useAutosave<T>(options: AutosaveOptions<T>) {
  const client = useQueryClient();
  const currentUser = () =>
    client.getQueryData<{ user: { id: string } | null }>(["session"])?.user?.id;
  const owner = useRef(currentUser());
  const form = useForm({
    defaultValues: { value: options.initialValue },
    validators: {
      onChange: ({ value }) => {
        const parsed = options.schema.safeParse(value.value);
        return parsed.success
          ? undefined
          : parsed.error.issues.map((issue) => issue.message).join(" ");
      },
    },
  });
  const value = useStore(form.store, (state) => state.values.value);
  function setValue(next: T) {
    // This fixed path holds T; the recursive library path type cannot reduce an unconstrained generic.
    form.setFieldValue("value", next as DeepValue<{ value: T }, "value">);
  }
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const config = useRef(options);
  config.current = options;
  const confirmed = useRef(options.initialValue);
  const pending = useRef<{ value: T; revision: number } | undefined>(undefined);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const busy = useRef(false);
  const mounted = useRef(true);
  const revision = useRef(0);

  async function drain(): Promise<void> {
    if (currentUser() !== owner.current) {
      pending.current = undefined;
      return;
    }
    if (busy.current || !pending.current) return;
    const next = pending.current;
    pending.current = undefined;
    busy.current = true;
    if (mounted.current) setSaving(true);
    try {
      await config.current.save(next.value);
      confirmed.current =
        config.current.normalizeSaved?.(next.value) ?? next.value;
      if (
        next.revision === revision.current &&
        currentUser() === owner.current
      ) {
        if (mounted.current) {
          setValue(confirmed.current);
          setError("");
        }
        toast.success(config.current.message, { id: config.current.toastId });
      }
    } catch (cause) {
      if (mounted.current && next.revision === revision.current) {
        setError(
          cause instanceof Error ? cause.message : "Unable to save changes",
        );
        if (config.current.rollbackOnError !== false)
          setValue(confirmed.current);
      }
    } finally {
      busy.current = false;
      if (mounted.current) setSaving(false);
      if (pending.current) await drain();
    }
  }

  function flush() {
    clearTimeout(timer.current);
    void drain();
  }
  function update(next: T) {
    revision.current++;
    setValue(next);
    setError("");
    clearTimeout(timer.current);
    pending.current = undefined;
    if (!config.current.schema.safeParse(next).success) return;
    if (
      JSON.stringify(next) === JSON.stringify(confirmed.current) &&
      !busy.current
    )
      return;
    pending.current = { value: next, revision: revision.current };
    if (config.current.delay)
      timer.current = setTimeout(flush, config.current.delay);
    else flush();
  }
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      clearTimeout(timer.current);
      void drain();
    };
  }, []);
  const parsed = options.schema.safeParse(value);
  const validationError = parsed.success
    ? ""
    : parsed.error.issues.map((issue) => issue.message).join(" ");
  return { value, update, flush, saving, error, validationError, form };
}
