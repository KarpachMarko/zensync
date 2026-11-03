# Project conventions

- Use pnpm for all JavaScript package installation, scripts, and tooling. Do not use npm or npx.
- Keep route components in `web/src/pages`, reusable UI in `web/src/components`, and shared API/types/helpers in `web/src/lib`.
- Initialize and install shadcn components with the official CLI (`pnpm dlx shadcn@latest add <component>` from `web/`). Keep `components.json` usable for future additions.
- Reuse the customized shadcn primitives in `web/src/components/ui`; preserve the existing ZenSync appearance. Use Sidebar, Tabs, Badge, Label, Separator, Progress, Collapsible, and Sonner for matching UI behavior. Keep domain wrappers thin and avoid success toasts from polling/background refreshes.
- Run `pnpm --dir web build` and relevant Playwright regression tests for frontend behavior changes.

- Prefer TypeScript equivalents for configuration and application files whenever supported (for example, `vite.config.ts`).
- Use nuqs for URL query state and serialization.
- Use TanStack React Query for API reads, mutations, pagination, polling, and cache management.
- Use other TanStack libraries, including Table and Form, when they suit the feature.
- Avoid the TypeScript `any` type. Use explicit domain types, generics, or `unknown` with narrowing.
- Keep Tailwind styling in React component `className` values. Extract shared typography, form, and layout components to reduce duplication. Avoid global component selectors or `@apply` recipes; global CSS should contain only framework imports, theme tokens, document defaults, and animation definitions.
