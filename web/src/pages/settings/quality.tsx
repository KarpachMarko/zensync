import { Caption, SectionHeading, Text } from "@/components/typography";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useApiMutation } from "@/hooks/use-api";
import { useAutosave } from "@/hooks/use-autosave";
import { qualitySchema } from "@/lib/form-schemas";
import type { User } from "@/lib/types";
import { cn } from "@/lib/utils";
import {
  ArrowDownIcon,
  ArrowUpIcon,
  AudioLinesIcon,
  CheckIcon,
} from "lucide-react";

const qualities = [
  { id: "lossless", name: "FLAC", detail: "Lossless audio" },
  { id: "320", name: "MP3 320 kbps", detail: "High quality, smaller files" },
  { id: "128", name: "MP3 128 kbps", detail: "Smallest files" },
];

export function Quality({
  user,
  reload,
}: {
  user: User;
  reload: () => Promise<void>;
}) {
  const { api } = useApiMutation({ errorToastId: "quality-preferences" });
  const editor = useAutosave({
    initialValue: user.quality,
    schema: qualitySchema,
    save: async (quality) => {
      await api("/quality", "PUT", { quality });
      void reload().catch(() => {});
    },
    message: "Quality preference saved.",
    toastId: "quality-preferences",
  });
  const order = [
    ...editor.value,
    ...qualities
      .map((quality) => quality.id)
      .filter((id) => !editor.value.includes(id)),
  ];
  function move(id: string, direction: number) {
    const next = [...editor.value],
      index = next.indexOf(id),
      target = index + direction;
    if (index < 0 || target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    editor.update(next);
  }
  return (
    <div className="max-w-[780px]">
      <SectionHeading>Your quality preference</SectionHeading>
      <Text>
        Start with your favorite format. If it is unavailable, ZenSync tries the
        next enabled quality. Changes save automatically.
      </Text>
      <div className="mt-7 mb-3 flex items-center justify-between gap-3">
        <span className="text-sm font-semibold">Preferred order</span>
        <Caption>
          {editor.error ? (
            "Not saved"
          ) : editor.saving ? (
            "Saving…"
          ) : (
            <span className="flex items-center gap-1">
              <CheckIcon aria-hidden className="size-3.5" /> Saved automatically
            </span>
          )}
        </Caption>
      </div>
      <ol
        className="m-0 flex list-none flex-col gap-3 p-0"
        aria-label="Quality preferences"
      >
        {order.map((id) => {
          const quality = qualities.find((item) => item.id === id);
          if (!quality) return null;
          const index = editor.value.indexOf(id),
            enabled = index >= 0;
          return (
            <li
              key={id}
              data-quality={id}
              className={cn(
                "flex flex-wrap items-center gap-3 rounded-lg border border-solid border-border p-4",
                enabled
                  ? "bg-card"
                  : "border-dashed bg-transparent text-muted-foreground",
              )}
            >
              <span
                aria-hidden
                className="flex size-8 shrink-0 items-center justify-center rounded-md bg-background text-sm tabular-nums text-muted-foreground"
              >
                {enabled ? index + 1 : <AudioLinesIcon className="size-4" />}
              </span>
              <div className="min-w-0 flex-1">
                <Label
                  htmlFor={`quality-${id}`}
                  className="font-semibold leading-normal text-foreground"
                >
                  {quality.name}
                </Label>
                <Caption className="mt-1">{quality.detail}</Caption>
              </div>
              {index === 0 && (
                <Badge
                  variant="secondary"
                  className="border-0 bg-primary/15 text-primary max-[500px]:hidden"
                >
                  First choice
                </Badge>
              )}
              <div
                className="flex gap-1"
                aria-label={`Reorder ${quality.name}`}
              >
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="border-0"
                  aria-label={`Move ${quality.name} up`}
                  disabled={!enabled || index === 0}
                  onClick={() => move(id, -1)}
                >
                  <ArrowUpIcon className="size-4" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="border-0"
                  aria-label={`Move ${quality.name} down`}
                  disabled={!enabled || index === editor.value.length - 1}
                  onClick={() => move(id, 1)}
                >
                  <ArrowDownIcon className="size-4" />
                </Button>
              </div>
              <Switch
                id={`quality-${id}`}
                aria-label={`Enable ${quality.name}`}
                checked={enabled}
                disabled={enabled && editor.value.length === 1}
                onCheckedChange={(checked) =>
                  editor.update(
                    checked
                      ? [...editor.value, id]
                      : editor.value.filter((value) => value !== id),
                  )
                }
              />
            </li>
          );
        })}
      </ol>
      {editor.error && <Text role="alert">{editor.error}</Text>}
      <Text className="mt-5 text-[13px]">
        Keep at least one quality enabled. Existing playable music is reused.
        New requests keep the quality order chosen when they were submitted.
      </Text>
    </div>
  );
}
