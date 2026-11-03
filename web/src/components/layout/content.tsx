import { AppIcon } from "@/components/app-icon";
import { cn } from "@/lib/utils";
import { Slot } from "radix-ui";
import type { ComponentProps } from "react";

export function Row({
  className,
  asChild = false,
  ...props
}: ComponentProps<"div"> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "div";
  return (
    <Comp
      data-slot="list-row"
      className={cn(
        "flex min-h-[52px] items-center justify-between gap-5 border-b border-solid border-sidebar-border px-0 py-2.5 [&_strong]:text-sm max-[700px]:items-start",
        className,
      )}
      {...props}
    />
  );
}

export function Actions({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn("flex flex-wrap items-center gap-2.5", className)}
      {...props}
    />
  );
}

export function PageTitle({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-4 [&_h1]:max-[700px]:text-[27px]",
        className,
      )}
      {...props}
    />
  );
}

export function FieldGrid({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "grid max-w-[850px] grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-[18px]",
        className,
      )}
      {...props}
    />
  );
}

const searchLayout = "my-6 mx-0 flex items-end gap-3.5 max-[700px]:flex-wrap";

export function SearchForm({ className, ...props }: ComponentProps<"form">) {
  return <form className={cn(searchLayout, className)} {...props} />;
}

export function SearchBar({ className, ...props }: ComponentProps<"div">) {
  return <div className={cn(searchLayout, className)} {...props} />;
}

export function Notice({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "my-6 mx-0 max-w-[850px] border-l-3 border-solid border-primary bg-[#29384f] px-[18px] py-[13px] text-sm",
        className,
      )}
      {...props}
    />
  );
}

export function Brand({
  className,
  children = "ZenSync",
  ...props
}: ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 text-[19px] font-bold whitespace-nowrap text-primary",
        className,
      )}
      {...props}
    >
      <AppIcon />
      {children}
    </div>
  );
}

export function AuthLayout({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "mx-auto my-[12vh] max-w-[420px] p-[25px] max-[700px]:my-[5vh]",
        className,
      )}
      {...props}
    />
  );
}

export function SettingsSection({
  className,
  ...props
}: ComponentProps<"section">) {
  return (
    <section
      className={cn(
        "max-w-[850px] border-b border-solid border-sidebar-border pt-3 pb-[26px] [&_input]:max-w-[700px]",
        className,
      )}
      {...props}
    />
  );
}
