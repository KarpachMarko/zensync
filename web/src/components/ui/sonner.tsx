import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
} from "lucide-react";
import { Toaster as Sonner, type ToasterProps } from "sonner";
import "sonner/dist/styles.css";

const Toaster = ({ ...props }: ToasterProps) => {
  return (
    <Sonner
      theme="dark"
      position="bottom-right"
      closeButton
      duration={5000}
      className="toaster group"
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      toastOptions={{
        unstyled: true,
        classNames: {
          toast:
            "relative flex w-full items-center gap-3 rounded-lg border border-solid border-border bg-card p-4 pr-10 font-sans text-sm text-foreground shadow-lg",
          title: "font-semibold",
          description: "mt-1 text-muted-foreground",
          success: "text-[#a2d5bd]",
          error: "text-[#ffb5bb]",
          closeButton:
            "absolute top-2 right-2 rounded-md border-0 bg-transparent! p-1 text-muted-foreground cursor-pointer hover:bg-muted! focus-visible:outline-2 focus-visible:outline-primary",
          actionButton:
            "ml-auto rounded-md border-0 bg-primary px-3 py-1 text-primary-foreground cursor-pointer",
          cancelButton:
            "ml-auto rounded-md border border-solid border-border bg-transparent px-3 py-1 text-muted-foreground cursor-pointer",
        },
      }}
      {...props}
    />
  );
};

export { Toaster };
