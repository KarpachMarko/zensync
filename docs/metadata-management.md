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
- `POST /api/v2/metadata/merge`: `{kind: "artist"|"album"|"song", source, target, allowFormatChange?}`. Returns `202`, job ID and affected song count.
- `GET /api/v2/metadata/jobs`: latest 20 metadata operations and progress.
- `POST /api/v2/metadata/jobs`: `{id}` retries a failed operation if no metadata operation is active.
- `GET /api/v2/metadata/lyrics?q=…`: up to 20 LRCLIB results.
- `GET /api/v2/metadata/covers?q=…`: front artwork for the first five MusicBrainz releases with available art.

Provider requests have fixed starting destinations, a 25-second search deadline, bounded JSON reads, client identification, a sequential one-second queue, and cooldowns for 429/503 respecting Retry-After seconds or dates. Metadata requests follow at most five HTTPS redirects restricted to MusicBrainz, LRCLIB, Cover Art Archive and archive.org hosts. This fixes Cover Art Archive's HTTP 307 responses without changing the shared client's redirect policy. Provider searches use React Query caching and no automatic error retries. Queries are sent only after explicit search submission. Artwork previews load from the provider URL.

Regression coverage includes native library browsing, suggestions, duplicate identities/durations, permissions/CSRF, queuing, actual file tags and audio preservation, MP3/FLAC/M4A cover embedding, album/artist grouping tags, restart recovery, changed-file protection, redirect handling and provider cooldowns. Playwright covers all library paths, compact/mobile editing, autocomplete, provider selection, deletion, merge confirmation and member access.

### Manual Library merges

Administrators can use **Merge artist**, **Merge album**, or **Merge/Replace** on Library rows, search for a destination by name, select the entry to keep, and confirm. Names do not need to resemble one another. Artist and album merges use the existing queued tag-update pipeline; follow progress in Activity and refresh after Navidrome scans.

For two library songs, **Merge entries** is the default. It keeps the selected song's existing file and Navidrome ID, preserves its encoded audio, and removes the original entry's file using the existing recovery-copy workflow. For example, merging an original FLAC into a selected uncensored MP3 leaves one MP3, rather than copying that MP3 into the FLAC entry and leaving both files. The same operation can consolidate two MP3 copies produced by an earlier audio replacement.

The same dialog compares title, artist, album and album artist, highlights differences, and suggests reviewing the edition and credits. Each field starts with the selected song's value; **Use original**, **Use selected** and direct edits choose the survivor's metadata before confirmation. Duration and encoding differences are shown separately. The selected cover, file location and library ID are retained; listening histories are not summed. **Flip merge direction** swaps the survivor and resets the metadata review to that entry.

Reviewed metadata is verified before any original is removed. Editable playlists owned by connected accounts replace the original ID with the selected ID, preserving ordering and repeated entries. Readonly playlists and unconnected accounts cannot be rewritten through their owners. Imports resolved to the original ID are redirected to the selected ID. Checksum guards, playlist journals and the recovery copy allow failed merges to resume after restart; a dependent removal cannot be retried independently before its parent merge completes. Activity names these jobs **Merge songs**.

**Replace audio — keep both entries** is a separate library action. It retains the original entry's tags and copies the selected recording's encoded audio into it while keeping the donor entry. Format changes use the native new extension without transcoding and warn that Navidrome IDs and playlist references may change. Provider and upload choices use this replacement workflow without creating an additional library donor.


Metadata updates request an incremental scan of the changed folders when Navidrome exposes one accessible music library and library-relative song paths. Duplicate folders are scanned once per job. When paths cannot be mapped safely, multiple libraries are accessible, or targeted scanning is rejected, ZenSync requests a regular incremental scan instead. Metadata updates no longer force a full library rescan.

## Utilities: Library metadata

**Utilities → Library metadata** (`/utilities/metadata`) queues administrator-only jobs:

- **Fetch all missing covers** checks songs without Navidrome artwork and preserves existing embedded covers.
- **Fetch all missing lyrics** inspects file tags and preserves existing lyrics. LRCLIB supplies plain and synced lyrics; synced lyrics take priority when embedding.
- Select songs or albums and choose **Replace selected covers** to overwrite their artwork directly. Album selection updates every song in that album.
- **Correct selected album order** writes Deezer's track and disc numbers without moving files or reordering playlists.

These actions apply successful matches directly, without a separate approval step. Deezer supplies covers and ordering through the existing public metadata client; no new credentials are required. Apple Music and Spotify integrations are not included. MusicBrainz/Cover Art Archive remains available in the individual metadata editor.

Automatic matching requires normalized title, artist and album equality and a known duration within two seconds. Ambiguous results, instrumental lyrics, unavailable matches and unsupported cover containers are skipped. Covers support MP3, FLAC and M4A. A failed provider request fails the job visibly and can be retried from Activity; resolved results and completed writes are persisted so retries resume remaining work. Activity reports updated and skipped totals. The 50,000-song index limit blocks incomplete bulk operations. Independent jobs can queue together; overlapping file updates retain the existing conflict protection. Completed writes use the same atomic replacement, audio verification and targeted/incremental scan workflow as manual metadata edits.

The utility menu uses consistent icon rows. Library metadata has a debounced text filter and a **Missing covers only** switch (based on Navidrome's current artwork metadata). Albums with at least one missing cover remain visible; **View songs** narrows the list for individual **Edit cover** actions using the existing editor. Refresh after Navidrome scans to see updated artwork status.

Song duplicate candidates compare primary artist credits, title and duration, allowing additional guest credits to differ. These remain review candidates, not automatic deletions. An unrelated primary artist is not matched solely because it shares a guest credit.

Track-order correction accepts album selections only. It reads the current Navidrome album name and artist, finds the matching Deezer album edition, and loads that edition’s complete ordered tracklist. Local files are matched within that tracklist by title, structured primary artist and duration; no original Deezer ID or download provenance is required. For example, a local Deluxe album uses Deluxe positions even for files originally obtained from the standard release or another source. Intro tracks absent from the local library still count toward reference positions. Unmatched local songs are skipped individually and duplicates may share a position. Positions come from the reference album tracklist, never a standalone track lookup. Existing number tags are replaced with native format-specific tags; the selected reference and per-song positions are logged. Album merges update local grouping tags without automatically correcting track numbers; run this utility afterward to normalize them to the currently named edition. Utility detail pages use shared breadcrumbs.



Song duplicate rows include the original file's reported format, bitrate, size in MiB, and bit depth/sample rate when Navidrome provides them. Each row has a Listen action that uses the shared custom player with seeking, volume and mute controls; choosing another song replaces the current playback. Audio loads on demand through an administrator-only endpoint, using the verified mounted music file without exposing local paths or Navidrome credentials. Unsupported browser formats or unavailable files show an error and Retry action. The quality labels describe the stored file, not a transcoded preview.

In song duplicate review, **Keep song** uses the prepared candidate group: it retains the clicked song and queues recoverable deletion requests for every other member after a five-second Undo window. Leaving review cancels a scheduled action. There is no separate Delete action or destination picker in these rows. This action first replaces the removed song ID with the kept ID in editable playlists owned by the shared Navidrome account and linked personal accounts, preserving positions and repetitions. It verifies those updates before deleting the duplicate file. Read-only, smart/imported and other owners’ playlists are skipped. It does not copy audio or transfer album membership, ratings or play counts. Deletion jobs remain visible in Activity. Partial request failures can be retried using the same idempotency keys for the current group, avoiding duplicate deletion requests. Manual song merge elsewhere retains its existing audio-replacement behavior.

Playlist-preserving deletion stores the kept song and Navidrome server with each new Keep song request. Before each remote playlist write it journals the ordered before/after song IDs; retries reconcile an uncertain response against the remote list. A changed playlist, missing kept file, changed server, inaccessible linked account or failed update/verification fails the deletion job before its source file is removed. Some playlists may already have been updated when a later step fails; retry resumes from the journal. Navidrome offers no playlist compare-and-swap operation, so concurrent edits during the remote write itself cannot be made atomic with deletion. Avoid editing affected playlists during this operation. Deletions queued by older versions have no kept-song association and retain their original plain-deletion behavior.

### Deleted-file recovery and trash limits

Deletion recovery copies are kept for at most **30 days**, with a combined **500 MB (500,000,000 bytes)** limit. Startup and worker maintenance remove expired copies and then the oldest copies until usage fits. Space is reserved before copying a newly deleted file. A file larger than the entire allowance is not deleted: its job fails and the original remains. The policy applies to existing trash as well as new deletions.

Completed deletion rows in **Activity** offer administrators **Restore file** while a recovery copy is available. Restore copies verified original bytes to the original music path without overwriting any existing file, then requests a Navidrome scan. A failed scan can be retried using the same action without copying again. The used recovery copy is removed after success. Expired/evicted copies have no Restore action. Restoring does not undo playlist replacements, recreate playlist positions, or restore ratings/play counts. Restored files may receive different Navidrome IDs.

### Editing albums and renaming artists

Administrators can use **Edit metadata** on Library album rows (including albums under an artist) and the opened album heading. The existing metadata dialog opens in album mode with title, ordered album-artist rows and the same artwork search, HTTPS URL, upload and removal controls as song editing. Saving queues updates across the complete album. Track titles, track artists and lyrics are not copied from the representative song; unchanged collection fields preserve each file’s existing values.

Artwork edits replace the embedded image in each affected audio file; they do not overwrite image files beside the music. Navidrome's default album artwork priority prefers `cover.*`, `folder.*` and `front.*` over embedded images, so an old folder image can remain the album cover even after a full scan. To make embedded edits authoritative, set `CoverArtPriority = "embedded, cover.*, folder.*, front.*, external"` (Docker environment: `ND_COVERARTPRIORITY` with the same value), restart Navidrome and run a full scan. Alternatively, update the album's folder image. See [Navidrome artwork resolution](https://www.navidrome.org/docs/usage/library/artwork/). Track artwork and album artwork can use different sources; a changed track cover alone does not verify the album cover.

Album edits preserve native MusicBrainz release and release-group identifiers. Renaming an existing album must not clear those grouping identifiers on only the members whose original titles differ; songs with mixed original titles continue to belong to the same release. This prevents future splits but does not automatically rejoin albums split by an earlier edit. Use **Review duplicates** to merge those albums, choosing the intended album as the destination.

**Rename artist** is available on artist rows and opened artist headings. It queues tag changes for that artist’s track-artist and album-artist credits, including album-artist-only appearances. Matching uses library artist IDs; other credited artists and unrelated artists with the same spelling are preserved. Native artist identifiers that cannot be retained after changing a name follow the existing metadata editor’s identity-clearing rules. Paths and audio are preserved. These actions use the serialized metadata queue and its audio verification and library-scan workflow. Refresh the Library after scanning; artist/album IDs may change with Navidrome grouping. Collection edits require the complete bounded library index, as merges do.


## Merge review and library playback

Manual song, album and artist merges use the same responsive review layout as import recording review. Source and selected destination are shown separately. Song candidates include title, artist, album, duration, artwork, native file format/quality and listening controls. Selecting an album or artist loads its recordings with pagination so they can be inspected and played before confirmation. Searching, selecting or listening does not queue a merge. Closing the dialog stops its audio.

Song replacement checks the actual source and destination files through the administrator-only `GET /api/v2/metadata/merge-compatibility?source=…&target=…` endpoint. A format mismatch is shown as a warning. Confirmation sends `allowFormatChange: true`; without that acknowledgement the merge POST refuses a format change. The worker retains its final guard in case files change afterward. Cross-format publication checks filename collisions, verifies audio packets, durably publishes the new native extension before removing the old path, and journals both paths for crash recovery. Album and artist tag merges do not replace audio and do not require matching formats.

Authenticated members and administrators can listen from Library, Search, import review and merge review through `GET /api/v2/library/stream?id=…`. ZenSync verifies the song with Navidrome and proxies its OpenSubsonic stream, requesting MP3 at a maximum of 128 kbps when the server supports transcoding. Single byte ranges, partial and unsatisfiable responses are forwarded for seeking; Navidrome credentials and response cookies remain server-side. Artwork is proxied through `/api/v2/library/cover?id=…`, with an 8 MiB image limit. Listening requires no acquisition request and writes no library files.

All player interactions use `useAudioPreview` for playback state, seeking, volume, errors and cleanup, and `AudioPlayer`/`DockedAudioPlayer` for rendering. `useRecordingPreview` adds URL resolution for provider recordings and Navidrome library songs. Duplicate comparison continues to use the administrator-only original-file endpoint so native audio quality can be compared; it shares the same hook and rendering components. Playback regression tests use generated browser-playable audio; a live Navidrome instance remains unverified.


The song action is **Merge/Replace**. Its search starts with the existing title. **Library**, **Providers** and **Upload file** tabs select recording sources; provider search runs across all catalogs and ranks results by relevance. Library choices can flip the replacement direction. A selected choice has one **Selected** control, without a duplicate destination badge. Search and selection never start a replacement.

For provider recordings, `POST /api/v2/metadata/merge` accepts `{kind: "song", source, providerTarget, allowFormatChange: true}` and queues native acquisition into private staging. The provider recording is validated using the captured quality profile and exact recording identity; it is not published as a separate library asset or music request. The original song's tags are applied after validation. Provider streams remain previews at the configured low playback quality; replacement downloads use the acquisition quality profile.

Local audio can be staged with administrator-only `POST /api/v2/metadata/replacement-upload?source=…` (raw file bytes and `X-File-Name`). Validation enforces supported containers, a 1 KiB–1 GiB size range and full audio decoding without transcoding. `GET /api/v2/metadata/replacement-inputs/{id}` returns metadata, while `/audio` and `/cover` serve private review media. Inputs belong to their uploader and source song. Confirm with `{kind: "song", source, uploadId, allowFormatChange: true}`. Checksums protect retry inputs; failed operations retain staging, successful operations remove it, and abandoned uploads expire after a day during startup or a subsequent upload.

The shared bottom player starts at the sidebar edge (208 px expanded, 60 px collapsed) and spans the viewport on mobile, including when playing from a review dialog.


`POST /api/v2/metadata/merge` accepts `{kind: "song", songOperation: "merge", source, target, metadata?: {title?, artist?, album?, albumArtist?}}` for consolidation into the selected target. Omitting `songOperation`, or setting it to `"replace"`, retains the earlier audio-replacement API behavior. Consolidation does not require matching source/target formats because the selected file is retained as-is. The review record, metadata operation, and dependent removal/recovery journals are created in one transaction; conflicting metadata updates and removals are rejected while the merge is pending.
