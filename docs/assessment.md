# Music-only SoulSync rewrite assessment

Historical assessment from 1 November 2026. The [implementation plan](implementation-plan.md) and [multi-user design](multi-user-design.md), revised 2 November, supersede its product scope, build order and estimates. Phase 1 now uses Apple XML migration with explicit Navidrome handover, followed by acquisition-only operation; it excludes public-link/file-format imports and ongoing collection synchronization. Artist release notifications move to phase 2 without automatic acquisition. Retain the source investigation below as reference, not as the current delivery contract.

Investigated 1 November 2026; updated with the user’s d-fi and XML-import decisions. Target: Nezreka/SoulSync, not the unrelated streaming apps with the same name.

**Recommendation: Go backend + SQLite + a small React/TypeScript web UI, using the user’s working d-fi Go binary for Deezer downloads. Build a new music-only service with a SoulSync compatibility adapter.** A credible first version is feasible. The difficult work is reliable acquisition, track identity, library imports and observable compatibility, rather than serving HTTP requests.

Planning estimate for one experienced engineer familiar with Go: a technical proof in 2–4 working days; a limited personal-use MVP in 3–5 weeks; a dependable scoped v1 in 7–12 weeks total. These are engineering estimates, not measured implementation results. The dependable version includes Wavio artist watchlists, basic playlist refresh, failures/retries and recovery after restart. Private Apple library access, old database migration, broad third-party API parity is excluded from that original estimate. Multi-user support has since been added to the implementation plan and requires a new estimate.

## Implementation requirements at the original assessment

The user has confirmed a shared music library with personal user collections, admin-approved music requests, an optional direct-acquisition grant and admin-only file deletion; see [multi-user requirements](multi-user-design.md). The user has also specified a collapsible sidebar based on option 1, a playlist view based on option 3, dedicated routed pages, Library views for Songs/Artists/Albums/Favorites, a unified Activity queue/history, complete Apple XML library ingestion, and durable Skip for now placeholders for failed/unresolved playlist entries. These requirements supersede the earlier minimal-UI assumption and any recommendation to use option 2 as the main interface. See [the implementation plan](implementation-plan.md) for the authoritative product behavior, state model, build sequence and acceptance gates. UI prototypes are under `../mockups/`; application backend implementation has not started.

## What was inspected

- [SoulSync source snapshot](https://github.com/Nezreka/SoulSync/tree/1d1e383f5bc4821ea31952aee12645aa986e6471), main at inspection, commit dated 29 October 2026.
- [Wavio source snapshot](https://github.com/Joel-Mercier/wavio/tree/973ea8aaad7cb3513b209b675b8f75b90c558a32), main at inspection. An installed release may use an older contract; pin that version before implementation.
- SoulSync's API documentation, public route implementations, serializers, metadata clients, Apple playlist importer, Deezer download client, Navidrome client and identity-recovery documentation.
- Wavio's SoulSync services, hooks, types, artwork handling and connection screen.

This was source inspection. No live authenticated Deezer download, Apple playlist import, Navidrome write or Wavio device test was performed. The user reports that the d-fi Go binary works well in their setup. Its service integration has not been exercised here; that user validation is sufficient to select it as the initial backend. Reference source snapshots were inspected in the original investigation workspace; upstream commit links above preserve the inspected versions. Third-party source trees are not vendored into this project.

## Recommended v1 boundaries

Include:

- Track, album and artist metadata search through Deezer and Apple/iTunes; select a source explicitly and cache results.
- Deezer acquisition through the existing d-fi binary, with a persistent queue, retries, cancellation, quality policy, tagging, artwork and safe library placement.
- Navidrome as the playback server: share its music storage, trigger/poll scans, index server IDs, create/update playlists and skip already-owned recordings.
- Apple Music exported library/playlist XML import as a first-class source, public Apple Music playlist links, plus CSV/TSV/TXT/M3U/M3U8 files. Preserve track order, show unresolved tracks, support manual correction, and offer manual refresh plus a simple scheduled refresh.
- The Wavio-compatible API surface listed below, including real watchlist scans.
- Dedicated Library, Playlists, Search, Imports, Activity, review, followed-artist and settings pages with a collapsible sidebar; Docker packaging.
- Nonblocking Skip for now for missing metadata/import failures, persistent ordered playlist placeholders and later verified Navidrome fulfillment.
- Song favorites, including imported metadata and connected-user Navidrome reconciliation where supported.

Exclude recommendations, movies, audiobooks, podcasts, playback/transcoding, social features, full automation builder, enrichment workers, other media servers, MCP and Lidarr emulation. Navidrome and Wavio already cover playback.

A clean rewrite is preferable to deleting features from the Python app because the user wants a different language and a small product. The original still provides valuable behavioral examples and regression cases. Its main web server and database modules are each over a megabyte; translating them wholesale would preserve the coupling we want to avoid.

## Why Go

This workload consists mainly of HTTP integrations, queue workers and filesystem operations. Go's explicit types, concurrency facilities and compiled deployment suit it well. A single service can own the API, job workers, schedule loop and embedded UI, with SQLite for durable state. No Redis, message broker or microservices are needed initially. Media tooling may still be a bundled executable; a compiled backend does not make every tagging dependency disappear.

Suggested implementation: standard `net/http` routing (or a small router), `database/sql`, SQL migrations, typed query helpers such as sqlc, SQLite WAL, and a bounded worker pool. Store jobs in SQLite with transactional claims, lease expiry and retry timestamps. Use SSE for the new UI; continue polling for Wavio.

TypeScript/Node is a reasonable second choice if the maintainer is much more experienced with it or wants one language throughout. Rust would also work, but I would choose it here only for an established Rust preference: memory control is not the main problem being solved. These are design judgments, not performance benchmarks. [Go concurrency documentation](https://go.dev/doc/effective_go#concurrency).

Use the user’s existing working d-fi Go binary as the initial download provider. [GoFi/d-fi](https://github.com/d-fi/GoFi) is the upstream reference, but record the exact binary/version the user runs rather than assuming a release. Pin and bundle that version, invoke it as a child process with argument arrays, and isolate each job’s staging output. Keep credentials out of process arguments and logs where the tool supports a configuration file.

The remaining work is service integration: verify non-interactive operation, exit/error handling, available progress signals, process cancellation, output discovery and tagging. Keep detailed progress optional if only human-oriented terminal output is available; show an honest downloading state until reliable progress can be extracted. Retain a provider interface so the implementation can later use a library or another executable without changing the queue or API. No separate downloader bake-off or new Deezer protocol implementation is needed.

## Wavio compatibility: small surface, detailed behavior

Your expectation is correct if “same API” includes response shapes, IDs, side effects, errors and lifecycle behavior. Identical URLs alone do not establish compatibility. A music-only implementation can preserve selected clients; it cannot promise all integrations when some use features being removed.

The inspected Wavio client uses **13 method/path combinations** under `/api/v1`:

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/system/status` | Connect and poll readiness |
| POST | `/search/tracks` | Track search |
| POST | `/search/artists` | Artist search |
| GET | `/downloads` | Per-track progress and queue |
| GET | `/library/recently-added` | Imported album history |
| GET, POST | `/wishlist` | Read and add acquisition intent |
| DELETE | `/wishlist/{track_id}` | Remove intent |
| POST | `/wishlist/process` | Process selected `track_ids` |
| GET, POST | `/watchlist` | Read and follow artists |
| DELETE | `/watchlist/{artist_id}` | Unfollow artist |
| POST | `/watchlist/scan` | Find missing releases |

Evidence: [Wavio service directory](https://github.com/Joel-Mercier/wavio/tree/973ea8aaad7cb3513b209b675b8f75b90c558a32/apps/mobile/services/soulsync).

Important observed requirements:

1. Wavio downloads using `POST /wishlist`, then `/wishlist/process` with selected IDs. It deliberately does not use `/request`; implementing only that webhook would miss Wavio's flow. Its album search is also deliberately unused. [Wishlist implementation](https://github.com/Joel-Mercier/wavio/blob/973ea8aaad7cb3513b209b675b8f75b90c558a32/apps/mobile/services/soulsync/wishlist.ts).
2. Preserve `{success,data,error,pagination}`, Bearer authentication, `X-Profile-Id`, error codes, provider source names and the list payloads. Bind each API key to its authenticated user and authorize profile selection; multi-user support is now required. Unknown or unauthorized profiles must never fall back to admin. [Request wrapper](https://github.com/Joel-Mercier/wavio/blob/973ea8aaad7cb3513b209b675b8f75b90c558a32/apps/mobile/services/soulsync/index.ts).
3. Wishlist rows expose `spotify_track_id` even when the actual provider is Apple or Deezer. Keep that legacy field in the adapter and make it round-trip to the search-result ID. Store provider-qualified identity internally to avoid numeric Apple/Deezer ID collisions. [Types](https://github.com/Joel-Mercier/wavio/blob/973ea8aaad7cb3513b209b675b8f75b90c558a32/apps/mobile/services/soulsync/types.ts).
4. Wavio correlates queue entries by track name and artist and reads 0–100 progress. It infers success from terminal jobs or disappearance from a previously observed wishlist. Keep failed intents visible, retain terminal jobs long enough for polling, and mark completion only after import/scan reconciliation. [Request polling](https://github.com/Joel-Mercier/wavio/blob/973ea8aaad7cb3513b209b675b8f75b90c558a32/apps/mobile/hooks/soulsync/useTrackRequest.ts).
5. Artist watchlists are acquisition automation, so they still belong in scope even though recommendations do not. Preserve provider-specific artist fields and genuine scan behavior. [Watchlist service](https://github.com/Joel-Mercier/wavio/blob/973ea8aaad7cb3513b209b675b8f75b90c558a32/apps/mobile/services/soulsync/watchlist.ts).
6. Wavio's connection screen displays a warning when `services.soulseek` is false. A Deezer-only service must resolve this legacy readiness convention: either document/map that compatibility field to downloader readiness, or accept a small Wavio change. This needs a device check; returning a valid status document alone is insufficient. [Connection screen](https://github.com/Joel-Mercier/wavio/blob/973ea8aaad7cb3513b209b675b8f75b90c558a32/apps/mobile/components/downloaders/soulsync/SoulSyncConfigScreen.tsx).
7. Recent-album artwork depends on `server_source`, a usable cover URL/ID and the Navidrome server Wavio is connected to. Test real artwork rather than only JSON schemas. [Artwork handling](https://github.com/Joel-Mercier/wavio/blob/973ea8aaad7cb3513b209b675b8f75b90c558a32/apps/mobile/services/soulsync/artwork.ts).

Build the compatibility layer as a separate adapter over clean internal services. Add new API capabilities independently. Wider API parity, old UI compatibility and importing SoulSync's existing database are separate projects. SoulSync also has web routes, MCP and Lidarr emulation; supporting its public music REST API does not automatically reproduce those surfaces. [Original API reference](https://github.com/Nezreka/SoulSync/blob/1d1e383f5bc4821ea31952aee12645aa986e6471/Support/API.md).

## The main technical risks

### Deezer acquisition

The user has already validated d-fi in their setup, which substantially reduces uncertainty about basic acquisition. Treat download availability as an accepted dependency choice and focus the initial technical proof on wrapping that exact binary. Session expiry, unavailable tracks and changed upstream behavior still need clear job errors. SoulSync’s own client remains useful as a behavior reference, without porting its download protocol. [Original download client](https://github.com/Nezreka/SoulSync/blob/1d1e383f5bc4821ea31952aee12645aa986e6471/core/deezer_download_client.py).

Persist acquisition intent and attempts separately. Stage downloads outside the library, validate files, tag them, then move them safely. A failed download or tag write must never appear as a successful library import. Keep the provider replaceable so upstream changes do not require rewriting playlist and queue logic.

### Apple search versus Apple playlists

SoulSync's Apple metadata client primarily uses the public iTunes search/lookup API. This is easier than a full signed-in Apple Music integration, but the iTunes Store catalog and Apple Music streaming catalog are not identical. Keep storefront/country in the provider configuration. [Original client](https://github.com/Nezreka/SoulSync/blob/1d1e383f5bc4821ea31952aee12645aa986e6471/core/itunes_client.py), [Apple search reference](https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/iTuneSearchAPI/index.html).

SoulSync's public-playlist importer extracts a web token and paginates Apple web catalog requests. That has maintenance exposure. For the rewrite, prefer the official Apple Music API with a configured developer token when practical, and explicitly choose whether a no-credentials public-link mode is required. Private library playlists additionally require a music user token and authorization UI. Private library API access is excluded from v1. XML import should be the first Apple playlist path implemented, followed by public links. [Existing importer](https://github.com/Nezreka/SoulSync/blob/1d1e383f5bc4821ea31952aee12645aa986e6471/api/source_playlists.py), [developer tokens](https://developer.apple.com/documentation/applemusicapi/generating-developer-tokens), [private library playlist access](https://developer.apple.com/documentation/applemusicapi/get-a-library-playlist).

### Apple Music XML library and playlist import

Add both exported-library XML and exported-playlist XML as supported sources. Apple documents exporting an individual playlist as XML and saving the whole library via File → Library → Export Library. These exports describe music and playlists; they do not contain the audio. [Apple’s export instructions](https://support.apple.com/en-euro/guide/music/mus27cd5060f/mac).

Implementation: parse the property-list XML, build a track table from the export’s track IDs, and resolve each playlist’s ordered track references through that table. Preserve supplied names, artists, albums, durations, disc/track numbers and stable export IDs when available. Treat exported IDs as source identities rather than assuming they are Deezer or Apple catalog IDs. Ignore movies, audiobooks and other identifiable non-music records; show an import report for incomplete or ambiguous records.

Ingest the complete exported song catalog, including songs not in any playlist, plus playlist definitions and their ordered song memberships. Offer a preview of song/playlist counts and changes before committing; catalog ingestion is independent of network matching. Allow playlist selection, and keep matching/downloading separate: importing an XML file must not automatically enqueue every song for download. Preserve repeated entries and shared tracks across playlists while deduplicating acquisition jobs. Smart playlists become the exported membership snapshot, without executing their rules. Re-import should update the chosen source snapshot using stable IDs where available and avoid duplicating playlists. Changes in Apple Music require a fresh XML export; there is no live sync from this file.

Use an XML parser that does not fetch external DTDs/entities, and bound file size and nesting. Some fields may be absent, so build the importer against a representative export before relying on catalog IDs or library completeness. This does not require Apple login, developer credentials or scraping.

### Track matching and Navidrome reconciliation

Matching by artist/title alone is insufficient: live versions, remasters, clean/explicit variants and duplicate releases all collide. Prefer ISRC when available, then corroborate with duration, version and album; retain provider IDs and allow manual decisions. ISRC alone is not an edition guarantee.

Navidrome remains a separate server. Use its Subsonic/OpenSubsonic interfaces for inventory, scans and playlist writes. Expect a delay between placing a file and receiving its server ID. Keep local and server paths distinct, tolerate differing container mounts, and verify playlist membership after writes. Do not delete or replace a playlist based on a partial inventory. [Navidrome API coverage](https://www.navidrome.org/docs/developers/subsonic-api/).

These concerns have caused real upstream bugs: SoulSync documents stale Navidrome IDs, duplicated library rows and writes that report success while omitting entries. Use this source as the implementation specification and port its relevant regression scenarios to Go. Having the source makes the behavior concrete and reduces discovery work; it still requires implementing and verifying equivalent behavior in the new architecture. [Identity-recovery investigation](https://github.com/Nezreka/SoulSync/blob/1d1e383f5bc4821ea31952aee12645aa986e6471/docs/issue-1248-navidrome-recovery.md).

File imports need their own rules. CSV/TSV/TXT carry metadata with varying columns; M3U often contains local paths and optional EXTINF labels, not catalog IDs. Preserve missing entries for review instead of silently guessing. Start with a documented simple format and a column-mapping preview.

## Architecture and future torrent support

One deployable service:

`Wavio compatibility routes + small admin UI → application services → metadata providers / playlist sources / acquisition providers / Navidrome adapter`

SQLite stores canonical tracks, provider references, playlists and ordered entries, acquisition intents, jobs/attempts, watchlists, library mappings, manual matches and settings. Separate source-playlist entries from resolved recordings and downloaded files. Store credentials outside normal logs and redact configuration exports.

Use different interfaces for metadata search, playlist retrieval and acquisition. A future torrent flow is structurally different from one Deezer track: Prowlarr supplies search results, a torrent client runs the transfer, and an importer maps a completed album pack to requested tracks. Model a job that can produce multiple files now; implement only Deezer initially. Torrent support later still needs pack matching, completed-download handling, path mapping, duplicate policy and seeding rules. It is not just adding a Prowlarr URL field.

## Effort estimate

This is the original preliminary estimate. The subsequent dedicated Library/Activity pages and durable deferred-placeholder requirements are detailed in the implementation plan and require re-estimation before scheduling; the minimal-UI allowance below is no longer the complete product scope.

One engineer, full time; ranges represent engineering uncertainty and sum to the dependable-v1 estimate. The proof of concept is included, not an additional phase charged twice.

| Work | Engineer days |
| --- | ---: |
| d-fi wrapper proof and pinned Wavio contract | 2–4 |
| Metadata providers, normalization and cache | 3–5 |
| Deezer jobs, quality, tagging and file import | 7–12 |
| Navidrome inventory, scan and playlist reconciliation | 5–8 |
| Apple XML/public links and file import, matching and refresh | 4–7 |
| Wavio compatibility and artist watchlists | 6–10 |
| Minimal admin UI and Docker packaging | 3–5 |
| Integration testing, restart recovery and hardening | 5–9 |
| **Total** | **35–60 days / 7–12 working weeks** |

The 3–5 week personal-use milestone is a narrower happy-path subset: search, one chosen download quality, basic playlist imports, Navidrome placement and Wavio track requests. It is not all the reliability and watchlist behavior above.

## First implementation checkpoint

Spend 2–4 days proving a vertical slice before building the full UI:

1. Pin the installed Wavio version; capture representative request/response fixtures from its required routes.
2. Query both metadata sources and resolve an Apple result to a Deezer recording without confusing a live/alternate version.
3. Wrap the user’s pinned d-fi binary, acquire a sample with their intended quality, verify tags and artwork, import it and find its Navidrome ID after scanning.
4. Run Wavio's add-wishlist/process/poll flow against a Go skeleton and verify visible progress, errors, history and artwork.
5. Import a representative Apple Music library XML and select playlists without automatically downloading the entire library. Verify ordering, repeated entries, re-import and unresolved matches. Then prove public-link pagination, one metadata file and one M3U.

Acceptance tests for the final v1 should also cover duplicate requests, expired credentials, unavailable tracks, restarting mid-job, failed tagging, scan delays, ambiguous matches, stale Navidrome IDs and partial playlist writes. Contract tests should exercise Wavio payloads and behavior, not only validate endpoint names.

**Decision:** proceed with a fresh Go service and Wavio-specific compatibility first. Use d-fi as the accepted acquisition backend and prioritize Apple Music XML import. The early checks are reliable subprocess/job integration, XML matching and Wavio-to-Navidrome completion; public-link retrieval follows as an additional import source. Broaden API compatibility client by client after that contract is demonstrated.
