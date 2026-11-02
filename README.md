# ZenSync

A self-hosted Navidrome companion for initial music migration and ongoing music acquisition.

## Current status

The first migration workflow is implemented: first-access administrator setup, Apple Music XML preview/commit, recording review, Deezer search or direct song URLs, approved acquisition through pinned d-fi 2.3.5, validated shared-library placement, and personal Navidrome playlist/favorite publication. External edits pause playlist updates; final handover stops migration collection writes. Legacy ID/hash downloads are automatically reorganized into readable paths with verified Navidrome identity preservation and recovery backups.

Docker bundles the application, UI, downloader and audio validation tooling. Mount the same music directory Navidrome scans, start the container, create the admin in the browser, then configure connections and open Imports. No administrator password/hash environment variable is needed.

See [deployment and first-migration instructions](docs/deployment.md) and the [song deletion API](docs/deletion-api.md). Wavio compatibility, shared-file upgrades and the remaining multi-user lifecycle features are still pending; the bullets below describe the full planned phase 1, not a claim that every feature is complete.

## Phase 1

- Go + SQLite backend, compact React/TypeScript UI, Apple/iTunes and Deezer metadata, and acquisition through the existing d-fi Go binary.
- Apple Music XML migration of the complete music catalog, selected playlists and favorites. No ongoing Apple synchronization or updated-export merging.
- One admin-configured Navidrome instance and shared music library, with separate personal Navidrome accounts. Personal linking is needed for migration publication, not search or acquisition requests.
- Migration stays open to fill missing entries until explicit handover. External edits pause the affected playlist. After handover, Navidrome owns playlists/favorites; outstanding acquisitions continue without changing those collections.
- Persistent unresolved entries with Skip for now. Confidently matched existing music can fill skipped entries without a new download; alternative versions require review.
- Per-user ordered quality profiles: lossless first, then allowed lower qualities. Reuse existing audio unless Upgrade is explicitly requested; replacing a shared file requires admin approval and verification.
- Admin-approved member acquisitions by default, with an optional direct-acquisition grant. Song deletion has a separate direct-delete grant; other deletion requests require admin approval.
- Wavio connects directly to Navidrome for playback and collection management, and uses ZenSync's acquisition adapter. Approval may appear as Queued in Wavio; details are shown in ZenSync.
- Dedicated Library, Search, Imports, Requests, Activity and Settings pages with a collapsible sidebar and compact music rows.

Phase 2 adds followed-artist new-release notifications, without automatic downloads. Public playlist links and extra import formats are outside phase 1. ZenSync is not a playback engine, client gateway or ongoing playlist/favorites editor.

## Start here

- [Authoritative implementation plan](docs/implementation-plan.md)
- [Multi-user permissions and request workflow](docs/multi-user-design.md)
- [Historical source investigation and preliminary assessment](docs/assessment.md)
- [Earlier compact detail mockups](mockups/details/index.html)
- [Mockup instructions and validation](mockups/details/README.md)

Open mockup HTML directly; no server or build step is required. Use Preview as to compare sample admin/member behavior. Mockups have not yet been adapted to the revised migration/acquisition boundary, quality profiles or handover rules. The earlier sets under `mockups/revised/` and `mockups/` remain design history.

The original single-user effort estimate is superseded. Establish the supported integration contract and recovery behavior in the first executable slice, then estimate the remaining phase 1 milestones.
