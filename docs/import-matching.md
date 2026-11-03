# Import recording matching

Import matching searches the existing Navidrome library first, then Deezer. A confident Deezer match makes a song **Ready to request**. It never starts a download: requesting missing audio and approval remain separate steps.

## Why a single result used to require review

The original matcher required the complete title, artist and album strings to agree, with only case/whitespace normalization, and duration within two seconds. The search itself used one query containing all imported artist and title credits. The number of returned results did not establish confidence.

The supplied examples reveal several independent differences:

- Apple appends ` - Single` or ` - EP` to release names. This prevented the Young Thug songs, Tommy Cash and FEDUK examples from matching their Deezer releases.
- Platforms place featured artists in different fields and delimiters. `infinity (888) [feat. Joey Bada$$]` and `ОНА (feat. _lesny)` have shorter Deezer titles, with the guests present in contributor metadata. `$$$` and `False Mirror` use different formats/order for collaboration credits.
- Deezer sometimes exposes Cyrillic names/titles through Latin transliterations, such as Джарахов/Dzharakhov and Что-то ещё/Chto-to eshhyo.
- Searches containing every collaborator or a long featured-artist suffix can return no results. The main artist with the shorter title retrieves the supplied Cocky, Deep End, ОНА and Cadillac examples. LSP/лсп and the hyphenated YOLO-LAND query need additional search variants.
- Catalog title masking changes `Side Nigga` to `Side Ni**a`, despite the artist, release and duration agreeing.

## Current behavior

Comparison preserves the original metadata and accepts case, spacing, punctuation, known release-label suffixes and common Cyrillic/Latin transliterations. Featured-artist decorations can move between the title and contributor fields, but the credited names must still agree. Additional bracketed text, numbers, remasters, live recordings, remixes and other versions retain their meaning. A partially masked word must agree in length and visible letters; fully masked words cannot establish a match. Symbol titles such as `$$$` and `?` remain distinct.

Confidence requires title/version, artist credits, album and duration agreement. Duration tolerance is at most three seconds, capped more tightly for short clips. The public API’s `readable` flag is an account/region availability hint, separate from metadata confidence. A false flag appears as an informational warning and does not prevent selection or an explicit acquisition request. Actual acquisition still depends on the configured account and provider response. Multiple matching IDs can represent duplicate releases; they are grouped only when their nonempty ISRC agrees and their known explicit/clean flags do not conflict. Otherwise review remains necessary.

Search retries use the main credited artist, remove featured-artist text, try a Cyrillic spelling for short uppercase acronyms with Cyrillic titles, and simplify hyphenated titles or transliterate as needed. They stop as soon as a plausible recording is found, with a maximum of five search requests. At most two plausible candidates receive an additional metadata lookup to verify missing collaborator credits. Edited review queries and direct Deezer links remain literal. The same search and ranking apply in background matching and the initial review search.

Candidates with stronger metadata agreement appear first, and review lists the differences that prevent confidence. Matching uses only server-fetched metadata. Acquisition still fetches fresh provider metadata and validates the approved recording and resulting audio independently.

## Examples that still need checking

- The supplied **Esskeetit** link (`1287609912`) currently identifies **Pigott — Lil Pump Esskeetit**, from *Energetic Vibes*. It does not identify the Lil Pump artist/release in the XML. The actual **Lil Pump — Esskeetit** result (`484090122`) has matching metadata and ranks first.
- **Deep End** currently exposes Kizaru alone in Deezer's artist/contributor metadata; the XML additionally credits Smokepurpp. The candidate is retrieved, but the additional credit remains unverified.
- **Cadillac** credits Allj in Apple and Eldzhey in Deezer. These names are not equivalent under transliteration. The candidate is retrieved, but a different artist alias still needs confirmation.
- The supplied **YOLO-LAND** recording is 257 seconds; the XML is approximately 227 seconds. Search retrieves it, but a difference of about 30 seconds remains a reason for manual review.

The metadata-only fixture in `internal/app/testdata/import_recordings.json` contains the supplied examples and public Deezer responses captured during this investigation. It excludes the original XML's file locations, account identifiers and unrelated library contents. Offline regressions cover retrieval, confidence, ranking, version conflicts, incomplete metadata, duplicate recording IDs and bounded requests. They describe these examples rather than a measured match rate for the complete library.

Existing open imports can use **Scan again** to re-evaluate unresolved/review entries. Manual resolutions, existing recording selections and deferred acquisition choices are retained.
