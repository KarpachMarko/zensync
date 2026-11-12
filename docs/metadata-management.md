# Library metadata and duplicate resolution

The Library root has **Songs**, **Playlists**, **Artists** and **Albums** views. Select a playlist or album to browse songs. An artist opens their albums and offers **Show all songs**. The view, collection, filter and editor are reflected in the URL; browser history restores collection navigation. Artist song loading visits ten albums per page. Album listing loads 100 albums per page; a filter applies to each loaded page, so continue loading for matches farther into a large catalog.

Playlist browsing uses the signed-in user's linked personal Navidrome account. Other library views use the configured shared account. Members retain the existing deletion-request workflow. Administrators can edit metadata on every shared song row, including search results. Direct deletion retains its existing permission and confirmation flow.

## Compact editor

Track artists and album artists are edited as separate, ordered rows. Each row is one literal name; `/` and other punctuation inside a row are never treated as separators. Add, remove or move rows to change credits. Saving sends explicit `artistNames` and `albumArtistNames` arrays and writes native multi-value tags. Unchanged credits keep their existing identities, including on title/lyrics/artwork-only edits. Changed names lose their old identity IDs; unaffected names retain their aligned IDs. Removing all album-artist rows clears those credits. Legacy single-string API edits remain supported and do not infer multiple artists from slash-separated text.

**Edit metadata** opens one shared dialog, using TanStack Form and Zod. Details, Artwork and Lyrics have separate tabs. Artist and album fields suggest existing library names, support free text, and offer keyboard navigation. Suggestions share the bounded 50,000-song index and can be incomplete for larger libraries.

**Save changes** queues a file-tag update. Fields include title, artist, album, optional album artist, artwork URL, plain lyrics and synced LRC. Synced text takes priority in the embedded lyrics tag; both selected forms remain in the review record. Blank lyrics preserve existing embedded lyrics. Embedded artwork is preserved unless a replacement is selected. New cover embedding supports MP3, FLAC and M4A, with JPEG/PNG images from Cover Art Archive / archive.org. Artwork redirects are checked, image downloads are capped at 20 MiB, and dimensions at 50 megapixels.

The worker remuxes audio without re-encoding, verifies the encoded audio packet hash, preserves file permissions, atomically replaces the file and requests a Navidrome scan. It supports MP3, FLAC, M4A, OGG, OPUS, WAV and AIFF text-tag updates, subject to the container's supported tag fields. Paths do not change. Navidrome may assign new IDs depending on its persistent-ID configuration; confirm library and playlist membership after the scan. No live Navidrome compatibility benchmark or custom persistent-ID configuration support is claimed.

The status panel reports queued/running/completed/failed operations and per-song progress. Completed means file tags were updated and a scan was requested; Navidrome may still be scanning. Refresh the library or duplicate candidates afterward. Failed operations expose their error and offer **Retry update**. Merges may partially complete; retry resumes remaining files. A failed scan request can be retried without rewriting already-completed files. Independent metadata operations can be queued while another runs; the worker processes them one at a time. Requests that write the same song or change another pending merge’s destination are rejected until that operation finishes, so queued snapshots cannot overwrite one another. Several merges may share an unchanged destination. The same conflict rules apply to retries.

## Metadata writes and disk usage

Metadata edits do not create persistent backups or an undo archive. The worker reads the existing music file, writes one temporary working file beside it, verifies the encoded audio and tags, syncs the result, and atomically replaces the original. A failed write or verification leaves the original untouched. Working files and staged artwork are removed after success or failure. Startup removes identifiable working files left by a process crash; unrelated music files are left alone. One operation runs at a time. Disk checks reserve room for the working file, any selected artwork, and 256 MiB of free-space headroom.

The small `metadata_files` database journal records the relative music path and before/after checksums. After an interruption, retry recognizes an already-published result or rebuilds the working file from an unchanged source. It refuses to overwrite a source whose bytes match neither recorded checksum. No retained audio copy is needed to resume old jobs. Navidrome original-file downloads are identity checks against the local music file; they do not create stored backups and do not transcode audio.

### Removing copies from earlier versions

The previous `metadata-backups` directory is no longer read or written, including when retrying earlier jobs. The backup-retention environment variable and `metadata-backups list|cleanup` commands have been removed. Old commands fail explicitly instead of starting a worker. Old backup copies are not automatically destroyed during an upgrade.

After deploying the updated image, stop ZenSync and remove the legacy directory if you want to discard those copies. For the host path shown in this issue:

```sh
docker compose stop zensync
rm -rf -- /home/debian-server/docker-data/zensync/metadata-backups
docker compose up -d zensync
```

This discards the old tag-rollback copies. It does not delete music files, job progress, the database or the source-identity journal. Confirm the exact directory before running the command; do not remove the entire ZenSync data directory. Metadata edits have no built-in undo feature; reverting a successful edit requires another edit or an independent backup.

## Utilities: Review duplicates

Use **Keep as one artist** on a combined-credit suggestion when the complete name is one literal artist, such as `V $ X V PRiNCE`. This decision persists across refreshes and restarts and covers both track-artist and album-artist roles, including new library IDs with that name. Both bulk separation actions exclude dismissed names. Pending credit repairs re-check the decision, including immediately before publication. Dismissal does not edit existing tags or undo previously completed changes.

Use **Not duplicates** on a song, album or artist candidate group to hide that exact set of library IDs. A changed group with a new member is reviewed again. The toast offers **Undo**. Decisions apply to all administrators of the configured Navidrome server and are isolated from other server URLs. `POST /api/v2/metadata/dismissals` accepts `{kind:"credit",name}` or `{kind:"song"|"album"|"artist",ids:[...]}`; adding `restore:true` restores the suggestion. Writes require administrator/CSRF authorization.

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

### Manual Library merges

Administrators can use **Merge artist**, **Merge album**, or **Merge song** on Library rows, search for a destination by name, select the entry to keep, and confirm. Names do not need to resemble one another. Artist and album merges use the existing queued tag-update pipeline; follow progress in Activity and refresh after Navidrome scans.

A song merge replaces the source entry's encoded audio with the kept recording while retaining the source file location and its album/track metadata. This leaves existing playlist references and per-entry listening history in place, and can leave copies of the kept recording at multiple album locations. It requires matching file formats and verifies the replacement audio before publishing. It does not consolidate Navidrome song IDs or sum their play counts. OpenSubsonic playback reporting is not used to fabricate listening history.


Metadata updates request an incremental scan of the changed folders when Navidrome exposes one accessible music library and library-relative song paths. Duplicate folders are scanned once per job. When paths cannot be mapped safely, multiple libraries are accessible, or targeted scanning is rejected, ZenSync requests a regular incremental scan instead. Metadata updates no longer force a full library rescan.
