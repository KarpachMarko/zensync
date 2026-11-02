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
- `src/hooks/`: reusable stateful application behavior, including session setup and login.
- `src/lib/`: API client, domain types and pure helpers.
- `src/styles/`: existing ZenSync visual design and primitive styles.

Imports use the `@/` alias for `src/`. Keep reusable controls and domain presentation out of route components. Keep mutation and loading orchestration close to the page or feature that owns it.

## UI components

Button, Input and AlertDialog are manually installed adaptations of [shadcn/ui](https://ui.shadcn.com/docs/components). Button and AlertDialog derive from the official `new-york` registry sources; their MIT license is in `licenses/shadcn-ui.txt`.

We retain the component composition APIs, Radix Slot, Radix alert-dialog behavior and button variants, replacing Tailwind classes with our existing semantic CSS. This preserves ZenSync's navy/pink palette, compact spacing, typography and responsive layout without introducing a second styling system. There is intentionally no Tailwind/shadcn CLI configuration; adapt future registry components into this same CSS setup instead of overwriting the theme.

Use `Button` and `Input` throughout the app. `Button` preserves native HTML submit behavior; use `type="button"` for non-submit actions inside forms. Use `variant="outline"` or `variant="destructive"` for new controls. Existing feature classes remain supported. `DeletionDialog` owns the asynchronous deletion workflow; the underlying shared AlertDialog owns modal accessibility, focus trapping and scroll locking.

Historical static mockups were removed. Git history retains them; the running app is the current UI reference.
