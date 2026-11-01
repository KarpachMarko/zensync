# Revised mockup design plan

One coherent interface combining the selected directions, shown through connected dedicated pages rather than five competing visual themes. Keep the first mockup set unchanged.

Palette: midnight #172033 (workspace), navy #1c2940 (sidebar/panels), slate #29384f (selected controls), porcelain #f3f2f8 (text), fog #adb8cb (secondary text), rose #ecc7d9 (primary actions). Blue #a9c8f2, mint #a9d2be and amber #edc98c encode working, available and unresolved states.

Type: Avenir Next with system sans fallback for controls; Georgia for the main playlist name only. Left-aligned content, tabular numeric durations, compact clear status labels. No uppercase decorative labels.

Layout: [collapsible sidebar | breadcrumb + connection / dedicated page]. Playlist: [sleeve + source / ordered track list + persistent placeholder]. Library: [Songs / Artists / Albums / Favorites tabs / list or grid]. Activity: [status tabs / mixed operation list / selected task page]. Imports: [source selection / full library and membership preview / explicit download choice]. Review: [original source record / candidate recordings / Skip for now].

Primary visual character: playlist sleeve and restrained rose actions. The navigation and operational screens stay quiet. On phones the sidebar becomes a drawer; every major destination remains a full page, with no workflow dialogs.

Critique before build: avoid repeating the initial prototype's modal-only navigation. Replace ambiguous import counts with a complete source-library preview including songs outside playlists. Make Skip for now equal in discoverability to Resolve. Show the unresolved entry in place; never imply that resolving metadata already created an audio file. Keep Activity a queue and history, not a generic metrics dashboard.

Interactions: hash-based dedicated routes for a server-free artifact; sidebar collapse and state persisted where local storage is available; library filters and favorites; source import preview and commit simulation; skip and resolve deferred playlist entries; task details and a deliberately labeled demo completion step. All data and server operations are simulated. The XML flow demonstrates the complete exported catalog and playlist memberships without reading a real user file.
