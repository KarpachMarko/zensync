# Revised ZenSync mockups

Open `index.html`. This is one connected interface built from the chosen preferences, with dedicated pages rather than workflow dialogs. The original five concepts in the parent directory are unchanged.

Direct entry files:

- `playlist.html`: sidebar, playlist sleeve, ordered songs and deferred placeholders.
- `library.html`: Songs, Artists, Albums and Favorites; artist/album/song detail pages.
- `activity.html`: running and queued downloads, metadata lookups, imports, scans and playlist sync, plus needs attention/history and task detail pages.
- `import.html`: complete Apple XML catalog and ordered playlist memberships, including songs outside playlists.
- `review.html`: candidate recordings, original source metadata and Skip for now. Failed file imports retain their existing match.

The standalone HTML uses hash routes, so normal browser back/forward and direct page links work without a web server. Sample preferences and decisions are stored locally when browser storage is available. Reset sample restores the initial state. All graphics, styles and JavaScript are embedded. ZenSync is the chosen project name.

Try this flow: open Night Drive → Skip for now on Teardrop → see its retained position and deferred status → open Resolve → accept a match → view its dedicated Activity page → use the explicitly labeled Simulate completed import control → return to the playlist with the same position now fulfilled. This is a simulation of downloading, file import, library scan and playlist sync, not real service work.

The sample XML scenario contains 12 songs and 3 playlists; Dayvan Cowboy and Two Weeks belong to no playlist and remain in the catalog. Deselecting a playlist never removes its songs from the full catalog import. Public-link imports have their own, smaller playlist-only summary. Importing metadata does not trigger downloads.

Validation: desktop visual previews at 1000px, inline at 728px and phone previews at 390px. The browser-executed `qa.js`, injected into html_preview at the QA hook, checks 22 dedicated routes, horizontal overflow, sidebar and mobile drawer, skipped-state serialization, placeholder ordering and later fulfillment, import retry, favorites and filtering, complete XML semantics and playlist-only imports. No server application, music provider or actual XML parser is implemented here.
