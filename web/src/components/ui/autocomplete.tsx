import { useId, useState, type ComponentProps } from "react";
import { Input } from "./input";
import { cn } from "@/lib/utils";

type Props = Omit<ComponentProps<typeof Input>, "value" | "onChange"> & {
  value: string;
  onValueChange: (value: string) => void;
  options: string[];
};
export function Autocomplete({
  value,
  onValueChange,
  options,
  className,
  ...props
}: Props) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  function select(option: string) {
    onValueChange(option);
    setOpen(false);
    setActive(-1);
  }
  return (
    <div className="relative min-w-0">
      <Input
        {...props}
        value={value}
        className={className}
        role="combobox"
        autoComplete="off"
        aria-autocomplete="list"
        aria-expanded={open && options.length > 0}
        aria-controls={id}
        aria-activedescendant={
          open && active >= 0 ? `${id}-${active}` : undefined
        }
        onChange={(event) => {
          onValueChange(event.target.value);
          setOpen(true);
          setActive(-1);
        }}
        onFocus={() => setOpen(true)}
        onBlur={(event) => {
          setOpen(false);
          props.onBlur?.(event);
        }}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            setOpen(true);
            setActive((current) =>
              options.length
                ? (current +
                    (event.key === "ArrowDown" ? 1 : -1) +
                    options.length) %
                  options.length
                : -1,
            );
          } else if (event.key === "Enter" && open && active >= 0) {
            event.preventDefault();
            select(options[active]);
          } else if (event.key === "Escape" && open) {
            event.preventDefault();
            event.stopPropagation();
            setOpen(false);
          }
        }}
      />
      {open && options.length > 0 && (
        <div
          role="listbox"
          id={id}
          className="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded border border-border bg-popover p-1 shadow-md"
        >
          {options.map((option, index) => (
            <div
              key={option}
              id={`${id}-${index}`}
              role="option"
              aria-selected={index === active}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => select(option)}
              className={cn(
                "cursor-pointer rounded px-2 py-1.5 text-sm",
                index === active && "bg-accent",
              )}
            >
              {option}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
