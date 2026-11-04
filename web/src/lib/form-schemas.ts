import { z } from "zod";

const bytes = (value: string) => new TextEncoder().encode(value).length;
export const usernameSchema = z
  .string()
  .trim()
  .min(1, "Enter a username.")
  .refine((value) => bytes(value) <= 80, "Username is too long.");
export const passwordSchema = z
  .string()
  .min(1, "Enter your password.")
  .refine((value) => bytes(value) <= 72, "Password is too long.");
export const newPasswordSchema = passwordSchema.refine(
  (value) => value.length >= 12,
  "Use at least 12 characters.",
);
export const credentialsSchema = (setup: boolean) =>
  z.object({
    username: usernameSchema,
    password: setup ? newPasswordSchema : passwordSchema,
  });
export type CredentialsInput = z.infer<ReturnType<typeof credentialsSchema>>;
export const inviteSchema = z.object({ username: usernameSchema });
export const acceptInviteSchema = z.object({ password: newPasswordSchema });
export const searchSchema = z.object({
  query: z
    .string()
    .trim()
    .min(1, "Enter a search query.")
    .max(500, "Keep your search within 500 characters."),
});
export const declineSchema = z.object({
  reason: z
    .string()
    .trim()
    .min(1, "Enter a reason for declining.")
    .max(1000, "Keep the reason within 1,000 characters."),
});
export const uploadSchema = z.object({
  file: z
    .instanceof(File)
    .nullable()
    .refine((file) => file !== null, "Choose an XML or TXT export.")
    .refine(
      (file) => !file || /\.(xml|txt)$/i.test(file.name),
      "Choose an XML or TXT export.",
    )
    .refine(
      (file) => !file || file.size <= 32 * 1024 * 1024,
      "Choose an export smaller than 32 MiB.",
    ),
});
export const audioUploadSchema = z.object({
  file: z
    .instanceof(File)
    .nullable()
    .refine((file) => file !== null, "Choose an audio file.")
    .refine(
      (file) =>
        !file || /\.(flac|mp3|m4a|ogg|opus|wav|aif|aiff)$/i.test(file.name),
      "Choose FLAC, MP3, M4A, OGG, OPUS, WAV or AIFF audio.",
    )
    .refine(
      (file) => !file || (file.size >= 1024 && file.size <= 1024 ** 3),
      "Audio must be between 1 KiB and 1 GiB.",
    ),
});

export const migrationChoicesSchema = z.object({
  playlists: z.array(z.string().min(1)),
  favorites: z.boolean(),
});
export const downloadLimitSchema = z
  .string()
  .refine(
    (value) =>
      /^\d+$/.test(value) &&
      Number.isSafeInteger(Number(value)) &&
      Number(value) >= 1,
    "Enter a positive whole number.",
  );
export const qualitySchema = z
  .array(z.enum(["lossless", "320", "128"]))
  .min(1, "Keep at least one quality enabled.")
  .max(3)
  .refine(
    (value) => new Set(value).size === value.length,
    "Each quality may appear only once.",
  );
export const permissionsSchema = z.object({
  direct: z.boolean(),
  directDelete: z.boolean(),
});
export type ConnectionKind =
  | "shared"
  | "personal"
  | "deezer"
  | "lossless"
  | "lossless-login";
export const connectionSchema = (kind: ConnectionKind, ready: boolean) =>
  z
    .object({
      url: z.string(),
      username: z.string(),
      password: z.string(),
      arl: z.string(),
    })
    .superRefine((draft, ctx) => {
      if (!ready) {
        ctx.addIssue({
          code: "custom",
          path: ["username"],
          message: "Connect the shared server first.",
        });
        return;
      }
      if (kind === "lossless-login") {
        if (!z.email().safeParse(draft.username.trim()).success)
          ctx.addIssue({
            code: "custom",
            path: ["username"],
            message: "Enter your Lossless.am email.",
          });
        if (draft.password.length < 8 || draft.password.length > 4096)
          ctx.addIssue({
            code: "custom",
            path: ["password"],
            message: "Enter your Lossless.am password (at least 8 characters).",
          });
        return;
      }
      if (kind === "lossless") {
        if (
          draft.arl.length < 3 ||
          draft.arl.length > 4096 ||
          !draft.arl.includes("=") ||
          /[\r\n]/.test(draft.arl)
        )
          ctx.addIssue({
            code: "custom",
            path: ["arl"],
            message: "Enter the complete session Cookie header.",
          });
        return;
      }
      if (kind === "deezer") {
        if (draft.arl.length < 32 || draft.arl.length > 4096)
          ctx.addIssue({
            code: "custom",
            path: ["arl"],
            message: "Enter a complete Deezer ARL cookie.",
          });
        return;
      }
      if (!draft.username.trim())
        ctx.addIssue({
          code: "custom",
          path: ["username"],
          message: "Enter a username.",
        });
      if (!draft.password)
        ctx.addIssue({
          code: "custom",
          path: ["password"],
          message: "Enter a password to verify this connection.",
        });
      if (kind === "shared") {
        try {
          const url = new URL(draft.url);
          if (
            !["http:", "https:"].includes(url.protocol) ||
            url.username ||
            url.password ||
            url.search ||
            url.hash
          )
            throw new Error();
        } catch {
          ctx.addIssue({
            code: "custom",
            path: ["url"],
            message: "Enter an http(s) Navidrome base URL.",
          });
        }
      }
    });
