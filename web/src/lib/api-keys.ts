import { z } from "zod";

export const apiKeyScopes = [
  { id: "search:read", label: "Search music" },
  { id: "library:read", label: "Read shared library" },
  { id: "requests:read", label: "Read your requests and progress" },
  { id: "requests:create", label: "Request music downloads" },
  { id: "deletion:request", label: "Request song deletion and read its status" },
] as const;
export type APIKey = {
  id: string;
  label: string;
  scopes: string[];
  expires: number | null;
  created: string;
};
export const apiKeySchema = z.object({
  label: z.string().trim().min(1, "Enter a label.").max(100, "Use at most 100 characters."),
  expires: z.string().refine(
    (value) => !value || new Date(value).getTime() > Date.now(),
    "Choose a future expiration date.",
  ),
  scopes: z.array(z.string()).min(1, "Select at least one scope."),
});
