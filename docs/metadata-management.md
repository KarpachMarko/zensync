# Metadata management (initial implementation)

Administrators can open **Metadata** in the sidebar, search the shared library, edit a song's title/artist/album, search for covers or lyrics, preview candidates, and explicitly save their selection. Saved records survive reloads. **Restore source metadata** removes the local review record.

## Scope

Edits are stored in SQLite separately from source metadata. They do **not** rewrite audio tags, download artwork, create LRC files, rename files, change acquisition matching, or update Navidrome. The Library and duplicate audit continue to use source metadata. Artist and album names can be edited per song; there is no bulk artist/album editor yet. Saved records include the last editing administrator and update time. Restoring source metadata removes selected covers and lyrics too.

This is a review workspace, not yet a tag publishing pipeline. A follow-up should add file identity checks, backups, format-specific tag writing, artwork/lyrics sidecars, job recovery, scan verification and a reviewable change summary before applying changes to shared files. Metadata review records are keyed by Navidrome song ID; changing the configured Navidrome server should be accompanied by clearing/reconciling those records.

## Duplicate candidates

The audit uses the existing shared Navidrome song index (cached for 30 seconds), capped at 50,000 songs. A partial scan is explicitly marked. It groups:

- Distinct artist IDs with matching normalized names.
- Distinct album IDs with matching normalized artist and album names.
- Distinct song IDs with matching normalized artist/title and known durations within two seconds.

Normalization ignores case, spaces and punctuation, retaining letters, numbers and Unicode marks. It preserves edition labels such as “live” or “remaster”. It does not do acoustic fingerprinting, transliteration, edit-distance matching or duplicate file hashing. Albums and artists without IDs cannot be compared reliably and are excluded from those groups. Songs with unknown durations are excluded. Duplicate groups are suggestions: editions, compilations and different artists sharing a name are expected false positives. No automatic merging or deletion is performed.

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

## API and operational behavior

All endpoints require an administrator session. Writes require the normal same-origin and CSRF protections.

- `GET /api/v2/metadata/duplicates`: candidate groups, scanned song count, partial-scan flag.
- `GET /api/v2/metadata/songs/{id}`: source defaults or saved edit, plus `saved` flag.
- `PUT /api/v2/metadata/songs/{id}`: replace the reviewed metadata record after verifying the source song exists. Fields: `title`, `artist`, `album`, `coverUrl`, `lyrics`, `syncedLyrics`, `lyricsSource`. Title and artist are required. Cover URLs must use HTTPS.
- `DELETE /api/v2/metadata/songs/{id}`: discard the reviewed record.
- `GET /api/v2/metadata/lyrics?q=…`: LRCLIB results (up to 20), including plain and synced lyrics.
- `GET /api/v2/metadata/covers?q=…`: front cover candidates with release source URLs.

Provider requests have fixed server-side destinations, a 25-second search deadline, bounded JSON reads, an identifying User-Agent, a shared sequential queue paced at one second, and cooldowns for 429/503 responses including Retry-After seconds or dates. The browser caches provider searches for five minutes and does not automatically retry failed searches. Artwork previews load directly from the selected provider URL. Search requests only run after explicit submission; their query text is sent to the named external service.

Tests cover normalization, distinct identities, duration/version separation, permissions/CSRF, persistence/reset, URL validation, provider decoding/query escaping, cancellation and cooldown handling, plus browser flows for edits, lyrics selection, duplicate review, provider errors and member access.
