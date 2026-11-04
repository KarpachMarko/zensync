import { useFieldContext } from "@/hooks/form-context";
import { cn } from "@/lib/utils";
import { useId, useState, type ComponentProps, type ReactNode } from "react";
import { FieldLabel } from "./forms";
import { Text } from "./typography";
import { Checkbox } from "./ui/checkbox";
import { Input } from "./ui/input";
import { Button } from "./ui/button";
import {
  FileUpload,
  FileUploadDropzone,
  FileUploadTrigger,
} from "./ui/file-upload";
import { FileTextIcon, UploadIcon, XIcon } from "lucide-react";
import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentMedia,
  AttachmentTitle,
} from "./ui/attachment";

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

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

export function FileField({
  label,
  disabled,
  accept,
  onFileValidate,
  description,
}: Pick<ComponentProps<typeof FileUpload>, "accept" | "disabled" | "onFileValidate"> & {
  label: string;
  description?: string;
}) {
  const field = useFieldContext<File | null>(),
    id = useId();
  const [rejection, setRejection] = useState("");
  const invalid = field.state.meta.isTouched && !field.state.meta.isValid;
  const file = field.state.value;
  return (
    <div className="mb-[18px] flex flex-col gap-[7px]">
      <span id={id + "-label"} className="text-sm">{label}</span>
      <FileUpload
        label={label}
        name={field.name}
        value={file ? [file] : []}
        onValueChange={(files) => {
          field.handleChange(files[0] ?? null);
          field.handleBlur();
          setRejection("");
        }}
        onFileValidate={onFileValidate}
        onFileReject={(_file, message) => setRejection(message)}
        accept={accept}
        maxFiles={1}
        disabled={disabled}
        invalid={invalid || !!rejection}
        aria-describedby={rejection || invalid ? id + "-error" : undefined}
      >
        {!file && (
          <FileUploadDropzone
            aria-label="Drop file here"
            aria-describedby={rejection || invalid ? id + "-error" : id + "-help"}
            className="gap-3 border-border py-7 text-center data-dragging:border-primary data-dragging:bg-accent"
          >
            <UploadIcon className="size-6 text-muted-foreground" aria-hidden="true" />
            <span className="text-sm">Drag and drop your file here</span>
            <span id={id + "-help"} className="text-xs text-muted-foreground">
              {description ?? "Choose one file"}
            </span>
            <FileUploadTrigger asChild>
              <Button variant="outline" type="button">Choose file</Button>
            </FileUploadTrigger>
          </FileUploadDropzone>
        )}
        {file && (
          <Attachment
            state={disabled ? "uploading" : "idle"}
            className="w-full box-border flex-nowrap border-solid border-border"
            aria-labelledby={id + "-filename"}
            aria-describedby={id + "-metadata"}
          >
            <AttachmentMedia aria-hidden="true">
              <FileTextIcon className="size-5" />
            </AttachmentMedia>
            <AttachmentContent>
              <AttachmentTitle id={id + "-filename"} title={file.name}>
                {file.name}
              </AttachmentTitle>
              <AttachmentDescription id={id + "-metadata"} aria-live="polite">
                {file.name.split(".").pop()?.toUpperCase()} · {formatFileSize(file.size)} · {disabled ? "Uploading…" : "Ready to import"}
              </AttachmentDescription>
            </AttachmentContent>
            <AttachmentActions className="pr-2">
              <AttachmentAction
                type="button"
                disabled={disabled}
                aria-label={"Remove " + file.name}
                className="inline-flex items-center justify-center max-[700px]:min-w-11"
                onClick={() => {
                  field.handleChange(null);
                  field.handleBlur();
                  setRejection("");
                }}
              >
                <XIcon className="size-4" aria-hidden="true" />
              </AttachmentAction>
            </AttachmentActions>
          </Attachment>
        )}
      </FileUpload>
      {rejection ? (
        <Text id={id + "-error"} role="alert" className="m-0 text-xs">
          {rejection}
        </Text>
      ) : invalid ? (
        <ValidationErrors id={id + "-error"} errors={field.state.meta.errors} />
      ) : null}
    </div>
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
