# ZenSync

A self-hosted Navidrome companion for initial music migration and ongoing music acquisition.

## Current status

The first migration workflow is implemented: first-access administrator setup, Apple Music XML/text preview/commit, recording review, Deezer search or direct song URLs, approved acquisition through pinned d-fi 2.3.5, validated shared-library placement, and personal Navidrome playlist/favorite publication. External edits pause playlist updates; final handover stops migration collection writes. Legacy ID/hash downloads are automatically reorganized into readable paths with verified Navidrome identity preservation and recovery backups.

Docker bundles the application, UI, downloader and audio validation tooling. Mount the same music directory Navidrome scans, start the container, create the admin in the browser, then configure connections and open Imports. No administrator password/hash environment variable is needed.

Search also includes experimental lossless.am and Hitmo providers for individual song requests. Hitmo requires no account and supplies MP3; enable a lossy quality tier to use it. See [media provider support and limitations](docs/media-providers.md).

See [deployment and first-migration instructions](docs/deployment.md) and the [song deletion API](docs/deletion-api.md). Wavio compatibility, shared-file upgrades and the remaining multi-user lifecycle features are still pending; the bullets below describe the full planned phase 1, not a claim that every feature is complete.

The Library supports song, playlist, artist and album browsing, with compact metadata editing and existing deletion actions on song rows. Administrators can search LRCLIB lyrics and MusicBrainz/Cover Art Archive artwork, publish verified file-tag updates through atomic replacement, and resolve duplicate artists, albums and songs from **Utilities → Review duplicates**. See [library metadata, duplicate resolution and provider research](docs/metadata-management.md).

## Phase 1

- Go + SQLite backend, compact React/TypeScript UI, Apple/iTunes and Deezer metadata, and acquisition through the existing d-fi Go binary.
- Apple Music XML migration of the complete music catalog, selected playlists and favorites, plus single-playlist imports from Apple Music tab-separated `.txt` exports (UTF-16 or UTF-8). Text imports preserve order and use the filename as the playlist name. No ongoing Apple synchronization or updated-export merging.
- Recording review supports uploading your own audio when a download source has no match or you prefer another version. Choose **Upload your version**, then upload FLAC, MP3, M4A, OGG, OPUS, WAV or AIFF (up to 1 GiB). Files are validated and imported through the usual approval and Navidrome indexing flow; existing library audio is preserved.
- New MP3 and FLAC uploads preserve embedded cover art and extract a front-cover thumbnail for Requests. Original artwork stays embedded for Navidrome; thumbnail access is limited to the owner and administrators. Recording previews use a compact player with cover art, seeking, volume and the 30-second preview limit.
- One admin-configured Navidrome instance and shared music library, with separate personal Navidrome accounts. Personal linking is needed for migration publication, not search or acquisition requests.
- Migration stays open to fill missing entries until explicit handover. External edits pause the affected playlist. After handover, Navidrome owns playlists/favorites; outstanding acquisitions continue without changing those collections.
- Persistent unresolved entries with Skip for now. Confidently matched existing music can fill skipped entries without a new download; alternative versions require review.
- Import matching normalizes release labels, featured credits, punctuation and common Cyrillic transliterations, and retries shorter queries when necessary. Automatic selection still requires agreement on artist credits, album, title/version and duration; review explains remaining differences. Use **Scan again** to apply updated matching to unresolved entries without changing manual choices. See [matching behavior and example analysis](docs/import-matching.md).
- Per-user ordered quality profiles: lossless first, then allowed lower qualities. Reuse existing audio unless Upgrade is explicitly requested; replacing a shared file requires admin approval and verification.
- Admin-approved member acquisitions by default, with an optional direct-acquisition grant. Song deletion has a separate direct-delete grant; other deletion requests require admin approval.
- Wavio connects directly to Navidrome for playback and collection management. Select its **SoulSync** integration, enter the ZenSync server URL and a key with the default scopes, and keep profile ID **1** for search and download requests. Approval may appear as Queued in Wavio; details are shown in ZenSync. See [supported SoulSync endpoints and setup](docs/soulsync-api.md).
- Dedicated Library, Search, Imports, Requests, Activity and Settings pages with a collapsible sidebar and compact music rows.

Phase 2 adds followed-artist new-release notifications, without automatic downloads. Public playlist links and extra import formats are outside phase 1. ZenSync is not a playback engine, client gateway or ongoing playlist/favorites editor.

## Logs

Every API request is logged as structured JSON, including successful calls. View logs with `docker compose logs -f zensync`. Set `ZENSYNC_LOG_LEVEL=debug` for upstream request timing and health probes. See [logging and diagnostics](docs/logging.md).

## Development

Metadata edits and artist/album merges require Python 3 with Mutagen, in addition to ffmpeg/ffprobe, to preserve native multi-valued credits. The Docker image includes these dependencies. For local testing, install your distribution’s Python Mutagen package or set `ZENSYNC_PYTHON` to a virtual environment’s Python executable.

Use **pnpm** for frontend dependencies and scripts. See [frontend structure and component conventions](web/README.md).

## Start here

- [Authoritative implementation plan](docs/implementation-plan.md)
- [Multi-user permissions and request workflow](docs/multi-user-design.md)
- [Historical source investigation and preliminary assessment](docs/assessment.md)
The original single-user effort estimate is superseded. Establish the supported integration contract and recovery behavior in the first executable slice, then estimate the remaining phase 1 milestones.
