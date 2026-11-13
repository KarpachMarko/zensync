import type { ComponentProps } from "react";
import { cn } from "cn";
import { DialogContent } from "@/components/ui/dialog";

// Avoid transforms so the shared fixed player stays anchored to the viewport.
export function ReviewDialogContent({ className, ...props }: ComponentProps<typeof DialogContent>) {
  return <DialogContent className={cn("data-open:animate-none data-closed:animate-none left-4 right-4 top-[5dvh] translate-none mx-auto max-sm:inset-0 flex h-[min(840px,90dvh)] w-[calc(100vw-2rem)] max-w-[1100px] flex-col gap-0 overflow-hidden p-0 sm:max-w-[1100px] max-sm:h-dvh max-sm:w-screen max-sm:max-w-none max-sm:rounded-none", className)} {...props} />;
}
