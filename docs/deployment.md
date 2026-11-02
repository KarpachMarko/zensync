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

The image includes the API, compiled UI, **d-fi 2.3.5**, ffmpeg/ffprobe, CA certificates and time zone data. d-fi is built from release commit `fa5612209bccf3ec64aae518c76bf86dcc63c7b8`; its MIT license is included in the image. See the [pinned d-fi source](https://github.com/d-fi/GoFi/tree/fa5612209bccf3ec64aae518c76bf86dcc63c7b8).

Compose runs as UID/GID 10001 with a read-only application filesystem, dropped capabilities, a persistent named `/data` volume, a shared `/music` bind mount and temporary `/tmp`. Keep your existing `/data` mount when upgrading. You may replace the named volume with your established local-disk bind mount, provided it is writable by UID 10001. Do not place the SQLite database on NFS/SMB or run multiple replicas against it; a service lock prevents concurrent owners.

`zensync:local` is the image built by Compose, not a published registry image. No secrets are baked into the image or required at container startup.

## Configure the connections

Open **Settings → Connections**:

1. **Shared Navidrome server:** enter its base URL and a service account with shared-library access and scan permission. Use a hostname reachable from the container, such as `http://navidrome:4533` on a shared Docker network, or your server's LAN address. `localhost` inside ZenSync refers to the ZenSync container. Saving verifies the account through OpenSubsonic.
2. **Your personal Navidrome account:** link the existing Navidrome account that should own imported playlists/favorites. ZenSync never falls back to the service account for those writes. It is fine to use the same account for both roles in a single-user installation, but enter both links explicitly.
3. **Deezer acquisition:** save the ARL cookie from the Deezer account you use with d-fi. The cookie is verified by d-fi during acquisition. ZenSync uses encrypted storage and creates a mode-0600 per-job config while the downloader runs. It never puts the ARL in command-line arguments or returns it to the browser. Raw downloader output is discarded to avoid leaking credentials.

After migrations exist, changing the shared server URL is rejected. After a migration has bound its personal account, that account identity cannot be changed underneath it. Passwords can be refreshed for the same account.

No public URL environment variable is needed. A reverse proxy must preserve the original public `Host` header (including port) and browser `Origin`. Mutations validate same-host Origin; authenticated mutations also require a session CSRF token. Forwarded host/proto headers do not establish trust. HTTPS Origin enables Secure cookies behind a TLS-terminating proxy. Cookies are HttpOnly and SameSite=Strict; sessions expire in 24 hours and logout revokes them.

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

**Search** also accepts a title/artist query or a direct Deezer song URL. Select **Request** on a result; track ID, title and artist are fetched automatically. Deezer share links (`https://link.deezer.com/s/...`) are also resolved automatically. Album links in Search show the album’s ordered tracks for individual requests; playlist links are not supported. Migration recording review still requires an individual song.

New library files use `Album artist/Album/03 - Song.flac` (track number when available). Album artist and numbering come from validated audio tags; Unicode names are preserved and unsafe path characters are replaced. Disc folders are used when multiple discs are identified. Filename collisions receive a Deezer ID suffix, and a short checksum suffix if still necessary. Ordinary filenames contain no hashes. Internal staging names remain opaque. Existing assets, including the earlier `ZenSync/<id>/<hash>.flac` layout, keep their recorded paths and can be retried without moving or downloading again.

The sidebar includes a compact search field. Enter navigates to `/search?q=...`; collapsed navigation uses aligned icons and an icon-only expand control.

## Conflicts, skip and recovery

- **Skip for now** prevents a new acquisition attempt for the source entry. It does not cancel an already-running transfer or another user's work. Existing verified music may still fill a resolved deferred entry without a download. Resume allows pending requested work to run again.
- Shared requests for the same provider recording are processed serially and reuse a verified existing asset instead of downloading it again. Preferences do not upgrade existing audio. **Shared-file upgrades/replacement remain disabled.**
- External playlist renaming, reordering, additions, removals or deletion pauses that playlist. Its detail page shows the last verified and observed snapshots. You can hand it over or explicitly authorize replacing the external song order. Deleted or uncertain remote playlists are never silently recreated. Resuming preserves the reviewed current name/settings, then permits membership publication.
- Published names include a short `· ZenSync <id>` suffix. This gives uncertain playlist creation a unique recovery marker. Do not rename an actively managed playlist unless you want to pause migration updates.
- Favorites are positive-only and applied once. Absent flags never unstar a song, and later publication does not undo a user's unfavorite. An uncertain favorite attempt enters review; the owner can keep its current remote state or explicitly retry once.
- Failed acquisition tasks offer **Retry**. An interrupted transfer is retained as failed for explicit reconciliation. Valid staged audio is reused; invalid staged audio is discarded before retry. Atomic publication uses a temporary destination file and a no-overwrite hard link. Music filesystems must support hard links. Existing shared files are never replaced.
- A missing/unwritable music mount is an error. Audio is not marked available until ZenSync verifies the indexed original file. Navidrome normally returns a virtual path, so ZenSync checks the original download’s SHA-256 against the validated local file rather than requiring the reported path to equal its disk location. The service account needs download permission; verification streams the original without saving another copy. Scan checks are spaced by at least 30 seconds and avoid starting another scan while one is active. After five minutes without a verified match, the task fails with mount/read-permission guidance and keeps the validated audio for retry. Docker logs include the job ID, scan status, expected relative file path and lookup candidate count.
- One worker processes durable migration/acquisition tasks. Work survives browser closure and restarts. On Linux, child downloaders receive a death signal if the service is killed. A normal shutdown cancels the child, closes HTTP and retains recoverable state.
- Compare-before-write and read-back verification cannot eliminate a race with an external client editing during a remote write; Navidrome does not supply conditional revisions for this contract. Detected divergence is preserved for review.

## Operational limits

Initial XML limits are 32 MiB, 20,000 tracks, 100,000 playlist occurrences, 32 nesting levels and one parser at a time. DTD/entity retrieval is disabled; XML locations are metadata and are never opened as host files. Changed XML is a separate migration. Repeated identical uploads return the owner's existing migration, including after restart.

Downloads have a 15-minute attempt timeout, a 1 GiB per-quality staging cap and disk-headroom checks. Successful staging is removed once Navidrome availability is verified; failed staging is retained for retry. Monitor `/data/staging` on busy installations—automatic age-based failure retention is not implemented. Metadata calls are limited to four per second. Long libraries are processed incrementally with durable results and progress in Activity; worker-pool concurrency is intentionally one in this first migration version.

Request and Activity lists currently show the newest 200 records. The migration retains its complete source catalog and offers filtering and incremental display. Account password recovery, account disable/deletion/grant editing, API keys, SSE, Wavio compatibility, automated asset upgrades and artist notifications are not implemented. These are outside the first usable migration path; the broader phase-1 plan remains in [implementation-plan.md](implementation-plan.md).

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
npm --prefix web ci
npm --prefix web run build
go test -race ./...
go vet ./...
go run ./cmd/zensync
```

Go serves the built UI. For Vite development, `npm --prefix web run dev` proxies `/api` to Go. Local acquisition additionally needs d-fi 2.3.5, ffprobe and writable music storage; Docker bundles the tooling.

```sh
npm --prefix web exec -- playwright install chromium
npm --prefix web run test:e2e
docker build -t zensync:migration-test .
docker pull deluan/navidrome:0.64.2
python3 scripts/test-navidrome.py
```

Go tests cover first-setup races, approval/isolation, hostile XML, ingestion/idempotency, an acquisition fixture using real generated audio and ffprobe, playlist recovery, external edits, handover and filesystem safety. The browser test uses the real local Go backend for setup/import/review; only search/request metadata interactions use controlled browser fixtures. The disposable Docker test exercises real Navidrome 0.64.2 playlist/favorite writes and restart persistence with synthetic local music. These tests do **not** perform an authenticated live Deezer download: your ARL/account's availability and quality entitlement are verified during your first actual acquisition.
