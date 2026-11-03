import { Button } from "@/components/ui/button";
import { useAppForm } from "@/hooks/use-app-form";
import { declineSchema } from "@/lib/form-schemas";

export function DeclineForm({
  title,
  busy,
  onDecline,
}: {
  title: string;
  busy: boolean;
  onDecline: (reason: string) => Promise<void>;
}) {
  const form = useAppForm({
    defaultValues: { reason: "" },
    validators: { onSubmit: declineSchema },
    onSubmit: async ({ value }) => onDecline(value.reason.trim()),
  });
  return (
    <form
      className="flex items-start gap-2 max-[700px]:flex-wrap"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        void form.handleSubmit();
      }}
    >
      <form.AppField name="reason">
        {(field) => (
          <field.TextField
            label={`Reason for declining ${title}`}
            hideLabel
            labelClassName="m-0"
            placeholder="Reason for declining"
          />
        )}
      </form.AppField>
      <Button variant="outline" disabled={busy}>
        Decline
      </Button>
    </form>
  );
}
