# Five music manager UI directions

Scope: Go/SQLite music manager using d-fi, Apple/Deezer metadata, Apple XML and public playlist imports, Navidrome and Wavio. These are local interactive mockups with sample data, not the application. No playback controls or recommendation feeds.

Shared scenario: Night Drive contains 24 tracks: 18 already owned, 4 downloads pending, 2 matches need review. Show the same scenario in five genuinely different information architectures. All text left aligned except intentional artwork lettering and numeric data.

## Collection
Palette: ink #101c32, panel #182741, paper #eef3fc, mist #a8b8d1, blue #82b4ff, mint #8bd5b0.
Type: Avenir Next for both display and interface, fall back to system sans.
Layout: persistent narrow navigation, art-forward playlist header, album shelf and a small actionable queue.
Wireframe: [nav | playlist artwork + title] / [nav | coverage + albums] / [nav | review + queue].
Principle: make an owned collection feel tangible, with acquisition state attached directly to music.

## Workbench
Palette: workspace #edf1f5, white #ffffff, ink #213044, grey #617083, blue #2459c4, green #287153.
Type: system sans with compact, carefully aligned table numerals.
Layout: horizontal navigation, import selector, searchable/selectable track table and a contextual match inspector.
Wireframe: [tabs / status] / [source + summary] / [track table | selected-track inspector].
Principle: review many tracks quickly with clear selection and predictable bulk actions.

## Mixtape
Palette: plum #302639, panel #3d3047, pink #f3c1d2, pale #f4e9f1, muted #bfaec5, lilac #baacd9.
Type: Georgia for the playlist name, Avenir Next for interface text.
Layout: a large custom playlist sleeve alongside an ordered track list, with a quiet bottom action strip.
Wireframe: [wordmark / tabs] / [playlist sleeve | title + ordered tracks] / [import summary / action].
Principle: playlist sequencing and cover identity are the main visual expression; avoid turning every track into a card.

## Signal
Palette: slate #192c35, panel #233c47, bright #e6f4f6, fog #a4bec8, cyan #83d8e4, amber #e8c58b.
Type: Trebuchet MS headings, system sans interface.
Layout: an acquisition board organized into review, download and ready columns, with progress and Navidrome handoff below.
Wireframe: [navigation + source] / [review | download | ready] / [transfer details / destination].
Principle: show what needs attention and what will happen next without exposing internal implementation jargon.

## Pocket
Palette: canvas #f2eddc, paper #fffdf4, ink #323329, muted #6b6d60, moss #4d6847, gold #d9b86b.
Type: Trebuchet MS for friendly titles, system sans for controls.
Layout: a compact mobile-first workspace with a playlist picker, simple segmented views and large touch targets; a detail sheet becomes a second column on desktop.
Wireframe: [top bar] / [playlist selector] / [track list | selected item] / [bottom navigation].
Principle: all core work should be comfortable from a phone, without mimicking a music playback app.

## Critique before implementation

Rejected five color variations of the same dashboard. Each direction changes hierarchy and the primary unit: albums, table rows, playlist sequence, jobs, or touch-first tasks. Removed generic metrics cards, hero marketing copy, autoplay motion, player controls and recommendations because they do not serve the requested scope. The custom sleeve art is geometric SVG/CSS created for the mockups; no external album-art downloads or network requests are required.

Interactive review: switch directions, search sample tracks, inspect matches, simulate queueing downloads, and preview the Apple XML/public-link import dialog. No control connects to a real music service or reads a real file. The UI explicitly identifies sample data and simulated actions.
