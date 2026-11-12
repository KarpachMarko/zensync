import {
  AutocompleteField,
  CheckboxField,
  FileField,
  TextField,
  TextareaField,
} from "@/components/form-fields";
import { createFormHook } from "@tanstack/react-form";
import { fieldContext, formContext } from "./form-context";

export const { useAppForm } = createFormHook({
  fieldContext,
  formContext,
  fieldComponents: {
    AutocompleteField,
    TextField,
    FileField,
    CheckboxField,
    TextareaField,
  },
  formComponents: {},
});
