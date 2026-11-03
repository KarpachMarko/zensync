import { CheckboxField, FileField, TextField } from "@/components/form-fields";
import { createFormHook } from "@tanstack/react-form";
import { fieldContext, formContext } from "./form-context";

export const { useAppForm } = createFormHook({
  fieldContext,
  formContext,
  fieldComponents: { TextField, FileField, CheckboxField },
  formComponents: {},
});
