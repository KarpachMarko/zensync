# Frontend

Use pnpm (the version is pinned in `package.json`):

```sh
pnpm install --frozen-lockfile
pnpm dev
pnpm build
pnpm exec playwright install chromium
pnpm test:e2e
```

Run these commands from `web/`, or use `pnpm --dir web` from the repository root. Vite proxies API requests to the Go server on port 8080. End-to-end tests run against the production build and start an isolated Go server; build first. If Go is not on PATH, set `ZENSYNC_GO` to its executable path.

## Responsibilities

- `src/main.tsx`: React and router bootstrap.
- `src/app.tsx`: authenticated application routes.
- `src/pages/`: route-specific orchestration and data loading; settings and import routes have their own subdirectories.
- `src/components/ui/`: shared, customized shadcn primitives.
- `src/components/`: shared layout, expandable sections and progress display, plus domain components for music, imports and requests.
- `src/hooks/`: shared TanStack Query reads/mutations and session setup/login.
- `src/lib/`: API client, domain types and pure helpers.
- `src/styles/`: Tailwind setup, theme tokens, document defaults, and animation definitions. Component styling lives in React `className` values.

Imports use the `@/` alias for `src/`. Keep reusable controls and domain presentation out of route components. Keep mutation and loading orchestration close to the page or feature that owns it.

## UI components

Button, Input, AlertDialog, Sidebar, Tabs, Badge, Label, Separator, Progress, Collapsible, and Sonner are installed using the official shadcn CLI with the Radix Nova preset. Sidebar also uses the generated Sheet, Tooltip, and Skeleton dependencies. `components.json` records the aliases and theme location. The MIT license is in `licenses/shadcn-ui.txt`.

From `web/`, add future components with:

```sh
pnpm dlx shadcn@latest add <component>
```

Tailwind v4 is integrated through the Vite plugin. The generated theme in `src/styles/shadcn.css` uses ZenSync's navy/pink palette and Avenir typography. Tailwind Preflight is omitted to preserve browser defaults. All component styling is inline Tailwind in React; there is no `app.css`. Shared typography, field controls, navigation, and content layouts own their styles, while the sidebar owns its collapsed and responsive states. Review customizations before overwriting installed components.

Use `Button` and `Input` throughout the app. `Button` preserves native HTML submit behavior; use `type="button"` for non-submit actions inside forms. Use `variant="outline"` or `variant="destructive"` for new controls. `DeletionDialog` owns the asynchronous deletion workflow; the shared AlertDialog owns modal accessibility, focus trapping and scroll locking.

`WorkspaceSidebar` composes the shadcn Sidebar primitives, retains the saved desktop collapse preference, and uses a navigation drawer on mobile. Settings Tabs follow the existing routes and support manual keyboard activation. Status badges retain the app's state colors. Progress handles both known percentages and indeterminate transfers, including reduced motion. Shared field labels and expandable sections use Label and Collapsible.

Use Sonner success toasts after completed user actions. Mutation failures produce error toasts; keep persistent form errors inline so users can correct them. Query/polling errors stay inline without repeated notifications. Keep durable request state, progress, and actionable request links on their owning pages. API keys and credentials must never appear in a toast. Toasts are dismissed at authentication boundaries.

Historical static mockups were removed. Git history retains them; the running app is the current UI reference.

## State and type conventions

Use TypeScript configuration files where supported (`vite.config.ts`). `pnpm typecheck` checks the app and configs and rejects explicit `any` types in project TypeScript files, including tests and scripts.

Use nuqs for URL query state and serialization. Search query `q` uses browser history; the `type` filter is parsed as a validated string literal and survives reloads. Query strings may encode spaces as `+` or `%20`; compare parsed values in tests.

Use TanStack Query for server state: `useApiQuery` for reads and polling, `useInfiniteQuery` for paginated library/search results, and `useApiMutation` for writes. The low-level API client handles HTTP and CSRF only. Queries are keyed by resource and input, and writes invalidate their resource. Automatic mutation retries are disabled. Personal query caches are cleared at login/logout. Keep form drafts and transient notices in local state. Use other TanStack libraries when the feature benefits from them.

## Styling conventions

Keep styling next to the React markup with Tailwind classes. Reuse `components/typography.tsx`, `components/forms.tsx`, and `components/layout/content.tsx` for shared typography, controls, and layouts. Use the shadcn Button's `variant` and `asChild` APIs for secondary/destructive actions and button-shaped links. Prefer explicit component props for state-dependent styles instead of stylesheet selectors. Keep global CSS limited to Tailwind setup, theme tokens, document defaults, and keyframes.
