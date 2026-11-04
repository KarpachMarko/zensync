# Docker deployment and your first migration

ZenSync now imports an Apple Music/iTunes XML snapshot, matches existing Navidrome songs and Deezer recordings, acquires explicitly requested missing tracks through d-fi 2.3.5, and publishes ordered playlists and positive favorites to your personal Navidrome account.

## Start the container

1. Set `ZENSYNC_MUSIC_PATH` in `.env` to the host music directory that Navidrome scans:

   ```dotenv
   ZENSYNC_MUSIC_PATH=/path/to/your/music
   ```

   The container sees this directory as `/music`. Navidrome can mount it at a different container path; both services must see the same directory contents. Do not use a separate empty music volume for ZenSync. Ensure UID/GID `10001:10001` can create artist and album directories, and Navidrome can read it. The default `./music` path is useful only if Navidrome is also pointed at that directory.

2. Build and start:

   ```sh
   docker compose up --build -d
   ```

3. Open `http://<server-ip>:8080`. On an empty database, the first page asks you to create the administrator account. There is no bootstrap password/hash environment variable. Setup closes atomically after the first administrator is created. Existing databases keep their accounts and passwords; upgrading never resets them.

The image includes the API, compiled UI, **d-fi 2.3.5**, ffmpeg/ffprobe, CA certificates and time zone data. d-fi is built from release commit `fa5612209bccf3ec64aae518c76bf86dcc63c7b8`; its MIT license is included in the image. A small source adapter exposes its existing byte-progress callbacks to ZenSync; the downloader still runs in an isolated process. See the [pinned d-fi source](https://github.com/d-fi/GoFi/tree/fa5612209bccf3ec64aae518c76bf86dcc63c7b8).

Compose runs as UID/GID 10001 with a read-only application filesystem, dropped capabilities, a persistent named `/data` volume, a shared `/music` bind mount and temporary `/tmp`. Keep your existing `/data` mount when upgrading. You may replace the named volume with your established local-disk bind mount, provided it is writable by UID 10001. Do not place the SQLite database on NFS/SMB or run multiple replicas against it; a service lock prevents concurrent owners.

`zensync:local` is the image built by Compose, not a published registry image. No secrets are baked into the image or required at container startup.

## Configure the connections

Open **Settings → Connections**:

1. **Shared Navidrome server:** enter its base URL and a service account with shared-library access and scan permission. Use a hostname reachable from the container, such as `http://navidrome:4533` on a shared Docker network, or your server's LAN address. `localhost` inside ZenSync refers to the ZenSync container. Complete credentials are verified through OpenSubsonic and saved automatically.
2. **Your personal Navidrome account:** link the existing Navidrome account that should own imported playlists/favorites. ZenSync never falls back to the service account for those writes. It is fine to use the same account for both roles in a single-user installation, but enter both links explicitly.
3. **Deezer acquisition:** save the ARL cookie from the Deezer account you use with d-fi. The cookie is verified by d-fi during acquisition. ZenSync uses encrypted storage and creates a mode-0600 per-job config while the downloader runs. It never puts the ARL in command-line arguments or returns it to the browser. Raw downloader output is discarded to avoid leaking credentials; only bounded, validated numeric transfer records and fixed phase names from the adapter are accepted.

After migrations exist, changing the shared server URL is rejected. After a migration has bound its personal account, that account identity cannot be changed underneath it. Passwords can be refreshed for the same account.

No public URL environment variable is needed. A reverse proxy must preserve the original public `Host` header (including port) and browser `Origin`. Mutations validate same-host Origin; authenticated mutations also require a session CSRF token. Forwarded host/proto headers do not establish trust. HTTPS Origin enables Secure cookies behind a TLS-terminating proxy. Cookies are HttpOnly and SameSite=Strict; sessions expire in 24 hours and logout revokes them.

## Settings and invitations

Settings save automatically and show a success or failure toast. Download concurrency defaults to three and has no fixed upper ceiling. Text edits are debounced; updates are serialized so an older response cannot overwrite a newer choice. Connection credentials save as a complete group after validation and verification; passwords/cookies are cleared from the editor after a successful save. Enter the password again when changing a saved Navidrome connection.

In **Settings → Quality**, enable the formats you allow and move them into preference order. FLAC has one lossless option, alongside MP3 320 kbps and MP3 128 kbps. Keep at least one enabled. New requests snapshot this order; changing it does not rewrite existing requests.

In **Settings → Users**, choose **Invite user**, enter a username, and share the generated link manually. It expires after seven days and can be used once. The recipient chooses their own password and then signs in. Invitation tokens are stored as hashes, and administrators can revoke pending invitations. Members begin with approval required for both acquisitions and deletions. Each member's **Permissions** button opens one dialog; its independent permission switches save automatically. The application does not send invitation emails.

## Make the first migration

1. In Apple Music, choose **File → Library → Export Library**, or export a selected playlist as XML.
2. Open **Imports → New migration**, choose the XML and select **Upload and preview**.
3. Review counts, excluded non-music items, missing references and playlist selections. The complete catalog, including unplaylisted music, is always retained. Choose whether to import positive favorite/loved flags, then **Commit migration**. No downloads occur at this step.
4. Select **Find matches**. Existing-library reuse requires matching title, artist, album and duration; alternate versions stay in review. Provider outages leave the committed snapshot intact; retry the failed task in Activity.
5. Open an unresolved song to search candidates. You can also paste a direct URL such as `https://www.deezer.com/track/3135556` or its language-prefixed form. Choose **Use existing** or **Use this recording** after checking version, album and duration. Resolving alone does not request a new download.
6. Select ready missing songs and choose **Request selected**, or choose **Request missing audio** in the song review. Admin requests start automatically when connections/storage are ready; ordinary members wait for an administrator's decision in **Requests**.
7. Select **Publish to Navidrome**. It binds the personal account and creates playlists using currently available songs. Later acquisitions fill their original positions automatically while the migration stays open. Duplicate occurrences are preserved. Unresolved/failed/skipped entries remain in ZenSync; no fake tracks are inserted in Navidrome.
8. Track downloads and publication in **Activity**. Audio is staged under `/data/staging`, checked with ffprobe and a full ffmpeg audio decode, placed without overwriting existing files, then scanned and verified in Navidrome. A successful downloader process alone never means availability. Quality follows the request's saved order: lossless (`flac`), 320, 128; excluded qualities and alternate-track fallback are disabled.
9. When ready, choose **Finish migration**. Handover is final and waits for any running collection writer. Outstanding acquisitions continue, but later arrivals no longer change playlists or favorites. The migration report retains unresolved entries.

**Search** accepts keyword queries for songs, artists or album names and returns separate Albums and Songs sections. Album results offer **Request album** (one approval) and **View tracks**. It also accepts a direct Deezer song URL. Select **Request** on a result; track ID, title and artist are fetched automatically. Deezer share links (`https://link.deezer.com/s/...`) are also resolved automatically. Album links in Search show an album summary with **Request album**, plus individual tracks. Request album creates one approval request for the whole saved track list. Playlist links are not supported. Migration recording review still requires an individual song.

New library files use `Album artist/Album/03 - Song.flac` (track number when available). Album artist and numbering come from validated audio tags; Unicode names are preserved and unsafe path characters are replaced. Disc folders are used when multiple discs are identified. Filename collisions receive a Deezer ID suffix, and a short checksum suffix if still necessary. Ordinary filenames contain no hashes. Internal staging names remain opaque. On upgrade, tracked files in the earlier `ZenSync/<id>/<hash>.flac` layout are automatically reorganized into readable paths. Progress appears as **Organize legacy download** in Activity. No download is repeated. This requires Navidrome 0.64.2–0.64.x, whose rename behavior has been verified to retain song IDs, favorites and playlist entries. Other versions leave files untouched and report an actionable failed task; an administrator can retry it after correcting the server/storage.

Relocation validates the source, saves a verified recovery copy under `/data/relocation-backups`, journals the old/new paths, then uses an atomic rename that cannot overwrite a destination. It updates ZenSync’s recorded path, scans Navidrome and verifies the original song ID still serves the same audio. It does not rewrite private playlists or favorites. Interrupted moves resume from the journal. If identity cannot be verified within five minutes after scans settle, it attempts to restore the original path and leaves a failure report. Readable-path collisions, unsafe/symlinked paths and filesystems without safe no-overwrite rename support stop the move. Only empty old `ZenSync` directories are removed.

Recovery copies stay outside the watched library and are retained after success. Include them in backups; they can be removed after you have verified the completed relocation and backed up the readable library. Do not delete backups for failed or unfinished relocations. The `/data` directory must be outside Navidrome’s watched music storage.

The sidebar includes a compact search field. Enter navigates to `/search?q=...`; collapsed navigation uses aligned icons and an icon-only expand control.

An album request snapshots its concrete Deezer track IDs/recordings and the user’s quality preference when submitted. An administrator approves or declines that single request; there are no separate track approval rows. Admin/direct-acquisition requests follow the same automatic approval policy as songs. The Requests page expands to show track progress, and Activity shows the album total and individual transfers. Existing playable music is reused. The album becomes available only when every track has been verified; failures remain visible. Retry on a failed album retries its failed tracks while preserving completed work. Editing quality preferences or changing the provider’s later album membership does not expand previously approved work. Revoking a direct-acquisition grant returns unstarted bypass-authorized album work to one pending album approval.

## Conflicts, skip and recovery

- **Skip for now** prevents a new acquisition attempt for the source entry. It does not cancel an already-running transfer or another user's work. Existing verified music may still fill a resolved deferred entry without a download. Resume allows pending requested work to run again.
- Requests for the same provider recording share an exclusive acquisition claim and reuse a verified existing asset instead of downloading it again. Preferences do not upgrade existing audio. **Shared-file upgrades/replacement remain disabled.**
- External playlist renaming, reordering, additions, removals or deletion pauses that playlist. Its detail page shows the last verified and observed snapshots. You can hand it over or explicitly authorize replacing the external song order. Deleted or uncertain remote playlists are never silently recreated. Resuming preserves the reviewed current name/settings, then permits membership publication.
- Published names include a short `· ZenSync <id>` suffix. This gives uncertain playlist creation a unique recovery marker. Do not rename an actively managed playlist unless you want to pause migration updates.
- Favorites are positive-only and applied once. Absent flags never unstar a song, and later publication does not undo a user's unfavorite. An uncertain favorite attempt enters review; the owner can keep its current remote state or explicitly retry once.
- Failed acquisition tasks offer **Retry**. An interrupted transfer is retained as failed for explicit reconciliation. Valid staged audio is reused; invalid staged audio is discarded before retry. Atomic publication uses a temporary destination file and a no-overwrite hard link. Music filesystems must support hard links. Existing shared files are never replaced.
- A missing/unwritable music mount is an error. Audio is not marked available until ZenSync verifies the indexed original file. Navidrome normally returns a virtual path, so ZenSync checks the original download’s SHA-256 against the validated local file rather than requiring the reported path to equal its disk location. Lookup tries the full title, an artist-qualified query and shorter queries in the original script to handle punctuation, featured credits and emoticons. Broad search pages are combined with the library metadata fallback and ranked globally by path, file size, title, primary artist and album before original files are checked, so short/common titles cannot exhaust the verification budget on the first pages. At most ten checks are spent on small complete search results before that ranking. If text search misses, it browses the service account's library metadata and checks candidates using file size, title, path or duration hints. These hints never replace byte verification, and a stale indexed size does not reject matching original bytes. The paginated metadata fallback is cached per connection for 30 seconds, with a 50,000-song cap and at most 200 original-file checks per lookup. A capped lookup cannot establish uniqueness for legacy relocation. The service account needs download permission; verification streams the original without saving another copy. Scan checks are spaced by at least 30 seconds and avoid starting another scan while one is active. After five minutes without a verified match, the task reports whether no indexed candidate was found or candidate originals had different bytes, and identifies the retained file. **Retry** verifies this retained file again without another Deezer download. Docker logs include the job ID, scan status, expected relative file path, lookup counts and the verified song ID.
- A bounded worker pool processes acquisitions. Metadata search, migration publication and filesystem placement retain their own serialization/rate limits. Work survives browser closure and restarts. On Linux, child downloaders receive a death signal if the service is killed. A normal shutdown cancels the child, closes HTTP and retains recoverable state.
- Compare-before-write and read-back verification cannot eliminate a race with an external client editing during a remote write; Navidrome does not supply conditional revisions for this contract. Detected divergence is preserved for review.

Approved tracks waiting for a worker show **Queued**, not Blocked. **Checking** means the worker is checking permissions/library availability; **Blocked** includes a reason that needs attention (for example missing Deezer credentials). Album tracks use the same global pool and still require only the single album approval. Different recordings can acquire concurrently; free slots prefer users with fewer active acquisitions, while identical provider targets are serialized to prevent duplicate files. Library placement is serialized to resolve filename collisions safely. Navidrome scan requests are coalesced. Legacy relocation drains active acquisition work before moving files, and migration publication records a follow-up wakeup when another track becomes available during a write.

## Operational limits

Initial XML limits are 32 MiB, 20,000 tracks, 100,000 playlist occurrences, 32 nesting levels and one parser at a time. DTD/entity retrieval is disabled; XML locations are metadata and are never opened as host files. Changed XML is a separate migration. Repeated identical uploads return the owner's existing migration, including after restart.

Downloads have a 15-minute attempt timeout, a 1 GiB per-quality staging cap and disk-headroom checks. Successful staging is removed once Navidrome availability is verified; failed staging is retained for retry. Monitor `/data/staging` on busy installations—automatic age-based failure retention is not implemented. Metadata calls are limited to four per second. Long libraries are processed incrementally with durable results and progress in Activity; acquisition concurrency defaults to three and is configurable in Settings → Downloads (admin only, any positive whole number). The limit is global across users and persists across restarts. Changes apply immediately to new work; lowering it does not cancel active transfers. Waiting-for-scan, deferred and blocked jobs do not hold idle transfer slots.

Request and Activity lists currently show the newest 200 records. The migration retains its complete source catalog and offers filtering and incremental display. Account password recovery, account disable/deletion, SSE, Wavio compatibility, automated asset upgrades and artist notifications are not implemented. These are outside the first usable migration path; the broader phase-1 plan remains in [implementation-plan.md](implementation-plan.md).

## Storage, health and backups

| Variable | Default | Purpose |
| --- | --- | --- |
| `ZENSYNC_MUSIC_PATH` | `./music` in Compose | Host directory mounted at `/music` |
| `ZENSYNC_DATA_DIR` | `/data` in Docker | SQLite, encrypted credentials, key and staging |
| `ZENSYNC_MUSIC_DIR` | `/music` | Container path to shared music |
| `ZENSYNC_WEB_DIR` | `/app/web` in Docker | Built UI |
| `ZENSYNC_DFI_PATH` | `/usr/local/bin/d-fi` | Pinned executable |
| `ZENSYNC_FFPROBE_PATH` | `ffprobe` | Audio verifier |
| `ZENSYNC_LISTEN` | `:8080` | HTTP listen address; Docker health check uses port 8080 |

`/healthz` reports liveness; `/readyz` checks SQLite. Readiness does not claim downstream authentication/availability. Docker invokes `/app/zensync healthcheck`. Compose allows 15 seconds for graceful shutdown; a long audio verification may need to reconcile after forced termination.

Back up the entire stopped data directory, including SQLite WAL files and **`credentials.key`**. Credentials cannot be decrypted without this key; startup refuses to generate a replacement if encrypted records already exist. Keep backups private. Back up the music directory separately.

```sh
docker compose stop zensync
mkdir -p backups
docker compose cp zensync:/data ./backups/zensync-data
docker compose start zensync
```

Restore the contents into an empty data volume using the same image version and UID/GID 10001 ownership. Never overlay a running database or combine snapshots. Check readiness, login, Connections, Imports and Activity after restoring. Back up the Compose configuration; no administrator password environment file is needed.

## Development and verification

```sh
pnpm --dir web install --frozen-lockfile
pnpm --dir web run build
go test -race ./...
go vet ./...
go run ./cmd/zensync
```

Go serves the built UI. For Vite development, `pnpm --dir web run dev` proxies `/api` to Go. Local acquisition additionally needs d-fi 2.3.5, ffprobe and writable music storage; Docker bundles the tooling.

```sh
pnpm --dir web exec playwright install chromium
pnpm --dir web run test:e2e
docker build -t zensync:migration-test .
docker pull deluan/navidrome:0.64.2
python3 scripts/test-navidrome.py
```

Go tests cover legacy relocation, restart recovery, collision protection, admin-only retry, first-setup races, approval/isolation, hostile XML, ingestion/idempotency, an acquisition fixture using real generated audio and ffprobe, playlist recovery, external edits, handover and filesystem safety. The browser test uses the real local Go backend for setup/import/review; only search/request metadata interactions use controlled browser fixtures. The disposable Docker test also exercises real Navidrome 0.64.2 relocation and confirms unchanged song identity, favorites and duplicate playlist references, alongside playlist/favorite writes and restart persistence with synthetic local music. These tests do **not** perform an authenticated live Deezer download: your ARL/account's availability and quality entitlement are verified during your first actual acquisition.

## Removing shared music

Use Library → Request deletion, or Delete song when you have the separate deletion permission. Administrators review pending deletions in Requests and manage the grant in Settings → Users. Settings → API keys creates scoped keys for external deletion clients. See the [deletion API and recovery guide](deletion-api.md) for endpoints, storage requirements and recovery instructions. Navidrome keeps its read-only music mount.

## Download progress

Activity and individual song requests show byte-based download percentage, transferred/total bytes, and transfer speed when the bundled downloader reports them. Album requests show these metrics on their individual tracks. Transfer percentage describes the network download only; decryption, tagging, validation, placement, and Navidrome indexing continue afterward with an animated running bar. All work with an unknown percentage uses the same animated bar, with a visible static fill when reduced motion is enabled.

A custom stock d-fi executable remains supported, with indeterminate progress. Use the updated Docker image to enable the source adapter. Live transfer samples are in memory, reset per attempt/quality and cleared on phase changes; a resumed file's existing bytes do not inflate speed. Four seconds without samples reports zero speed. The UI polls updates every few seconds. Completion still requires validation and verified Navidrome availability.

The bundled adapter also emits fixed diagnostic codes for country restrictions, quality-license restrictions, timeouts and unavailable recordings. ZenSync shows a reason for each attempted quality without retaining raw downloader output, cookies or signed URLs. A stock/custom binary still reports exit/no-output diagnostics; rebuilding the Docker image enables the structured adapter messages.

Acquisition checks exact provider IDs, tags and ISRC before publishing fully decoded audio. Small catalog duration discrepancies allow up to three seconds, or up to five seconds when the downloaded ISRC verifies the selected recording; short clips use a tighter relative limit. Errors include expected/measured durations. Approval revalidation accepts equivalent metadata formatting and small duration rounding changes, while changed IDs, known ISRCs, artists, versions or materially different durations require review. Activity moves an older failed/blocked attempt into Completed as **superseded** when a newer request from the same owner succeeds for the same recording, upgrade intent and quality profile. The original failure remains in history.

Native `/api/v2/activity` items, song `/api/v2/requests` items and album request tracks may include a `transfer` object while downloading: `percent` (omitted when total is unknown), `downloadedBytes`, `totalBytes` (0 if unknown), `bytesPerSecond` (omitted until two samples), and `updatedAt` (RFC 3339). Metrics follow the existing per-user authorization. No estimated percentages are invented for migration, scanning or processing.
