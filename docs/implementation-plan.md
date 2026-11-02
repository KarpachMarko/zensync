# ZenSync implementation plan

Status: first XML-to-Navidrome migration workflow implemented, including explicit missing-song acquisition with d-fi 2.3.5. Real Navidrome integration and controlled acquisition tests pass; authenticated live Deezer verification and the remaining phase-1 scope are pending. See [deployment notes](deployment.md) for the exact implemented scope. Updated 2 November 2026 after the product-decision review. This document and [multi-user design](multi-user-design.md) are authoritative. The assessment and existing mockups are historical references where they describe broader or different behavior.

## Product purpose and ownership

ZenSync is a self-hosted companion for migrating music into Navidrome and acquiring additional music afterward. Use Go, SQLite, a compact React/TypeScript UI and the user's existing working d-fi Go binary for Deezer acquisition. Apple/iTunes and Deezer supply metadata.

One Navidrome instance is configured by the administrator for all ZenSync users. Users share the base audio library, but have separate Navidrome accounts for personal playlists, favorites and listening state. Wavio and other players connect directly to Navidrome for browsing, playback and collection edits. ZenSync is not a playback server or an OpenSubsonic gateway.

Navidrome is the source of truth for personal collections. ZenSync has limited permission to populate migration playlists and imported favorites while a migration is open. After explicit handover, it stops writing those collections. Normal ongoing use is search → request/approval → acquisition → Navidrome indexing; users organize newly acquired songs through Navidrome or Wavio.

Keep application login separate from Navidrome account linking. The shared service connection supports common-library lookup, scans and acquisition verification. Personal linking is required to publish migration playlists and favorites into the correct user's account, but is not required for search or acquisition requests. Never fall back to the shared account for a personal migration write.

## Confirmed phase 1 scope

- Apple Music XML migration only: full exported music catalog, playlist membership, and favorite/loved information when present. Also accept individual playlist XML exports.
- One-time migration from a fixed source snapshot, not persistent Apple synchronization. No source refresh schedules or updated-XML merge engine.
- Matching, manual review, persistent unresolved migration entries and **Skip for now**. Importing metadata never automatically downloads the whole library.
- Search for tracks, albums and artists; acquisition through d-fi; safe file placement and verified Navidrome availability.
- Admin approval for member acquisitions by default, with an independent **Add music without approval** grant. Members cannot delete shared audio.
- Per-user ordered quality profiles, explicit quality upgrades, and admin-approved replacement of old files.
- Persistent requests/tasks, Activity, retries, cancellation, restart recovery, user isolation and Wavio acquisition compatibility.
- Dedicated routed pages with the chosen collapsible sidebar, compact song rows and Mixtape-inspired migration playlist detail.

Phase 2: followed-artist checks and new-release notifications only. Notifications do not automatically acquire music; an explicit request follows the usual approval rules. Phase 1 does not implement artist monitoring or automatic release downloads.

Outside phase 1: public Apple playlist links; CSV/TSV/TXT/M3U/M3U8 imports; private Apple API access; ongoing Apple synchronization; ordinary playlist/favorites management in ZenSync; collaborative playlist editing; additional media servers; torrents; recommendations; movies, audiobooks and podcasts; playback/transcoding; broad SoulSync API parity. Public links and extra file formats have no committed delivery phase.

## Migration lifecycle

A migration belongs to one ZenSync user and records a fixed Apple source snapshot, target Navidrome account, selected playlists, source songs, ordered entries, imported favorite intent, matching decisions and acquisition links.

1. **Stage and preview.** Parse XML without external DTD/entity retrieval and with bounded size, nesting and memory use. Read source-library and persistent IDs where supplied; track IDs are references scoped to the snapshot, not provider/global identifiers. Preserve original metadata separately from enriched values.
2. **Record the complete music catalog.** Include songs outside every playlist. Preserve playlist folders/parents, repeated occurrences and ordered references. Missing references become explicit placeholders. Exclude identifiable non-music records and report them. Treat special/system playlists explicitly; smart playlists use exported membership only.
3. **Choose migration contents.** Preview total songs, unplaylisted songs, playlist counts, missing references and parse problems. Default to catalog metadata plus normal music playlists, allowing playlist deselection. Allow favorite import when fields exist. Retain source context for unresolved records.
4. **Commit independently of providers.** Save source records and entries before matching. Provider outages must not lose the migration. Retrying the same upload/commit must not create duplicate migrations, playlists or jobs: use owner-scoped upload identity and idempotency keys, and offer to return to the existing migration. This is retry protection, not re-import merging. A changed XML file is a separate explicit migration, never an automatic update to existing collections.
5. **Match and acquire separately.** Automatically accept confident matches of the intended recording/version. Remasters, alternate editions or other substitutions require user review. Resolve can attach existing music without a download. Downloading missing songs is an explicit selection, subject to approval and the quality policy.
6. **Publish and fill while open.** Use the linked personal account to create migration playlists from currently available entries and import favorite intent. Missing songs retain their local positions; successful later acquisitions can fill them while migration remains open and the playlist is not paused.
7. **Finish migration explicitly.** Show coverage, unresolved entries, pending requests and paused playlists. Handover need not wait for all songs. Finish all remaining migration-owned playlists and stop all playlist/favorite writes for that migration. Preserve a report of unfinished items. A per-playlist handover is also available when resolving an external-edit pause.

Outstanding acquisition requests survive handover: approved work continues, pending approval still needs approval, and deferred acquisition remains deferred. Later songs enter the shared library, but are not inserted into handed-over playlists or starred by ZenSync. Users organize late arrivals themselves. Handover is not an implicit retry, approval or download request.

Implementation default: make handover final for that migration rather than automatically reopening it on another upload. Serialize handover with in-flight collection writes, stop scheduling new writes, and acknowledge completion only after any already-started write has settled and been reported. Queued or replayed completion events must recheck the migration/playlist write gate.

## Migration playlist writes and external edits

Use durable migration playlist entries with their own IDs and order keys. Entries reference source metadata and nullable recording/server-song mappings. Never represent the full migration solely as an array of playable IDs.

During open migration, Navidrome receives only available entries with valid server IDs. For `[A, unresolved B, C]`, publish `[A, C]`; when B is available, publish `[A, B, C]`. Preserve duplicate occurrences. Do not create fake songs, silent audio or invalid IDs for placeholders. Show both migration coverage and remote write status.

Store the remote playlist ID, owner, last verified ordered membership and managed metadata. Serialize ZenSync writes per playlist; fetch and compare remote state before changing it, then verify after the write. A partial/untrusted inventory or failed scan cannot authorize a destructive write.

**External edits pause updates to the affected playlist.** This includes remote reordering, additions/removals, renaming or deletion. Preserve the external version and display a dedicated review page. The owner can:

- Finish migration for that playlist, leaving its remote state in control of Navidrome.
- Explicitly resume migration after reviewing the proposed difference and authorizing any replacement of external changes. Do not silently overwrite or attempt automatic merging.

Other migration playlists and acquisition jobs continue. A deleted remote playlist must not be automatically recreated without a user decision.

The standard remote write contract does not provide a conditional revision write. Compare-before-write and read-back verification reduce risk but cannot guarantee exclusion of an external edit during the write. Preserve prior snapshots and proposed writes for recovery, surface detected divergence, and do not claim atomic cross-client conflict prevention. Verify actual behavior against the pinned Navidrome/Wavio versions in the integration proof.

## Imported favorites

Favorites belong to the linked Navidrome user. During open migration, apply explicit favorite/loved information to identified available songs. An absent XML field never means unfavorite. No continuous two-way favorite synchronization or general favorite editor is needed in ZenSync.

Implementation default: import positive favorite intent once per source song, with durable attempt/outcome tracking. Do not repeatedly re-star a song after the user unfavorites it in Navidrome. If a remote write outcome is uncertain after a crash, reconcile or surface review instead of blindly overwriting possible user changes. Newly available songs can receive their pending imported star only while migration is open. After handover, stop all imported favorite writes, including retries.

## Identity, Skip for now and state boundaries

Preserve source song identity separately from canonical recording, release/provider track identity, file asset and Navidrome mapping. Provider references are provider-qualified. Match using corroborating evidence such as version, duration, album and identifiers; a title/artist collision or ISRC alone is not proof of the intended edition. Keep matching evidence and reversible manual decisions. Corrections are scoped to the owner's source records unless an explicit broader correction is authorized.

**Skip for now defers work, not reuse of existing music.** It stops new acquisition/retry work for that entry and survives restarts. It does not cancel work needed by another subscriber. If a confidently matched song later exists in the shared library, the entry may fill automatically without Resume or a new download. Playlist insertion still requires an open migration and an unpaused playlist. Alternative versions still require review. After handover, update the report/library availability only.

Resolving a match also resumes already-requested work, subject to approval, without a second Resume click. Resolving metadata alone must never create an acquisition request. If the approved target changes, re-evaluate approval before acquisition. An import failure retains the accepted identity; retry the failed stage rather than forcing a fresh match.

Use separate persisted states for:

- Identification: unresolved, ambiguous or resolved, with evidence and chosen identity.
- Acquisition request: decision (awaiting approval, approved, declined, withdrawn), acquisition policy (active/deferred), target/profile snapshot and fulfillment outcome.
- Execution: queued, downloading, validating/importing, awaiting scan, available, failed or cancelled; attempts and errors are separate records.
- Migration: staged, open or handed over; each playlist additionally has active, external-edit-paused or handed-over write state.
- Projection: desired membership versus last verified remote membership. File availability and playlist write success are distinct.

A request is durable acquisition intent, not a worker task. Bulk requests have item-level targets, approvals and outcomes; unresolved children cannot become implicitly approved substitutions. Jobs can serve multiple requests while each request retains its owner, policy and approval. Skipping/removing one entry never suppresses other subscribers or deletes audio.

## Quality profiles and explicit upgrades

Users select/configure an ordered list of allowed qualities, similar in purpose to SoulSync quality profiles. The default prefers lossless, then allows supported lower qualities in descending preference. Pin the exact d-fi quality mapping during integration; do not assume every advertised quality is available for every recording or account.

Try allowed qualities in order and report the actual acquired quality and any fallback. Never silently use a quality excluded by the selected profile. Snapshot profile settings on submission/approval so editing preferences does not silently change pending approved work; an explicit request update re-evaluates authorization where needed. Deduplicate only when a shared job's target/version and quality outcome can satisfy its subscribers.

An existing playable file of the correct recording/version satisfies an ordinary request regardless of a preference for higher quality. Profile changes do not automatically upgrade the library. A user must explicitly choose **Upgrade**.

A successful upgrade replaces the old file only after validating the new asset and reconciling affected Navidrome references. Replacement requires explicit admin approval even for members with direct-acquisition permission, because it removes shared audio. Keep the old file recoverable until the replacement has been verified. If reference preservation cannot be demonstrated, stop replacement and retain the old asset for review.

This is a narrow maintenance exception to the post-migration collection-write boundary: preserve references to the same recording during an approved asset replacement, without changing playlist membership/order or favorite intent. Prefer stable server identity; prove reference handling across users before enabling replacement. Never use the shared admin identity to assume ownership of private collections. Replacement feasibility is an early integration gate, not a promise that IDs survive every file change.

## Library and dedicated web pages

Keep the Collection-inspired collapsible sidebar and Mixtape-inspired playlist sleeve/ordered rows. Ordinary desktop song rows target about 44px, with additional space for unresolved reasons and comfortable mobile touch targets. Persist sidebar preference locally. Mobile navigation must manage focus and close after navigation.

All normal workflows use dedicated URLs, browser history and reload-safe state. Use dialogs only for short confirmations. Phase 1 sidebar: Library, Search, Imports, Requests, Activity and Settings. Migration playlist detail is reached from Imports. Do not build a competing ongoing playlist/favorites editor. Followed artists is a phase 2 destination.

| Page | Example URL | Purpose |
| --- | --- | --- |
| Library / Songs, Artists, Albums | `/library/songs`, `/library/artists`, `/library/albums` | Shared availability, metadata, acquisition/upgrade actions |
| Song / Artist / Album | `/library/songs/:id`, `/library/artists/:id`, `/library/albums/:id` | Identity, availability and relevant acquisition state |
| Search | `/search?q=...&source=deezer` | Provider-specific track, album and artist results |
| Imports / New migration | `/imports`, `/imports/new` | XML migration history and upload |
| Migration preview | `/imports/:id/preview` | Complete catalog and playlist selection |
| Migration detail | `/imports/:id` | Coverage, review, favorite import status and handover |
| Migration playlist | `/imports/:id/playlists/:playlistId` | Ordered entries, placeholders, external-edit pause and handover |
| Review item | `/review/:id` | Candidate evidence, Resolve, Retry and Skip for now |
| Requests / Request detail | `/requests`, `/requests/:id` | Own requests or admin approval inbox and decisions |
| Activity / Task detail | `/activity`, `/activity/:id` | Running, queued, needs attention and history |
| Settings | `/settings/connections`, `/settings/quality`, `/settings/people` | Shared connections, personal linking/quality, admin user management |

Show available shared music separately from owner-private imported/unavailable source records. Unplaylisted imported songs remain visible to their owner. Do not leak another user's XML metadata through global search/groupings. Availability, identification, deferral and failure are separate concepts in filters and responses. Imported metadata is not evidence that a playable file exists.

## Activity, persistence and file handling

Persist tasks, attempts, dependency edges and events in SQLite from the first executable slice. Activity covers downloads, metadata lookup, XML parsing/matching, validation/tagging/placement, scans, migration writes and approved upgrades. Show actual stages and indeterminate progress when totals are unknown. Approval waits are decisions, not running transfers. Deferred review holds no worker slot. Bulk counts distinguish completed, failed, deferred and pending work.

Use bounded worker pools, per-provider rate limits, global/per-user fairness and coalesced scans. Use transactional job claims, leases and stale-worker commit guards. Reconcile filesystem and remote state before retrying side effects; process exit is not proof of playable availability. A worker must acknowledge a safe cancellation boundary before releasing its claim. Browser closure must not stop work. Native live updates use SSE with authorized snapshots/replay; Wavio keeps polling.

Implementation defaults for the first release:

- One service process owns API, workers and scheduling infrastructure; local-disk SQLite with WAL, short transactions and migrations. Do not place the database on a network share. Music storage may use a mounted path with explicitly configured local/server path mapping.
- Stage each download outside the watched music library. Validate audio, metadata/artwork and output paths before publication. Invoke the pinned d-fi binary with argument arrays; protect credentials and redact output. Do not treat an XML location as permission to read arbitrary host files.
- Do not overwrite existing files based on filename alone. Use stable asset identity, collision handling and an import journal. For cross-filesystem placement, copy into a temporary destination and validate before final publication; do not assume a move is atomic across mounts.
- Check disk headroom, bound staging/log/event retention and retain recoverable failures under an explicit cleanup policy. Missing/unmounted storage must not be interpreted as an empty library or a reason to remove remote entries.
- Provide health/readiness, structured redacted errors, safe database/config/key backup and a documented restore path. Validate migrations and restart recovery before release. Protect stored personal credentials independently of normal configuration exports.
- Initial packaging target: Docker on Linux, with supported architectures determined by the pinned downloader/tooling. Other native deployments are not a phase 1 commitment.

Operational limits and supported versions are implementation choices to record during the proof. Establish a representative large XML fixture and numerical import-memory, query-latency and concurrency budgets before the catalog/UI milestone; the earlier estimate is not a release commitment.

## Wavio adapter and native APIs

Keep `/api/v1` as a compatibility adapter for the phase 1 Wavio acquisition flow: readiness, search, wishlist/process, downloads and recent additions/artwork. Wavio talks directly to Navidrome for ordinary playlist/favorite operations. No client change is required for approval details: **Queued in Wavio is acceptable; the ZenSync Requests page explains approval status and rejection reasons.**

Scope keys to authenticated ZenSync users and authorize profile selectors; never default an unknown profile to admin. Failed/deferred/declined intent must not disappear in a way Wavio interprets as success. Test withdrawal and terminal-task retention explicitly. Avoid exposing internal enum values or fake playable placeholders. Existing music may satisfy deferred intent without starting acquisition; report completion only after validating availability.

Phase 1 does not claim the original full Wavio watchlist contract. Determine the pinned client's handling of unavailable watchlist operations in the proof, provide an explicit unsupported response where appropriate, and document the limitation. Do not fabricate successful scans or require phase 2 artist checks for phase 1 release.

Native `/api/v2` resources cover catalog availability, owned migrations/snapshots/entries, review decisions, requests and item approvals, quality profiles, tasks/attempts/events, users/grants, keys and connections. Mutations that can duplicate work accept idempotency keys. Restrict native playlist/favorite writes to the migration lifecycle and the separately approved reference-preserving upgrade path.

## Build sequence and acceptance gates

1. **Decisions, fixtures and protected foundation.** Pin d-fi, Wavio and Navidrome versions; obtain representative XML and contract fixtures. Define ownership, identity, request/approval, migration write gates, quality snapshots and durable task schema. Implement bootstrap/login, scoped queries and approval before wiring acquisition workers.
2. **Real integration proof.** Exercise member request → approval → d-fi → validated placement → Navidrome scan → verified availability → Wavio completion. Add two linked users, a migration playlist and a favorite. Restart and cancel at side-effect boundaries. Prove duplicate/version matching, external-edit detection, handover fencing, quality fallback and upgrade reference preservation. Unsupported Wavio watchlist behavior must be understood here.
3. **XML migration and catalog.** Implement staged complete-library ingestion, preview, ordered entries, manual matching, skip/reuse, favorite import, external-edit pause and explicit handover. Retry the same snapshot idempotently; do not build source-refresh merging. Validate the agreed large-library performance budgets.
4. **Complete acquisition and web workflows.** Build dedicated Search, Library, Imports, review, Requests, Activity and Settings pages against the native API. Complete per-user quality settings, approval inbox, direct-acquisition grants, explicit upgrade approval and safe replacement. Activity and recovery already exist; this milestone exposes them fully.
5. **Release verification and packaging.** Run real Wavio/device flows and isolation tests; test expired credentials, disk-full/mount failures, scan delays, conflicting remote edits, partial writes, backup/restore and migration upgrades. Package the service, UI, database migrations and pinned executable/tooling. Re-estimate against evidence from the proof rather than the historical single-user schedule.
6. **Phase 2, separately scheduled.** Follow artists, check for new releases and notify users. Downloading remains explicit and subject to normal approval.

Required end-to-end acceptance scenarios:

- Full XML includes unplaylisted songs, mixed content, missing references, shared songs across playlists and duplicate occurrences. Source data and order survive partial provider failure.
- Retrying an upload/commit creates no duplicate migration, remote playlist or acquisition. A changed XML cannot silently modify a previous migration.
- Importing metadata or resolving identity without acquisition intent starts no download. Substitutions require review; changed approved targets require renewed approval where applicable.
- In `[A, B, C]`, skip B and restart. A/C complete. A later confident existing-library match for B fills it without Resume or new acquisition while migration is open. Paused/handed-over playlists are not changed.
- Two users share one acquisition/file while source records, decisions, playlists, stars and quality preferences remain personal. Identical source IDs or playlist names do not collide.
- Wavio member requests remain queued pending approval; decline, defer and withdrawal never become false download successes. Library availability, cover art and terminal polling work on the pinned client.
- External changes, including playlist deletion, pause only the affected migration playlist. Explicit resume previews consequences; per-playlist handover preserves external edits.
- Finish migration with pending requests. Existing requests continue under approval/defer rules, but late completion, restarts and replayed events cannot write playlists or favorites after handover.
- Favorite import is owner-scoped, never infers unstar from absent data, and does not repeatedly undo an external unstar. Uncertain write outcomes are actionable.
- Default profile falls back through allowed qualities and reports the result; disallowed qualities are never silently chosen. Existing audio is reused until an explicit Upgrade request.
- Direct-acquisition permission does not authorize replacement/deletion. Approved upgrade validates new audio and preserves affected references before retiring the old file; failure retains a recoverable old asset.
- Crash tests cover download, validation, placement before database commit, scan, remote playlist/star write, handover and replacement. Repeated events do not duplicate assets or entries.
- Authorization, revocation, private counts/errors/SSE replay, safe credential handling, recovery and backup/restore pass the [multi-user acceptance criteria](multi-user-design.md).
