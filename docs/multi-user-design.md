# Multi-user requirements and access model

Confirmed user decision: all accounts share the base music library and downloaded files. Playlists, favorites, imports and other personal collections belong to individual users. New music requested by a regular user requires administrator approval unless that user is explicitly granted permission to add music directly. Non-admin users cannot delete song files, including users allowed to download without approval.

This document supplements `implementation-plan.md`; it replaces the earlier single-profile assumption. These are implementation requirements, not implemented application security.

## Accounts, identity and permissions

- Separate application accounts with individual sessions. Use an established password/session implementation, protected cookies, CSRF defenses for cookie-authenticated mutations, rate limits and explicit session revocation. Admin creates/invites accounts; public registration is off initially. First-run setup creates an administrator. Prevent disabling/deleting/demoting the last active administrator.
- Two initial roles: **Admin** and **Member**. Members may browse the shared library, maintain their personal collection, and request music. The independent permission **Add music without approval** defaults off. Only admins may grant or revoke it.
- File deletion is admin-only. A member granted direct acquisition is still forbidden from deleting files. No file deletion controls should appear in member views, and the server must reject direct API calls attempting deletion. Removing a personal playlist, removing its entry, withdrawing a request, or unfavoriting a song never removes shared audio.
- Admin-only settings include shared provider credentials, music storage, users/permissions, request decisions and service configuration. Members manage their profile, own sessions/API keys and personal Navidrome connection mapping. Credentials and hashes are never returned in normal configuration responses or logs.
- Authenticate every request and authorize the resource/action on the server. A route, hidden button, user ID, profile header, SSE topic, URL or client-side role is never an authorization boundary.

## Shared versus personal data

Shared: canonical music metadata, downloaded files, safe public provider caches, storage usage and library availability. A newly approved/imported recording becomes available to everyone who can access the common library.

Personal: playlists and their ordered entries/placeholders; favorites; XML source libraries and imports; followed artists; acquisition intents; deferred/review decisions; API keys and credentials; personal history/preferences. Scope these by owner user ID. The same source persistent ID in two users' XML files does not make them the same source record.

A shared recording may satisfy several personal acquisition intents. Preserve separate requester, playlist positions and deferred policies even when sharing one download job/file. Explicit playlist sharing may grant view or edit access; private is the default. Shared edit access does not grant file deletion, user administration or acquisition bypass. Record administrative inspection/actions in audit history rather than implying personal data is inaccessible to administrators.

Favorites are per user, never global. Use that account's Navidrome stars and playlist identity when available. The app uses a common music library, but server IDs and visible inventories must be interpreted through the linked identity; never grant a member access to a private remote playlist simply because the app has an admin connection. Keep application login and Navidrome account linking separate. Do not distribute the shared administrative Navidrome credential to member clients; link them to individual server accounts with appropriate rights, including no file-deletion rights. Source import and private playlist names must not leak into another member's Activity or search responses.

## Requests and approval

A music request is durable acquisition intent owned by its requester. It can target a recording, album or a selected set of missing playlist songs. It stores source/provider identifiers and matching evidence, target quality, requester, submitted time, linked personal entries, decision actor/time/reason and processing outcomes.

1. Re-check library availability before submission. Existing music needs no approval/download; attach the existing recording to the requesting user's collection.
2. For a member without bypass permission, create **Awaiting approval**. Do not enqueue a download or run acquisition merely because the request came through a different endpoint/client. Catalog lookup and importing personal XML metadata can occur without acquisition approval.
3. Admin sees a Requests inbox, requested recording/release, requester, quality, existing coverage and all unresolved items. Approve, decline with a reason, or return a request for more detail. Review decisions use a dedicated page. Bulk approval does not silently approve unmatched/substituted recordings.
4. Admin approval records an audit event and creates or joins a deduplicated acquisition job. Concurrent approvals and retries must not start duplicate downloads. Direct-acquisition users enter this stage immediately, while retaining the same request/audit trail with the reason “Allowed without approval.”
5. Track Queued → Downloading → Importing → Waiting for library scan → Available, with Failed and Deferred outcomes as appropriate. “Approved” does not mean playable. Each personal playlist retains placeholders until the recording is available and its playlist projection is verified.
6. Declined, unavailable or unmatched requests retain their source/playlist slot. The owner may Skip for now, update and resubmit, or explicitly remove their entry. Declining is not file deletion or removal from every playlist.
7. Requesters can inspect/withdraw their own pending requests. A withdrawal does not cancel a shared job still needed by someone else. Disallow arbitrary edits of already approved targets; a changed target requires a new approval where applicable.

Permissions are evaluated at submission and immediately before acquisition starts. Revoking bypass permission moves not-yet-started bypass-only work back to approval; do not revoke an administrator's explicit approval implicitly. Define active-transfer handling clearly: stop new downstream acquisition where safe, retain completed valid files and report any already committed work. Approval, cancellation and grants/revocations must be transactional and audited.

Apply the approval gate to UI actions, playlist downloads, XML missing-library acquisition, schedules, followed-artist release downloads, Wavio wishlist processing, request endpoints and future providers. There must be no “process wishlist” or album/playlist bulk bypass.

## Wavio and API keys

Each API key belongs to an account, has bounded scopes and can be revoked. It cannot exceed the owner's current permissions. Preserve the legacy `/api/v1` contract through the adapter, but resolve ownership from the authenticated key. Treat `X-Profile-Id` as a compatible selector only among profiles that key can access; it must not permit impersonation or default unknown users to the admin profile.

Wavio requests from a member without bypass permission create approval requests, not download jobs. Keep pending intent visible in the wishlist and return compatible nonterminal status plus an actionable reason such as “Awaiting administrator approval.” Never let removing a wishlist row or dropping a task make Wavio infer a false download success. Wavio may not expose an approval screen; the dedicated web Requests page is the source of decision details. Pin and test the actual client behavior. Its existing interfaces must remain usable even if an admin now needs to approve acquisition.

The native API adds authenticated Requests, users/permissions and sharing resources. Do not accept owner IDs or arbitrary permission changes from ordinary member payloads. Redact requests/jobs outside the caller's scope in paginated lists, counts, search, exports, task errors, events and SSE reconnect/backlog data.

## Navigation and compact interaction rules

- Requests is a dedicated sidebar destination. Members see My requests. Admins get an approval inbox plus filtering by requester/state. Activity links to request decisions but does not replace the approval workflow.
- People & permissions is a dedicated admin Settings page. Show role, linked Navidrome identity and the independent Add music without approval permission. State explicitly that members never delete shared files.
- Use compact song lists: reduce vertical padding, align status/actions, use about 42–46px ordinary desktop rows and allow additional height only for important unresolved reasons/actions. Keep text legible and retain larger touch targets on phones/coarse pointers. Compactness must not rely only on reducing font size.
- All ordinary workflows remain dedicated routed pages. Keep the chosen collapsible sidebar and Mixtape-inspired playlist view. User/account state is visible in the header.

## Activity, jobs and fairness

Members see their own work and authorized shared work; admins can switch between their own view and a server overview. Expose shared job progress without other users' private playlist/source details. Requests Awaiting approval are waiting decisions, never Running downloads. Review and defer decisions do not hold worker slots.

Persist requester/subscriber relationships and parent/child jobs separately. Use fair scheduling, global/per-user concurrency and request limits. A member cannot starve others or bypass limits with different API keys. Cancelling one user's intent does not cancel another subscriber's work; admin forced cancellation is explicit and audited.

Disabling an account revokes sessions/API keys, prevents new work and handles that user's unstarted intents without removing shared files. Deleting an account requires an explicit disposition for personal data and shared playlists; never cascade into audio-file deletion.

## Schema and implementation changes

Add users, roles/grants, sessions, user-owned API keys, per-user Navidrome links, playlist ACLs, personal favorites, owned source libraries/import snapshots, music requests and decision events. Add owner/requester/actor relationships to intents, jobs, events and audit records. Keep shared recording/file tables separate from personal records and avoid cascading deletes across that boundary.

Build authorization/query scoping and approval service before connecting external acquisition workers. Acquisition providers receive already-authorized job inputs; they do not decide authorization themselves. Add account bootstrap/invites and native Requests pages before declaring multi-user v1 usable.

Required tests:

- Two users import the same XML/source IDs and favorite different songs without data collision or cross-user leakage.
- A member can request music and edit personal playlists but cannot start acquisition before approval, delete files, alter grants or act as another profile via URL/body/header/API key.
- A member with Add music without approval can download, yet all file-deletion paths remain denied.
- Wavio wishlist/process, album bulk requests, source refresh, follow schedules and direct API calls pass through the same approval gate.
- Two requests for one recording share one acquisition while maintaining separate approval, ownership and playlist entries.
- Approve twice, approve concurrently, retry and reconnect do not duplicate downloads. Decline/withdraw do not report false success or destroy placeholders.
- Revoking bypass permission rechecks pending work. A request carrying previous admin approval is distinguished from a bypass-only job.
- A user withdrawing/defering their entry does not cancel another user's request or erase shared files.
- List counts, task details, errors and SSE replay do not reveal another member's private sources, playlists or credentials.
- Account disable revokes access; account/playlist deletion cannot cascade into shared audio. The last admin remains recoverable.

The previous single-user time estimate is superseded. Multi-user identity, approval, authorization and isolation are foundational work and must be estimated explicitly before implementation scheduling.
