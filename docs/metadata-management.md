# Library metadata and duplicate resolution

The Library root has **Songs**, **Playlists**, **Artists** and **Albums** views. Select a playlist or album to browse songs. An artist opens their albums and offers **Show all songs**. The view, collection, filter and editor are reflected in the URL; browser history restores collection navigation. Artist song loading visits ten albums per page. Album listing loads 100 albums per page; a filter applies to each loaded page, so continue loading for matches farther into a large catalog.

Playlist browsing uses the signed-in user's linked personal Navidrome account. Other library views use the configured shared account. Members retain the existing deletion-request workflow. Administrators can edit metadata on every shared song row, including search results. Direct deletion retains its existing permission and confirmation flow.

## Compact editor

**Edit metadata** opens one shared dialog, using TanStack Form and Zod. Details, Artwork and Lyrics have separate tabs. Artist and album fields suggest existing library names, support free text, and offer keyboard navigation. Suggestions share the bounded 50,000-song index and can be incomplete for larger libraries.

**Save changes** queues a file-tag update. Fields include title, artist, album, optional album artist, artwork URL, plain lyrics and synced LRC. Synced text takes priority in the embedded lyrics tag; both selected forms remain in the review record. Blank lyrics preserve existing embedded lyrics. Embedded artwork is preserved unless a replacement is selected. New cover embedding supports MP3, FLAC and M4A, with JPEG/PNG images from Cover Art Archive / archive.org. Artwork redirects are checked, image downloads are capped at 20 MiB, and dimensions at 50 megapixels.

The worker remuxes audio without re-encoding, verifies the encoded audio packet hash, preserves file permissions, atomically replaces the file and requests a Navidrome scan. It supports MP3, FLAC, M4A, OGG, OPUS, WAV and AIFF text-tag updates, subject to the container's supported tag fields. Paths do not change. Navidrome may assign new IDs depending on its persistent-ID configuration; confirm library and playlist membership after the scan. No live Navidrome compatibility benchmark or custom persistent-ID configuration support is claimed.

The status panel reports queued/running/completed/failed operations and per-song progress. Completed means file tags were updated and a scan was requested; Navidrome may still be scanning. Refresh the library or duplicate candidates afterward. Failed operations expose their error and offer **Retry update**. Merges may partially complete; retry resumes remaining files. A failed scan request can be retried without rewriting already-completed files. Only one metadata operation can be queued/running at a time.

Recovery files live outside the music library under `metadata-backups/{job-id}-{song-id-digest}/original.{extension}`. These are full audio files, not tag-only records. `selected-cover` is a temporary downloaded/uploaded image. The `metadata_files` journal records the original relative music path and before/after SHA-256 checksums. Separate jobs can create separate full copies of the same song.

### Recovery storage policy

Published copies are retained for **up to 7 days**, with a **1 GiB total recovery budget** by default. Oldest published copies are removed first when space is needed. Cleanup runs at startup, hourly, before new recovery allocations and after metadata jobs. Successfully processed songs in a partly failed job are eligible too; the unfinished entries remain protected. Existing unknown directories and unfinished copies are counted but never automatically deleted. If those exceed the budget, new metadata writes stop rather than evict recovery data. Legacy installations can therefore remain over budget until protected work is resolved. Existing successful copies above the budget are pruned on the first startup of the updated version.

Set `ZENSYNC_METADATA_BACKUP_LIMIT_MIB` in the service environment to choose a different budget (positive MiB, default `1024`). This includes original files and staged covers inside `metadata-backups`; it does not limit the music library or other ZenSync data. Working-file and backup writes also require 256 MiB of free-space headroom. Retention is a short recovery window, not a permanent backup or a guarantee of seven days of undo history. Keep an independent backup if you need long-term rollback.

### Inventory and cleanup

Stop the service before offline maintenance. Replace the Compose service name if yours differs. Use the updated image with the same data/music mounts; do not start its worker before saving any old copies you want to retain.

```sh
docker compose stop zensync
docker compose run --rm --no-deps zensync metadata-backups list > metadata-backups.json
```

The JSON inventory lists `key`, `jobId`, `songId`, `musicPath` (relative to the shared music root), `beforeHash`, `afterHash`, size, job state, and `reclaimable`. **`reclaimable: true` means deletion will not break job resumption**, but it does remove that copy's manual rollback option. Unrecognized or unfinished copies are `false`. A job can fail after publishing earlier songs, so the job state alone is insufficient to classify every directory. Copies made before journal creation may not have a known `musicPath` or checksums; keep them until investigated.

To enforce the budget and age policy immediately, without starting the worker:

```sh
docker compose run --rm --no-deps zensync metadata-backups cleanup
```

This prints removed entries; it does not remove unfinished copies, music files, the database or Activity history. `list` is read-only with respect to recovery files. The maintenance commands refuse to run while another service owns the data-directory lock. Restart with `docker compose up -d zensync` after maintenance. Changing the budget is not a way to force deletion of protected originals.

### Manual restoration

There is no rollback UI. Before cleanup, choose one inventory entry and use its full original copy and recorded destination. Stop ZenSync, back up `zensync.db` consistently, and ensure no queued operation will reapply edits to the song. Do not infer the destination from the opaque directory name.

On the host, set these values from the inventory and your actual mount paths:

```sh
backup='/home/debian-server/docker-data/zensync/metadata-backups/JOB-SONG_DIGEST/original.flac'
music_file='/actual/host/music/Artist/Album/song.flac'
before_hash='BEFORE_HASH_FROM_INVENTORY'
after_hash='AFTER_HASH_FROM_INVENTORY'
```

Verify the original copy matches `beforeHash` and the current music file matches `afterHash`. If either fails or either checksum is absent, stop: the file may have later edits or may need interrupted-write reconciliation. Do not overwrite it blindly. Run each command only if the previous one succeeds:

```sh
printf '%s  %s\n' "$before_hash" "$backup" | sha256sum -c -
printf '%s  %s\n' "$after_hash" "$music_file" | sha256sum -c -
restore_tmp=$(mktemp --tmpdir="$(dirname -- "$music_file")" .zensync-restore.XXXXXX)
cp -- "$backup" "$restore_tmp"
printf '%s  %s\n' "$before_hash" "$restore_tmp" | sha256sum -c -
chmod --reference="$music_file" "$restore_tmp"
chown --reference="$music_file" "$restore_tmp"
sync -f "$restore_tmp"
mv -f -- "$restore_tmp" "$music_file"
sync -f "$(dirname -- "$music_file")"
```

This restores that song's complete pre-edit file, including tags and artwork. Before restarting, abandon the originating job if it is failed/queued/running so it cannot reapply the edit, and clear that song's saved metadata review. With the service stopped, use the exact IDs from the inventory in `zensync.db`:

```sql
BEGIN;
UPDATE metadata_jobs SET state='cancelled',message='Stopped after manual restoration'
 WHERE id='JOB_ID_FROM_INVENTORY' AND state IN ('failed','queued','running');
DELETE FROM metadata_edits WHERE song_id='SONG_ID_FROM_INVENTORY';
COMMIT;
```

Abandoning a partially completed batch leaves its remaining songs unprocessed. Restart ZenSync, request a full Navidrome scan, and refresh the library. This is manual recovery, not a transactionally journaled rollback feature; retain the original until the restored song is confirmed. The review-record DELETE API only discards a stored review and does not undo published file tags.


## Utilities: Review duplicates

**Utilities → Review duplicates** finds distinct library IDs with matching normalized names. Artist candidates compare artist names; album candidates compare artist and album names. Song candidates additionally require known durations within two seconds. Normalization ignores case, spaces and punctuation while retaining Unicode letters/numbers/marks and version labels. It is a candidate heuristic, without fingerprints or fuzzy matching.

- Extra songs can be deleted through the existing shared-library deletion and recovery workflow.
- For albums, choose the destination under **Keep album**, then merge another candidate into it. A confirmation shows source, destination and affected song count. Source tracks retain their track titles and artists; album name, album artist and destination grouping tags (including MusicBrainz release ID, version and date) are copied so Navidrome can regroup the album.
- For artists, choose **Keep artist**, then merge another candidate into it. Source songs adopt the canonical artist/album-artist credits and destination artist identifiers, preserving distinct album titles. Multi-valued artist fields are updated as well.

The audit uses the shared Navidrome index cached for 30 seconds and capped at 50,000 songs. Partial scans are labeled and cannot be used for merges. No merge or deletion happens without explicit user action. Editions and artists sharing a name can be legitimate; inspect candidates before confirming. Custom Navidrome album-grouping rules (for example folder-based IDs) may still require configuration or manual reconciliation. Operations are bound to the Navidrome server URL configured when queued and stop if it changes.

## Provider research and decisions

Reviewed November 5, 2026. There is no universal best lyrics provider: catalog coverage and timing accuracy need evaluation against the actual library. We have not performed a comparative catalog benchmark.

| Provider | Findings | Decision |
| --- | --- | --- |
| [LRCLIB](https://lrclib.net/docs.txt) | Public API, no required key, plain lyrics, synced LRC and instrumental markers. Search exposes artist, album and duration for review. Exact signature lookup can use duration within ±2 seconds. Requires client identification, pacing and respecting Retry-After. Optional registration supports higher limits. | Implemented on-demand search; explicit selection and save. No background library harvesting or publishing. Community coverage and timing vary. |
| [MusicBrainz + Cover Art Archive](https://musicbrainz.org/doc/Cover_Art_Archive/API) | Release IDs, artwork types and thumbnails provide traceable edition-specific artwork. [MusicBrainz requests](https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting) need application identification and rate limiting. | Implemented release search, then actual CAA lookup for front artwork in the first five results. A release without artwork is skipped. Source release links are shown. |
| [Musixmatch](https://github.com/musixmatch/musixmatch-sdk) | Official search/lyrics API uses an API key. Commercial access and applicable display/storage terms need agreement with the provider. | Potential licensed provider integration; no unofficial endpoints or borrowed keys. |
| [LyricFind](https://www.lyricfind.com/) | Offers lyrics/data licensing and rights-managed lyric display products. Requires a partner arrangement. | Potential commercial alternative; no public anonymous integration selected. |
| [Apple iTunes Search](https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/iTuneSearchAPI/index.html) | Search includes artwork, but the published promotional-content terms tie artwork use to promotion and purchase links. | Not selected as the default source for persistent local artwork management. |

API access does not itself establish redistribution rights for artwork or lyrics. The implementation retains source references and makes no claim that content is freely licensed. Provider quality and permitted uses should be assessed before a public distribution or publishing workflow.

## API and provider behavior

All `/metadata/*` endpoints require an administrator session. Writes use the existing same-origin and CSRF protections.

- `GET /api/v2/library/browse?view=playlists|artists|albums&id=…&q=…&offset=…`: collections or songs; artists support `contents=songs`. Authenticated library access is unchanged.
- `GET /api/v2/metadata/suggestions?field=artist|album&q=…`: up to 20 existing names, plus a partial-index flag.
- `GET /api/v2/metadata/duplicates`: candidate groups with member song details/counts, scanned count and partial-scan flag.
- `GET /api/v2/metadata/songs/{id}`: source defaults or saved edit, with source duration and saved flag.
- `PUT /api/v2/metadata/songs/{id}`: validate and save the review record, queue tag updates; returns `202` and job ID.
- `POST /api/v2/metadata/merge`: `{kind: "artist"|"album", source, target}`. Returns `202`, job ID and affected song count.
- `GET /api/v2/metadata/jobs`: latest 20 metadata operations and progress.
- `POST /api/v2/metadata/jobs`: `{id}` retries a failed operation if no metadata operation is active.
- `GET /api/v2/metadata/lyrics?q=…`: up to 20 LRCLIB results.
- `GET /api/v2/metadata/covers?q=…`: front artwork for the first five MusicBrainz releases with available art.

Provider requests have fixed starting destinations, a 25-second search deadline, bounded JSON reads, client identification, a sequential one-second queue, and cooldowns for 429/503 respecting Retry-After seconds or dates. Metadata requests follow at most five HTTPS redirects restricted to MusicBrainz, LRCLIB, Cover Art Archive and archive.org hosts. This fixes Cover Art Archive's HTTP 307 responses without changing the shared client's redirect policy. Provider searches use React Query caching and no automatic error retries. Queries are sent only after explicit search submission. Artwork previews load from the provider URL.

Regression coverage includes native library browsing, suggestions, duplicate identities/durations, permissions/CSRF, queuing, actual file tags and audio preservation, MP3/FLAC/M4A cover embedding, album/artist grouping tags, restart recovery, changed-file protection, redirect handling and provider cooldowns. Playwright covers all library paths, compact/mobile editing, autocomplete, provider selection, deletion, merge confirmation and member access.
