import { clsx, type ClassValue } from "clsx";

// Components use semantic CSS classes rather than Tailwind utilities.
export function cn(...inputs: ClassValue[]) {
  return clsx(inputs);
}
