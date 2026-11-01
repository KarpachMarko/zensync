# Multi-user requirements and access model

Updated 2 November 2026. This document supplements the authoritative [implementation plan](implementation-plan.md). These are requirements, not implemented security. Phase 1 uses one admin-configured Navidrome server and a common audio library, with separate accounts for personal collections. ZenSync owns migration/acquisition state; Navidrome owns ongoing playlists and favorites.

## Accounts and server connections

- Separate ZenSync accounts and individual sessions. Admin creates/invites accounts; public registration is off initially. First-run setup creates an administrator. Prevent disabling/deleting/demoting the last active administrator and provide a documented recovery procedure.
- Use an established password/session implementation, protected cookies, CSRF defenses for cookie-authenticated mutations, rate limits and explicit session revocation. Authenticate requests and authorize resource/action access on the server.
- One Navidrome server URL and shared service connection are configured by the admin. All intended users receive access to the same Navidrome music library; separate accounts do not require duplicate audio files. The service connection supports shared inventory, scan and acquisition verification.
- Users link their own account on that configured Navidrome server to publish migration playlists and favorites. Search and acquisition requests need no personal link. Linking does not create a second server configuration or replace ZenSync login. Native Navidrome account provisioning is not a phase 1 requirement; link existing accounts.
- Verify the personal login and target identity before publication. Store credentials securely, scoped to their owner. Never distribute the shared service credential to clients or fall back to it when a personal link is missing, expired or fails. Pause affected migration writes and ask the owner to repair the link.
- Wavio and other players connect directly to Navidrome with their own user credentials. ZenSync's API key is a separate credential used for acquisition requests.

## Roles, permissions and quality settings

Two roles: **Admin** and **Member**. Members browse the common library, manage their migrations and quality preferences, and request music. **Add music without approval** is an independent grant, off by default; only admins grant/revoke it.

Shared credentials, server/storage configuration, account/grant administration, request decisions and file deletion are admin-only. Members manage their profile, own sessions/API keys, personal Navidrome link and ordered quality profile. Credentials/hashes are never returned in normal responses, logs or configuration exports.

Quality profiles order supported allowed qualities. Default: lossless first, followed by lower qualities in descending preference. Per-user preferences do not isolate or duplicate shared files. Ordinary requests reuse a playable matching recording already present; explicit **Upgrade** is required to seek better quality. Requests store a quality-profile snapshot so later preference changes do not silently modify approved work.

Members, including those with direct-acquisition permission, cannot delete shared files. An upgrade that replaces a shared asset requires explicit admin approval. Keep the old file until the new asset and affected Navidrome references are verified; failed verification must not destroy the old asset. Removing an entry, withdrawing a request, deleting an import or changing a favorite never deletes shared audio.

A route, hidden button, user ID, profile header, SSE topic or client-side role is never an authorization boundary. Do not accept owner IDs or permission changes from ordinary member payloads.

## Shared versus personal data

Shared: canonical/provider music metadata that is safe to publish, downloaded assets, common-library availability and safe provider caches. Acquired music becomes available to all users with access to the common Navidrome library.

Personal in ZenSync: XML source records/snapshots, migrations and ordered entries, review/defer decisions, imported favorite intent/outcomes, acquisition requests, quality preferences, credentials/API keys, personal history and settings. Phase 2 artist follows/notifications are also personal. Source persistent IDs are scoped by owner and source snapshot; never merge users' source records because identifiers match.

Personal in Navidrome: playlists, favorites and listening state under each linked account. ZenSync stores migration correspondence and verified remote baselines, not a continuously synchronized personal collection. No separate collaborative playlist ACL/editor system is required in ZenSync phase 1; ongoing sharing/editing is handled by Navidrome/clients.

A shared job/file can satisfy multiple personal requests. Keep requester, linked entries, decision actor, quality requirements and deferral independently. Do not expose other subscribers' private playlist names or import metadata. An admin's broad remote visibility does not authorize returning private remote collections to members. Record administrative inspection/actions in audit history rather than claiming personal data is inaccessible to administrators.

## Migration write authority and handover

Personal linking is required for remote writes, not for parsing or previewing XML. Create migration playlists and import explicit favorite intent as the linked user. A migration's target identity must not be silently changed while work is running; changing a link requires pausing and reviewing affected writes.

While open, migration may fill missing positions and apply pending favorite imports. External playlist edits pause writes to that playlist; preserve the remote version. The owner can explicitly resume after reviewing differences or hand that playlist over to Navidrome. Never infer an external deletion is permission to recreate a playlist.

Finishing the migration hands over all remaining collections, including favorite import. Fence queued/in-flight writes before acknowledging handover. Late acquisitions may update the migration report but cannot insert playlist entries or star songs after handover. Imported favorites must not repeatedly override a user's later Navidrome changes.

Outstanding acquisition requests continue under their existing approval and defer rules after handover. Finishing is not cancellation, approval or a request to acquire every unresolved song. Do not cancel a shared job that another user still needs.

Approved asset replacement may require a narrow reference-preservation operation after handover. It must preserve the same recording, playlist membership/order and favorite intent; it is not ongoing collection synchronization. If the implementation cannot preserve references safely across affected users, keep the old asset and require review rather than assuming the admin connection can edit every private playlist.

## Requests and approval

A request is durable acquisition intent owned by its requester, distinct from execution jobs/attempts. Targets may be a recording, album or explicit selection of missing migration songs. Bulk requests retain item-level targets, approval and outcomes. Store source/provider IDs, matching evidence, quality snapshot, requester/time, linked migration entries, decision actor/time/reason and fulfillment outcome.

1. Re-check common-library availability. An existing playable file of the correct recording/version satisfies an ordinary request without acquisition approval or an automatic upgrade. Linking that file into a migration still obeys its write gate.
2. For members without the grant, create **Awaiting approval**; do not enqueue acquisition. Metadata search, XML staging and identity resolution do not themselves need acquisition approval.
3. Admin reviews the concrete target/version, quality preference, coverage and unresolved children. Approve, decline with a reason, or return for clarification. Bulk approval does not approve unknown substitutions.
4. Approval records an audit event and transactionally creates/joins compatible deduplicated work. Direct-acquisition users follow the same request/audit trail with the reason “Allowed without approval.” Upgrade replacement always requires explicit admin approval, regardless of that grant.
5. Execution reports Queued → Downloading → Validating/Importing → Waiting for scan → Available, with stage failures and retry history. Approval is not playable availability; availability is not proof of a successful migration playlist write.
6. Decline/failure retains the request and source position. Owners may defer, correct and resubmit, or remove their migration entry without deleting shared files. Alternative versions require explicit review; a changed approved target needs fresh approval where applicable.
7. Requesters can withdraw pending requests. Withdrawal/defer removes only that subscriber's need for work. It must not cancel another user's acquisition or become false download success in Wavio.

Permissions are checked at submission and immediately before acquisition begins. Revoking the direct grant returns not-yet-started bypass-only work to approval, while preserving explicit admin approvals. Active workers stop further unauthorized work at a safe boundary; retain/report valid committed files and never release a claim while the same worker can still commit. Approval, cancellation and permission changes are transactional and audited.

Skip for now stops new acquisition/retry work for the affected intent. A confidently matched existing song may satisfy it later without Resume, including when another user acquires the song. No new acquisition is triggered by this reuse. Insertion/star application still obeys migration handover and external-edit gates.

All entry points use the same approval service: native UI/API, XML acquisition selections, album bulk requests and Wavio wishlist/process. Phase 2 notifications only suggest requests, never auto-download. Future acquisition integrations must use this gate too.

## Wavio and native APIs

Every ZenSync API key belongs to an account, has bounded scopes and can be revoked. Its authority cannot exceed current owner permissions. Preserve the supported `/api/v1` acquisition contract through an adapter; `X-Profile-Id` selects only authorized profiles and cannot impersonate another user or default to admin.

Wavio requests from members without bypass create approval requests, not download jobs. The user accepts **Queued** in unchanged Wavio; ZenSync's Requests page provides approval details and rejection reasons. Do not promise a distinct approval screen/status in Wavio. Keep unsuccessful intent or compatible terminal information visible so wishlist removal/task expiry cannot imply false success. Test withdrawal, decline, deferred fulfillment, duplicate titles and terminal retention with the pinned client.

Watchlist/release notifications are phase 2. Phase 1 must explicitly document unsupported watchlist operations and test that they do not break supported connection/search/acquisition flows. Do not fabricate scan success to claim full compatibility.

Native `/api/v2` includes owned migrations, review, requests/item decisions, quality profiles, user/grant/key management, tasks and connections. Scope every list, count, search result, error, export and SSE snapshot/replay. Global catalog/search must not expose private imported metadata just because a source record may later match common music.

## Activity, fairness and account lifecycle

Members see their own work and safe shared progress. Admins can inspect a server overview. Approval waits, external-edit pauses and deferred reviews do not occupy transfer slots. Keep shared subscriber relations separate from parent/child task structure. Apply global/per-user concurrency, request and staging limits; changing API keys must not bypass limits.

Disabling an account revokes sessions/API keys and prevents new work. Pause its migration writes and remove its authorization for unstarted personal work without deleting shared assets or cancelling another subscriber's job. Deletion requires an explicit disposition for personal migration/request records and any remote collections; no automatic cascade into Navidrome playlists, favorites or audio. Retain necessary audit records and preserve the last admin's recoverability.

Requests and People & permissions remain dedicated pages. Show current role, personal Navidrome link and direct-acquisition grant. Keep compact rows, accessible mobile controls and dedicated routes consistent with the implementation plan. Mockup role simulation is not production authorization.

## Schema and implementation gates

Model users, roles/grants, sessions, owned keys, one shared server configuration, per-user Navidrome links, ordered quality profiles/snapshots, owned migrations/source records/entries, imported favorite outcomes, requests/item decisions and audit events. Shared recording/release/file/server-mapping tables are separate from personal data. Persist jobs, attempts, dependencies and subscriptions. Avoid cascading deletes across the personal/shared boundary.

Implement scoped queries and approval before connecting external acquisition workers. Workers receive authorized inputs but must recheck current permission and migration write gates at side-effect boundaries. User provisioning, Requests and People pages are required before declaring multi-user phase 1 usable.

Required acceptance tests:

- Two users import identical source IDs and identically named playlists; their playlists and imported favorites land under different linked Navidrome accounts while the shared audio is stored once.
- A user without a personal link can search/request music, but cannot publish migration collections. An expired/broken link never falls back to the shared account.
- Members cannot acquire before approval, delete files, replace shared assets without admin approval, alter grants or impersonate through body/URL/header/key.
- A direct-acquisition member can acquire new music but still cannot approve replacement or deletion.
- Per-user ordered profiles remain separate; fallback is reported and bounded by allowed qualities. Existing audio is reused without an explicit upgrade. Profile edits cannot silently alter pending approved targets.
- All acquisition entry points enforce approval. Concurrent approval/retry creates one compatible job, not duplicate downloads; incompatible target/quality requirements are not silently combined.
- Grant revocation rechecks unstarted work and distinguishes explicit admin approval from bypass-only authorization.
- Defer/withdraw affects only its subscriber. A confident existing-file match can fulfill a deferred entry without a new download, while paused/handed-over remote collections stay unchanged.
- External edits pause affected migration writes; handover prevents queued, late and replayed playlist/favorite writes while pending acquisitions continue under their normal rules.
- Wavio queued approval, decline, withdrawal and terminal history do not imply false success. Unsupported phase 1 watchlist actions are explicit.
- Private imports, source names, favorite intent, keys and credentials never leak through lists, aggregate counts, errors or SSE reconnect/replay.
- Account disable revokes access; account/import deletion cannot cascade into shared files or remote personal collections. Last-admin recovery and backup/restore are tested.

Estimate foundational identity, approval, migration write gating and isolation explicitly. The original single-user estimate is historical and is not the phase 1 schedule.
