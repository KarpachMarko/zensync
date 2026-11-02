# Project conventions

- Use pnpm for all JavaScript package installation, scripts, and tooling. Do not use npm or npx.
- Keep route components in `web/src/pages`, reusable UI in `web/src/components`, and shared API/types/helpers in `web/src/lib`.
- Reuse the customized shadcn primitives in `web/src/components/ui`; preserve the existing ZenSync appearance.
- Run `pnpm --dir web build` and relevant Playwright regression tests for frontend behavior changes.
