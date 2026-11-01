# Music-only SoulSync implementation plan

Status: implementation planning; application backend work has not started. UI prototypes are maintained separately under `../mockups/`. This document records the user's decisions after reviewing the five mockups and takes precedence over the earlier UI recommendation in the assessment.

## Product decisions

- Go backend, SQLite, a small React/TypeScript UI and the user's working d-fi Go binary for Deezer downloads.
- Apple/iTunes and Deezer metadata; Apple Music XML library exports and public playlist links; other playlist files as previously scoped.
- Navidrome stores/serves playable music; Wavio is the first client compatibility target.
- A collapsible sidebar based on Collection (option 1), combined with the playlist sleeve and ordered list from Mixtape (option 3). The chosen project name is ZenSync; repository/folder name: zensync.
- Dedicated routed pages for normal workflows. Do not implement navigation, search, library browsing, imports, queues, settings or match resolution as dialogs. Dialogs are reserved for short confirmations, such as removing a playlist.
- A Library with Songs, Artists, Albums and personal Favorites.
- Multiple users share the base audio library; personal collections remain user-owned. Member acquisition requires admin approval unless explicitly granted, and members cannot delete song files.
- Compact rows and controls: about 44px ordinary desktop song rows, with additional space only for unresolved reasons/actions and comfortable mobile touch targets.
- Use **Activity** as the navigation label for work running or queued on the server, plus review needs and history. It covers more than downloads without exposing the term “jobs” as the main product concept.
- Failure to identify or import one song must not force immediate manual matching or block the rest of a library/playlist import. Provide **Skip for now** and keep a persistent placeholder in the song's playlist position.

## Multiple users and acquisition approval

Confirmed scope: one shared base music library, with separate user accounts and personal playlists, favorites, imports and followed artists. Members request new music for administrator approval by default. Admin may grant **Add music without approval** independently; this never grants permission to delete song files. File deletion and user/service administration remain admin-only. Apply the same ownership and approval checks to Wavio, schedules, wishlist processing and all native API paths.

See [multi-user design and acceptance criteria](multi-user-design.md) for roles, permission gates, request states, personal/shared data, identity mapping, API keys, job deduplication, account lifecycle and isolation tests. These requirements replace the earlier single-profile assumptions. Build authorization and request approval before connecting acquisition workers.

## Navigation and dedicated pages

The sidebar contains Library, Playlists, Search, Imports, Requests, Activity, Followed artists and Settings. Expanded form shows labels and relevant counts. Collapsed form keeps icons, accessible labels/tooltips and the active page. Persist the desktop collapse preference locally; it must not depend on connection credentials. On small screens use an accessible slide-out navigation drawer that closes after navigation and returns focus correctly.

All main pages have stable URLs, browser back/forward support, a page title and consistent breadcrumbs where a parent exists. Preserve selected tabs, filters and search in query parameters when useful. A page reload should return to the same item, rather than the start screen.

| Page | Example URL | Main content |
| --- | --- | --- |
| Library / Songs | `/library/songs` | Searchable track list, availability and source filters, favorites |
| Library / Artists | `/library/artists` | Artist collection and links to detail pages |
| Artist | `/library/artists/:id` | Albums, songs, ownership and follow controls |
| Library / Albums | `/library/albums` | Album artwork grid with availability and missing tracks |
| Album | `/library/albums/:id` | Ordered disc/track list with download/import state |
| Library / Favorites | `/library/favorites` | Favorited songs; artist/album stars can be added later |
| Playlists | `/playlists` | Imported and managed playlists, coverage, source and refresh state |
| Playlist | `/playlists/:id` | Mixtape-style sleeve and ordered song list, including placeholders |
| Search | `/search?q=...&source=deezer` | Track, album and artist search; explicit metadata source |
| New import | `/imports/new` | Choose Apple XML, public link or another file |
| Import preview | `/imports/:id/preview` | All-song summary, playlist selection and import choices |
| Import result | `/imports/:id` | Counts, partial failures, source snapshot and re-import action |
| Requests | `/requests` | Member request history or administrator approval inbox |
| Request detail | `/requests/:id` | Requested music, requester, approval decision and acquisition progress |
| People & permissions | `/settings/people` | Admin account management and direct-acquisition grants |
| Activity | `/activity?view=running` | Running, queued, needs attention and history views |
| Activity detail | `/activity/:id` | Task stages, counts, attempts, errors and supported actions |
| Review item | `/review/:id` | Source metadata, candidate matches, retry and Skip for now |
| Followed artists | `/followed-artists` | Follow management and release checks required by Wavio |
| Settings | `/settings/connections` | Navidrome, Deezer, quality and API settings |

Track rows link to a dedicated song page at `/library/songs/:id` when resolved, or the review page when unresolved. A playlist entry's review page provides a link back to the original playlist and position. Candidate comparison is part of that page, not a forced modal.

## Library semantics

The application catalog includes both actual Navidrome music and music described by imported sources. Do not pretend that an XML record is a downloaded file. Show availability explicitly: In library, Not downloaded, Needs matching, Import failed or Deferred. Library Songs supports All and Available in Navidrome filters, plus source, artist, album, favorite and review status. Imported songs outside every playlist must still be visible.

Use a source record before identification and a canonical recording after identification. A song can have several source records and files. Artist/album groupings should preserve imported names for unresolved records and merge only with adequate evidence; a name collision is not enough to establish identity. Search and pagination must work for large full-library imports.

Favorites initially apply to songs. For resolved library songs, reconcile favorites with the connected user's Navidrome stars through the adapter after checking the supported API behavior. Keep local favorite intent for imported/unavailable songs and apply it when a server mapping is established. Import XML favorite/loved information only when present; never assume an absent field means “unfavorite.” Do not overwrite newer explicit user changes on re-import. Do not promise reverse synchronization into Apple Music. Favorite changes and acquisition intent are independent.

## Apple Music XML is a complete source library

A library export contains a track dictionary for all exported songs, playlist definitions and ordered references identifying which songs belong to each playlist. It is not a flat list of unrelated playlists. Support standalone playlist exports too.

Import phases:

1. Parse the property-list XML into a staged source snapshot. Read source-library identity and persistent IDs when supplied. Track IDs are references scoped to that source snapshot; do not treat them as Deezer, Apple catalog or globally unique application IDs.
2. Ingest the complete exported music-track dictionary, including tracks referenced by no playlist. Preserve original title, artist, album, duration, disc/track numbers and additional useful identifiers. Keep original source metadata separate from later matched/enriched values.
3. Ingest playlist definitions, folders/parent relationships where present, and ordered entry references. Preserve repeated occurrences of a song. One source song may belong to multiple playlists. Missing track references become explicit placeholders with the known source reference, rather than being silently dropped.
4. Show a preview: total music songs, songs not in any playlist, playlists, ignored non-music records, missing references and parse problems. Default to importing the catalog metadata and all normal music playlists; allow deselecting playlists and previewing changes. Treat special/system playlists explicitly rather than duplicating the entire library as an ordinary playlist. Mixed-content playlists retain their music entries and report excluded non-music entries.
5. Commit the chosen source catalog and playlist structure independently of network matching. A temporary provider failure must not undo or block recording the source library. Matching and reconciliation run as background Activity tasks.
6. Downloading is an explicit separate choice: selected songs, missing songs in selected playlists, or missing songs in the entire imported library. Never automatically download all songs merely because the XML describes all songs.
7. Re-import into the same source library, diff by source-scoped persistent IDs when present, then use careful fallback identity rules when absent. Present ambiguous changes for review. Importing a new source library is a separate explicit choice. Preserve deferred decisions, manual matches, favorite overrides and stable playlist-entry identity where correspondence is reliable.

Store import versions and previous source snapshots so a malformed/partial XML does not silently erase playlists or catalog records. Show removals in the preview; applying a new source snapshot must never delete audio files as a side effect. Smart playlists initially use exported membership only. A fresh XML upload is required to see subsequent Apple Music changes.

The exported file describes library state and metadata; it is not the audio. Parse without external DTD/entity retrieval and enforce bounded resource usage.

## Activity: live server work and history

Activity is an overview of authorized work, not only work initiated from the current browser. Members see their own work; administrators can inspect a server overview. Requests awaiting approval are separate from running acquisitions. Include requests originating in Wavio, schedules, imports and manual actions.

Visible activity types:

- Downloading music through d-fi.
- Looking up or refreshing metadata through Apple/iTunes and Deezer.
- Importing files: validation, tagging, artwork and library placement.
- Scanning/reconciling the Navidrome library.
- Importing XML/file records and matching imported songs.
- Writing/updating a Navidrome playlist.
- Checking followed artists for missing releases.

The page separates Running, Queued, Needs attention and History; show type filters and an aggregate All view. Each item exposes its subject (song/album/playlist/library), operation, stage, status, origin, created/started time, elapsed duration, real progress if available and a concise error when relevant. Use indeterminate progress when a provider has no reliable total. Display dependencies honestly, for example “Waiting for Navidrome scan”; do not label waiting work as actively downloading.

Bulk tasks show both aggregate progress and child items. Counts distinguish processed, completed, failed and deferred. A batch can finish with issues; never report a fully successful import if some songs were skipped or failed. A deferred review item belongs under Needs attention, not Running. Queued work includes the reason it is waiting, such as concurrency or a prerequisite.

Offer Retry, Cancel, View song/playlist and Skip for now where appropriate. Only show pause/resume when the underlying operation actually supports it; do not promise resumable d-fi transfers without verifying support. Cancellation of one subscriber to a shared download must not cancel another playlist's acquisition request. Stopping an operation prevents its pending downstream work; already committed files are retained and reported.

Persist tasks, attempts and events in SQLite. Use bounded worker pools and separate concurrency/rate limits for downloads, metadata and import/scan operations. Reuse or coalesce server scans when possible. Recover interrupted tasks with leases and idempotent stages; reconcile the filesystem/server before repeating side effects. Closing the browser must not stop work. Expose live updates with SSE plus a snapshot/reconnect path, while preserving Wavio's polling contract.

## Skip for now and persistent placeholders

### User behavior

Every unresolved metadata match, missing track and failed file import has **Skip for now** beside Resolve and Retry. This is an optional decision at the affected item; there is no mandatory matching step that traps the user. Batch actions also allow Skip unresolved items for now.

Skipping records the deferred decision immediately and prevents new downstream work for that item. An active worker must acknowledge a safe stop/cancellation boundary before releasing its claim; do not release a lease while the same worker can still commit side effects. Preserve original metadata and already staged valid files, with a clear retained-file/cleanup policy. Other songs continue. It must survive restarts, re-imports and scheduled playlist refreshes until the user chooses Resume/Retry/Resolve. Retry policy must not repeatedly retry an explicitly deferred item. Offer Remove from playlist as a separate action with clear consequences; skipping does not remove an entry.

The ordered playlist keeps a visible placeholder showing its intended position, original title/artist where known, state and actions: Resolve, Retry, Resume or Remove. Distinguish reasons such as “No reliable metadata match,” “Song not available from Deezer,” and “File could not be imported.” A known matched recording with an import failure retains its identity; it does not need to be matched again merely because the file import failed.

Accepting a match through Resolve also resumes that entry’s remaining work; avoid requiring a second Resume click for the same explicit decision. Resolving identity does not mean a song is playable. The placeholder remains until a valid file is imported and a usable Navidrome mapping exists. It can show Matched → Downloading → Importing → Waiting for library scan along the way. A later failure returns to an actionable state without losing the intended playlist slot.

### Data and state design

Use a durable `playlist_entry` with its own ID, playlist ID, ordinal/order key and source-entry correspondence. It stores or references the original source metadata and can have nullable resolved-recording and Navidrome-song mappings. Never model a playlist solely as an array of playable server IDs.

Separate three concerns:

- Identification: unresolved, ambiguous or resolved; candidate evidence and chosen mapping.
- Acquisition/import: absent, queued, downloading, importing, awaiting server scan, available or failed; keep stage errors and retry history.
- User policy: active or deferred, with reason and time. Removal is an explicit playlist edit, not another download state.

An import creates source records and entries first. Matching attaches a canonical recording without replacing the entry. Downloading produces a file; import/reconciliation attaches the server-song mapping. Duplicate requests for the same recording can share acquisition work, while repeated playlist occurrences remain separate entries. Resolve a shared source-song identity once; playlist inclusion/deferral decisions stay per entry or acquisition intent so skipping one does not suppress unrelated requests. Provide an explicit “Apply to all occurrences of this source song” option when useful.

Auto-matching can fill a deferred placeholder only after the user resumes it or explicitly accepts a suggested match; a background refresh must not silently override the decision to defer.

### Navidrome projection

The application's full ordered playlist is authoritative for source-managed playlists, including placeholders. Navidrome receives the ordered projection of only available entries with valid server IDs. Do not upload silent audio, fake files or invalid IDs to represent placeholders. Wavio/Navidrome can therefore show fewer playable entries than the application's full source playlist; show a clear coverage summary in the web app.

For source order `[A, unresolved B, C]`, Navidrome initially has `[A, C]`. When B is imported and reconciled, update it to `[A, B, C]`. The full source ordering remains intact throughout, including duplicate occurrences.

Record the last successfully verified remote playlist revision/membership. Check for external edits and partial/unstable inventories before applying a new projection. If remote changes conflict, mark the sync as needing attention rather than silently discarding them. Defer any sync whose snapshot cannot be trusted. Serialize application writes per playlist and verify membership after writing. Mark a placeholder fulfilled only when its file/server identity is valid; mark playlist synchronization separately so a failed playlist write cannot masquerade as success.

## API compatibility and native APIs

Keep the already documented Wavio `/api/v1` contract as an adapter. Expose new Activity, import, placeholder and defer controls through a native `/api/v2` API. Do not force Wavio to understand new enum values or placeholder records to retain its existing functionality.

Planned native resource groups: source libraries and import snapshots; catalog songs/artists/albums/favorites; playlists and entries; candidate matches and review decisions; acquisition intents; tasks/attempts/events; connections/settings. Mutation requests must be idempotent or accept idempotency keys where repeated submission would create duplicate work.

A deferred/failed Wavio acquisition remains visible in the legacy wishlist with compatible failure information and without being marked completed. Do not remove it in a way that Wavio interprets as success. Honor entry/intent deferral when Wavio calls wishlist processing; an explicit resume is required in the native web UI. Preserve known legacy statuses and map richer internal states through the adapter. Validate this with the pinned Wavio release, including queue polling, rate limits, artwork and restart behavior.

Do not create fake playable songs for placeholders in either legacy library responses or Navidrome. Native API responses can return entry metadata, nullable recording/server IDs, reason and allowed actions explicitly.

## Build sequence and acceptance gates

First implement account/session identity, ownership/query scoping, role/grant authorization and the shared acquisition approval gate described in the multi-user design. Include native Requests and People pages in the web milestones below. Every acquisition entry point must enforce authenticated ownership and the shared approval gate.


1. **Model and contract foundation.** Define source libraries/tracks, canonical recordings, ordered playlist entries, files/server mappings, favorite overrides, acquisition intents and persistent tasks. Capture the pinned Wavio contract. Test duplicate source IDs, playlist duplicates and deferred state before wiring downloads.
2. **XML import and library catalog.** Stage complete library exports, preview songs plus playlist memberships, commit catalog records independently of matching and implement idempotent re-import. Verify songs outside playlists and mixed/non-music records.
3. **Acquisition and server integration.** Wrap the existing d-fi binary, add metadata adapters, safe file import, Navidrome inventory/scans and verified playlist projection. Use SoulSync's relevant implementation and regression cases as references.
4. **Failure handling and Activity.** Persist task dependencies/history, partial-success summaries, retry/cancel behavior, Skip for now and deferred placeholders. Verify a failure never blocks unrelated songs, and resuming a placeholder inserts the correct playable entry later.
5. **Dedicated web pages.** Build the collapsible sidebar, Library views, Mixtape-based playlist detail, import routes, search, review routes, Activity and connection settings against native APIs. Navigation and task state must survive reloads; no core workflow should require a dialog.
6. **Wavio and deployment verification.** Run real request/poll flows plus contract tests; verify favorites and cover art with Navidrome; exercise restart recovery, conflicting remote edits, large XML exports and partial provider failures. Package Go, UI assets, SQLite migrations and the pinned d-fi executable for the target platforms.

Required end-to-end scenarios:

- A full XML import includes an unplaylisted song, two playlists sharing a song and a playlist containing that song twice; catalog and ordered membership remain correct.
- Re-importing the same XML creates neither duplicate playlists nor duplicate acquisition jobs; applying an updated XML preserves stable placeholders and manual decisions.
- A metadata lookup fails for B in `[A, B, C]`. The user skips B, leaves the page and restarts the service. A/C complete, B retains its position and deferred policy, and Navidrome contains A/C.
- The user resolves and resumes B later. After successful file import and a Navidrome scan, the application verifies A/B/C on the server. Repeating the completion event does not add a second B.
- File import fails after a correct match. Retry resumes acquisition/import work without discarding the match; other songs complete and Activity reports the issue accurately.
- Skipping one playlist occurrence does not cancel a shared download still needed elsewhere. Deferred placeholders are not automatically retried by source refresh or Wavio processing.
- A deferred Wavio request does not disappear as a false success. Native placeholders never appear as fake playable tracks.
- Running work started by Wavio appears on Activity in another browser, continues after the originating browser closes and remains inspectable after restart.
- Favorites from imported data survive resolution and re-import without overwriting a newer explicit user change.
- An external Navidrome playlist edit or failed scan prevents an unverified destructive projection update and produces an actionable sync issue.

The previous single-user assessment is a preliminary estimate, not a fixed commitment for this expanded plan. Multi-user identity, approval requests and permission/isolation tests add foundational scope. Dedicated library pages, unified Activity and durable deferred placeholders are required v1 work. Refine the estimate against these acceptance gates before implementation scheduling; do not treat them as cosmetic adjustments to the mockups.
