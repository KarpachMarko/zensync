# ZenSync: five UI mockups

Open `index.html` to compare all five directions. ZenSync is the chosen project name.

- `collection.html`: persistent navigation and album artwork.
- `workbench.html`: dense track review with a contextual inspector.
- `mixtape.html`: playlist sleeve and ordered track sequence.
- `signal.html`: review, acquisition and library state in a board.
- `pocket.html`: a compact touch-oriented interface.

All files are self-contained, with embedded SVG illustration, CSS and JavaScript. No external assets, server, build step or network connection is required. Buttons open sample flows for Apple Music XML/public links, match review, download queues, artist checks and settings. They do not download music or connect to Navidrome, Wavio, Apple or Deezer. Six representative tracks stand in for the 24-track playlist; status totals include the omitted tracks.

The visual plan is in `design-plan.md`. All five concepts were visually checked at 1000px desktop and 390px phone widths; the gallery was checked at 728px. `qa.js` is a browser-executed interaction smoke check used by injecting it after the inline application script in html_preview. It verifies concept selection, overflow, import preview, match resolution, queue/cancellation, catalog search, playlist filtering and quality settings.
